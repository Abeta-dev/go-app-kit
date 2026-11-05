---
name: go-app-kit
description: "Compose and maintain enterprise Go application workflows using github.com/umesh0492/go-app-kit (outbox, pdf, notifications, audit, export). Use when building transactional outboxes, GST PDF invoices, multi-channel notifications, compliance audit logs, streaming CSV exports, or running go-app-kit truth gates."
---

# `go-app-kit` — Enterprise Application Accelerator Skill

Use this umbrella skill when integrating `github.com/umesh0492/go-app-kit` into a Go microservice or maintaining the `go-app-kit` repository. For deep package-level APIs and DDL schemas, load the dedicated subskill.

## 1. Subskill Routing Table

| Task / Domain | Subskill to Load | Path |
| :--- | :--- | :--- |
| PostgreSQL transactional outbox, `SKIP LOCKED` polling, lease fencing, retry/dead-letter | `go-app-kit-outbox` | `skills/go-app-kit-outbox/SKILL.md` |
| HTML-to-PDF compilation, `wkhtmltopdf` concurrency semaphore, GST invoice & receipt templates | `go-app-kit-pdf` | `skills/go-app-kit-pdf/SKILL.md` |
| Multi-channel broker (`SendAsync`), SMTP, WhatsApp, DLT SMS, Slack, HMAC-SHA256 webhooks | `go-app-kit-notifications` | `skills/go-app-kit-notifications/SKILL.md` |
| Range-partitioned PostgreSQL audit logs, append-only triggers, JSON `ComputeDiff` | `go-app-kit-audit` | `skills/go-app-kit-audit/SKILL.md` |
| Low-memory streaming CSV export (`CSVStreamer[T]`, `StreamHTTP[T]`), Excel UTF-8 BOM, CSV escaping | `go-app-kit-export` | `skills/go-app-kit-export/SKILL.md` |
| Indian GSTIN, PAN, Aadhaar Verhoeff D5, IFSC, UPI, exact paise `Money` math | `go-fintech-india` | Companion module `github.com/umesh0492/go-fintech-india` |

---

## 2. End-to-End Composition Pattern (Reference: `examples/invoice_service`)

When combining `go-app-kit` packages in a business transaction (e.g., issuing an invoice):

1. **Validate Statutory Inputs & Compute Exact Paise Math** via `github.com/umesh0492/go-fintech-india`:
   - `fintech.ValidateGSTIN(buyerGSTIN)`, `fintech.ValidatePAN(pan)`
   - Compute CGST/SGST/IGST in integer paise (`fintech.Money`).
2. **Persist Domain State + Outbox Event in One Postgres Transaction** (`go-app-kit/outbox`):
   - `evt, err := outbox.NewEvent("Invoice", inv.ID, "InvoiceIssued", payload)`
   - `err = outboxStore.Insert(ctx, tx, *evt)`
3. **Record Compliance Audit Trail** (`go-app-kit/audit`):
   - `auditEvt := audit.NewEvent(ctx, "INVOICE_CREATE", "Invoice", inv.ID, nil, inv)`
   - `recorder.RecordAsync(auditEvt)`
4. **Render GST Tax Invoice PDF** (`go-app-kit/pdf`):
   - `pdfBytes, err := pdf.GenerateFromTemplate(pdf.GSTInvoiceTemplate, invoiceData, pdf.WithPageSize("A4"))`
5. **Dispatch Multi-Channel Notification** (`go-app-kit/notifications`):
   - `err = broker.SendAsync(ctx, notifications.Message{...})`
6. **Stream Bulk CSV Reports** (`go-app-kit/export`):
   - `err = export.StreamHTTP(w, "invoices.csv", columns, invoices)`

---

## 3. Repository Maintenance & Truth-Gate Loop

Before finishing any code or documentation change inside `go-app-kit`, run this exact verification loop:

```bash
# 1. Run static Bash 3.2 compatibility guard (no mapfile/readarray allowed)
./scripts/test_bash_compat.sh

# 2. Run version, symbol inventory, package count, and coverage truth gate
./scripts/check_version.sh

# 3. Run race detector across all packages
go test -race ./...

# 4. Before tagging a release vX.Y.Z
make check-release-readiness TAG=vX.Y.Z
```

### Common Failure Modes & Fixes
- **`CHANGELOG.md` symbol check fails**: `./scripts/verify_changelog_symbols.sh` verifies every backticked symbol under `### Added` against non-test `.go` files. Put CI/doc/dependency changes under `### Changed` or `### Fixed`, never under `### Added`.
- **Coverage mismatch in `README.md`**: `./scripts/check_coverage.sh` compares every package row and the global total in `README.md` against `go test -coverprofile=coverage.out ./...`. If statement coverage changes by even `0.1%`, update the table in `README.md` to match the measured numbers.
