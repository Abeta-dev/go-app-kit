# Versioning Policy, Leaf-Import Isolation & Deprecation Contract

This document defines the Semantic Versioning contract, the 7-Package Maturity & Leaf-Import Isolation Matrix, the automated **`N + 2` Minor-Release Deprecation Window Policy**, and the path to `v1.0.0` for `github.com/abeta-dev/go-app-kit` (current release: `v0.7.0`, toolchain baseline: `Go 1.26.0`).

---

## 1. Semantic Versioning Contract (`v0.7.0`)

`go-app-kit` adheres to [Semantic Versioning 2.0.0](https://semver.org/):

- **Current Published Release**: `github.com/abeta-dev/go-app-kit@v0.7.0` (built on `github.com/abeta-dev/go-libs` `v0.5.0` and paired with statutory companion `github.com/abeta-dev/go-fintech-india` `v0.4.0`).
- **Minor Releases (`v0.X.0`)**: Introduce new enterprise application capabilities, package enhancements, or scheduled removals of deprecated symbols whose `N + 2` minor-release deprecation window has elapsed.
- **Patch Releases (`v0.x.Y`)**: Strictly reserved for backward-compatible bug fixes, security hardening, and performance optimizations. Zero breaking API changes or symbol removals are permitted in patch releases.
- **Changelog Symbol Parity**: Enforced automatically on every commit by `./scripts/verify_changelog_symbols.sh` and `./scripts/check_version.sh`.

---

## 2. Strict `N + 2` Minor-Release Deprecation Window Policy

From Day 1, `go-app-kit` enforces a deterministic, machine-verified **`N + 2` Minor-Release Deprecation Window Contract** via [`./scripts/check_deprecation.sh`](../scripts/check_deprecation.sh):

1. **Mandatory Godoc Annotation Format**:
   Every deprecated symbol in non-test `*.go` source files must carry a standardized godoc annotation matching:
   ```go
   // Deprecated: Since vX.Y.Z (remove in >= vA.B.C). Use <Replacement> instead.
   ```
2. **Minimum 2-Minor-Release Window (`N + 2`)**:
   For any symbol deprecated in `vX.Y.Z` with target removal version `vA.B.C`, CI verifies:
   $$(A > X) \lor (A = X \land B \ge Y + 2)$$
   A symbol deprecated in `v0.7.0` cannot declare a removal target earlier than `v0.9.0`.
3. **Git Tag Continuity Enforcement**:
   `./scripts/check_deprecation.sh` compares `HEAD` against the previous release tag in Git. If a `// Deprecated: Since vX.Y.Z (remove in >= vA.B.C)` annotation existed in the prior release tag and is removed in `HEAD`, CI verifies that the current release version in [`CHANGELOG.md`](../CHANGELOG.md) is $\ge \text{vA.B.C}$. Premature symbol removal fails the CI build immediately.

### Active Deprecation Registry

| Symbol | Package | Annotation | Deprecated Since | Earliest Removal | Replacement |
| :--- | :--- | :--- | :---: | :---: | :--- |
| `StatusFailed` | `outbox` | `// Deprecated: Since v0.1.0 (remove in >= v0.8.0). Use StatusDeadLetter instead.` | `v0.1.0` | `v0.8.0` | `outbox.StatusDeadLetter` |

---

## 3. 7-Package Maturity & Leaf-Import Isolation Matrix

Every core package in `go-app-kit` is designed as an independent, self-contained leaf domain module. Enforced in CI by [`./scripts/check_leaf_imports.sh`](../scripts/check_leaf_imports.sh) and [`./scripts/check_coverage.sh`](../scripts/check_coverage.sh):

- **Gate 1 — Zero Cross-Package Sibling Coupling**: None of the 7 core packages (`audit`, `export`, `notifications`, `outbox`, `pdf`, `scheduler`, `storage`) may import any sibling `github.com/abeta-dev/go-app-kit/*` package across production, internal test, or external test files. Multi-package composition lives exclusively in reference applications such as `examples/invoice_service`.
- **Gate 2 — Heavy Dependency & Domain Isolation**: Verified via both `go list -deps` and isolated temporary `go.mod` consumer sandboxes (`go mod init` + `go mod edit -replace` + `go mod tidy`):
  - Importing `export`, `pdf`, `notifications`, `storage`, or `scheduler` never drags PostgreSQL drivers (`github.com/jackc/pgx`), web frameworks (`github.com/gin-gonic/gin`), or Indian statutory primitives (`github.com/abeta-dev/go-fintech-india`) into a consumer's dependency graph or `go.sum`.
  - Importing `audit` or `outbox` never drags `github.com/gin-gonic/gin` or `github.com/abeta-dev/go-fintech-india` into a consumer's dependency graph or `go.sum`.

| Package | Maturity Tier | Measured Coverage | Sibling `go-app-kit` Imports | Forbidden Transitive Dependencies (`check_leaf_imports.sh`) | API Status & Production Guarantee |
| :--- | :--- | :---: | :---: | :--- | :--- |
| **`audit`** | **Stable** | **100.0%** | `0` | Zero `gin`, zero `go-fintech-india` | Partitioned PostgreSQL compliance audit trail, SHA-256 hash chaining (`ComputeHash`), JSON `ComputeDiff`, append-only trigger DDL. |
| **`export`** | **Stable** | **100.0%** | `0` | Zero `pgx`, zero `gin`, zero `go-fintech-india` | Constant-memory RFC 4180 CSV streaming (`CSVStreamer[T]`, `StreamHTTP[T]`, `StreamHTTPCompressed[T]`, `NegotiateEncoding`), Brotli/Gzip pool hygiene, Excel UTF-8 BOM, formula-injection defense. |
| **`notifications`** | **Stable** | **100.0%** | `0` | Zero `pgx`, zero `gin`, zero `go-fintech-india` | Multi-channel broker (`Send`, `SendAsync`) across SMTP Email, Brevo v3 API, Meta WhatsApp Cloud API v20+, Indian DLT SMS, Slack, and HMAC-SHA256 replay-protected Webhooks. |
| **`outbox`** | **Stable** | **100.0%** | `0` | Zero `gin`, zero `go-fintech-india` | PostgreSQL transactional outbox (`Store`, `NewPGStore`, `Relay`) with `SELECT FOR UPDATE SKIP LOCKED`, UUID `lease_token` fencing, and full-jitter exponential backoff. |
| **`pdf`** | **Stable** | **100.0%** | `0` | Zero `pgx`, zero `gin`, zero `go-fintech-india` | Pluggable `Renderer` (`NewWkhtmlRenderer`), zero-temp-file stdin/stdout compilation, bounded concurrency semaphore, SSRF guard, and embedded GST Tax Invoice & Receipt templates. |
| **`storage`** | **Stable** | **100.0%** | `0` | Zero `pgx`, zero `gin`, zero `go-fintech-india` | Zero-leak object storage (`Store`, `NewMemoryStore`, `ValidateKey`), SHA-256 content digests (`SHA256Hex`), bounded upload size limits, and HMAC-SHA256 presigned URLs (`PresignGet`, `PresignPut`, `VerifyPresignedURL`). |
| **`scheduler`** | **Stable** | **100.0%** | `0` | Zero `pgx`, zero `gin`, zero `go-fintech-india` | Distributed lease-fenced periodic job scheduler (`Scheduler`, `NewScheduler`, `LeaseStore`, `NewMemoryLeaseStore`, `Register`, `RunOnce`, `Start`) with panic recovery and graceful in-flight draining. |

---

## 4. Path to `v1.0.0`

`v1.0.0` marks a permanent backward-compatibility commitment under the Go module compatibility promise. Before cutting `v1.0.0`, all of the following gates must be satisfied:

1. **API Surface Stability**: All 7 core packages (`audit`, `export`, `notifications`, `outbox`, `pdf`, `scheduler`, `storage`) maintain **Stable** status with zero unscheduled breaking changes across at least two consecutive minor releases.
2. **Deprecation Queue Resolution**: Any deprecated symbol whose `remove in >= vA.B.C` target is $\le \text{v1.0.0}$ is cleanly retired in accordance with `./scripts/check_deprecation.sh`.
3. **100.0% Statement Coverage & Race-Free Concurrency**: Continuous **100.0%** statement coverage across all 7 core packages and `examples/invoice_service` under `go test -race ./...`.
4. **Zero-Drift Automated Governance**: 100% pass rate across `./scripts/test_bash_compat.sh`, `./scripts/check_secrets.sh`, `./scripts/check_leaf_imports.sh`, `./scripts/check_deprecation.sh`, `./scripts/check_version.sh`, `./scripts/check_coverage.sh`, and `./scripts/module_agent_review.sh`.
