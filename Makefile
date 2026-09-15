.PHONY: all fmt-check test test-race cover test-integration lint vulncheck verify tidy build decouple couple workspace-init clean help

all: fmt-check verify lint test-race vulncheck build

# Verify that all Go source files are formatted with gofmt
fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "❌ Unformatted files detected. Run 'gofmt -w .':" && gofmt -l . && exit 1)
	@echo "✅ All Go files are formatted with gofmt."

test:
	go test -v ./...

test-race:
	go test -v -race ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

test-integration:
	go test -tags=integration -v ./outbox/...

lint:
	golangci-lint run ./...

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

verify:
	go mod verify

tidy:
	go mod tidy

build:
	go build -v ./examples/invoice_service

# Clean build and test artifacts
clean:
	rm -f invoice_service examples/invoice_service/invoice_service coverage.out coverage.out.packages .packages coverage.txt coverage.html *.test
	@echo "✅ Cleaned build and test artifacts."

# Decouple go.mod by dropping the local replace directive for open-source distribution/release
decouple:
	go mod edit -dropreplace github.com/umesh0492/go-libs
	@echo "Dropped replace directive in go.mod for standalone distribution."

# Couple go.mod for local companion workspace development alongside ../go-libs
couple:
	go mod edit -replace github.com/umesh0492/go-libs=../go-libs
	@echo "Configured replace directive in go.mod pointing to ../go-libs."

# Initialize multi-module Go workspace at parent directory without needing replace in go.mod
workspace-init:
	@cd .. && (go work init ./go-app-kit ./go-libs 2>/dev/null || go work use ./go-app-kit ./go-libs)
	@echo "Multi-module Go workspace configured in parent directory."
