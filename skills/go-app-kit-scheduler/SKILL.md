---
name: go-app-kit-scheduler
description: "Run distributed lease-fenced periodic background jobs with panic recovery and graceful in-flight draining using github.com/abeta-dev/go-app-kit/scheduler. Use when scheduling recurring billing cycles, GST aging snapshots, outbox dead-letter sweeps, or PostgreSQL partition maintenance across multiple service replicas."
---

# `go-app-kit/scheduler` — Distributed Lease-Fenced Periodic Job Scheduler Subskill

Use this skill when scheduling singleton periodic background jobs across horizontally scaled service replicas with `github.com/abeta-dev/go-app-kit/scheduler`.

## 1. Concurrency & Reliability Guarantees

- **Local OS `flock` + PID-File Lease Store (`NewFileLeaseStore`)**: Combines non-blocking POSIX advisory `flock(2)` locks with `0600` JSON PID/TTL lease files for single-machine cross-process mutual exclusion (desktop GUI + LaunchAgent/systemd daemon) with automatic crash/stale-PID recovery. Always `defer store.Close()` to release open lock file descriptors.
- **Distributed Lease Fencing (`LeaseStore`)**: Calls `TryAcquire(ctx, jobName, nodeID, now, leaseTTL)` before every job tick. If another replica holds an active lease, `RunOnce` returns `(false, nil)` without duplicate execution.
- **Panic Recovery Barrier**: Wraps every `JobFunc` invocation in a `recover()` guard that converts panics into structured errors and forwards them to `WithErrorHandler`.
- **Graceful In-Flight Draining (`Start(ctx)`)**: Stops all periodic tickers on context cancellation and waits via `sync.WaitGroup` for all in-flight jobs to complete before returning.

---

## 2. Registering & Running Lease-Fenced Periodic Jobs

```go
import (
	"context"
	"log"
	"time"

	"github.com/abeta-dev/go-app-kit/scheduler"
)

func RunMaintenanceScheduler(ctx context.Context, lockDir, nodeID string) error {
	// Use NewFileLeaseStore for single-machine cross-process exclusion (LaunchAgent + GUI)
	// or NewMemoryLeaseStore() for single-process/unit tests.
	store, err := scheduler.NewFileLeaseStore(lockDir)
	if err != nil {
		return err
	}
	defer store.Close()

	sched, err := scheduler.NewScheduler(
		store,
		nodeID,
		scheduler.WithErrorHandler(func(jobName string, err error) {
			log.Printf("scheduled job %s failed: %v", jobName, err)
		}),
	)
	if err != nil {
		return err
	}

	err = sched.Register(scheduler.JobConfig{
		Name:       "gst-aging-snapshot",
		Interval:   15 * time.Minute,
		LeaseTTL:   14 * time.Minute,
		RunOnStart: true,
		Fn: func(ctx context.Context) error {
			// Execute singleton periodic task here
			return nil
		},
	})
	if err != nil {
		return err
	}

	return sched.Start(ctx)
}
```
