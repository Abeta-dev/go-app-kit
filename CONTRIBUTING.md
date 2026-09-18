# Contributing to go-app-kit

Thank you for your interest in contributing to `go-app-kit`!

`go-app-kit` is an enterprise application accelerator kit for Go, providing business capabilities: **Indian localized fintech helpers (GSTIN, PAN, IFSC, Aadhaar), transactional outbox with PostgreSQL DDL, multi-channel notifications, PDF document generation with GST invoice templates, partitioned compliance audit logging, and streaming data exports**.

We welcome contributions ranging from bug fixes and documentation clarifications to performance optimizations and test coverage improvements.

This guide provides everything you need to get started, set up your development environment, run the test suites, understand our architectural conventions, and submit a pull request that passes all automated quality gates.

---

## 1. Ground Rules & Maintainer Expectations

`go-app-kit` maintains strict enterprise reliability and backward compatibility standards:

- **Zero Breaking Changes**: We preserve backward compatibility across minor releases. Any change to public interfaces or exported symbols must be discussed beforehand in an issue or proposal.
- **Truth-Gate Protocol**: Every claim in documentation, coverage metric, and version tag is verified against live code in CI.
- **Maintainer SLA**: We aim to review and respond to contributor pull requests within **5 business days**. Issues are triaged weekly.

---

## 2. Development Setup & Prerequisites

### Toolchain Baseline
- **Go**: Version `1.26.0` or higher (`go version`)
- **golangci-lint**: Version `v1.64.0` or higher (`golangci-lint version`)
- **make**: Standard GNU Make (`make --version`)
- **Docker**: Required for running `./outbox/...` integration tests via testcontainers (`docker ps`)
- **wkhtmltopdf** *(optional)*: Host binary required only if executing real PDF rendering tests without the mock generator (`wkhtmltopdf --version`)

### Quickstart
1. Fork the repository on GitHub and clone your fork locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/go-app-kit.git
   cd go-app-kit
   ```

2. Run the baseline verification commands:
   ```bash
   # Verify module dependencies
   make verify

   # Run linters (zero issues allowed)
   make lint

   # Run unit tests
   make test

   # Run tests with the Go race detector enabled
   make test-race

   # Measure statement coverage
   make cover

   # Scan for known vulnerabilities
   make vulncheck

   # Build reference microservice
   make build
   ```

---

## 3. Integration Testing Prerequisites (`outbox`)

The `outbox` package includes comprehensive integration tests (`outbox/outbox_integration_test.go`) that validate real PostgreSQL behavior:
- `SELECT ... FOR UPDATE SKIP LOCKED` concurrent worker leasing
- Atomic lease renewal and lease expiration fencing (`ErrLeaseExpired`)
- Dead-letter queue transition on poison pills or retry exhaustion
- SQL identifier validation and custom table naming

### Running Integration Tests
Integration tests are protected by the `integration` build tag and require an active Docker daemon:

```bash
# Verify Docker daemon is active
docker ps

# Run the PostgreSQL integration test suite
make test-integration
# or directly:
go test -tags=integration -v ./outbox/...
```

When invoked, `testcontainers-go` automatically pulls and starts an official PostgreSQL 16 Alpine container, executes the outbox DDL migrations, runs the concurrent relay stress scenarios, and tears down the container upon completion.

---

## 4. Coding Conventions & Best Practices

Contributors should adhere to the following core engineering principles:

### A. Code Formatting
All Go source files must be formatted with standard `gofmt`. Run the format check target:
```bash
make fmt-check
# or fix formatting automatically:
gofmt -w .
```

### B. Error Handling & Error Wrapping (`%w`)
- Always wrap errors using `fmt.Errorf("%w", err)` to preserve causal error chains.
- Define sentinel errors (e.g., `ErrLeaseExpired`, `ErrInvalidGSTIN`) or typed errors (e.g., `*NonRetryableError`) for error checking via standard library `errors.Is` and `errors.As`.
- Avoid naked strings in `errors.New` where callers need programmatic inspection.

### C. Zero Reflection in Hot Paths
- Keep hot execution paths (outbox polling, notification fan-out, audit diffing, CSV streaming) free of runtime reflection (`reflect` package).
- Favor compile-time safety, standard interfaces, generic type constraints (e.g., `export.Column[T]`), and explicit type assertions.

### D. Clean Repository Practice (`make clean`)
Before committing changes or opening a pull request, always run:
```bash
make clean
```
This removes local build binaries (`invoice_service`, `examples/invoice_service/invoice_service`), coverage output files (`coverage.out`, `coverage.out.packages`, `.packages`), and temporary test binaries, ensuring your git working tree remains pristine.

---

## 5. Submitting a Pull Request

1. **Branch Naming**: Create a topic branch from `main`:
   ```bash
   git checkout -b feat/upi-validation
   # or
   git checkout -b fix/outbox-lease-timeout
   ```

2. **Commit Message Discipline**: We follow the Conventional Commits specification:
   - `feat(pkg)`: New exported function, type, or capability
   - `fix(pkg)`: Bug fix with regression test
   - `test(pkg)`: Additional test cases or benchmarks
   - `docs(pkg)`: Documentation corrections or examples
   - `refactor(pkg)`: Code refactoring without public API changes
   - `ci(...)`: Workflow or script updates
   - `chore(...)`: Maintenance or governance updates

3. **Checklist Before Opening a PR**:
   - [ ] `make fmt-check` reports no unformatted files.
   - [ ] `make lint` exits with code 0 (zero issues).
   - [ ] `make test-race` passes with zero race warnings.
   - [ ] `make test-integration` passes (if modifying `outbox` storage/relay semantics).
   - [ ] `make vulncheck` reports 0 vulnerabilities.
   - [ ] `make clean` was executed; `git status` shows no untracked artifacts.
   - [ ] Unit tests cover new code paths and edge cases.
   - [ ] Godoc comments added for all newly exported identifiers.

4. **Pull Request Template**: Fill in `.github/PULL_REQUEST_TEMPLATE.md` completely. PRs with failing automated CI checks will be blocked from merging until resolved.

---

## 6. Need Help?

- **Questions & Discussions**: Open a [GitHub Discussion](https://github.com/Abeta-dev/go-app-kit/discussions) for architecture or design questions.
- **Security Inquiries**: Please consult [SECURITY.md](SECURITY.md) for our private vulnerability disclosure process. Do NOT file public issues for security vulnerabilities.
