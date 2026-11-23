# `go-app-kit` — Agent Instructions (`AGENTS.md`)

`github.com/abeta-dev/go-app-kit` (`v0.7.0`, `Go 1.26.0`, **100.0% statement coverage**) is the production enterprise application and domain accelerator kit for Go (`1.26.0`+). It provides 7 core enterprise application infrastructure packages built atop `github.com/abeta-dev/go-libs` (`v0.5.0`, 36 foundational packages) and paired with statutory companion `github.com/abeta-dev/go-fintech-india` (`v0.4.0`, 127 exported symbols across 15 domain files).

---

## 1. Module & Subskill Routing

When working inside or consuming `go-app-kit`, load the subskill matching the target package:

| Package | Import Path | Subskill Path | Core Capability |
| :--- | :--- | :--- | :--- |
| **Umbrella** | `github.com/abeta-dev/go-app-kit` | `skills/go-app-kit/SKILL.md` | Cross-package composition, `go.work` setup, truth gates (`check_version.sh`, `check_coverage.sh`, `check_secrets.sh`, `check_leaf_imports.sh`, `check_deprecation.sh`, `check_tag_readiness.sh`). |
| **`outbox`** | `github.com/abeta-dev/go-app-kit/outbox` | `skills/go-app-kit-outbox/SKILL.md` | PostgreSQL transactional outbox (`Store`, `NewPGStore`, `Relay`, `SELECT FOR UPDATE SKIP LOCKED`, `lease_token` fencing, DDL `001` & `002`). |
| **`pdf`** | `github.com/abeta-dev/go-app-kit/pdf` | `skills/go-app-kit-pdf/SKILL.md` | HTML-to-PDF compilation (`Renderer`, `NewWkhtmlRenderer`, `GenerateFromTemplate`, `GSTInvoiceTemplate`, `ReceiptTemplate`). |
| **`notifications`** | `github.com/abeta-dev/go-app-kit/notifications` | `skills/go-app-kit-notifications/SKILL.md` | Multi-channel notification broker (`Broker`, `SendAsync`, SMTP Email, Brevo Email (`NewBrevoSender`), WhatsApp Cloud API, Indian DLT SMS, Slack, HMAC-SHA256 Webhooks). |
| **`audit`** | `github.com/abeta-dev/go-app-kit/audit` | `skills/go-app-kit-audit/SKILL.md` | Range-partitioned PostgreSQL compliance audit trail (`Recorder`, `NewPGRecorder`, `NewEvent`, `ComputeDiff`, `ComputeHash`, append-only trigger DDL). |
| **`export`** | `github.com/abeta-dev/go-app-kit/export` | `skills/go-app-kit-export/SKILL.md` | Low-memory streaming CSV exporter (`CSVStreamer[T]`, `StreamHTTP[T]`, `StreamHTTPCompressed[T]`, `NegotiateEncoding`, UTF-8 BOM, formula-injection escaping `=,+,-,@`). |
| **`storage`** | `github.com/abeta-dev/go-app-kit/storage` | `skills/go-app-kit-storage/SKILL.md` | Zero-leak object storage & presigned URL engine (`Store`, `NewMemoryStore`, `ValidateKey`, `SHA256Hex`, HMAC-SHA256 `PresignGet`, `PresignPut`, `VerifyPresignedURL`). |
| **`scheduler`** | `github.com/abeta-dev/go-app-kit/scheduler` | `skills/go-app-kit-scheduler/SKILL.md` | Distributed lease-fenced periodic job runner (`Scheduler`, `NewScheduler`, `LeaseStore`, `NewMemoryLeaseStore`, `Register`, `RunOnce`, `Start`). |

---

## 2. Module Code-Owner Agents & Automated PR Review

Every file and package in `go-app-kit` is governed by one of **8 Module Code-Owner Agents** defined in [`.github/CODEOWNERS`](.github/CODEOWNERS) and [`.github/module-agents.json`](.github/module-agents.json), paired with its companion skill in `skills/*/SKILL.md` and enforced automatically on every PR via [`.github/workflows/module-agent-review.yml`](.github/workflows/module-agent-review.yml) and [`./scripts/module_agent_review.sh`](scripts/module_agent_review.sh):

