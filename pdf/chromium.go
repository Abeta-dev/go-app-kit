package pdf

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	// ErrChromiumNotFound is returned when no Chrome or Chromium executable can be located.
	ErrChromiumNotFound = errors.New("pdf: chromium or chrome binary not found")
	// ErrInvalidPDFOutput is returned when a renderer produces empty or non-PDF output bytes.
	ErrInvalidPDFOutput = errors.New("pdf: renderer produced invalid or empty PDF output")
)

// ChromiumCommandRunner executes the headless Chromium CLI command and returns combined stdout/stderr.
// Supplying a custom runner via WithChromiumCommandRunner enables container wrappers or deterministic tests.
type ChromiumCommandRunner func(ctx context.Context, binary string, args []string) ([]byte, error)

// ChromiumOption configures a ChromiumRenderer instance.
type ChromiumOption func(*ChromiumRenderer)

// WithChromiumBinary sets an explicit path to the Chrome or Chromium executable.
func WithChromiumBinary(path string) ChromiumOption {
	return func(r *ChromiumRenderer) {
		r.binaryPath = strings.TrimSpace(path)
	}
}

// WithChromiumExtraArgs appends additional CLI flags passed to the headless Chromium invocation.
func WithChromiumExtraArgs(args ...string) ChromiumOption {
	return func(r *ChromiumRenderer) {
		for _, a := range args {
			if trimmed := strings.TrimSpace(a); trimmed != "" {
				r.extraArgs = append(r.extraArgs, trimmed)
			}
		}
	}
}

// WithChromiumMaxConcurrency bounds simultaneous headless Chromium processes (default: DefaultMaxConcurrency).
func WithChromiumMaxConcurrency(n int) ChromiumOption {
	return func(r *ChromiumRenderer) {
		if n > 0 {
			r.maxConcurrency = n
		}
	}
}

// WithChromiumNoSandbox appends --no-sandbox when running inside unprivileged Linux containers.
func WithChromiumNoSandbox(disable bool) ChromiumOption {
	return func(r *ChromiumRenderer) {
		r.noSandbox = disable
	}
}

// WithChromiumCommandRunner overrides the subprocess execution function for ChromiumRenderer.
func WithChromiumCommandRunner(runner ChromiumCommandRunner) ChromiumOption {
	return func(r *ChromiumRenderer) {
		if runner != nil {
			r.runner = runner
		}
	}
}

// ChromiumRenderer compiles HTML into PDF bytes using modern headless Chromium/Chrome
// (`--headless=new --print-to-pdf=...`) behind the Renderer interface.
// When Options.EnableLocalFileAccess is false (the default), HTML is served over an ephemeral
// loopback HTTP endpoint rather than a file:// URL so Chromium's browser origin sandbox blocks
// local filesystem exfiltration (e.g. file:///etc/passwd).
type ChromiumRenderer struct {
	binaryPath     string
	extraArgs      []string
	maxConcurrency int
	noSandbox      bool
	sem            chan struct{}
	runner         ChromiumCommandRunner
	findBinary     func() (string, error)
	mkdirTemp      func(string, string) (string, error)
	listenLoopback func(ctx context.Context) (net.Listener, error)
}

