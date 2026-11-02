---
name: go-app-kit-audit
description: "Record tamper-evident compliance audit trails and field-level JSON state diffs using github.com/umesh0492/go-app-kit/audit and PostgreSQL range partitioning. Use when tracking entity mutations, computing before/after state diffs, or applying append-only PostgreSQL audit DDL."
---

# `go-app-kit/audit` — Partitioned Compliance Audit Trail Subskill

Use this skill when implementing compliance audit logging and automated before/after JSON state diffing with `github.com/umesh0492/go-app-kit/audit`.

## 1. Required PostgreSQL Schema (`audit/ddl/001_audit_logs.sql`)

Apply `audit/ddl/001_audit_logs.sql` to provision:
- `audit_logs` partitioned by `RANGE (created_at)` for zero-downtime retention management.
- `trg_prevent_audit_log_modification`: PostgreSQL trigger that rejects `UPDATE` and `DELETE` statements on `audit_logs`, enforcing an immutable append-only ledger.

---

## 2. Context Enrichment & Recording Mutations

Attach actor identity, client IP, and reason metadata to `context.Context` in HTTP middleware, then create and record audit events in domain handlers:

```go
import (
	"context"
	"github.com/umesh0492/go-app-kit/audit"
)

// 1. Enrich context in auth/request middleware
ctx = audit.WithActor(ctx, "user-42")
ctx = audit.WithIP(ctx, clientIP)
ctx = audit.WithComment(ctx, "Approved credit note adjustment")

// 2. Initialize PostgreSQL recorder with background workerpool
recorder := audit.NewPGRecorder(pgPool, audit.WithWorkers(4, 128))
defer recorder.Close()

// 3. Compute field-level JSON diff (beforeState vs afterState) and record
evt := audit.NewEvent(
	ctx,
	"INVOICE_STATUS_CHANGE",
	"Invoice",
	invoiceID,
	beforeInvoice, // struct or map (pass nil for CREATE)
	afterInvoice,  // struct or map (pass nil for DELETE)
)

// Non-blocking async persistence via bounded workerpool:
if err := recorder.RecordAsync(evt); err != nil {
	// fallback or log queue saturation
}
```

---

## 3. Standalone Field-Level Diffing (`ComputeDiff`)

You can call `audit.ComputeDiff(oldState, newState)` directly to inspect property changes (`map[string]audit.FieldDiff{Old, New}`) for custom validation or change-log UI views.
