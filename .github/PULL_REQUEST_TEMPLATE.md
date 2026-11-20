## Summary
<!-- Provide a concise summary of the enterprise infrastructure or domain changes introduced by this PR. -->

## Related Issue
<!-- Fixes #(issue) or Relates to #(issue) -->

## Type of Change
- [ ] `feat`: New capability across `audit`, `export`, `notifications`, `outbox`, or `pdf`
- [ ] `fix`: Bug fix, concurrency/lease fix, or security hardening
- [ ] `perf`: Throughput or streaming memory optimization
- [ ] `docs` / `skills`: Documentation, `docs/index.html`, `llms.txt`, or `skills/*/SKILL.md` update
- [ ] `ci` / `chore`: Dependency update, workflow hardening, or verification gate update

## Package(s) Affected
- [ ] `audit` (PostgreSQL range-partitioned compliance audit trail & SHA-256 hash chaining)
- [ ] `export` (Streaming RFC 4180 CSV writer, formula-injection sanitization & `StreamHTTP`)
- [ ] `notifications` (Multi-channel dispatcher: SMTP, WhatsApp Cloud API, Indian DLT SMS, Slack, HMAC Webhooks)
- [ ] `outbox` (PostgreSQL transactional outbox, UUID lease fencing & exponential backoff relay)
- [ ] `pdf` (HTML-to-PDF generator & localized Indian GST tax invoice renderer)
- [ ] `examples/invoice_service` (End-to-end reference service)
- [ ] `docs/` · `skills/` · `.github/workflows/` · `scripts/`

## Mandatory Quality & Verification Gates
- [ ] `./scripts/test_bash_compat.sh` passed (Bash 3.2 compatibility across all scripts).
- [ ] `./scripts/check_secrets.sh` passed with **0 leaked credentials or verified TruffleHog findings**.
- [ ] `./scripts/check_coverage.sh` passed with **100.0% statement coverage** across all 5 packages (`audit`, `export`, `notifications`, `outbox`, `pdf`) and `examples/invoice_service`.
- [ ] `./scripts/check_version.sh` and `./scripts/test_verify_release.sh` passed (`vMAJOR.MINOR.PATCH` parity, companion `abeta-dev` module pins, `gofmt`, and `CHANGELOG.md` symbol verification).
- [ ] `go test -race ./...` passed with **0 data races**.
- [ ] `golangci-lint run ./...` and `govulncheck ./...` passed with **0 issues**.
- [ ] **Documentation & AI Context Sync**: Updated `README.md`, package `README.md`s, `CHANGELOG.md`, `docs/index.html`, `docs/llms.txt` (`llms.txt`), `AGENTS.md`, and `skills/*/SKILL.md` where applicable.

## Security Invariants Checklist
- [ ] **PDF Generator (`pdf`)**: `EnableLocalFileAccess` defaults to `false` (no SSRF or arbitrary local file read).
- [ ] **Data Export (`export`)**: CSV formula-injection prefixes (`=`, `+`, `-`, `@`, `\t`, `\r`) escaped, `Content-Disposition` filename sanitized against CRLF/path traversal, and `X-Content-Type-Options: nosniff` set.
- [ ] **Notifications & Webhooks (`notifications`)**: Constant-time HMAC-SHA256 signature verification and timestamp replay protection enforced.
- [ ] **Transactional Outbox (`outbox`)**: Lease fencing tokens (`lease_token UUID`), identifier validation, and overflow-safe `BackoffWithJitter` preserved.
- [ ] **Audit Trail (`audit`)**: Append-only range-partitioned DDL and SHA-256 tamper-evident hash chain (`PrevHash` -> `ComputeHash`) preserved.
