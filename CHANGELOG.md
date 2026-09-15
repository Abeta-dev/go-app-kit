# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-09-15

Initial public release. Production-grade enterprise application and domain accelerator kit for Go.

### Added
- audit: Structured PostgreSQL audit trail with append-only partitioning, trigger DDL, and JSON state diffing (`Recorder`, `Event`, `PGRecorder`).
- export: Streaming CSV export with Excel UTF-8 BOM, cell formula injection escaping (`=,+,-,@`), and batch flushing (`StreamWriter`, `ExportConfig`).
- india: Statutory PAN, GSTIN (Mod-36), Aadhaar (Verhoeff D5), and IFSC validation helpers, Indian financial year calculations, and exact integer paise currency arithmetic (`Money`, `ValidatePAN`, `ValidateGSTIN`, `ValidateAadhaar`, `ValidateIFSC`).
- notifications: Multi-channel notification broker (Email, Slack, Webhook) with RFC 2047 MIME encoding, HMAC-SHA256 webhook signatures, and mandatory 5-minute replay defense (`Broker`, `EmailMessage`, `WebhookVerifier`).
- outbox: PostgreSQL transactional outbox implementation using `SELECT FOR UPDATE SKIP LOCKED`, worker relay, exponential jitter backoff, and strict lease fencing (`Store`, `NewPGStore`, `Relay`, `Event`).
- pdf: In-memory wkhtmltopdf document compilation with embedded GST invoice and payment receipt templates (`Compiler`, `InvoiceData`, `WithGenerator`).
- examples/invoice_service: Complete reference implementation demonstrating outbox relay, multi-channel notifications, GST invoice generation, and statutory Indian financial validation.

### Known limitations
- PDF generator requires a pre-installed wkhtmltopdf binary on the host system (NOT Chromium or Google Chrome).
- Outbox storage requires PostgreSQL 12+ for SKIP LOCKED concurrency support.
- Email sending uses standard net/smtp without connection pooling.
