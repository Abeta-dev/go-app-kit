# go-app-kit

> Current main-line published release: `v0.4.0`. The lightweight `v0.3.0` tag points to an earlier commit and remains an immutable, separate artifact. See [Release Baseline](docs/RELEASE_BASELINE.md) before selecting a release or publishing a reconciliation.

[![CI](https://github.com/Abeta-dev/go-app-kit/actions/workflows/ci.yml/badge.svg)](https://github.com/Abeta-dev/go-app-kit/actions/workflows/ci.yml)
[![Code Quality: golangci-lint](https://img.shields.io/badge/code%20quality-golangci--lint-brightgreen?logo=go)](https://golangci-lint.run/)
[![Go Version](https://img.shields.io/badge/Go-1.26.0-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Vulnerabilities](https://img.shields.io/badge/govulncheck-0%20vulns-brightgreen)](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck)

**Production-grade enterprise application and domain accelerator kit for Go.**

While [`go-libs`](https://github.com/Abeta-dev/go-libs) provides low-level, zero-dependency microservice systems engineering (resilience, concurrency pools, rate limiting, SRE golden signals), **`go-app-kit`** delivers 5 core enterprise application infrastructure packages: **transactional outbox with PostgreSQL DDL, PDF document generation with GST invoice templates, multi-channel notifications, partitioned compliance audit logging, and streaming data exports** (composing seamlessly with statutory companion [`go-fintech-india`](https://github.com/Abeta-dev/go-fintech-india)).

---

## Architecture & Ecosystem Role

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                           Enterprise Microservices                             │
│                     (Invoicing, Orders, Fintech, B2B SaaS)                     │
└─────────────────────────┬────────────────────────────┬─────────────────────────┘
                          │ imports                    │ imports
                          ▼                            ▼
┌─────────────────────────────────────────┐  ┌───────────────────────────────────┐
│     github.com/umesh0492/go-app-kit     │  │ github.com/umesh0492/             │
│                                         │  │   go-fintech-india (Companion)    │
│   ├── outbox/        PostgreSQL Outbox  │  │                                   │
│   ├── pdf/           HTML-to-PDF / GST  │  │   ├── GSTIN (mod-36), PAN, IFSC   │
│   ├── notifications/ Multi-channel      │  │   ├── Aadhaar (Verhoeff D5)       │
│   ├── audit/         Compliance Trails  │  │   ├── Money (exact paise math)    │
│   └── export/        Streaming CSV/BOM  │  │   └── FY & AP/AR Aging            │
└────────────────────┬────────────────────┘  └───────────────────────────────────┘
                     │ builds upon
                     ▼
┌────────────────────────────────────────────────────────────────────────────────┐
│                        github.com/umesh0492/go-libs                            │
│                                                                                │
│   ├── workerpool/      Bounded panic-safe concurrent workers                   │
│   ├── circuitbreaker/  Microsecond circuit breaking (Three-state machine)      │
│   ├── ratelimit/       Token Bucket, Leaky Bucket, Sliding Window Counter      │
│   ├── retry/           Jittered exponential backoff                            │
│   └── logger/          Structured context-aware SRE logging                    │
└────────────────────────────────────────────────────────────────────────────────┘
```

---

## Installation & Workspace Configuration

### Standalone Import
When consuming `go-app-kit` in your microservice:
```bash
go get github.com/umesh0492/go-app-kit@v0.4.0
```

### Multi-Module Local Development (`go.work`)
When working across both `go-app-kit` and `go-libs` simultaneously in a monorepo or local directory, use Go workspaces (`go.work`) to cleanly resolve dependencies without hardcoding machine-specific relative paths:

```bash
# Initialize a Go workspace at the root directory containing both repositories
go work init ./go-app-kit ./go-libs
```

With `go.work` in place, any changes in `go-libs` are immediately reflected in `go-app-kit` during compilation, testing, and debugging.

### Standard Distribution
`go-app-kit` contains no local `replace` directives. Published builds resolve `github.com/umesh0492/go-libs@v0.3.0` and `github.com/umesh0492/go-fintech-india@v0.2.3` from the Go proxy. The repository's immutable release and isolated-consumer verification are documented in [Release Baseline](docs/RELEASE_BASELINE.md).

### Concurrency Architecture & Dependency on `go-libs/workerpool`
`go-app-kit`'s asynchronous background execution in `notifications` (async multi-channel fan-out) and `audit` (asynchronous audit log ingestion) imports [`go-libs/workerpool`](https://github.com/Abeta-dev/go-libs/tree/main/workerpool) directly. It leverages bounded concurrency, graceful draining, panic resilience, and Prometheus saturation metrics without maintaining any duplicated forks.

---

## Package Modules

### 1. `outbox` - PostgreSQL Transactional Outbox Engine
Guarantees at-least-once message delivery without dual-write race conditions:

> [!IMPORTANT]
> **Store Interface & Reference Implementation**: `NewPGStore(db DBOperator, opts ...StoreOption) Store` is the production-ready reference implementation for PostgreSQL DDL (`ddl/001_outbox_events.sql` and `ddl/002_outbox_concurrency_index.sql`), implementing SKIP LOCKED worker leasing, lease-token fencing, and retry backoff.
>
> This package defines the Store interface; production use requires implementing Store against your schema; see outbox_integration_test.go as the reference for correct SKIP LOCKED + fencing semantics.

- **DDL** (`001_outbox_events.sql` & `002_outbox_concurrency_index.sql`): Production PostgreSQL schema with composite index `idx_outbox_poll ON outbox_events (status, next_retry_at, created_at)` for high-throughput, contention-free polling, `lease_token UUID` fencing, and `idx_outbox_aggregate ON outbox_events (aggregate_type, aggregate_id, created_at DESC)` for entity history lookups.
- **PostgreSQL Exclusivity**: Operates exclusively with PostgreSQL via `github.com/jackc/pgx/v5` parameterized queries (`$1, $2, ...`). *(Note: No MySQL dialect support is implemented or supported at runtime).*
- **Relay Poller** (`NewRelay`): Queries ready events using `SELECT ... FOR UPDATE SKIP LOCKED` and atomic lease renewal (`WithLeaseDuration`) with fencing tokens to allow multiple service replicas to poll concurrently without duplicate dispatches or lease clobbering.
- **Dead-Lettering & Backoff**: Full-jitter exponential backoff and configurable max retries transitioning unresolvable poison pills or exhausted retries to `DEAD_LETTER`.

```go
import "github.com/umesh0492/go-app-kit/outbox"

// Transactionally write domain event inside database transaction
evt, _ := outbox.NewEvent("Invoice", "INV-100", "InvoiceIssued", invoicePayload)
err := outboxStore.Insert(ctx, tx, *evt)

// Autonomous background relay
relay, _ := outbox.NewRelay(outbox.RelayConfig{
    Store:        outboxStore,
    Publisher:    kafkaPublisher,
    PollInterval: 1 * time.Second,
})
go relay.Start(ctx)
```

---

### 2. `pdf` - HTML-to-PDF Document Generator
In-memory compilation via `wkhtmltopdf` (requires host `wkhtmltopdf` binary, NOT Chromium or Google Chrome) with responsive page options and embedded production templates:
- **Options**: Margins (mm), PageSize (`A4`, `Letter`), Orientation (`Portrait`, `Landscape`), DPI, and Title metadata.
- **Embedded Templates**:
  - `pdf.GSTInvoiceTemplate`: Indian GST-compliant B2B Tax Invoice with Supplier/Buyer GSTINs, HSN/SAC codes, CGST/SGST/IGST breakdown, Bank NEFT/RTGS details, and Authorized Signatory block.
  - `pdf.ReceiptTemplate`: Clean, modern payment receipt for SaaS and transaction settlements.

```go
import "github.com/umesh0492/go-app-kit/pdf"

// Compile GST Tax Invoice into in-memory PDF buffer
pdfBuf, err := pdf.GenerateFromTemplate(pdf.GSTInvoiceTemplate, invoiceData,
    pdf.WithPageSize("A4"),
    pdf.WithOrientation("Portrait"),
    pdf.WithMargins(10, 10, 10, 10),
)
```

---

### 3. `notifications` - Multi-Channel Notification Dispatcher
Central broker powered by bounded workerpools (adhering to the [`go-libs/workerpool`](https://github.com/Abeta-dev/go-libs/tree/main/workerpool) concurrency architecture) supporting sync and async delivery:
- **SMTP Email** (`NewEmailSender`): RFC 2822 / MIME multipart messaging (text/plain, text/html, attachments, and authentication).
- **Slack** (`NewSlackSender`): Structured Slack Webhook adapter with priority color bars (Red for Critical, Orange for High, Blue for Normal, Green for Low) and metadata fields.
- **Webhook** (`NewWebhookSender`, `VerifyWebhook`, `WebhookVerifier`): HTTP POST webhook with `X-Signature-SHA256` HMAC tamper-proofing, replay prevention via timestamp binding, and constant-time signature verification.

```go
import "github.com/umesh0492/go-app-kit/notifications"

broker := notifications.NewBroker(notifications.DefaultConfig())
broker.RegisterSender(notifications.NewEmailSender(emailConfig))
broker.RegisterSender(notifications.NewSlackSender(slackConfig))

// Non-blocking async dispatch via workerpool
err := broker.SendAsync(ctx, notifications.Message{
    Title:      "Invoice INV-2026 Ready",
    Body:       "Your invoice is attached.",
    Priority:   notifications.PriorityHigh,
    Recipients: []string{"billing@client.com"},
    Channels:   []notifications.Channel{notifications.ChannelEmail},
    Attachments: []notifications.Attachment{
        {Filename: "INV-2026.pdf", ContentType: "application/pdf", Data: pdfBytes},
    },
})
```

---

### 4. `audit` - Audit Trail & State Diffing
Structured audit logging with relational persistence and change tracking:
- **DDL** (`001_audit_logs.sql`): Partitioned by range on `created_at` with trigger-enforced append-only constraints (`trg_prevent_audit_log_modification`).
- **State Diffing** (`ComputeDiff`): Computes field-level property changes (`Old` vs `New`) between before and after JSON states.
- **Context Extraction**: Pulls `Actor`, IP address, and comments from context without contaminating domain signatures.

```go
import "github.com/umesh0492/go-app-kit/audit"

// Computes field-level differences and records asynchronously
event := audit.NewEvent(ctx, "INVOICE_UPDATE", "Invoice", "INV-100", beforeState, afterState)
recorder.RecordAsync(event)
```

---

### 5. `export` - Streaming Data Exporter
High-throughput, low-memory CSV streaming:
- Stream directly to `io.Writer` or `http.ResponseWriter` without buffering complete datasets in memory.
- Prepend UTF-8 BOM (`\xEF\xBB\xBF`) for seamless Microsoft Excel rendering.
- Configurable delimiters (`,`, `;`, `\t`), CRLF endings, and row batch flushing.

```go
import (
    fintech "github.com/umesh0492/go-fintech-india"
    "github.com/umesh0492/go-app-kit/export"
)

columns := []export.Column[InvoiceRow]{
    {Header: "Invoice ID", Extractor: func(i InvoiceRow) string { return i.ID }},
    {Header: "Amount (INR)", Extractor: func(i InvoiceRow) string { return fintech.FormatINRPaise(i.AmountPaise) }},
    {Header: "GSTIN", Extractor: func(i InvoiceRow) string { return i.BuyerGSTIN }},
}

streamer := export.NewCSVStreamer(responseWriter, columns, export.WithBOM(true))
for rows.Next() {
    streamer.WriteRow(fetchNextRow())
}
streamer.Flush()
```

---

### Companion Architecture: Indian Statutory Compliance (go-fintech-india)

For pure domain statutory checks, zero-allocation paise arithmetic, and Indian banking/tax validation, developers should import [`github.com/umesh0492/go-fintech-india`](https://github.com/Abeta-dev/go-fintech-india) directly as a companion library:

```go
import (
    "fmt"
    fintech "github.com/umesh0492/go-fintech-india"
)

// 1. Exact Paise Integer Arithmetic (Zero floating-point inaccuracies)
taxable := fintech.NewMoneyFromFloat(15000.50) // 1500050 paise
cgst := fintech.NewMoney(taxable.Paise() * 9 / 100) // 9% CGST
sgst := fintech.NewMoney(taxable.Paise() * 9 / 100) // 9% SGST
total := taxable.Add(cgst).Add(sgst)
fmt.Println(total.String()) // "17,700.59"

// 2. Statutory GSTIN Mod-36 Checksum Validation
if err := fintech.ValidateGSTIN("27AAPFU0939F1ZV"); err != nil {
    fmt.Printf("Invalid GSTIN: %v\n", err)
}

// 3. Aadhaar UIDAI Verhoeff D5 Dihedral Checksum & Masking
if fintech.IsValidAadhaar("234567890128") {
    fmt.Printf("Masked: %s\n", fintech.MaskAadhaar("234567890128"))
}
```

---

## When, Where, and Why to Use

| Problem | Recommended Module | Why Use It |
| :--- | :--- | :--- |
| **Indian Tax & Banking Compliance** | [`go-fintech-india`](https://github.com/Abeta-dev/go-fintech-india) *(Companion)* | Zero dependencies; pure domain statutory primitives for GSTIN mod-36, Aadhaar Verhoeff $D_5$, PAN legal entities, IFSC codes, and integer paise math. |
| **B2B Billing Documents** | `go-app-kit/pdf` | Pre-bundled GST tax invoice & payment receipt templates; in-memory byte rendering with customizable layout. |
| **Cross-Service Dual-Write Safety** | `go-app-kit/outbox` | Atomically commits domain state and events in the same Postgres TX; `SKIP LOCKED` scales poller across $N$ instances. |
| **Multi-Channel User Alerts** | `go-app-kit/notifications` | Unified broker routing to SMTP, Slack, and HMAC-signed webhooks; non-blocking delivery via `workerpool`. |
| **Audit Logging & State Diffing** | `go-app-kit/audit` | Range-partitioned PostgreSQL table; automated JSON state diffing; asynchronous ingestion with append-only triggers. |
| **Large Report & Data Exports** | `go-app-kit/export` | Low-memory streaming CSV writer with Excel UTF-8 BOM support. |

---

## Reference Implementation

A fully working microservice example combining **GST validation, PDF generation, audit recording, transactional outbox, and notifications** is located in [`examples/invoice_service`](examples/invoice_service):

```bash
cd examples/invoice_service
go test -v ./...
```

---

## 📊 Verified Statement Coverage Status

Coverage across packages in `go-app-kit` is measured using Go's statement-level coverage tool (`go test -coverprofile=coverage.out ./...`):

> **Overall Repository Statement Coverage: 94.1%** (Zero data races across `-race`)

| Package | Purpose | Statement Coverage |
|---|---|---|
| `export` | Low-memory streaming CSV exporter with Excel UTF-8 BOM & formula injection protection | **97.5%** |
| `notifications` | Multi-channel notification broker (SMTP Email, Slack, Webhook HMAC-SHA256 & versioning) | **95.6%** |
| `outbox` | PostgreSQL transactional outbox engine with row-level locked poller & lease fencing | **93.0%** |
| `audit` | Partitioned PostgreSQL audit logging with automated JSON diffing & append-only triggers | **93.0%** |
| `pdf` | In-memory HTML-to-PDF compilation & embedded GST invoice templates | **90.8%** |
| `examples/invoice_service` | Reference microservice with end-to-end integration test & exact paise math | **95.1%** |
| **Total Statement Coverage** | **Cumulative across all packages** | **94.1%** |

---

## Verification & Quality Gates

Run the full verification suite locally:

```bash
# Run all tests with race detector
make test-race

# Verify release/module provenance and a proxy-only isolated consumer
make verify-release-baseline

# Run linter
make lint

# Run Go vulnerability scanner
make vulncheck
```

---

## Known Limitations

- **PDF Generation**: Requires a pre-installed `wkhtmltopdf` binary on the host system (NOT Chromium or Google Chrome). In test environments or CI runners without `wkhtmltopdf`, the pluggable `pdf.Generator` interface can be mocked.
- **Outbox Storage**: Requires PostgreSQL 12+ for `FOR UPDATE SKIP LOCKED` concurrency support.
- **Email Sending**: Uses standard Go `net/smtp` without connection pooling.

---

## License

MIT License. See [LICENSE](LICENSE) for details.
