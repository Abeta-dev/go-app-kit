# Security Policy

## Supported Versions

`go-app-kit` provides security updates for the current minor release series:

| Version Series | Security Updates        | Status               |
| -------------- | ----------------------- | -------------------- |
| 0.2.x          | :white_check_mark: Yes  | **Active / Current** |
| 0.1.x          | :white_check_mark: Yes  | Maintenance          |
| < 0.1.0        | :x: No                  | End-of-Life          |

---

## Reporting a Vulnerability

The maintainers of `go-app-kit` take security, data integrity, and compliance seriously. If you discover a potential vulnerability, **please do not open a public GitHub issue**. Publicly disclosing a vulnerability could put downstream enterprise consumers and production workloads at risk.

Instead, report vulnerabilities through one of the following confidential channels:

### 1. GitHub Private Vulnerability Reporting (Preferred)
Submit a confidential advisory directly via GitHub:
- Navigate to the **Security** tab of `github.com/umesh0492/go-app-kit`.
- Click **"Report a vulnerability"** to open a private advisory draft.
- Include a description, affected package(s), minimal reproduction or proof-of-concept (PoC), and potential impact.

### 2. Direct Security Contact
If you cannot use GitHub Security Advisories, email the maintainer directly:
- **Email**: [umesh0492@gmail.com](mailto:umesh0492@gmail.com)
- **Subject**: `[SECURITY] go-app-kit Vulnerability Report: <Package Name>`
- Please include full reproduction steps and environment details.

---

## Response SLA

As a focused engineering team, we commit to the following response timeline:

- **Initial Acknowledgement**: Within **48 hours** of report receipt.
- **Assessment & Triage**: Within **5 business days**, confirming severity and scope.
- **Fix & Patch Release**: Targeted within **14 business days** depending on complexity.
- **Coordinated Disclosure**: Coordinated with the reporter via a GitHub Security Advisory and published alongside a patch release.

---

## Security Model for Sensitive Surfaces

`go-app-kit` provides enterprise accelerator primitives that interact directly with databases, file rendering engines, network dispatchers, and statutory compliance data. Maintainers and contributors must uphold the following security invariants:

### 1. Database DDL & Partitioning (`audit`, `outbox`)
- **Safe Migrations & Partitioning**: The audit log schema (`audit/ddl/001_audit_logs.sql`) uses PostgreSQL declarative range partitioning (`PARTITION BY RANGE (created_at)`). A catch-all default partition (`audit_logs_default`) is mandatory to guarantee fail-safe, unblocked ingestion even before date-specific partitions are provisioned.
- **Append-Only Enforcement**: Database triggers (`prevent_audit_log_modification`) prohibit `UPDATE` and `DELETE` operations on audit tables for application roles (`app_user`, `PUBLIC`).
- **Row-Level Lease Fencing**: The transactional outbox engine (`outbox/store.go`, `outbox/relay.go`) coordinates concurrent workers using `SELECT ... FOR UPDATE SKIP LOCKED`. State transitions (`MarkPublished`, `MarkFailed`) enforce atomic row-level lease token fencing (`lease_token UUID`), rejecting stale or pre-empted workers with `ErrLeaseExpired` to eliminate double-publishing race conditions.
- **Table Identifier Sanitization**: To guard against SQL injection in dynamic table configurations, `WithTableName` strictly validates all table and schema names against the identifier regular expression `^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)?$` prior to query interpolation.

### 2. PDF Document Generation (`pdf`)
- **HTML Sanitization**: Template inputs compiled via `pdf.GenerateFromTemplate` and `pdf.CompileHTML` must be properly escaped to prevent HTML injection.
- **Local File Access Protection (SSRF / Arbitrary File Read)**:
  `EnableLocalFileAccess` is strictly **disabled by default (`false`)** in `pdf.Options` and passed to `wkhtmltopdf`. This prevents Server-Side Request Forgery (SSRF) and arbitrary local file inclusion (such as `<iframe src="file:///etc/passwd">` or exfiltration of local container credentials). Applications must never enable local file access unless all referenced resources are locally bounded, trusted, and verified.

### 3. Notifications & Webhooks (`notifications`)
- **Mandatory HMAC-SHA256 Signatures**: All outgoing webhooks are signed using HMAC-SHA256 (`X-Signature-SHA256`). Receivers must verify signatures using `notifications.VerifyWebhook` or `notifications.NewWebhookVerifier`, which enforces constant-time cryptographic comparison (`subtle.ConstantTimeCompare`) to eliminate timing side-channel attacks.
- **Strict Timestamp Freshness (Anti-Replay Window)**: Webhook payloads must bind the dispatch timestamp (`X-Webhook-Timestamp` or `t=<ts>,v1=<sig>`). The verifier strictly enforces a configurable freshness window (`DefaultWebhookTolerance = 5 minutes`) to reject expired or replayed requests.

### 4. Data Export (`export`)
- **CSV Formula Injection Mitigation (CWE-1236)**: `export.CSVStreamer` enables formula sanitization by default (`WithFormulaSanitization(true)`).
- **Trigger Character Escaping**: `export.SanitizeCSVCell` prefixes dangerous leading characters (`=`, `+`, `-`, `@`, `\t`, `\r`) with a single quote (`'`), preventing spreadsheet applications (Microsoft Excel, LibreOffice Calc, Google Sheets) from executing arbitrary DDE formulas or macro payloads.
- **Whitespace Evasion Immunity**: The sanitizer strips leading whitespace characters, newlines (`\n`, `\r\n`), and non-breaking spaces (NBSP `\u00a0`) before evaluating formula triggers, neutralizing whitespace evasion bypasses while preserving valid positive and negative numbers (e.g. `-123.45`).

### 5. Indian Localized Fintech (`india`)
- **Algorithmic Integrity (Zero External Dependencies)**: Statutory Indian financial and identity algorithms run completely in-memory with zero third-party dependencies:
  - **Aadhaar**: Validated using the UIDAI-mandated Verhoeff Dihedral ($D_5$) permutation algorithm (`ValidateAadhaar`) and masked by default (`MaskAadhaar` -> `XXXX-XXXX-1234`).
  - **GSTIN**: Validated using the official 15-character Goods and Services Tax format and statutory Mod-36 check digit calculation (`ValidateGSTIN`, `CalculateGSTINCheckDigit`) across 38 state/UT alphanumeric codes.
  - **PAN**: Validated using strict alphanumeric pattern matching (`^[A-Z]{5}[0-9]{4}[A-Z]$`) and statutory 4th-character legal entity mapping (Company, Individual, LLP, HUF, Trust, Government Agency).
  - **Integer Paise Arithmetic**: `india.Money` stores monetary values as 64-bit integer paise (`int64`), preventing IEEE 754 floating-point rounding errors across GST split calculations, invoices, and ledgers.
