# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.4.0] - 2026-09-20

### Removed
- india: Discontinued deprecated backward-compatibility façade. Canonical statutory Indian primitives (GSTIN, PAN, Aadhaar Verhoeff D5, IFSC, Money) should be imported directly from `github.com/umesh0492/go-fintech-india`.

### Changed
- examples/invoice_service: Migrated reference billing pipeline to import `github.com/umesh0492/go-fintech-india` directly.
- docs: Refactored documentation portal, architecture diagrams, and README to showcase the 5 core enterprise application infrastructure packages (outbox, pdf, notifications, audit, export) and direct companion composition with `go-fintech-india`.

## [0.3.2] - 2026-09-18

### Changed
- release: Reconciled release baseline following v0.3.1 truth-gate trip; published monotonic v0.3.2.
- dependencies: Bumped `github.com/umesh0492/go-libs` to `v0.3.0`.
- outbox: Added `TypeMap()` implementation to test `mockRows` for pgx/v5 5.11.0 compatibility.
- tooling: Added `scripts/check_tag_readiness.sh`, `scripts/git-hooks/pre-push`, and `make tag-release` to structurally prevent untracked changelog releases.

## [0.3.1] - 2026-09-17

### Changed
- ci: Replace all Bash 4+ `mapfile`/`readarray` built-ins with POSIX/Bash 3.2-compatible `while IFS= read -r` array loops in `scripts/check_version.sh` and `scripts/check_coverage.sh`, fixing macOS CI failures.
- ci: Add `scripts/test_bash_compat.sh` — static compatibility guard integrated into CI, release workflow, and Makefile.
- release: Remove fragile zero-retry isolated consumer step from `release.yml`; replaced with `scripts/verify_release.sh` using bounded exponential backoff (up to 12 attempts, per-attempt isolated GOMODCACHE).
- release: Add `scripts/test_verify_release.sh` — unit-tests proxy retry logic, backoff timing, manifest generation, and pre-release SemVer parsing.
- ci: Add `scripts/verify_release_baseline.sh` — post-merge downstream consumer integrity check.
- docs: Add `docs/RELEASE_BASELINE.md` documenting full release procedure and version gates.

## [0.2.1] - 2026-09-17

Encapsulated PDF subprocess semaphore, india façade deprecation notice, and go-libs v0.2.1 dependency bump.

### Added
- pdf: Encapsulated instance semaphore and constructor `NewWkhtmlRenderer` for bounded wkhtmltopdf subprocess concurrency.

### Deprecated
- india: Package marked deprecated in favor of direct imports of `github.com/umesh0492/go-fintech-india` for canonical statutory Indian primitives.

### Changed
- dependencies: Bumped `github.com/umesh0492/go-libs` to `v0.2.1`.
- pdf: Removed package-global semaphore maps in favor of struct-encapsulated semaphore bounding on `WkhtmlRenderer`.

## [0.2.0] - 2026-09-17

Clean-slate architecture: Decoupled PDF compilation and unified statutory Indian domain engine.

### Added
- pdf: Pluggable `Renderer` interface (`Renderer`, `WithRenderer`, `ErrNoRendererAvailable`) decoupling PDF generation from mandatory host `wkhtmltopdf` binaries.
- india: Statutory validation, financial year arithmetic, and zero-allocation currency engines unified by delegating to `github.com/umesh0492/go-fintech-india@v0.2.2`.
- outbox: PostgreSQL transactional outbox implementation with `SELECT FOR UPDATE SKIP LOCKED` lease fencing.

### Changed
- dependencies: Upgraded `github.com/umesh0492/go-fintech-india` to `v0.2.2` with zero-allocation `AppendINR`.

## [0.1.0] - 2026-09-15

Initial public release. Production-grade enterprise application and domain accelerator kit for Go.

Clean-slate architecture: No backward compatibility preserved. Statutory Indian validation engines unified under github.com/umesh0492/go-fintech-india@v0.1.0, and deprecated interfaces dropped, as no external developers are actively consuming pre-release revisions.

### Added
- audit: Structured PostgreSQL audit trail with append-only partitioning, trigger DDL, and JSON state diffing (`Recorder`, `Event`, `PGRecorder`).
- export: Streaming CSV export with Excel UTF-8 BOM, cell formula injection escaping (`=,+,-,@`), and batch flushing (`StreamWriter`, `ExportConfig`).
- india: Statutory PAN, GSTIN (Mod-36), Aadhaar (Verhoeff D5), and IFSC validation helpers, Indian financial year calculations, and exact integer paise currency arithmetic (`Money`, `ValidatePAN`, `ValidateGSTIN`, `ValidateAadhaar`, `ValidateIFSC`).
- notifications: Multi-channel notification broker (Email, Slack, Webhook) with RFC 2047 MIME encoding, HMAC-SHA256 webhook signatures, and mandatory 5-minute replay defense (`Broker`, `EmailMessage`, `WebhookVerifier`).
- outbox: PostgreSQL transactional outbox implementation using `SELECT FOR UPDATE SKIP LOCKED`, worker relay, exponential jitter backoff, and strict lease fencing (`Store`, `NewPGStore`, `Relay`, `Event`).
- pdf: In-memory wkhtmltopdf document compilation with embedded GST invoice and payment receipt templates (`Compiler`, `InvoiceData`, `WithRenderer`).
- examples/invoice_service: Complete reference implementation demonstrating outbox relay, multi-channel notifications, GST invoice generation, and statutory Indian financial validation.

### Known limitations
- PDF generator requires a pre-installed wkhtmltopdf binary on the host system (NOT Chromium or Google Chrome).
- Outbox storage requires PostgreSQL 12+ for SKIP LOCKED concurrency support.
- Email sending uses standard net/smtp without connection pooling.
