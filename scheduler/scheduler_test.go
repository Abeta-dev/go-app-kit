package scheduler_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abeta-dev/go-app-kit/scheduler"
)

type failingLeaseStore struct {
	err error
}

func (f failingLeaseStore) TryAcquire(context.Context, string, string, time.Time, time.Duration) (bool, error) {
	return false, f.err
}

func (f failingLeaseStore) Release(context.Context, string, string) error {
	return f.err
}

func TestMemoryLeaseStore(t *testing.T) {
	t.Parallel()

	store := scheduler.NewMemoryLeaseStore()
	ctx := context.Background()
	now := time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	if _, err := store.TryAcquire(canceledCtx, "job-a", "node-1", now, time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled on TryAcquire, got %v", err)
	}
	if err := store.Release(canceledCtx, "job-a", "node-1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled on Release, got %v", err)
	}

	if _, err := store.TryAcquire(ctx, "   ", "node-1", now, time.Minute); !errors.Is(err, scheduler.ErrInvalidJobName) {
		t.Fatalf("expected ErrInvalidJobName, got %v", err)
	}
	if _, err := store.TryAcquire(ctx, "job-a", "   ", now, time.Minute); !errors.Is(err, scheduler.ErrEmptyNodeID) {
		t.Fatalf("expected ErrEmptyNodeID, got %v", err)
	}
	if _, err := store.TryAcquire(ctx, "job-a", "node-1", now, 0); !errors.Is(err, scheduler.ErrInvalidLeaseTTL) {
		t.Fatalf("expected ErrInvalidLeaseTTL, got %v", err)
	}

	if err := store.Release(ctx, "", "node-1"); !errors.Is(err, scheduler.ErrInvalidJobName) {
		t.Fatalf("expected ErrInvalidJobName on Release, got %v", err)
	}
	if err := store.Release(ctx, "job-a", ""); !errors.Is(err, scheduler.ErrEmptyNodeID) {
		t.Fatalf("expected ErrEmptyNodeID on Release, got %v", err)
	}

	// Node 1 acquires lease
	acquired, err := store.TryAcquire(ctx, "gst-aging-snapshot", "node-1", now, 30*time.Second)
	if err != nil || !acquired {
		t.Fatalf("expected node-1 to acquire lease, got acquired=%v err=%v", acquired, err)
	}

	// Node 2 cannot acquire while active
	acquired2, err := store.TryAcquire(ctx, "gst-aging-snapshot", "node-2", now.Add(10*time.Second), 30*time.Second)
	if err != nil || acquired2 {
		t.Fatalf("expected node-2 to be fenced out, got acquired=%v err=%v", acquired2, err)
	}

	// Node 1 can renew its own lease
	renewed, err := store.TryAcquire(ctx, "gst-aging-snapshot", "node-1", now.Add(15*time.Second), 30*time.Second)
	if err != nil || !renewed {
		t.Fatalf("expected node-1 to renew lease, got renewed=%v err=%v", renewed, err)
	}

	// Node 2 releasing does not drop node 1's lease
	if err := store.Release(ctx, "gst-aging-snapshot", "node-2"); err != nil {
		t.Fatalf("unexpected error releasing from non-holder: %v", err)
	}
	stillFenced, err := store.TryAcquire(ctx, "gst-aging-snapshot", "node-2", now.Add(20*time.Second), 30*time.Second)
	if err != nil || stillFenced {
		t.Fatalf("expected node-1 lease to remain active")
	}

	// After lease expiry, node 2 can take over
	takenOver, err := store.TryAcquire(ctx, "gst-aging-snapshot", "node-2", now.Add(60*time.Second), 30*time.Second)
	if err != nil || !takenOver {
		t.Fatalf("expected node-2 to acquire expired lease, got %v err=%v", takenOver, err)
	}

	// Node 2 releases lease explicitly, allowing node 1 immediate acquisition
	if err := store.Release(ctx, "gst-aging-snapshot", "node-2"); err != nil {
		t.Fatalf("unexpected error releasing lease: %v", err)
	}
	reacquired, err := store.TryAcquire(ctx, "gst-aging-snapshot", "node-1", now.Add(61*time.Second), 30*time.Second)
	if err != nil || !reacquired {
		t.Fatalf("expected node-1 to acquire released lease, got %v err=%v", reacquired, err)
	}
}

