// Package scheduler provides a distributed, lease-fenced periodic job runner
// for recurring billing cycles, GST aging snapshots, and PostgreSQL partition maintenance.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	// ErrInvalidJobName is returned when a job name is empty or unregistered.
	ErrInvalidJobName = errors.New("scheduler: invalid or unknown job name")
	// ErrNilJobFunc is returned when a job configuration has a nil execution function.
	ErrNilJobFunc = errors.New("scheduler: job function must not be nil")
	// ErrInvalidInterval is returned when a job configuration has a non-positive execution interval.
	ErrInvalidInterval = errors.New("scheduler: job interval must be positive")
	// ErrInvalidLeaseTTL is returned when a lease TTL is non-positive.
	ErrInvalidLeaseTTL = errors.New("scheduler: lease TTL must be positive")
	// ErrDuplicateJob is returned when registering a job name that is already registered.
	ErrDuplicateJob = errors.New("scheduler: duplicate job name")
	// ErrNilLeaseStore is returned when NewScheduler is called with a nil LeaseStore.
	ErrNilLeaseStore = errors.New("scheduler: lease store must not be nil")
	// ErrEmptyNodeID is returned when a node ID is empty or whitespace-only.
	ErrEmptyNodeID = errors.New("scheduler: node ID must not be empty")
)

// JobFunc is the callback signature executed by the scheduler when a node holds the job lease.
type JobFunc func(ctx context.Context) error

// JobConfig defines the schedule, distributed lease duration, and callback for a periodic job.
type JobConfig struct {
	Name       string
	Interval   time.Duration
	LeaseTTL   time.Duration
	RunOnStart bool
	Fn         JobFunc
}

// LeaseStore coordinates distributed mutual exclusion across multiple service replicas.
type LeaseStore interface {
	TryAcquire(ctx context.Context, jobName, nodeID string, now time.Time, ttl time.Duration) (bool, error)
	Release(ctx context.Context, jobName, nodeID string) error
}

type leaseRecord struct {
	nodeID    string
	expiresAt time.Time
}

// MemoryLeaseStore is a thread-safe in-memory LeaseStore tracking active lease holders
// and expiration timestamps for single-node deployments and deterministic unit testing.
type MemoryLeaseStore struct {
	mu     sync.Mutex
	leases map[string]leaseRecord
}

// NewMemoryLeaseStore creates an initialized MemoryLeaseStore.
func NewMemoryLeaseStore() *MemoryLeaseStore {
	return &MemoryLeaseStore{
		leases: make(map[string]leaseRecord),
	}
}

// TryAcquire attempts to acquire or renew the named job lease for nodeID until now.Add(ttl).
// If another node currently holds an unexpired lease, TryAcquire returns (false, nil).
func (m *MemoryLeaseStore) TryAcquire(ctx context.Context, jobName, nodeID string, now time.Time, ttl time.Duration) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	name := strings.TrimSpace(jobName)
	if name == "" {
		return false, ErrInvalidJobName
	}
	holder := strings.TrimSpace(nodeID)
	if holder == "" {
		return false, ErrEmptyNodeID
	}
	if ttl <= 0 {
		return false, ErrInvalidLeaseTTL
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	rec, exists := m.leases[name]
	if exists && now.Before(rec.expiresAt) && rec.nodeID != holder {
		return false, nil
	}

	m.leases[name] = leaseRecord{
		nodeID:    holder,
		expiresAt: now.Add(ttl),
	}
	return true, nil
}

