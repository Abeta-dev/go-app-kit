---
name: go-app-kit-outbox
description: "Implement reliable at-least-once event publishing using github.com/Abeta-dev/go-app-kit/outbox with PostgreSQL SELECT FOR UPDATE SKIP LOCKED, UUID lease-token fencing, and exponential jitter backoff. Use when solving dual-write problems, wiring outbox relays, or applying PostgreSQL outbox DDL."
---

# `go-app-kit/outbox` — PostgreSQL Transactional Outbox Subskill

Use this skill when implementing transactional event publishing in Go microservices with PostgreSQL (`github.com/jackc/pgx/v5`).

## 1. Required PostgreSQL Schema (DDL)

Apply both DDL migrations in order before starting the outbox store or relay:
- `outbox/ddl/001_outbox_events.sql` — Creates `outbox_events` with `lease_token UUID`, `locked_until TIMESTAMPTZ`, `retry_count`, and `status` (`PENDING`, `PROCESSING`, `PUBLISHED`, `DEAD_LETTER`).
- `outbox/ddl/002_outbox_concurrency_index.sql` — Creates composite partial index `idx_outbox_poll` for contention-free `SKIP LOCKED` polling and `idx_outbox_aggregate` for entity history lookups.

> **Important**: `outbox` exclusively targets PostgreSQL (`$1, $2` parameter placeholders and `FOR UPDATE SKIP LOCKED`). Do not use with MySQL or SQLite.

---

## 2. Transactional Producer Pattern (Dual-Write Prevention)

Always insert the outbox event inside the **same database transaction (`pgx.Tx`)** that mutates domain tables:

```go
import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/Abeta-dev/go-app-kit/outbox"
)

func CreateInvoiceTx(ctx context.Context, pool *pgxpool.Pool, store outbox.Store, inv Invoice) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Mutate domain state using tx
	if _, err := tx.Exec(ctx, `INSERT INTO invoices (id, amount_paise) VALUES ($1, $2)`, inv.ID, inv.AmountPaise); err != nil {
		return err
	}

	// 2. Construct and insert outbox event using the SAME tx
	evt, err := outbox.NewEvent("Invoice", inv.ID, "InvoiceIssued", inv)
	if err != nil {
		return err
	}
	if err := store.Insert(ctx, tx, *evt); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
```

---

## 3. Autonomous Background Relay Pattern

Run `outbox.Relay` across $N$ service replicas safely; `SELECT ... FOR UPDATE SKIP LOCKED` and UUID `lease_token` fencing prevent duplicate claims or stale worker clobbering:

```go
store := outbox.NewPGStore(pool, outbox.WithLeaseDuration(30*time.Second))

relay, err := outbox.NewRelay(outbox.RelayConfig{
	Store:          store,
	Publisher:      myKafkaOrWebhookPublisher, // implements outbox.Publisher
	PollInterval:   500 * time.Millisecond,
	PublishTimeout: 5 * time.Second,
	LeaseDuration:  30 * time.Second,
	BatchSize:      50,
	Concurrency:    2,
})
if err != nil {
	log.Fatal(err)
}

// Run until context is cancelled during graceful shutdown
go func() {
	if err := relay.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("outbox relay stopped with error: %v", err)
	}
}()
```

---

## 4. Anti-Patterns to Avoid

- **Never** pass the root `*pgxpool.Pool` to `store.Insert(ctx, db, evt)` when a domain transaction is active—pass the active `pgx.Tx` so domain state and outbox insertion commit or roll back atomically.
- **Never** omit `Idempotency-Key` / `event.ID` handling in downstream consumers; transactional outbox guarantees **at-least-once** delivery, so downstream subscribers must be idempotent.