func TestScheduler_RegisterAndRunOnce(t *testing.T) {
	t.Parallel()

	store := scheduler.NewMemoryLeaseStore()

	if _, err := scheduler.NewScheduler(nil, "node-1"); !errors.Is(err, scheduler.ErrNilLeaseStore) {
		t.Fatalf("expected ErrNilLeaseStore, got %v", err)
	}
	if _, err := scheduler.NewScheduler(store, "   "); !errors.Is(err, scheduler.ErrEmptyNodeID) {
		t.Fatalf("expected ErrEmptyNodeID, got %v", err)
	}

	now := time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	var capturedErrors []string

	sched1, err := scheduler.NewScheduler(
		store,
		"node-1",
		nil,
		scheduler.WithClock(nil),
		scheduler.WithErrorHandler(nil),
		scheduler.WithClock(func() time.Time { return now }),
		scheduler.WithErrorHandler(func(jobName string, err error) {
			mu.Lock()
			capturedErrors = append(capturedErrors, jobName+":"+err.Error())
			mu.Unlock()
		}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nopFn := func(context.Context) error { return nil }
	if err := sched1.Register(scheduler.JobConfig{Name: "", Interval: time.Second, LeaseTTL: time.Second, Fn: nopFn}); !errors.Is(err, scheduler.ErrInvalidJobName) {
		t.Fatalf("expected ErrInvalidJobName, got %v", err)
	}
	if err := sched1.Register(scheduler.JobConfig{Name: "j", Interval: 0, LeaseTTL: time.Second, Fn: nopFn}); !errors.Is(err, scheduler.ErrInvalidInterval) {
		t.Fatalf("expected ErrInvalidInterval, got %v", err)
	}
	if err := sched1.Register(scheduler.JobConfig{Name: "j", Interval: time.Second, LeaseTTL: 0, Fn: nopFn}); !errors.Is(err, scheduler.ErrInvalidLeaseTTL) {
		t.Fatalf("expected ErrInvalidLeaseTTL, got %v", err)
	}
	if err := sched1.Register(scheduler.JobConfig{Name: "j", Interval: time.Second, LeaseTTL: time.Second, Fn: nil}); !errors.Is(err, scheduler.ErrNilJobFunc) {
		t.Fatalf("expected ErrNilJobFunc, got %v", err)
	}

	var runs atomic.Int32
	jobCfg := scheduler.JobConfig{
		Name:     "recurring-billing",
		Interval: 50 * time.Millisecond,
		LeaseTTL: 10 * time.Minute,
		Fn: func(context.Context) error {
			runs.Add(1)
			return nil
		},
	}
	if err := sched1.Register(jobCfg); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if err := sched1.Register(jobCfg); !errors.Is(err, scheduler.ErrDuplicateJob) {
		t.Fatalf("expected ErrDuplicateJob, got %v", err)
	}

	ctx := context.Background()
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := sched1.RunOnce(canceledCtx, "recurring-billing"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled on RunOnce, got %v", err)
	}
	if _, err := sched1.RunOnce(ctx, "non-existent"); !errors.Is(err, scheduler.ErrInvalidJobName) {
		t.Fatalf("expected ErrInvalidJobName on unknown job, got %v", err)
	}

	// Node 1 executes job
	ran, err := sched1.RunOnce(ctx, "recurring-billing")
	if err != nil || !ran || runs.Load() != 1 {
		t.Fatalf("expected job to run once on node-1, ran=%v err=%v count=%d", ran, err, runs.Load())
	}

	// Node 2 attempts same job while lease is held by node 1
	sched2, err := scheduler.NewScheduler(store, "node-2", scheduler.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = sched2.Register(jobCfg)
	ran2, err := sched2.RunOnce(ctx, "recurring-billing")
	if err != nil || ran2 || runs.Load() != 1 {
		t.Fatalf("expected node-2 to skip leased job, ran2=%v err=%v count=%d", ran2, err, runs.Load())
	}

	// Error and panic recovery in RunOnce
	errBoom := errors.New("database timeout")
	_ = sched1.Register(scheduler.JobConfig{
		Name:     "failing-job",
		Interval: time.Second,
		LeaseTTL: time.Minute,
		Fn: func(context.Context) error {
			return errBoom
		},
	})
	ranFail, err := sched1.RunOnce(ctx, "failing-job")
	if !ranFail || !errors.Is(err, errBoom) {
		t.Fatalf("expected errBoom from failing-job, got ran=%v err=%v", ranFail, err)
	}

	_ = sched1.Register(scheduler.JobConfig{
		Name:     "panicking-job",
		Interval: time.Second,
		LeaseTTL: time.Minute,
		Fn: func(context.Context) error {
			panic("nil pointer dereference")
		},
	})
	ranPanic, err := sched1.RunOnce(ctx, "panicking-job")
	if !ranPanic || err == nil || !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("expected panic to be recovered as error, got ran=%v err=%v", ranPanic, err)
	}

	// LeaseStore failure
	storeErr := errors.New("redis connection lost")
	schedFailStore, err := scheduler.NewScheduler(failingLeaseStore{err: storeErr}, "node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = schedFailStore.Register(jobCfg)
	if ranStoreFail, err := schedFailStore.RunOnce(ctx, "recurring-billing"); ranStoreFail || !errors.Is(err, storeErr) {
		t.Fatalf("expected storeErr from TryAcquire failure, got ran=%v err=%v", ranStoreFail, err)
	}

	mu.Lock()
	errCount := len(capturedErrors)
	mu.Unlock()
	if errCount != 2 {
		t.Fatalf("expected 2 errors reported to errorHandler, got %d", errCount)
	}
}

func TestScheduler_StartAndGracefulShutdown(t *testing.T) {
	t.Parallel()

	store := scheduler.NewMemoryLeaseStore()
	sched, err := scheduler.NewScheduler(store, "node-primary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	canceledCtx, cancelImmediate := context.WithCancel(context.Background())
	cancelImmediate()
	if err := sched.Start(canceledCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled on pre-canceled Start, got %v", err)
	}

	var tickCount atomic.Int32
	if err := sched.Register(scheduler.JobConfig{
		Name:       "partition-maintenance",
		Interval:   15 * time.Millisecond,
		LeaseTTL:   5 * time.Millisecond,
		RunOnStart: true,
		Fn: func(context.Context) error {
			tickCount.Add(1)
			return nil
		},
	}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 65*time.Millisecond)
	defer cancel()

	err = sched.Start(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded when Start finishes, got %v", err)
	}
	if tickCount.Load() < 2 {
		t.Fatalf("expected job to execute on start and at least one tick, got %d", tickCount.Load())
	}
}