// Release relinquishes the named job lease if it is currently held by nodeID.
func (m *MemoryLeaseStore) Release(ctx context.Context, jobName, nodeID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name := strings.TrimSpace(jobName)
	if name == "" {
		return ErrInvalidJobName
	}
	holder := strings.TrimSpace(nodeID)
	if holder == "" {
		return ErrEmptyNodeID
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if rec, exists := m.leases[name]; exists && rec.nodeID == holder {
		delete(m.leases, name)
	}
	return nil
}

// Option configures a Scheduler instance.
type Option func(*Scheduler)

// WithClock sets a custom clock function for lease acquisition timestamps.
func WithClock(nowFunc func() time.Time) Option {
	return func(s *Scheduler) {
		if nowFunc != nil {
			s.now = nowFunc
		}
	}
}

// WithErrorHandler registers a callback invoked whenever lease acquisition fails,
// a job returns an error, or a job panics during execution.
func WithErrorHandler(fn func(jobName string, err error)) Option {
	return func(s *Scheduler) {
		if fn != nil {
			s.errorHandler = fn
		}
	}
}

// Scheduler orchestrates periodic lease-fenced jobs with panic recovery and graceful shutdown.
type Scheduler struct {
	mu           sync.RWMutex
	store        LeaseStore
	nodeID       string
	jobs         map[string]JobConfig
	order        []string
	now          func() time.Time
	errorHandler func(jobName string, err error)
}

// NewScheduler constructs a new Scheduler backed by the given LeaseStore and node identifier.
func NewScheduler(store LeaseStore, nodeID string, opts ...Option) (*Scheduler, error) {
	if store == nil {
		return nil, ErrNilLeaseStore
	}
	trimmedNode := strings.TrimSpace(nodeID)
	if trimmedNode == "" {
		return nil, ErrEmptyNodeID
	}

	sched := &Scheduler{
		store:  store,
		nodeID: trimmedNode,
		jobs:   make(map[string]JobConfig),
		now:    time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(sched)
		}
	}
	return sched, nil
}

// Register validates and registers a periodic job configuration.
func (s *Scheduler) Register(cfg JobConfig) error {
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return ErrInvalidJobName
	}
	if cfg.Interval <= 0 {
		return ErrInvalidInterval
	}
	if cfg.LeaseTTL <= 0 {
		return ErrInvalidLeaseTTL
	}
	if cfg.Fn == nil {
		return ErrNilJobFunc
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.jobs[name]; exists {
		return ErrDuplicateJob
	}
	cfg.Name = name
	s.jobs[name] = cfg
	s.order = append(s.order, name)
	return nil
}

// RunOnce attempts to acquire the lease for jobName and, if acquired, executes the job
// with panic recovery. Returns (false, nil) when another replica holds the active lease.
func (s *Scheduler) RunOnce(ctx context.Context, jobName string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	name := strings.TrimSpace(jobName)
	s.mu.RLock()
	cfg, ok := s.jobs[name]
	s.mu.RUnlock()
	if !ok {
		return false, ErrInvalidJobName
	}

	acquired, err := s.store.TryAcquire(ctx, cfg.Name, s.nodeID, s.now().UTC(), cfg.LeaseTTL)
	if err != nil {
		s.reportError(cfg.Name, err)
		return false, err
	}
	if !acquired {
		return false, nil
	}

	execErr := executeJobSafe(ctx, cfg.Name, cfg.Fn)
	if execErr != nil {
		s.reportError(cfg.Name, execErr)
		return true, execErr
	}
	return true, nil
}

// Start runs all registered jobs on their configured intervals (and immediately on startup
// when RunOnStart is true) until ctx is canceled, waiting for all in-flight jobs to complete.
func (s *Scheduler) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.RLock()
	configs := make([]JobConfig, 0, len(s.order))
	for _, name := range s.order {
		configs = append(configs, s.jobs[name])
	}
	s.mu.RUnlock()

	var wg sync.WaitGroup
	for _, cfg := range configs {
		wg.Add(1)
		go func(job JobConfig) {
			defer wg.Done()
			if job.RunOnStart {
				_, _ = s.RunOnce(ctx, job.Name)
			}
			ticker := time.NewTicker(job.Interval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_, _ = s.RunOnce(ctx, job.Name)
				}
			}
		}(cfg)
	}

	<-ctx.Done()
	wg.Wait()
	return ctx.Err()
}

func (s *Scheduler) reportError(jobName string, err error) {
	if s.errorHandler != nil {
		s.errorHandler(jobName, err)
	}
}

func executeJobSafe(ctx context.Context, jobName string, fn JobFunc) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("scheduler: job %q panicked: %v", jobName, r)
		}
	}()
	return fn(ctx)
}
