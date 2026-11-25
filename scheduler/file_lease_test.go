package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewFileLeaseStore_OptionsAndErrors(t *testing.T) {
	t.Parallel()

	if _, err := NewFileLeaseStore("   "); !errors.Is(err, ErrInvalidLockDir) {
		t.Fatalf("expected ErrInvalidLockDir, got %v", err)
	}

	dir := t.TempDir()
	if _, err := NewFileLeaseStore(dir, WithFileLeaseMode(0)); !errors.Is(err, ErrInvalidFileMode) {
		t.Fatalf("expected ErrInvalidFileMode on file mode 0, got %v", err)
	}
	if _, err := NewFileLeaseStore(dir, WithFileLeaseDirMode(0)); !errors.Is(err, ErrInvalidFileMode) {
		t.Fatalf("expected ErrInvalidFileMode on dir mode 0, got %v", err)
	}

	// Passing a file path as lock dir causes MkdirAll failure.
	blocker := filepath.Join(dir, "file-not-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("setup write failed: %v", err)
	}
	if _, err := NewFileLeaseStore(filepath.Join(blocker, "nested")); err == nil {
		t.Fatalf("expected MkdirAll error when parent is a file")
	}

	nestedDir := filepath.Join(dir, "locks")
	store, err := NewFileLeaseStore(
		nestedDir,
		nil,
		WithFileLeaseMode(0o600),
		WithFileLeaseDirMode(0o700),
		WithFileLeasePID(0),
		WithFileLeasePID(4242),
		WithProcessAliveChecker(nil),
		WithProcessAliveChecker(func(int) bool { return true }),
	)
	if err != nil {
		t.Fatalf("NewFileLeaseStore failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	if store.pid != 4242 {
		t.Fatalf("expected pid 4242, got %d", store.pid)
	}
	if p := store.LockFilePath("   "); !strings.HasPrefix(filepath.Base(p), "job-") {
		t.Fatalf("expected fallback job slug, got %q", p)
	}
	if p := store.LockFilePath("a/b/../sync:job"); strings.Contains(filepath.Base(p), "/") || strings.Contains(filepath.Base(p), "..") {
		t.Fatalf("expected traversal-safe lock filename, got %q", p)
	}
}

func TestFileLeaseStore_AcquireRenewFencingAndRelease(t *testing.T) {
	t.Parallel()

	lockDir := t.TempDir()
	now := time.Date(2026, 4, 15, 9, 0, 0, 0, time.UTC)
	ctx := context.Background()

	store1, err := NewFileLeaseStore(lockDir, WithFileLeasePID(1001), WithProcessAliveChecker(func(int) bool { return true }))
	if err != nil {
		t.Fatalf("NewFileLeaseStore store1 failed: %v", err)
	}
	defer func() { _ = store1.Close() }()

	store2, err := NewFileLeaseStore(lockDir, WithFileLeasePID(1002), WithProcessAliveChecker(func(int) bool { return true }))
	if err != nil {
		t.Fatalf("NewFileLeaseStore store2 failed: %v", err)
	}
	defer func() { _ = store2.Close() }()

	// Input validation
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store1.TryAcquire(canceledCtx, "sync", "node-1", now, time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled on TryAcquire, got %v", err)
	}
	if err := store1.Release(canceledCtx, "sync", "node-1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled on Release, got %v", err)
	}
	if _, err := store1.TryAcquire(ctx, " ", "node-1", now, time.Minute); !errors.Is(err, ErrInvalidJobName) {
		t.Fatalf("expected ErrInvalidJobName, got %v", err)
	}
	if _, err := store1.TryAcquire(ctx, "sync", " ", now, time.Minute); !errors.Is(err, ErrEmptyNodeID) {
		t.Fatalf("expected ErrEmptyNodeID, got %v", err)
	}
	if _, err := store1.TryAcquire(ctx, "sync", "node-1", now, 0); !errors.Is(err, ErrInvalidLeaseTTL) {
		t.Fatalf("expected ErrInvalidLeaseTTL, got %v", err)
	}
	if err := store1.Release(ctx, "", "node-1"); !errors.Is(err, ErrInvalidJobName) {
		t.Fatalf("expected ErrInvalidJobName on Release, got %v", err)
	}
	if err := store1.Release(ctx, "sync", ""); !errors.Is(err, ErrEmptyNodeID) {
		t.Fatalf("expected ErrEmptyNodeID on Release, got %v", err)
	}

	// Releasing non-existent lock file is a clean no-op
	if err := store1.Release(ctx, "nonexistent", "node-1"); err != nil {
		t.Fatalf("expected nil releasing non-existent lock, got %v", err)
	}

	// Store1 acquires OS flock + writes 0600 JSON PID metadata
	ok, err := store1.TryAcquire(ctx, "desktop-sync", "launchagent", now, 30*time.Second)
	if err != nil || !ok {
		t.Fatalf("expected store1 to acquire lease, ok=%v err=%v", ok, err)
	}

	info, err := os.Stat(store1.LockFilePath("desktop-sync"))
	if err != nil {
		t.Fatalf("stat lock file failed: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("expected 0600 permissions on lock file, got %04o", perm)
	}

	// Store2 (separate open fd / simulates sibling process) is blocked by OS flock
	ok2, err := store2.TryAcquire(ctx, "desktop-sync", "gui-app", now.Add(5*time.Second), 30*time.Second)
	if err != nil || ok2 {
		t.Fatalf("expected store2 to be fenced by OS flock, ok=%v err=%v", ok2, err)
	}

	// Store2 releasing while store1 holds flock is a safe no-op
	if err := store2.Release(ctx, "desktop-sync", "gui-app"); err != nil {
		t.Fatalf("expected nil releasing lock held by another fd, got %v", err)
	}

	// Another node on the same store1 instance is fenced while unexpired
	okSameStore, err := store1.TryAcquire(ctx, "desktop-sync", "other-node", now.Add(5*time.Second), 30*time.Second)
	if err != nil || okSameStore {
		t.Fatalf("expected other-node on same store to be fenced, ok=%v err=%v", okSameStore, err)
	}

	// Same holder renews in-place
	renewed, err := store1.TryAcquire(ctx, "desktop-sync", "launchagent", now.Add(10*time.Second), 30*time.Second)
	if err != nil || !renewed {
		t.Fatalf("expected launchagent to renew lease, renewed=%v err=%v", renewed, err)
	}

	// Non-holder Release on same store1 does not drop launchagent's lease
	if err := store1.Release(ctx, "desktop-sync", "other-node"); err != nil {
		t.Fatalf("expected nil releasing non-owned held lease, got %v", err)
	}

	// After expiry, other-node on same store1 can take over in-place
	takenSameStore, err := store1.TryAcquire(ctx, "desktop-sync", "other-node", now.Add(60*time.Second), 30*time.Second)
	if err != nil || !takenSameStore {
		t.Fatalf("expected takeover after TTL expiry, ok=%v err=%v", takenSameStore, err)
	}

	// Release unlocks OS flock so store2 can immediately acquire
	if err := store1.Release(ctx, "desktop-sync", "other-node"); err != nil {
		t.Fatalf("Release failed: %v", err)
	}
	ok2, err = store2.TryAcquire(ctx, "desktop-sync", "gui-app", now.Add(61*time.Second), 30*time.Second)
	if err != nil || !ok2 {
		t.Fatalf("expected store2 to acquire after release, ok=%v err=%v", ok2, err)
	}

	// Close releases all held locks idempotently
	if err := store2.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if err := store2.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
}

func TestFileLeaseStore_PersistedRecordFencingAndStalePIDRecovery(t *testing.T) {
	t.Parallel()

	lockDir := t.TempDir()
	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	ctx := context.Background()

	alivePID := 7777
	deadPID := 8888
	alive := true

	store, err := NewFileLeaseStore(
		lockDir,
		WithFileLeasePID(9999),
		WithProcessAliveChecker(func(pid int) bool {
			return pid == alivePID && alive
		}),
	)
	if err != nil {
		t.Fatalf("NewFileLeaseStore failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Pre-populate an un-flocked active lease file for a live PID (e.g. non-flock peer or reopened file)
	lockPath := store.LockFilePath("billing-sync")
	rawRec, _ := json.Marshal(fileLeaseRecord{
		JobName:    "billing-sync",
		NodeID:     "live-peer",
		PID:        alivePID,
		AcquiredAt: now,
		ExpiresAt:  now.Add(10 * time.Minute),
	})
	if err := os.WriteFile(lockPath, rawRec, 0o600); err != nil {
		t.Fatalf("setup write failed: %v", err)
	}

	// Different node must respect unexpired live-PID record
	ok, err := store.TryAcquire(ctx, "billing-sync", "node-b", now.Add(time.Minute), time.Minute)
	if err != nil || ok {
		t.Fatalf("expected unexpired live-PID lease to block node-b, ok=%v err=%v", ok, err)
	}

	// Releasing disk-only record as non-owner is a no-op
	if err := store.Release(ctx, "billing-sync", "node-b"); err != nil {
		t.Fatalf("unexpected error releasing disk record as non-owner: %v", err)
	}

	// Releasing disk-only record as owner truncates it cleanly
	if err := store.Release(ctx, "billing-sync", "live-peer"); err != nil {
		t.Fatalf("unexpected error releasing disk record as owner: %v", err)
	}

	// Pre-populate a stale dead-PID unexpired lease file: store should recover & acquire immediately!
	staleRec, _ := json.Marshal(fileLeaseRecord{
		JobName:    "billing-sync",
		NodeID:     "crashed-peer",
		PID:        deadPID,
		AcquiredAt: now,
		ExpiresAt:  now.Add(10 * time.Minute),
	})
	if err := os.WriteFile(lockPath, staleRec, 0o600); err != nil {
		t.Fatalf("setup write failed: %v", err)
	}
	ok, err = store.TryAcquire(ctx, "billing-sync", "node-b", now.Add(time.Minute), time.Minute)
	if err != nil || !ok {
		t.Fatalf("expected dead-PID lease recovery to succeed, ok=%v err=%v", ok, err)
	}

	// Corrupt JSON file is safely overwritten on TryAcquire
	_ = store.Close()
	if err := os.WriteFile(lockPath, []byte("{corrupt-json"), 0o600); err != nil {
		t.Fatalf("setup write failed: %v", err)
	}
	ok, err = store.TryAcquire(ctx, "billing-sync", "node-b", now.Add(2*time.Minute), time.Minute)
	if err != nil || !ok {
		t.Fatalf("expected corrupt lockfile recovery to succeed, ok=%v err=%v", ok, err)
	}
}

func TestFileLeaseStore_CrossProcessExclusionAndScheduler(t *testing.T) {
	if os.Getenv("GO_APP_KIT_FLOCK_HELPER") == "1" {
		lockDir := os.Getenv("GO_APP_KIT_FLOCK_DIR")
		store, err := NewFileLeaseStore(lockDir)
		if err != nil {
			os.Exit(2)
		}
		defer func() { _ = store.Close() }()
		ok, err := store.TryAcquire(context.Background(), "cross-proc-job", "child-proc", time.Now().UTC(), time.Minute)
		if err != nil {
			os.Exit(3)
		}
		if ok {
			os.Exit(4) // Should NOT acquire while parent holds OS flock!
		}
		os.Exit(0)
	}

	lockDir := t.TempDir()
	parentStore, err := NewFileLeaseStore(lockDir)
	if err != nil {
		t.Fatalf("NewFileLeaseStore failed: %v", err)
	}
	defer func() { _ = parentStore.Close() }()

	ok, err := parentStore.TryAcquire(context.Background(), "cross-proc-job", "parent-proc", time.Now().UTC(), time.Minute)
	if err != nil || !ok {
		t.Fatalf("parent acquire failed: ok=%v err=%v", ok, err)
	}

	// Spawn a real sibling OS process and verify kernel flock blocks it.
	cmd := exec.Command(os.Args[0], "-test.run=^TestFileLeaseStore_CrossProcessExclusionAndScheduler$")
	cmd.Env = append(os.Environ(),
		"GO_APP_KIT_FLOCK_HELPER=1",
		"GO_APP_KIT_FLOCK_DIR="+lockDir,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cross-process flock check failed: err=%v output=%s", err, string(out))
	}

	// Verify integration with Scheduler + concurrent goroutines under -race.
	var runs atomic.Int32
	sched, err := NewScheduler(parentStore, "parent-proc")
	if err != nil {
		t.Fatalf("NewScheduler failed: %v", err)
	}
	if err := sched.Register(JobConfig{
		Name:     "concurrent-job",
		Interval: time.Second,
		LeaseTTL: time.Minute,
		Fn: func(context.Context) error {
			runs.Add(1)
			return nil
		},
	}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = sched.RunOnce(context.Background(), "concurrent-job")
		}()
	}
	wg.Wait()
	if runs.Load() == 0 {
		t.Fatalf("expected concurrent-job to run at least once")
	}
}

func TestFileLeaseStore_OSPrimitivesAndErrorBranches(t *testing.T) {
	t.Parallel()

	// Exercise real OS helpers (isProcessAliveOS and tryFlockExclusive error path)
	if isProcessAliveOS(0) || isProcessAliveOS(-5) {
		t.Fatalf("expected non-positive PID to report not alive")
	}
	if !isProcessAliveOS(os.Getpid()) {
		t.Fatalf("expected current process PID to report alive")
	}
	if isProcessAliveOS(99999999) {
		t.Fatalf("expected nonexistent PID to report not alive")
	}

	tmpFile, err := os.CreateTemp(t.TempDir(), "closed-*.lock")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	_ = tmpFile.Close()
	if locked, err := tryFlockExclusive(tmpFile); locked || err == nil {
		t.Fatalf("expected error from tryFlockExclusive on closed fd, got locked=%v err=%v", locked, err)
	}

	simErr := errors.New("simulated os error")
	ctx := context.Background()
	now := time.Date(2026, 4, 15, 11, 0, 0, 0, time.UTC)

	newStore := func() *FileLeaseStore {
		s, err := NewFileLeaseStore(t.TempDir())
		if err != nil {
			t.Fatalf("NewFileLeaseStore failed: %v", err)
		}
		return s
	}

	// OpenFile error on TryAcquire & Release
	s1 := newStore()
	s1.ops.openFile = func(string, int, os.FileMode) (*os.File, error) { return nil, simErr }
	if _, err := s1.TryAcquire(ctx, "j", "n", now, time.Minute); !errors.Is(err, simErr) {
		t.Fatalf("expected simErr on openFile TryAcquire, got %v", err)
	}
	if err := s1.Release(ctx, "j", "n"); !errors.Is(err, simErr) {
		t.Fatalf("expected simErr on openFile Release, got %v", err)
	}

	// Chmod error
	s2 := newStore()
	s2.ops.chmod = func(*os.File, os.FileMode) error { return simErr }
	if _, err := s2.TryAcquire(ctx, "j", "n", now, time.Minute); !errors.Is(err, simErr) {
		t.Fatalf("expected simErr on chmod, got %v", err)
	}

	// tryLock error
	s3 := newStore()
	s3.ops.tryLock = func(*os.File) (bool, error) { return false, simErr }
	if _, err := s3.TryAcquire(ctx, "j", "n", now, time.Minute); !errors.Is(err, simErr) {
		t.Fatalf("expected simErr on tryLock, got %v", err)
	}

	// writeLeaseRecord errors on initial acquire and renewal (marshal, truncate, seek, write, sync)
	s4 := newStore()
	defer func() { _ = s4.Close() }()
	// Year 10000 triggers json.Marshal error on time.Time
	badTime := time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := s4.TryAcquire(ctx, "j-badtime", "n", badTime, time.Minute); err == nil {
		t.Fatalf("expected json.Marshal error on year 10000")
	}

	ok, err := s4.TryAcquire(ctx, "j-renew", "n", now, time.Minute)
	if err != nil || !ok {
		t.Fatalf("initial acquire failed: %v", err)
	}

	origOps := s4.ops
	s4.ops.truncate = func(*os.File, int64) error { return simErr }
	if _, err := s4.TryAcquire(ctx, "j-renew", "n", now, time.Minute); !errors.Is(err, simErr) {
		t.Fatalf("expected simErr on truncate, got %v", err)
	}
	s4.ops = origOps

	s4.ops.seek = func(*os.File, int64, int) (int64, error) { return 0, simErr }
	if _, err := s4.TryAcquire(ctx, "j-renew", "n", now, time.Minute); !errors.Is(err, simErr) {
		t.Fatalf("expected simErr on seek, got %v", err)
	}
	// Also test readLeaseRecord seek error branch
	if _, ok := s4.readLeaseRecord(s4.held["j-renew"].file); ok {
		t.Fatalf("expected readLeaseRecord to return false on seek error")
	}
	s4.ops = origOps

	s4.ops.write = func(*os.File, []byte) (int, error) { return 0, simErr }
	if _, err := s4.TryAcquire(ctx, "j-renew", "n", now, time.Minute); !errors.Is(err, simErr) {
		t.Fatalf("expected simErr on write, got %v", err)
	}
	s4.ops = origOps

	s4.ops.sync = func(*os.File) error { return simErr }
	if _, err := s4.TryAcquire(ctx, "j-renew", "n", now, time.Minute); !errors.Is(err, simErr) {
		t.Fatalf("expected simErr on sync, got %v", err)
	}
	s4.ops = origOps

	// Close unlock & close error branches
	s5 := newStore()
	if ok, err := s5.TryAcquire(ctx, "j-close", "n", now, time.Minute); err != nil || !ok {
		t.Fatalf("acquire failed: %v", err)
	}
	realClose := s5.ops.close
	s5.ops.unlock = func(*os.File) error { return simErr }
	s5.ops.close = func(f *os.File) error {
		_ = realClose(f)
		return simErr
	}
	if err := s5.Close(); !errors.Is(err, simErr) {
		t.Fatalf("expected Close to join unlock/close errors, got %v", err)
	}
}