// NewChromiumRenderer constructs a ChromiumRenderer with an encapsulated concurrency semaphore.
func NewChromiumRenderer(opts ...ChromiumOption) *ChromiumRenderer {
	r := &ChromiumRenderer{
		maxConcurrency: DefaultMaxConcurrency,
		runner:         defaultChromiumRunner,
		findBinary:     FindChromiumBinary,
		mkdirTemp:      os.MkdirTemp,
		listenLoopback: func(ctx context.Context) (net.Listener, error) {
			var lc net.ListenConfig
			return lc.Listen(ctx, "tcp", "127.0.0.1:0")
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
	r.sem = make(chan struct{}, r.maxConcurrency)
	return r
}

var chromiumCandidateNames = []string{
	"chromium",
	"chromium-browser",
	"google-chrome",
	"google-chrome-stable",
}

var chromiumCandidatePaths = []string{
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	"/Applications/Chromium.app/Contents/MacOS/Chromium",
	"/usr/bin/chromium",
	"/usr/bin/chromium-browser",
	"/usr/bin/google-chrome",
}

// FindChromiumBinary locates a Chrome or Chromium executable via CHROMIUM_PATH, CHROME_PATH,
// PATH lookup, or standard macOS/Linux installation paths.
func FindChromiumBinary() (string, error) {
	return findChromiumBinaryWith(os.Getenv, exec.LookPath, os.Stat)
}

func findChromiumBinaryWith(
	getenv func(string) string,
	lookPath func(string) (string, error),
	stat func(string) (os.FileInfo, error),
) (string, error) {
	for _, envKey := range []string{"CHROMIUM_PATH", "CHROME_PATH"} {
		if candidate := strings.TrimSpace(getenv(envKey)); candidate != "" {
			if info, err := stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}
	for _, name := range chromiumCandidateNames {
		if found, err := lookPath(name); err == nil && found != "" {
			return found, nil
		}
	}
	for _, path := range chromiumCandidatePaths {
		if info, err := stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", ErrChromiumNotFound
}

func defaultChromiumRunner(ctx context.Context, binary string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binary, args...) //nolint:gosec // G204: binary is resolved via FindChromiumBinary or explicit WithChromiumBinary
	return cmd.CombinedOutput()
}

// Render compiles rawHTML into PDF bytes via headless Chromium, respecting context cancellation,
// page size/orientation/margin CSS @page rules, concurrency limits, and SSRF origin isolation.
func (r *ChromiumRenderer) Render(rawHTML string, opts Options) ([]byte, error) {
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	sem := r.sem
	if opts.Semaphore != nil {
		sem = opts.Semaphore
	}
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	binary, err := r.resolveBinary()
	if err != nil {
		return nil, err
	}

	tmpDir, err := r.mkdirTemp("", "go-app-kit-pdf-*")
	if err != nil {
		return nil, fmt.Errorf("pdf: failed to create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	styledHTML := injectChromiumPageCSS(rawHTML, opts)
	targetURL, cleanupServer, err := r.prepareTargetURL(ctx, tmpDir, styledHTML, opts.EnableLocalFileAccess)
	if err != nil {
		return nil, err
	}
	if cleanupServer != nil {
		defer cleanupServer()
	}

	outPath := filepath.Join(tmpDir, "output.pdf")
	args := r.buildChromiumArgs(outPath, targetURL, opts.EnableLocalFileAccess)
	return r.runAndReadPDF(ctx, binary, args, outPath)
}

func (r *ChromiumRenderer) runAndReadPDF(ctx context.Context, binary string, args []string, outPath string) ([]byte, error) {
	if out, err := r.runner(ctx, binary, args); err != nil {
		if trimmedOut := strings.TrimSpace(string(out)); trimmedOut != "" {
			return nil, fmt.Errorf("pdf: chromium render failed: %w: %s", err, trimmedOut)
		}
		return nil, fmt.Errorf("pdf: chromium render failed: %w", err)
	}

	pdfBytes, err := os.ReadFile(outPath) //nolint:gosec // G304: outPath is created inside an isolated package-owned temp directory
	if err != nil {
		return nil, fmt.Errorf("pdf: failed to read chromium output: %w", err)
	}
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
		return nil, ErrInvalidPDFOutput
	}
	return pdfBytes, nil
}

func (r *ChromiumRenderer) resolveBinary() (string, error) {
	if r.binaryPath != "" {
		return r.binaryPath, nil
	}
	found, err := r.findBinary()
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrNoRendererAvailable, err)
	}
	return found, nil
}

func (r *ChromiumRenderer) prepareTargetURL(ctx context.Context, tmpDir, styledHTML string, enableLocalFileAccess bool) (targetURL string, cleanup func(), err error) {
	if enableLocalFileAccess {
		inputPath := filepath.Join(tmpDir, "index.html")
		if err = os.WriteFile(inputPath, []byte(styledHTML), 0o600); err != nil {
			return "", nil, fmt.Errorf("pdf: failed to write html input: %w", err)
		}
		return "file://" + filepath.ToSlash(inputPath), nil, nil
	}
	targetURL, cleanup, err = startEphemeralHTMLServer(ctx, r.listenLoopback, styledHTML)
	if err != nil {
		return "", nil, fmt.Errorf("pdf: failed to start loopback html server: %w", err)
	}
	return targetURL, cleanup, nil
}

func (r *ChromiumRenderer) buildChromiumArgs(outPath, targetURL string, enableLocalFileAccess bool) []string {
	args := []string{
		"--headless=new",
		"--disable-gpu",
		"--disable-extensions",
		"--disable-background-networking",
		"--disable-sync",
		"--no-first-run",
		"--no-pdf-header-footer",
		"--run-all-compositor-stages-before-draw",
	}
	if r.noSandbox {
		args = append(args, "--no-sandbox")
	}
	if enableLocalFileAccess {
		args = append(args, "--allow-file-access-from-files")
	}
	args = append(args, r.extraArgs...)
	return append(args, "--print-to-pdf="+outPath, targetURL)
}

func startEphemeralHTMLServer(ctx context.Context, listen func(context.Context) (net.Listener, error), payload string) (targetURL string, stop func(), err error) {
	ln, err := listen(ctx)
	if err != nil {
		return "", nil, err
	}

	var tokenBytes [16]byte
	_, _ = rand.Read(tokenBytes[:])
	tokenPath := "/" + hex.EncodeToString(tokenBytes[:]) + ".html"

	mux := http.NewServeMux()
	mux.HandleFunc(tokenPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(payload))
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		_ = srv.Serve(ln)
	}()

	stop = func() {
		_ = srv.Close()
		_ = ln.Close()
	}
	return "http://" + ln.Addr().String() + tokenPath, stop, nil
}

func injectChromiumPageCSS(rawHTML string, opts Options) string {
	pageSize := strings.TrimSpace(opts.PageSize)
	if pageSize == "" {
		pageSize = "A4"
	}
	orientation := strings.ToLower(strings.TrimSpace(opts.Orientation))
	if orientation != "landscape" {
		orientation = "portrait"
	}

	var headInject strings.Builder
	fmt.Fprintf(&headInject,
		"<style>@page { size: %s %s; margin: %dmm %dmm %dmm %dmm; }</style>",
		pageSize, orientation, opts.MarginTop, opts.MarginRight, opts.MarginBottom, opts.MarginLeft,
	)
	if title := strings.TrimSpace(opts.Title); title != "" {
		fmt.Fprintf(&headInject, "<title>%s</title>", html.EscapeString(title))
	}

	lower := strings.ToLower(rawHTML)
	if idx := strings.Index(lower, "</head>"); idx >= 0 {
		return rawHTML[:idx] + headInject.String() + rawHTML[idx:]
	}
	return "<!DOCTYPE html><html><head><meta charset=\"utf-8\">" + headInject.String() + "</head><body>" + rawHTML + "</body></html>"
}
