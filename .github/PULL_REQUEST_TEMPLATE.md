## Description
<!-- Provide a concise summary of the changes introduced by this PR. -->

## Related Issue
<!-- Fixes #(issue) or Relates to #(issue) -->

## Package(s) Affected
- [ ] `audit`
- [ ] `export`
- [ ] `india`
- [ ] `notifications`
- [ ] `outbox`
- [ ] `pdf`
- [ ] `examples/invoice_service`
- [ ] Build / Toolchain / CI

## Quality Checklist
- [ ] `make fmt-check` reports no unformatted files (`gofmt -w .`).
- [ ] `make lint` passed with 0 warnings or errors (`golangci-lint run ./...`).
- [ ] `make test-race` passed with 0 data races (`go test -v -race ./...`).
- [ ] `make test-integration` passed (if touching `outbox` storage, DDL, or relay concurrency).
- [ ] `make vulncheck` passed with 0 known vulnerabilities (`govulncheck ./...`).
- [ ] `make clean` was executed before staging; workspace is clean of build and coverage artifacts.
- [ ] Error handling follows conventions: errors wrapped with `%w`, sentinel errors used with `errors.Is`/`errors.As`, zero reflection in hot paths.
- [ ] Unit tests added or updated to cover all new code paths and edge cases.
- [ ] Godoc comments added for all newly exported types, functions, and methods.

## Security Invariants Checklist
- [ ] **PDF Generator**: `EnableLocalFileAccess` defaults to `false` (no SSRF or arbitrary local file read).
- [ ] **Data Export**: CSV formula injection sanitization preserved (`=`, `+`, `-`, `@`, `\t`, `\r` escaped; whitespace evasion blocked).
- [ ] **Notifications & Webhooks**: HMAC-SHA256 constant-time comparison and timestamp freshness verification enforced.
- [ ] **Outbox**: Lease fencing tokens (`lease_token UUID`) verified in state transitions and table names sanitized.
- [ ] **Audit Trail**: Range-partitioned DDL and append-only trigger constraints preserved.
- [ ] **Indian Fintech**: Statutory validation integrity maintained (Verhoeff D5, Mod-36, PAN regex, exact integer paise arithmetic).
