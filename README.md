# go-app-kit

> Current main-line published release: `v0.8.0`. The lightweight `v0.3.0` tag points to an earlier commit and remains an immutable, separate artifact. See [Release Baseline](docs/RELEASE_BASELINE.md) before selecting a release or publishing a reconciliation.

[![CI](https://github.com/abeta-dev/go-app-kit/actions/workflows/ci.yml/badge.svg)](https://github.com/abeta-dev/go-app-kit/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/abeta-dev/go-app-kit.svg)](https://pkg.go.dev/github.com/abeta-dev/go-app-kit)
[![Coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/abeta-dev/go-app-kit/main/.github/badges/coverage.json)](https://github.com/abeta-dev/go-app-kit)
[![Docs Portal](https://img.shields.io/badge/docs-abeta--dev.github.io%2Fgo--app--kit-00ADD8?logo=read-the-docs&logoColor=white)](https://abeta-dev.github.io/go-app-kit/)
[![llms.txt](https://img.shields.io/badge/llms.txt-AI--Ready-7C3AED)](https://abeta-dev.github.io/go-app-kit/llms.txt)
[![Code Quality: golangci-lint](https://img.shields.io/badge/code%20quality-golangci--lint-brightgreen?logo=go)](https://golangci-lint.run/)
[![Go Version](https://img.shields.io/badge/Go-1.26.0-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/abeta-dev/go-app-kit/badge)](https://scorecard.dev/viewer/?uri=github.com/abeta-dev/go-app-kit)
[![Vulnerabilities](https://img.shields.io/badge/govulncheck-0%20vulns-brightgreen)](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck)

**Production-grade enterprise application and domain accelerator kit for Go.**

While [`go-libs`](https://github.com/abeta-dev/go-libs) provides low-level, zero-dependency microservice systems engineering (resilience, concurrency pools, rate limiting, SRE golden signals), **`go-app-kit`** delivers 7 core enterprise application infrastructure packages: **transactional outbox with PostgreSQL DDL, PDF document generation with GST invoice templates, multi-channel notifications (SMTP, WhatsApp, SMS, Slack, Webhooks), partitioned compliance audit logging, streaming data exports, zero-leak object storage with HMAC-SHA256 presigned URLs, and distributed lease-fenced periodic job scheduling** (composing seamlessly with statutory companion [`go-fintech-india`](https://github.com/abeta-dev/go-fintech-india)).

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
│      github.com/abeta-dev/go-app-kit    │  │ github.com/abeta-dev/             │
│                                         │  │   go-fintech-india (Companion)    │
│   ├── outbox/        PostgreSQL Outbox  │  │                                   │
│   ├── pdf/           HTML-to-PDF / GST  │  │   ├── GSTIN (mod-36), PAN, IFSC   │
│   ├── notifications/ Multi-channel      │  │   ├── Aadhaar (Verhoeff D5)       │
│   ├── audit/         Compliance Trails  │  │   ├── Money (exact paise math)    │
│   ├── export/        Streaming CSV/BOM  │  │   └── FY & AP/AR Aging            │
│   ├── storage/       Object & Presign   │  │                                   │
│   └── scheduler/     Lease-Fenced Jobs  │  │                                   │
└────────────────────┬────────────────────┘  └───────────────────────────────────┘
                     │ builds upon
                     ▼
┌────────────────────────────────────────────────────────────────────────────────┐
│                         github.com/abeta-dev/go-libs                           │
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
go get github.com/abeta-dev/go-app-kit@v0.8.0
```

### Multi-Module Local Development (`go.work`)
When working across both `go-app-kit` and `go-libs` simultaneously in a monorepo or local directory, use Go workspaces (`go.work`) to cleanly resolve dependencies without hardcoding machine-specific relative paths:

```bash
# Initialize a Go workspace at the root directory containing both repositories
go work init ./go-app-kit ./go-libs
```

With `go.work` in place, any changes in `go-libs` are immediately reflected in `go-app-kit` during compilation, testing, and debugging.

### Standard Distribution & Versioning Contract
`go-app-kit` contains no local `replace` directives. Published builds resolve `github.com/abeta-dev/go-libs` and `github.com/abeta-dev/go-fintech-india` from the Go proxy. The repository's immutable release and isolated-consumer verification are documented in [Release Baseline](docs/RELEASE_BASELINE.md), and the 7-Package Leaf-Import Isolation Matrix and strict **`N + 2` Minor-Release Deprecation Window Policy** are documented in [Versioning Policy](docs/VERSIONING.md).

### Concurrency Architecture & Dependency on `go-libs/workerpool`
`go-app-kit`'s asynchronous background execution in `notifications` (async multi-channel fan-out) and `audit` (asynchronous audit log ingestion) imports [`go-libs/workerpool`](https://github.com/abeta-dev/go-libs/tree/main/workerpool) directly. It leverages bounded concurrency, graceful draining, panic resilience, and Prometheus saturation metrics without maintaining any duplicated forks.

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
import "github.com/abeta-dev/go-app-kit/outbox"

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
Pluggable HTML-to-PDF compilation supporting **modern headless Chromium (`NewChromiumRenderer`)**, **zero-dependency pure-Go PDF 1.4 (`NewPureGoRenderer`)**, and legacy `wkhtmltopdf` (`NewWkhtmlRenderer`, deprecated since `v0.8.0`, remove in `>= v0.10.0`) with responsive page options and embedded production templates:
- **Modern Backends**:
  - `NewChromiumRenderer`: Drives headless Chrome/Chromium (`--headless=new --print-to-pdf=...`) with ephemeral loopback HTTP serving when `EnableLocalFileAccess` is `false` (blocking `file://` SSRF), CSS `@page` margin/orientation injection, and bounded concurrency.
  - `NewPureGoRenderer`: Zero external binaries, zero CGO, pure-Go PDF 1.4 compiler for headings, paragraphs, tables, lists, and multi-page pagination in scratch containers or desktop `.dmg` binaries.
- **Options**: Margins (mm), PageSize (`A4`, `Letter`, `Legal`), Orientation (`Portrait`, `Landscape`), DPI, and Title metadata.
- **Embedded Templates**:
  - `pdf.GSTInvoiceTemplate`: Indian GST-compliant B2B Tax Invoice with Supplier/Buyer GSTINs, HSN/SAC codes, CGST/SGST/IGST breakdown, Bank NEFT/RTGS details, and Authorized Signatory block.
  - `pdf.ReceiptTemplate`: Clean, modern payment receipt for SaaS and transaction settlements.

```go
import "github.com/abeta-dev/go-app-kit/pdf"

// 1. Headless Chromium backend (full CSS/Flexbox, loopback SSRF isolation)
chromium := pdf.NewChromiumRenderer(pdf.WithChromiumMaxConcurrency(4))
pdfBuf, err := pdf.GenerateFromTemplate(pdf.GSTInvoiceTemplate, invoiceData,
    pdf.WithRenderer(chromium),
    pdf.WithPageSize("A4"),
    pdf.WithOrientation("Portrait"),
    pdf.WithMargins(10, 10, 10, 10),
)

// 2. Zero-dependency pure-Go PDF 1.4 backend (no host browser binary required)
pureGoBuf, err := pdf.GenerateFromTemplate(pdf.ReceiptTemplate, receiptData,
    pdf.WithRenderer(pdf.NewPureGoRenderer()),
    pdf.WithTitle("Payment Receipt"),
)
```

---

### 3. `notifications` - Multi-Channel Notification Dispatcher
Central broker powered by bounded workerpools (adhering to the [`go-libs/workerpool`](https://github.com/abeta-dev/go-libs/tree/main/workerpool) concurrency architecture) supporting sync and async delivery:
- **SMTP Email** (`NewEmailSender`) & **Brevo Transactional Email** (`NewBrevoSender`): RFC 2822 / MIME multipart messaging (text/plain, text/html, attachments, and authentication) and Brevo v3 REST API.
- **WhatsApp** (`NewWhatsAppSender`): Meta WhatsApp Cloud API v20+ with Bearer authorization, structured payload dispatch, and simulated testing mode.
- **SMS** (`NewSMSSender`): Indian DLT-compliant transactional SMS gateway with Principal Entity ID (`pe_id`), DLT Template ID (`template_id` via `SMSConfig.TemplateID` or `Message.Metadata["template_id"]`), and approved sender headers.
- **Slack** (`NewSlackSender`): Structured Slack Webhook adapter with priority color bars (Red for Critical, Orange for High, Blue for Normal, Green for Low) and metadata fields.
- **Webhook** (`NewWebhookSender`, `VerifyWebhook`, `WebhookVerifier`): HTTP POST webhook with `X-Signature-SHA256` HMAC tamper-proofing, replay prevention via timestamp binding, and constant-time signature verification.

```go
import "github.com/abeta-dev/go-app-kit/notifications"

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
Structured audit logging with relational persistence, cryptographic hash-chaining, and change tracking:
- **DDL** (`001_audit_logs.sql`): Partitioned by range on `created_at` with trigger-enforced append-only constraints (`trg_prevent_audit_log_modification`).
- **Tamper-Evident Hash Chaining** (`ComputeHash`, `Event.PrevHash`, `Event.Hash`): Computes a deterministic SHA-256 hash chain across sequential audit events recorded by `NewPGRecorder`, storing `prev_hash` and `hash` in `metadata` for tamper detection.
- **State Diffing** (`ComputeDiff`): Computes field-level property changes (`Old` vs `New`) between before and after JSON states.
- **Context Extraction**: Pulls `Actor`, IP address, and comments from context without contaminating domain signatures.

```go
import "github.com/abeta-dev/go-app-kit/audit"

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
- **`StreamHTTP[T any]`**: Helper function to stream CSV directly to HTTP response writers (including Gin `c.Writer`), writing headers (`Content-Type`, sanitized `Content-Disposition`, `X-Content-Type-Options: nosniff`, `Cache-Control`) automatically with BOM and CRLF.
- **`StreamHTTPCompressed[T any]` & `NegotiateEncoding`**: RFC 9110 `Accept-Encoding` content negotiation (`br` and `gzip` with `q=` weights, preferring Brotli on ties) using `sync.Pool`-recycled Brotli and Gzip streaming writers and automatic `Vary: Accept-Encoding` headers.

```go
import (
    fintech "github.com/abeta-dev/go-fintech-india"
    "github.com/abeta-dev/go-app-kit/export"
)

columns := []export.Column[InvoiceRow]{
    {Header: "Invoice ID", Extractor: func(i InvoiceRow) string { return i.ID }},
    {Header: "Amount (INR)", Extractor: func(i InvoiceRow) string { return fintech.FormatINRPaise(i.AmountPaise) }},
    {Header: "GSTIN", Extractor: func(i InvoiceRow) string { return i.BuyerGSTIN }},
}

// 1. HTTP streaming directly to client / browser (uncompressed or Accept-Encoding br/gzip compressed)
err := export.StreamHTTP(w, "invoices.csv", columns, invoiceList)
err = export.StreamHTTPCompressed(w, r, "invoices.csv", columns, invoiceList)

// 2. Low-level chunked CSV streamer
streamer := export.NewCSVStreamer(responseWriter, columns, export.WithBOM(true))
for rows.Next() {
    streamer.WriteRow(fetchNextRow())
}
streamer.Flush()
```

---

### 6. `storage` - Zero-Leak Object Storage & Presigned URL Engine
Content-addressed artifact archiving (both in-memory via `NewMemoryStore` and durable local atomic filesystem via `NewLocalStore`) and time-bounded HMAC-SHA256 presigned URLs for generated `pdf` invoices and `export` CSV reports:
- **Local Atomic Filesystem Store (`NewLocalStore`)**: Streams payloads into sibling temporary files with strict owner-only permissions (`0600` files, `0700` directories by default), calls `fsync`, and atomically renames both the object payload and JSON metadata sidecar into place with zero partial writes on failure.
- **Path-Traversal Guard (`ValidateKey`)**: Rejects empty keys, leading/trailing slashes, empty segments (`//`), `.` or `..` traversal segments, Windows backslashes, and null bytes (`ErrInvalidKey`).
- **Bounded Stream Reading (`WithMaxObjectBytes` / `WithLocalMaxObjectBytes`)**: Enforces upload size limits via `io.LimitReader` (`ErrObjectTooLarge`), computes SHA-256 content digests (`ObjectMeta.SHA256Hex`), and defensively clones metadata maps.
- **Deterministic Presigned URLs (`PresignGet`, `PresignPut`, `VerifyPresignedURL`)**: Generates and verifies constant-time HMAC-SHA256 signed download and upload URLs bound to HTTP method, object key, content type, and expiration timestamp.

```go
import "github.com/abeta-dev/go-app-kit/storage"

// Local atomic filesystem store (0600 files, temp write + fsync + atomic rename)
store, _ := storage.NewLocalStore("/var/lib/abeta/artifacts",
    storage.WithLocalBaseURL("https://objects.enterprise.internal"),
    storage.WithLocalSigningSecret([]byte("32-byte-hmac-signing-secret-key")),
    storage.WithLocalMaxObjectBytes(16*1024*1024),
)

meta, _ := store.Put(ctx, storage.PutInput{
    Key:         "invoices/2026/INV-2026-001.pdf",
    ContentType: "application/pdf",
    Body:        bytes.NewReader(pdfBytes),
    Metadata:    map[string]string{"gstin": "27AAPFU0939F1ZV"},
})
presigned, _ := store.PresignGet(ctx, meta.Key, 15*time.Minute)
```

---

### 7. `scheduler` - Distributed & Local Lease-Fenced Periodic Job Scheduler
Singleton periodic background job runner with distributed and local OS `flock`/PID-file lease fencing, panic recovery, and graceful in-flight draining:
- **Local OS `flock` + PID-File Mutual Exclusion (`NewFileLeaseStore`)**: Combines non-blocking OS advisory file locks (`flock`) with `0600` JSON PID/TTL lease metadata files for crash-safe cross-process mutual exclusion on a single machine (e.g., macOS LaunchAgent + desktop GUI app or CLI daemon).
- **Distributed Mutual Exclusion (`LeaseStore`, `NewMemoryLeaseStore`)**: Acquires or renews a time-bounded job lease (`TryAcquire`) before each tick so horizontally scaled replicas never double-execute recurring billing or GST aging snapshots.
- **Panic Recovery Barrier (`RunOnce`)**: Converts job panics into structured errors forwarded to `WithErrorHandler` without crashing sibling background workers.
- **Graceful Shutdown (`Start`)**: Stops all tickers on `ctx.Done()` and waits for every in-flight job execution to complete before returning.

```go
import "github.com/abeta-dev/go-app-kit/scheduler"

// Local OS flock + PID file lease store for cross-process single-machine exclusion
leaseStore, _ := scheduler.NewFileLeaseStore("/var/run/abeta/locks")
defer leaseStore.Close()
sched, _ := scheduler.NewScheduler(leaseStore, "desktop-launchagent")

_ = sched.Register(scheduler.JobConfig{
    Name:       "gst-aging-snapshot",
    Interval:   1 * time.Hour,
    LeaseTTL:   55 * time.Minute,
    RunOnStart: true,
    Fn: func(ctx context.Context) error {
        return computeAgingSnapshot(ctx)
    },
})
go sched.Start(ctx)
```

---

### Companion Architecture: Indian Statutory Compliance (go-fintech-india)

For pure domain statutory checks, zero-allocation paise arithmetic, and Indian banking/tax validation, developers should import [`github.com/abeta-dev/go-fintech-india`](https://github.com/abeta-dev/go-fintech-india) directly as a companion library:

```go
import (
    "fmt"
    fintech "github.com/abeta-dev/go-fintech-india"
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
| **Indian Tax & Banking Compliance** | [`go-fintech-india`](https://github.com/abeta-dev/go-fintech-india) *(Companion)* | Zero dependencies; pure domain statutory primitives for GSTIN mod-36, Aadhaar Verhoeff $D_5$, PAN legal entities, IFSC codes, and integer paise math. |
| **B2B Billing Documents** | `go-app-kit/pdf` | Pre-bundled GST tax invoice & payment receipt templates; in-memory byte rendering with customizable layout. |
| **Cross-Service Dual-Write Safety** | `go-app-kit/outbox` | Atomically commits domain state and events in the same Postgres TX; `SKIP LOCKED` scales poller across $N$ instances. |
| **Multi-Channel User Alerts** | `go-app-kit/notifications` | Unified broker routing to SMTP, Slack, and HMAC-signed webhooks; non-blocking delivery via `workerpool`. |
| **Audit Logging & State Diffing** | `go-app-kit/audit` | Range-partitioned PostgreSQL table; automated JSON state diffing; asynchronous ingestion with append-only triggers. |
| **Large Report & Data Exports** | `go-app-kit/export` | Low-memory streaming CSV writer with Excel UTF-8 BOM support and Brotli/Gzip HTTP compression. |
| **Invoice & Report Archiving** | `go-app-kit/storage` | Zero-leak object storage with SHA-256 content digests, key traversal guards, and HMAC-SHA256 presigned URLs. |
| **Singleton Periodic Cron Jobs** | `go-app-kit/scheduler` | Distributed lease-fenced job runner with panic recovery and graceful in-flight draining. |

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

> **Overall Repository Statement Coverage: 100.0%** (Zero data races across `-race`)

| Package | Purpose | Statement Coverage |
|---|---|---|
| `export` | Low-memory streaming CSV exporter with Excel UTF-8 BOM & formula injection protection | **100.0%** |
| `notifications` | Multi-channel notification broker (SMTP Email, Brevo, WhatsApp, SMS, Slack, Webhook HMAC-SHA256) | **100.0%** |
| `outbox` | PostgreSQL transactional outbox engine with row-level locked poller & lease fencing | **100.0%** |
| `audit` | Partitioned PostgreSQL audit logging with automated JSON diffing, hash-chaining & append-only triggers | **100.0%** |
| `pdf` | In-memory HTML-to-PDF compilation & embedded GST invoice templates | **100.0%** |
| `storage` | Zero-leak object storage with SHA-256 content digests, size limits & HMAC-SHA256 presigned URLs | **100.0%** |
| `scheduler` | Distributed lease-fenced periodic job scheduler with panic recovery & graceful draining | **100.0%** |
| `examples/invoice_service` | Reference microservice with end-to-end integration test & exact paise math | **100.0%** |
| **Total Statement Coverage** | **Cumulative across all packages** | **100.0%** |

---

## 🤖 AI Agent Skills & Multi-Model Context Routing

`go-app-kit` ships with vendor-neutral **Agent Skills (`SKILL.md`)** and a root **[`AGENTS.md`](AGENTS.md)** router compatible with Claude Code, Google Antigravity, Gemini CLI, OpenAI Codex, Cursor, and Windsurf:

| Skill | Path | Coverage |
| :--- | :--- | :--- |
| **`go-app-kit` (Umbrella)** | [`skills/go-app-kit/SKILL.md`](skills/go-app-kit/SKILL.md) | Package selection, cross-package transactional wiring, and truth gates |
| **`go-app-kit-outbox`** | [`skills/go-app-kit-outbox/SKILL.md`](skills/go-app-kit-outbox/SKILL.md) | PostgreSQL `SELECT FOR UPDATE SKIP LOCKED`, lease fencing, relay worker, dead-letter handling |
| **`go-app-kit-pdf`** | [`skills/go-app-kit-pdf/SKILL.md`](skills/go-app-kit-pdf/SKILL.md) | Pluggable `Renderer` (`NewChromiumRenderer`, `NewPureGoRenderer`, legacy `NewWkhtmlRenderer`), SSRF defense, GST invoice templates |
| **`go-app-kit-notifications`** | [`skills/go-app-kit-notifications/SKILL.md`](skills/go-app-kit-notifications/SKILL.md) | `Broker` routing across `Email`, `Slack`, `Webhook`, `WhatsApp` (`NewWhatsAppSender`), and Indian DLT `SMS` (`NewSMSSender`) |
| **`go-app-kit-audit`** | [`skills/go-app-kit-audit/SKILL.md`](skills/go-app-kit-audit/SKILL.md) | Partitioned PostgreSQL append-only audit trail (`Recorder`, `Event`) and JSON diffing |
| **`go-app-kit-export`** | [`skills/go-app-kit-export/SKILL.md`](skills/go-app-kit-export/SKILL.md) | Constant-memory CSV streaming (`StreamHTTP`, `CSVStreamer`), UTF-8 BOM, and formula-injection escaping |
| **`go-app-kit-storage`** | [`skills/go-app-kit-storage/SKILL.md`](skills/go-app-kit-storage/SKILL.md) | Zero-leak object storage (`Store`, `NewLocalStore` atomic 0600 filesystem, `NewMemoryStore`, `ValidateKey`), SHA-256 digests, and HMAC-SHA256 presigned URLs |
| **`go-app-kit-scheduler`** | [`skills/go-app-kit-scheduler/SKILL.md`](skills/go-app-kit-scheduler/SKILL.md) | Lease-fenced periodic job scheduler (`Scheduler`, `NewScheduler`, `LeaseStore`, `NewFileLeaseStore` OS flock/PID, `NewMemoryLeaseStore`) |

Machine-readable LLM context index: [`llms.txt`](llms.txt) (published at [`https://abeta-dev.github.io/go-app-kit/llms.txt`](https://abeta-dev.github.io/go-app-kit/llms.txt)).

---

## 🌐 Abeta Go Ecosystem

| Repository | Architectural Role | Documentation Portal | Go Reference (`pkg.go.dev`) |
| :--- | :--- | :--- | :--- |
| **[`github.com/abeta-dev/go-libs`](https://github.com/abeta-dev/go-libs)** (`v0.5.0`) | Low-level microservice systems engineering (36 foundational packages: `workerpool`, `circuitbreaker`, `ratelimit`, `retry`, `cache`, `compress`, `ginmw`, `httpclient`, `db`, `rbac`, `jwks`, `telemetry`, `logger`) | [`abeta-dev.github.io/go-libs`](https://abeta-dev.github.io/go-libs/) | [`pkg.go.dev/github.com/abeta-dev/go-libs`](https://pkg.go.dev/github.com/abeta-dev/go-libs) |
| **[`github.com/abeta-dev/go-fintech-india`](https://github.com/abeta-dev/go-fintech-india)** (`v0.4.0`) | Zero-dependency Indian FinTech & statutory compliance engine (127 exported symbols across 15 domain files: GSTIN mod-36, PAN, Aadhaar Verhoeff $D_5$, IFSC, VPA/UPI, `Money` paise math, FY & AP/AR aging) | [`abeta-dev.github.io/go-fintech-india`](https://abeta-dev.github.io/go-fintech-india/) | [`pkg.go.dev/github.com/abeta-dev/go-fintech-india`](https://pkg.go.dev/github.com/abeta-dev/go-fintech-india) |
| **[`github.com/abeta-dev/go-app-kit`](https://github.com/abeta-dev/go-app-kit)** (`v0.8.0`) | Enterprise application infrastructure (7 core packages: PostgreSQL transactional `outbox`, Chromium/pure-Go `pdf`, multi-channel `notifications`, compliance `audit`, streaming CSV `export`, local atomic/presigned `storage`, OS flock/distributed `scheduler`) | [`abeta-dev.github.io/go-app-kit`](https://abeta-dev.github.io/go-app-kit/) | [`pkg.go.dev/github.com/abeta-dev/go-app-kit`](https://pkg.go.dev/github.com/abeta-dev/go-app-kit) |
| **[`github.com/abeta-dev/cloud-native-observability`](https://github.com/abeta-dev/cloud-native-observability)** (`v0.4.0`) | Production OpenTelemetry, Prometheus, Grafana, Loki, Tempo, and Kubernetes observability reference stack for Go microservices | [`abeta-dev.github.io/cloud-native-observability`](https://abeta-dev.github.io/cloud-native-observability/) | [`github.com/abeta-dev/cloud-native-observability`](https://github.com/abeta-dev/cloud-native-observability) |

---

## Verification & Quality Gates

Run the full verification suite locally:

```bash
# Run Bash 3.2 compatibility, TruffleHog secret scan, leaf-import isolation,
# strict N+2 deprecation window gate, 100.0% coverage gate, and version/symbol/doc truth gate
./scripts/test_bash_compat.sh
./scripts/check_secrets.sh
./scripts/check_leaf_imports.sh
./scripts/check_deprecation.sh
./scripts/check_coverage.sh
./scripts/check_version.sh

# Run Module Code-Owner Agent automated PR review
./scripts/module_agent_review.sh --base origin/main --head HEAD

# Run all tests with race detector
make test-race

# Verify release/module provenance and a proxy-only isolated consumer
make verify-release-baseline

# Run linter
make lint

# Run Go vulnerability scanner
make vulncheck
```

- **Leaf-Import & Dependency Isolation Gate ([`scripts/check_leaf_imports.sh`](scripts/check_leaf_imports.sh))**: Guarantees zero cross-package sibling coupling across all 7 core packages (`audit`, `export`, `notifications`, `outbox`, `pdf`, `scheduler`, `storage`) and verifies in isolated temporary `go.mod` sandboxes that importing `export`, `pdf`, `notifications`, `storage`, or `scheduler` never pulls `pgx`, `gin`, or `go-fintech-india` into a consumer's dependency graph or `go.sum`, and importing `audit` or `outbox` never pulls `gin` or `go-fintech-india`.
- **Strict `N + 2` Minor-Release Deprecation Window Gate ([`scripts/check_deprecation.sh`](scripts/check_deprecation.sh) & [`docs/VERSIONING.md`](docs/VERSIONING.md))**: Enforces that every `// Deprecated:` annotation in non-test `.go` files follows `// Deprecated: Since vX.Y.Z (remove in >= vA.B.C). Use <Replacement> instead.` with at least a 2-minor-release deprecation window (`N + 2`), and verifies against the previous Git release tag that no deprecated symbol is removed before `CHANGELOG.md` reaches `>= vA.B.C`.

---

## Known Limitations

- **PDF Generation**: For full CSS/Flexbox rendering, pass `pdf.NewChromiumRenderer()` (requires Chrome/Chromium binary in `$PATH` or `CHROMIUM_PATH`). For zero-binary environments, pass `pdf.NewPureGoRenderer()`. Legacy `NewWkhtmlRenderer` is deprecated since `v0.8.0` (remove in `>= v0.10.0`).
- **Outbox Storage**: Requires PostgreSQL 12+ for `FOR UPDATE SKIP LOCKED` concurrency support.
- **Email Sending**: Uses standard Go `net/smtp` without connection pooling.

---

## License

MIT License. See [LICENSE](LICENSE) for details.
