# `go-app-kit` — Agent Instructions (`AGENTS.md`)

`github.com/umesh0492/go-app-kit` is the production enterprise application and domain accelerator kit for Go (`1.26.0`+). It provides 5 core enterprise application infrastructure packages built atop `github.com/umesh0492/go-libs` and paired with statutory companion `github.com/umesh0492/go-fintech-india`.

---

## 1. Module & Subskill Routing

When working inside or consuming `go-app-kit`, load the subskill matching the target package:

| Package | Import Path | Subskill Path | Core Capability |
| :--- | :--- | :--- | :--- |
| **Umbrella** | `github.com/umesh0492/go-app-kit` | `skills/go-app-kit/SKILL.md` | Cross-package composition, `go.work` setup, truth gates (`check_version.sh`, `check_coverage.sh`, `check_tag_readiness.sh`). |
| **`outbox`** | `github.com/umesh0492/go-app-kit/outbox` | `skills/go-app-kit-outbox/SKILL.md` | PostgreSQL transactional outbox (`Store`, `NewPGStore`, `Relay`, `SELECT FOR UPDATE SKIP LOCKED`, `lease_token` fencing, DDL `001` & `002`). |
| **`pdf`** | `github.com/umesh0492/go-app-kit/pdf` | `skills/go-app-kit-pdf/SKILL.md` | HTML-to-PDF compilation (`Renderer`, `NewWkhtmlRenderer`, `GenerateFromTemplate`, `GSTInvoiceTemplate`, `ReceiptTemplate`). |
| **`notifications`** | `github.com/umesh0492/go-app-kit/notifications` | `skills/go-app-kit-notifications/SKILL.md` | Multi-channel notification broker (`Broker`, `SendAsync`, SMTP Email, WhatsApp Cloud API, Indian DLT SMS, Slack, HMAC-SHA256 Webhooks). |
| **`audit`** | `github.com/umesh0492/go-app-kit/audit` | `skills/go-app-kit-audit/SKILL.md` | Range-partitioned PostgreSQL compliance audit trail (`Recorder`, `PGRecorder`, `NewEvent`, `ComputeDiff`, append-only trigger DDL). |
| **`export`** | `github.com/umesh0492/go-app-kit/export` | `skills/go-app-kit-export/SKILL.md` | Low-memory streaming CSV exporter (`CSVStreamer[T]`, `StreamHTTP[T]`, UTF-8 BOM, formula-injection escaping `=,+,-,@`). |

---

## 2. Hard Invariants & Truth Gates

1. **No `india` Package**: The deprecated `go-app-kit/india` façade was removed in `v0.4.0`. Always import `github.com/umesh0492/go-fintech-india` directly for GSTIN, PAN, Aadhaar, IFSC, UPI, and exact paise `Money` arithmetic.
2. **No `replace` Directives in `go.mod`**: `scripts/check_version.sh` rejects any `replace` directive in `go.mod`. For local multi-module work, run `make workspace-init` to use a parent `go.work`.
3. **Go Toolchain Baseline**: Enforced at Go `1.26.0`. Never write documentation referencing older Go versions.
4. **No Machine-Specific Paths**: Never include absolute home-directory paths in any `.md` file.
5. **Mandatory Verification Before Every Commit/Release**:
   ```bash
   ./scripts/check_version.sh
   ./scripts/check_coverage.sh
   ./scripts/test_bash_compat.sh
   go test -race ./...
   ```
   - Global statement coverage floor: $\ge 88.0\%$ (currently $93.3\%$).
   - Per-package statement coverage floor: $\ge 75.0\%$.
   - Every documented coverage percentage in `README.md` must match the exact `go tool cover -func=coverage.out` output.
