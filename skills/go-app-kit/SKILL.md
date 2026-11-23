---
name: go-app-kit
description: "Compose and maintain enterprise Go application workflows using github.com/abeta-dev/go-app-kit (outbox, pdf, notifications, audit, export, storage, scheduler). Use when building transactional outboxes, GST PDF invoices, multi-channel notifications, compliance audit logs, streaming CSV exports, presigned object storage, lease-fenced job schedulers, or running go-app-kit truth gates."
---

# `go-app-kit` — Enterprise Application Accelerator Skill

Use this umbrella skill when integrating `github.com/abeta-dev/go-app-kit` into a Go microservice or maintaining the `go-app-kit` repository. For deep package-level APIs and DDL schemas, load the dedicated subskill.

## 1. Subskill Routing Table

| Task / Domain | Subskill to Load | Path |
| :--- | :--- | :--- |
| PostgreSQL transactional outbox, `SKIP LOCKED` polling, lease fencing, retry/dead-letter | `go-app-kit-outbox` | `skills/go-app-kit-outbox/SKILL.md` |
| HTML-to-PDF compilation, `wkhtmltopdf` concurrency semaphore, GST invoice & receipt templates | `go-app-kit-pdf` | `skills/go-app-kit-pdf/SKILL.md` |
| Multi-channel broker (`SendAsync`), SMTP, Brevo, WhatsApp, DLT SMS, Slack, HMAC-SHA256 webhooks | `go-app-kit-notifications` | `skills/go-app-kit-notifications/SKILL.md` |
| Range-partitioned PostgreSQL audit logs, append-only triggers, JSON `ComputeDiff` | `go-app-kit-audit` | `skills/go-app-kit-audit/SKILL.md` |
| Low-memory streaming CSV export (`CSVStreamer[T]`, `StreamHTTP[T]`, `StreamHTTPCompressed[T]`, `NegotiateEncoding`), Brotli/Gzip HTTP compression, Excel UTF-8 BOM, CSV escaping | `go-app-kit-export` | `skills/go-app-kit-export/SKILL.md` |
| Zero-leak object storage (`Store`, `NewMemoryStore`, `ValidateKey`), SHA-256 content digests, and HMAC-SHA256 presigned `GET`/`PUT` URLs (`PresignGet`, `PresignPut`, `VerifyPresignedURL`) | `go-app-kit-storage` | `skills/go-app-kit-storage/SKILL.md` |
| Distributed lease-fenced periodic job scheduler (`Scheduler`, `NewScheduler`, `LeaseStore`, `NewMemoryLeaseStore`, `Register`, `RunOnce`, `Start`) with panic recovery | `go-app-kit-scheduler` | `skills/go-app-kit-scheduler/SKILL.md` |
| Indian GSTIN, PAN, Aadhaar Verhoeff D5, IFSC, UPI, exact paise `Money` math | `go-fintech-india` | Companion module `github.com/abeta-dev/go-fintech-india` |

---

## 2. End-to-End Composition Pattern (Reference: `examples/invoice_service`)

When combining `go-app-kit` packages in a business transaction (e.g., issuing an invoice):

1. **Validate Statutory Inputs & Compute Exact Paise Math** via `github.com/abeta-dev/go-fintech-india`:
   - `fintech.ValidateGSTIN(buyerGSTIN)`, `fintech.ValidatePAN(pan)`
   - Compute CGST/SGST/IGST in integer paise (`fintech.Money`).
2. **Persist Domain State + Outbox Event in One Postgres Transaction** (`go-app-kit/outbox`):
   - `evt, err := outbox.NewEvent("Invoice", inv.ID, "InvoiceIssued", payload)`
   - `err = outboxStore.Insert(ctx, tx, *evt)`
3. **Record Compliance Audit Trail** (`go-app-kit/audit`):
   - `auditEvt := audit.NewEvent(ctx, "INVOICE_CREATE", "Invoice", inv.ID, nil, map[string]any{"invoice_id": inv.ID})`
   - `_ = recorder.RecordAsync(auditEvt)`
4. **Render GST Tax Invoice PDF & Archive with Presigned URL** (`go-app-kit/pdf` + `go-app-kit/storage`):
   - `pdfBuf, err := pdf.GenerateFromTemplate(pdf.GSTInvoiceTemplate, invoiceData, pdf.WithPageSize("A4"))`
   - `meta, err := objectStore.Put(ctx, storage.PutInput{Key: "invoices/" + inv.ID + ".pdf", ContentType: "application/pdf", Body: pdfBuf})`
   - `downloadURL, err := objectStore.PresignGet(ctx, meta.Key, 15*time.Minute)`
5. **Dispatch Multi-Channel Notification** (`go-app-kit/notifications`):
   - `err = broker.SendAsync(ctx, notifications.Message{...})`
6. **Stream Bulk CSV Reports & Run Lease-Fenced Periodic Jobs** (`go-app-kit/export` + `go-app-kit/scheduler`):
   - `err = export.StreamHTTP(w, "invoices.csv", columns, invoices)` (or `export.StreamHTTPCompressed(w, r, "invoices.csv", columns, invoices)` for RFC 9110 `br`/`gzip` negotiation)
   - `err = sched.Register(scheduler.JobConfig{Name: "gst-aging-snapshot", Interval: time.Hour, LeaseTTL: 55 * time.Minute, Fn: snapshotFn})`

---

## 3. Repository Maintenance & Truth-Gate Loop

Before finishing any code or documentation change inside `go-app-kit`, run this exact verification loop:

```bash
# 1. Run static Bash 3.2 compatibility guard (no mapfile/readarray allowed)
./scripts/test_bash_compat.sh

# 2. Run TruffleHog + deterministic secret scanning gate across all tracked files
./scripts/check_secrets.sh

# 2b. Run leaf-import sibling coupling & heavy dependency isolation gate
./scripts/check_leaf_imports.sh

# 2c. Run strict N+2 minor-release deprecation window gate (see docs/VERSIONING.md)
./scripts/check_deprecation.sh

# 3. Run statement coverage gate (verifies 100.0% coverage across all 7 packages + examples/invoice_service)
./scripts/check_coverage.sh

# 4. Run version, symbol inventory, package count, secret scan, and coverage truth gate
./scripts/check_version.sh

# 5. Run release proxy verification unit tests
./scripts/test_verify_release.sh

# 6. Run race detector across all packages
go test -race ./...

# 7. Before tagging a release vX.Y.Z
make check-release-readiness TAG=vX.Y.Z
```

### Common Failure Modes & Fixes
- **Secret scan failure (`./scripts/check_secrets.sh`)**: Runs `trufflehog git file://. --only-verified --fail` (when installed) and deterministic regex checks for AWS keys, GitHub tokens, Slack tokens, Brevo keys (`xkeysib-` + 32+ chars), PEM private keys, and embedded database/Redis URL credentials. Never commit real credentials; use obvious short placeholders in docs/tests.
- **`CHANGELOG.md` symbol check fails**: `./scripts/verify_changelog_symbols.sh` verifies every backticked symbol under `### Added` against non-test `.go` files. Put CI/doc/dependency changes under `### Changed` or `### Fixed`, never under `### Added`.
- **Coverage mismatch in `README.md`**: `./scripts/check_coverage.sh` enforces `100.0%` statement coverage across all 7 core packages (`audit`, `export`, `notifications`, `outbox`, `pdf`, `scheduler`, `storage`) plus `examples/invoice_service` and compares every package row and the global total in `README.md` against `go test -coverprofile=coverage.out ./...`. If statement coverage changes by even `0.1%`, update the table in `README.md` to match the measured numbers.