| Module Code-Owner Agent | Companion Skill | Owned Modules / Scope |
| :--- | :--- | :--- |
| **`@go-app-kit-audit-agent`** | [`skills/go-app-kit-audit/SKILL.md`](skills/go-app-kit-audit/SKILL.md) | `/audit/` |
| **`@go-app-kit-outbox-agent`** | [`skills/go-app-kit-outbox/SKILL.md`](skills/go-app-kit-outbox/SKILL.md) | `/outbox/` |
| **`@go-app-kit-notifications-agent`** | [`skills/go-app-kit-notifications/SKILL.md`](skills/go-app-kit-notifications/SKILL.md) | `/notifications/` |
| **`@go-app-kit-pdf-agent`** | [`skills/go-app-kit-pdf/SKILL.md`](skills/go-app-kit-pdf/SKILL.md) | `/pdf/` |
| **`@go-app-kit-export-agent`** | [`skills/go-app-kit-export/SKILL.md`](skills/go-app-kit-export/SKILL.md) | `/export/` |
| **`@go-app-kit-storage-agent`** | [`skills/go-app-kit-storage/SKILL.md`](skills/go-app-kit-storage/SKILL.md) | `/storage/` |
| **`@go-app-kit-scheduler-agent`** | [`skills/go-app-kit-scheduler/SKILL.md`](skills/go-app-kit-scheduler/SKILL.md) | `/scheduler/` |
| **`@go-app-kit-platform-governance-agent`** | [`skills/go-app-kit/SKILL.md`](skills/go-app-kit/SKILL.md) | `/examples/`, `/.github/`, `/scripts/`, `/docs/`, `/skills/`, `/AGENTS.md`, `/README.md`, `/CHANGELOG.md`, `/llms.txt`, `/go.mod`, `/go.sum` |

Run the Module Code-Owner Agent review locally against any branch or PR diff:

```bash
./scripts/module_agent_review.sh --base origin/main --head HEAD
```

---

## 3. Hard Invariants & Truth Gates

1. **No `india` Package**: The deprecated `go-app-kit/india` façade was removed in `v0.4.0`. Always import `github.com/abeta-dev/go-fintech-india` directly for GSTIN, PAN, Aadhaar, IFSC, UPI, and exact paise `Money` arithmetic.
2. **No `replace` Directives in `go.mod`**: `scripts/check_version.sh` rejects any `replace` directive in `go.mod`. For local multi-module work, run `make workspace-init` to use a parent `go.work`.
3. **Go Toolchain Baseline**: Enforced at Go `1.26.0`. Never write documentation referencing older Go versions.
4. **No Machine-Specific Paths**: Never include absolute home-directory paths in any `.md` file.
5. **Mandatory Verification Before Every Commit/Release**:
   ```bash
   ./scripts/test_bash_compat.sh
   ./scripts/module_agent_review.sh --base origin/main --head HEAD
   ./scripts/check_secrets.sh
   ./scripts/check_leaf_imports.sh
   ./scripts/check_deprecation.sh
   ./scripts/check_coverage.sh
   ./scripts/check_version.sh
   ./scripts/test_verify_release.sh
   go test -race ./...
   ```
   - **Secret Scanning Gate (`./scripts/check_secrets.sh`)**: Runs TruffleHog (`trufflehog git --only-verified --fail`) plus deterministic credential/token pattern checks across all tracked files (also invoked automatically by `./scripts/check_version.sh` and CI).
   - **Leaf-Import & Dependency Isolation Gate (`./scripts/check_leaf_imports.sh`)**: Verifies zero cross-package sibling imports across all 7 core packages (`audit`, `export`, `notifications`, `outbox`, `pdf`, `scheduler`, `storage`) and verifies via `go list -deps` and isolated temporary `go.mod` sandboxes that `export`, `pdf`, `notifications`, `storage`, and `scheduler` never pull `pgx`, `gin`, or `go-fintech-india` into a consumer's dependency graph or `go.sum`, and `audit` and `outbox` never pull `gin` or `go-fintech-india`.
   - **Strict Deprecation Window Gate (`./scripts/check_deprecation.sh` & [`docs/VERSIONING.md`](docs/VERSIONING.md))**: Enforces the strict `N + 2` Minor-Release Deprecation Window Contract (`// Deprecated: Since vX.Y.Z (remove in >= vA.B.C). Use <Replacement> instead.`) across all non-test `.go` files and verifies against the previous release tag in Git that no deprecated symbol is removed before `CHANGELOG.md` reaches `>= vA.B.C`.
   - **Coverage Gate (`./scripts/check_coverage.sh`)**: Enforces global statement coverage floor $\ge 88.0\%$ and per-package floor $\ge 75.0\%$, verifying **100.0%** statement coverage across all 7 core packages (`audit`, `export`, `notifications`, `outbox`, `pdf`, `scheduler`, `storage`) and `examples/invoice_service`.
   - Every documented coverage percentage in `README.md` must match the exact `go tool cover -func=coverage.out` output.
