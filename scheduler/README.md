# `scheduler`

Distributed, lease-fenced periodic job scheduler with panic recovery and graceful in-flight draining for recurring billing runs, GST aging snapshots, and PostgreSQL partition maintenance.

---

## When to Use

- **Singleton Cron / Periodic Jobs Across $N$ Replicas**: Ensuring only one service pod executes a recurring billing cycle, outbox dead-letter sweep, or monthly `audit_logs` PostgreSQL range-partition creation within each lease TTL window.
- **Panic-Isolated Background Workers**: Protecting the host process from crashing if a scheduled task panics, automatically converting panics into structured errors forwarded to `WithErrorHandler`.
- **Deterministic Unit & Integration Tests**: Testing multi-node lease contention and expiration takeover in microseconds using `NewMemoryLeaseStore()` and `WithClock`.

---

## Key Capabilities & Guarantees

1. **Distributed Lease Fencing (`LeaseStore`)**: Before every execution tick, `Scheduler` invokes `store.TryAcquire(ctx, jobName, nodeID, now, leaseTTL)`. Replicas that do not hold the active lease skip the tick cleanly with `(false, nil)`.
2. **Panic Recovery (`executeJobSafe`)**: Every job execution is wrapped in a `recover()` barrier that converts panics into errors and notifies `WithErrorHandler` without terminating sibling jobs.
3. **Graceful In-Flight Shutdown (`Start(ctx)`)**: Canceling `ctx` stops all job tickers immediately and waits via `sync.WaitGroup` for every in-flight job execution to complete before returning.

---

## Usage Example

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/abeta-dev/go-app-kit/scheduler"
)

func main() {
	store := scheduler.NewMemoryLeaseStore()
	sched, err := scheduler.NewScheduler(
		store,
		"billing-worker-pod-1",
		scheduler.WithErrorHandler(func(jobName string, err error) {
			fmt.Printf("job %s failed: %v\n", jobName, err)
		}),
	)
	if err != nil {
		panic(err)
	}

	err = sched.Register(scheduler.JobConfig{
		Name:       "gst-aging-snapshot",
		Interval:   1 * time.Hour,
		LeaseTTL:   55 * time.Minute,
		RunOnStart: true,
		Fn: func(ctx context.Context) error {
			fmt.Println("Computing statutory AP/AR GST aging buckets...")
			return nil
		},
	})
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_ = sched.Start(ctx)
}
```
