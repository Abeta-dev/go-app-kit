# `go-app-kit` — Agent Instructions (`AGENTS.md`)

`github.com/abeta-dev/go-app-kit` is the production enterprise application and domain accelerator kit for Go (`1.26.0`+). It provides 5 core enterprise application infrastructure packages built atop `github.com/abeta-dev/go-libs` and paired with statutory companion `github.com/abeta-dev/go-fintech-india`.

---

## 1. Module & Subskill Routing

When working inside or consuming `go-app-kit`, load the subskill matching the target package:

| Package | Import Path | Subskill Path | Core Capability |
| :--- | :--- | :--- | :--- |
| **Umbrella** | `github.com/abeta-dev/go-app-kit` | `skills/go-app-kit/SKILL.md` | Cross-package composition, `go.work` setup, truth gates (`check_version.sh`, `check_coverage.sh`, `check_secrets.sh`, `check_tag_readiness.sh`). |
| **`outbox`** | `github.com/abeta-dev/go-app-kit/outbox` | `skills/go-app-kit-outbox/SKILL.md` | PostgreSQL transactional outbox (`Store`, `NewPGStore`, `Relay`, `SELECT FOR UPDATE SKIP LOCKED`, `lease_token` fencing, DDL `001` & `002`). |
| **`pdf`** | `github.com/abeta-dev/go-app-kit/pdf` | `skills/go-app-kit-pdf/SKILL.md` | HTML-to-PDF compilation (`Renderer`, `NewWkhtmlRenderer`, `GenerateFromTemplate`, `GSTInvoiceTemplate`, `ReceiptTemplate`). |
| **`notifications`** | `github.com/abeta-dev/go-app-kit/notifications` | `skills/go-app-kit-notifications/SKILL.md` | Multi-channel notification broker (`Broker`, `SendAsync`, SMTP Email, Brevo Email (`NewBrevoSender`), WhatsApp Cloud API, Indian DLT SMS, Slack, HMAC-SHA256 Webhooks). |
| **`audit`** | `github.com/abeta-dev/go-app-kit/audit` | `skills/go-app-kit-audit/SKILL.md` | Range-partitioned PostgreSQL compliance audit trail (`Recorder`, `NewPGRecorder`, `NewEvent`, `ComputeDiff`, `ComputeHash`, append-only trigger DDL). |
| **`export`** | `github.com/abeta-dev/go-app-kit/export` | `skills/go-app-kit-export/SKILL.md` | Low-memory streaming CSV exporter (`CSVStreamer[T]`, `StreamHTTP[T]`, UTF-8 BOM, formula-injection escaping `=,+,-,@`). |

---

## 2. Hard Invariants & Truth Gates

1. **No `india` Package**: The deprecated `go-app-kit/india` façade was removed in `v0.4.0`. Always import `github.com/abeta-dev/go-fintech-india` directly for GSTIN, PAN, Aadhaar, IFSC, UPI, and exact paise `Money` arithmetic.
2. **No `replace` Directives in `go.mod`**: `scripts/check_version.sh` rejects any `replace` directive in `go.mod`. For local multi-module work, run `make workspace-init` to use a parent `go.work`.
3. **Go Toolchain Baseline**: Enforced at Go `1.26.0`. Never write documentation referencing older Go versions.
4. **No Machine-Specific Paths**: Never include absolute home-directory paths in any `.md` file.
5. **Mandatory Verification Before Every Commit/Release**:
   ```bash
   ./scripts/test_bash_compat.sh
   ./scripts/check_secrets.sh
   ./scripts/check_coverage.sh
   ./scripts/check_version.sh
   ./scripts/test_verify_release.sh
   go test -race ./...
   ```
   - **Secret Scanning Gate (`./scripts/check_secrets.sh`)**: Runs TruffleHog (`trufflehog git --only-verified --fail`) plus deterministic credential/token pattern checks across all tracked files (also invoked automatically by `./scripts/check_version.sh` and CI).
   - **Coverage Gate (`./scripts/check_coverage.sh`)**: Enforces global statement coverage floor $\ge 88.0\%$ and per-package floor $\ge 75.0\%$, verifying **100.0%** statement coverage across all 5 core packages (`audit`, `export`, `notifications`, `outbox`, `pdf`) and `examples/invoice_service`.
   - Every documented coverage percentage in `README.md` must match the exact `go tool cover -func=coverage.out` output.
