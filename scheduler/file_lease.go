package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultFileLeasePerm os.FileMode = 0o600
	defaultFileLeaseDir  os.FileMode = 0o700
)

var (
	// ErrInvalidLockDir is returned when NewFileLeaseStore is called with an empty lock directory path.
	ErrInvalidLockDir = errors.New("scheduler: lock directory must not be empty")
	// ErrInvalidFileMode is returned when a zero file or directory permission mode is supplied.
	ErrInvalidFileMode = errors.New("scheduler: permission mode must be non-zero")
)

type fileLeaseRecord struct {
	JobName    string    `json:"job_name"`
	NodeID     string    `json:"node_id"`
	PID        int       `json:"pid"`
	AcquiredAt time.Time `json:"acquired_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type heldFileLease struct {
	file   *os.File
	record fileLeaseRecord
}

type fileLeaseOps struct {
	mkdirAll func(string, os.FileMode) error
	openFile func(string, int, os.FileMode) (*os.File, error)
	chmod    func(*os.File, os.FileMode) error
	tryLock  func(*os.File) (bool, error)
	unlock   func(*os.File) error
	truncate func(*os.File, int64) error
	seek     func(*os.File, int64, int) (int64, error)
	write    func(*os.File, []byte) (int, error)
	sync     func(*os.File) error
	close    func(*os.File) error
}

func defaultFileLeaseOps() fileLeaseOps {
	return fileLeaseOps{
		mkdirAll: os.MkdirAll,
		openFile: os.OpenFile,
		chmod:    func(f *os.File, perm os.FileMode) error { return f.Chmod(perm) },
		tryLock:  tryFlockExclusive,
		unlock:   unlockFlock,
		truncate: func(f *os.File, size int64) error { return f.Truncate(size) },
		seek:     func(f *os.File, offset int64, whence int) (int64, error) { return f.Seek(offset, whence) },
		write:    func(f *os.File, data []byte) (int, error) { return f.Write(data) },
		sync:     func(f *os.File) error { return f.Sync() },
		close:    func(f *os.File) error { return f.Close() },
	}
}

// FileLeaseOption configures a FileLeaseStore instance.
type FileLeaseOption func(*FileLeaseStore) error

// WithFileLeaseMode sets the file permission mode for created lock files (default: 0600).
func WithFileLeaseMode(perm os.FileMode) FileLeaseOption {
	return func(s *FileLeaseStore) error {
		if perm == 0 {
			return ErrInvalidFileMode
		}
		s.filePerm = perm
		return nil
	}
}

// WithFileLeaseDirMode sets the directory permission mode used when creating the lock directory (default: 0700).
func WithFileLeaseDirMode(perm os.FileMode) FileLeaseOption {
	return func(s *FileLeaseStore) error {
		if perm == 0 {
			return ErrInvalidFileMode
		}
		s.dirPerm = perm
		return nil
	}
}

// WithFileLeasePID overrides the process ID written into lock files (defaults to os.Getpid()).
func WithFileLeasePID(pid int) FileLeaseOption {
	return func(s *FileLeaseStore) error {
		if pid > 0 {
			s.pid = pid
		}
		return nil
	}
}

// WithProcessAliveChecker overrides the liveness probe used to detect stale PIDs in lock files.
func WithProcessAliveChecker(fn func(pid int) bool) FileLeaseOption {
	return func(s *FileLeaseStore) error {
		if fn != nil {
			s.isProcessAlive = fn
		}
		return nil
	}
}

// FileLeaseStore is a local single-machine LeaseStore combining OS advisory file locks (flock)
// with JSON PID/TTL lease metadata files. It provides crash-safe cross-process mutual exclusion
// for desktop apps, CLI tools, and LaunchAgent/systemd daemons sharing a machine.
type FileLeaseStore struct {
	mu             sync.Mutex
	dir            string
	filePerm       os.FileMode
	dirPerm        os.FileMode
	pid            int
	isProcessAlive func(pid int) bool
	ops            fileLeaseOps
	held           map[string]*heldFileLease
}

// NewFileLeaseStore initializes a FileLeaseStore backed by lock files inside dir.
// The lock directory is created automatically with 0700 permissions unless overridden.
func NewFileLeaseStore(dir string, opts ...FileLeaseOption) (*FileLeaseStore, error) {
	trimmedDir := strings.TrimSpace(dir)
	if trimmedDir == "" {
		return nil, ErrInvalidLockDir
	}

	store := &FileLeaseStore{
		dir:            filepath.Clean(trimmedDir),
		filePerm:       defaultFileLeasePerm,
		dirPerm:        defaultFileLeaseDir,
		pid:            os.Getpid(),
		isProcessAlive: isProcessAliveOS,
		ops:            defaultFileLeaseOps(),
		held:           make(map[string]*heldFileLease),
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(store); err != nil {
			return nil, err
		}
	}

	if err := store.ops.mkdirAll(store.dir, store.dirPerm); err != nil {
		return nil, err
	}
	return store, nil
}

// LockFilePath returns the deterministic, traversal-safe lock file path for jobName.
func (s *FileLeaseStore) LockFilePath(jobName string) string {
	trimmed := strings.TrimSpace(jobName)
	sum := sha256.Sum256([]byte(trimmed))
	slug := sanitizeJobSlug(trimmed)
	return filepath.Join(s.dir, slug+"-"+hex.EncodeToString(sum[:6])+".lock")
}

// TryAcquire acquires or renews the OS advisory flock and PID/TTL lease record for jobName.
// Returns (false, nil) when another process holds the OS flock or another live node holds an unexpired lease.
func (s *FileLeaseStore) TryAcquire(ctx context.Context, jobName, nodeID string, now time.Time, ttl time.Duration) (bool, error) {
	name, holder, err := validateFileLeaseAcquireArgs(ctx, jobName, nodeID, ttl)
	if err != nil {
		return false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	nowUTC := now.UTC()
	nextRecord := fileLeaseRecord{
		JobName:    name,
		NodeID:     holder,
		PID:        s.pid,
		AcquiredAt: nowUTC,
		ExpiresAt:  nowUTC.Add(ttl),
	}

	if existing, ok := s.held[name]; ok {
		return s.tryAcquireHeld(existing, nextRecord)
	}

	return s.tryAcquireNew(name, nextRecord)
}

func validateFileLeaseAcquireArgs(ctx context.Context, jobName, nodeID string, ttl time.Duration) (name, holder string, err error) {
	if err = ctx.Err(); err != nil {
		return "", "", err
	}
	name = strings.TrimSpace(jobName)
	if name == "" {
		return "", "", ErrInvalidJobName
	}
	holder = strings.TrimSpace(nodeID)
	if holder == "" {
		return "", "", ErrEmptyNodeID
	}
	if ttl <= 0 {
		return "", "", ErrInvalidLeaseTTL
	}
	return name, holder, nil
}

func (s *FileLeaseStore) isLiveOtherHolder(rec fileLeaseRecord, holder string, nowUTC time.Time) bool {
	return rec.NodeID != holder && nowUTC.Before(rec.ExpiresAt) && s.isProcessAlive(rec.PID)
}

func (s *FileLeaseStore) tryAcquireHeld(existing *heldFileLease, nextRecord fileLeaseRecord) (bool, error) {
	if s.isLiveOtherHolder(existing.record, nextRecord.NodeID, nextRecord.AcquiredAt) {
		return false, nil
	}
	if err := s.writeLeaseRecord(existing.file, nextRecord); err != nil {
		return false, err
	}
	existing.record = nextRecord
	return true, nil
}

func (s *FileLeaseStore) tryAcquireNew(name string, nextRecord fileLeaseRecord) (bool, error) {
	lockPath := s.LockFilePath(name)
	file, err := s.ops.openFile(lockPath, os.O_CREATE|os.O_RDWR, s.filePerm)
	if err != nil {
		return false, err
	}
	if err := s.ops.chmod(file, s.filePerm); err != nil {
		_ = s.ops.close(file)
		return false, err
	}

	locked, err := s.ops.tryLock(file)
	if err != nil {
		_ = s.ops.close(file)
		return false, err
	}
	if !locked {
		_ = s.ops.close(file)
		return false, nil
	}

	if existingRec, ok := s.readLeaseRecord(file); ok && s.isLiveOtherHolder(existingRec, nextRecord.NodeID, nextRecord.AcquiredAt) {
		_ = s.ops.unlock(file)
		_ = s.ops.close(file)
		return false, nil
	}

	if err := s.writeLeaseRecord(file, nextRecord); err != nil {
		_ = s.ops.unlock(file)
		_ = s.ops.close(file)
		return false, err
	}

	s.held[name] = &heldFileLease{
		file:   file,
		record: nextRecord,
	}
	return true, nil
}

// Release relinquishes the named job lease and unlocks its OS advisory flock if currently held by nodeID.
func (s *FileLeaseStore) Release(ctx context.Context, jobName, nodeID string) error {
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

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.held[name]; ok {
		if existing.record.NodeID != holder {
			return nil
		}
		_ = s.ops.truncate(existing.file, 0)
		unlockErr := s.ops.unlock(existing.file)
		closeErr := s.ops.close(existing.file)
		delete(s.held, name)
		return errors.Join(unlockErr, closeErr)
	}

	lockPath := s.LockFilePath(name)
	file, err := s.ops.openFile(lockPath, os.O_RDWR, s.filePerm)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer func() { _ = s.ops.close(file) }()

	locked, err := s.ops.tryLock(file)
	if err != nil || !locked {
		return err
	}
	defer func() { _ = s.ops.unlock(file) }()

	if rec, ok := s.readLeaseRecord(file); ok && rec.NodeID == holder {
		return s.ops.truncate(file, 0)
	}
	return nil
}

// Close releases all OS advisory file locks and closes every open lock file descriptor held by this store.
// Calling Close multiple times is safe and idempotent.
func (s *FileLeaseStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var errs []error
	for name, existing := range s.held {
		_ = s.ops.truncate(existing.file, 0)
		if err := s.ops.unlock(existing.file); err != nil {
			errs = append(errs, err)
		}
		if err := s.ops.close(existing.file); err != nil {
			errs = append(errs, err)
		}
		delete(s.held, name)
	}
	return errors.Join(errs...)
}

func (s *FileLeaseStore) readLeaseRecord(f *os.File) (fileLeaseRecord, bool) {
	if _, err := s.ops.seek(f, 0, io.SeekStart); err != nil {
		return fileLeaseRecord{}, false
	}
	data, err := io.ReadAll(io.LimitReader(f, 4096))
	if err != nil || len(data) == 0 {
		return fileLeaseRecord{}, false
	}
	var rec fileLeaseRecord
	if err := json.Unmarshal(data, &rec); err != nil || rec.NodeID == "" {
		return fileLeaseRecord{}, false
	}
	return rec, true
}

func (s *FileLeaseStore) writeLeaseRecord(f *os.File, rec fileLeaseRecord) error {
	payload, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if err := s.ops.truncate(f, 0); err != nil {
		return err
	}
	if _, err := s.ops.seek(f, 0, io.SeekStart); err != nil {
		return err
	}
	if _, err := s.ops.write(f, payload); err != nil {
		return err
	}
	return s.ops.sync(f)
}

func sanitizeJobSlug(jobName string) string {
	var b strings.Builder
	for i := 0; i < len(jobName) && b.Len() < 32; i++ {
		c := jobName[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			b.WriteByte(c)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "job"
	}
	return b.String()
}
