package pdf

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFindChromiumBinaryWith(t *testing.T) {
	t.Parallel()

	tmpFile, err := os.CreateTemp(t.TempDir(), "fake-chromium-*")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	_ = tmpFile.Close()
	fakeBin := tmpFile.Name()

	// 1. Found via CHROMIUM_PATH env var
	found, err := findChromiumBinaryWith(
		func(k string) string {
			if k == "CHROMIUM_PATH" {
				return fakeBin
			}
			return ""
		},
		func(string) (string, error) { return "", errors.New("not in path") },
		os.Stat,
	)
	if err != nil || found != fakeBin {
		t.Fatalf("expected CHROMIUM_PATH match %q, got %q err=%v", fakeBin, found, err)
	}

	// 2. Found via LookPath
	found, err = findChromiumBinaryWith(
		func(string) string { return "" },
		func(name string) (string, error) {
			if name == "google-chrome" {
				return "/usr/local/bin/google-chrome", nil
			}
			return "", errors.New("miss")
		},
		func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
	)
	if err != nil || found != "/usr/local/bin/google-chrome" {
		t.Fatalf("expected LookPath match, got %q err=%v", found, err)
	}

	// 3. Found via standard candidate path
	fakeInfo, _ := os.Stat(fakeBin)
	found, err = findChromiumBinaryWith(
		func(string) string { return "" },
		func(string) (string, error) { return "", errors.New("miss") },
		func(p string) (os.FileInfo, error) {
			if p == "/usr/bin/chromium" {
				return fakeInfo, nil
			}
			return nil, os.ErrNotExist
		},
	)
	if err != nil || found != "/usr/bin/chromium" {
		t.Fatalf("expected standard path match, got %q err=%v", found, err)
	}

	// 4. Not found
	_, err = findChromiumBinaryWith(
		func(string) string { return "" },
		func(string) (string, error) { return "", errors.New("miss") },
		func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
	)
	if !errors.Is(err, ErrChromiumNotFound) {
		t.Fatalf("expected ErrChromiumNotFound, got %v", err)
	}

	// Exercise exported FindChromiumBinary wrapper safely (result depends on host OS).
	_, _ = FindChromiumBinary()
}

func TestChromiumRenderer_LoopbackSafeAndLocalFileAccess(t *testing.T) {
	t.Parallel()

	var capturedArgs []string
	var fetchedHTML string

	r := NewChromiumRenderer(
		nil,
		WithChromiumBinary("/usr/bin/chromium"),
		WithChromiumExtraArgs(" ", "--disable-dev-shm-usage"),
		WithChromiumMaxConcurrency(2),
		WithChromiumNoSandbox(true),
		WithChromiumCommandRunner(func(ctx context.Context, binary string, args []string) ([]byte, error) {
			capturedArgs = append([]string(nil), args...)
			var outPath, targetURL string
			for _, a := range args {
				if strings.HasPrefix(a, "--print-to-pdf=") {
					outPath = strings.TrimPrefix(a, "--print-to-pdf=")
				}
			}
			targetURL = args[len(args)-1]

			if strings.HasPrefix(targetURL, "http://127.0.0.1:") {
				resp, err := http.Get(targetURL) //nolint:gosec // ephemeral loopback test URL
				if err != nil {
					return nil, err
				}
				body, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				fetchedHTML = string(body)
			}
			return nil, os.WriteFile(outPath, []byte("%PDF-1.4 chromium mock output"), 0o600)
		}),
	)

	// 1. Default mode (EnableLocalFileAccess=false): serves over ephemeral loopback HTTP (zero file:// origin)
	buf, err := Generate(
		"<html><head></head><body><h1>GST Invoice</h1></body></html>",
		WithRenderer(r),
		WithPageSize("Letter"),
		WithOrientation("Landscape"),
		WithMargins(12, 14, 16, 18),
		WithTitle("Invoice <2026>"),
	)
	if err != nil {
		t.Fatalf("Generate via ChromiumRenderer failed: %v", err)
	}
	if !strings.HasPrefix(buf.String(), "%PDF-1.4") {
		t.Fatalf("unexpected PDF bytes: %q", buf.String())
	}
	if !strings.Contains(fetchedHTML, "@page { size: Letter landscape; margin: 12mm 18mm 14mm 16mm; }") {
		t.Fatalf("expected injected @page CSS, got %q", fetchedHTML)
	}
	if !strings.Contains(fetchedHTML, "<title>Invoice &lt;2026&gt;</title>") {
		t.Fatalf("expected escaped title tag, got %q", fetchedHTML)
	}
	joinedArgs := strings.Join(capturedArgs, " ")
	if !strings.Contains(joinedArgs, "--no-sandbox") || !strings.Contains(joinedArgs, "--disable-dev-shm-usage") {
		t.Fatalf("expected extra/no-sandbox flags in args: %q", joinedArgs)
	}
	if strings.Contains(joinedArgs, "--allow-file-access-from-files") {
		t.Fatalf("did not expect --allow-file-access-from-files when local file access is disabled")
	}

	// 2. Local file access enabled: uses file:// input + --allow-file-access-from-files + wraps fragment HTML
	var localTarget string
	rLocal := NewChromiumRenderer(
		WithChromiumBinary("/usr/bin/chromium"),
		WithChromiumCommandRunner(func(_ context.Context, _ string, args []string) ([]byte, error) {
			var outPath string
			for _, a := range args {
				if strings.HasPrefix(a, "--print-to-pdf=") {
					outPath = strings.TrimPrefix(a, "--print-to-pdf=")
				}
			}
			localTarget = args[len(args)-1]
			return nil, os.WriteFile(outPath, []byte("%PDF-1.4 local file mode"), 0o600)
		}),
	)
	opts := Options{EnableLocalFileAccess: true}
	data, err := rLocal.Render("<p>Fragment without head</p>", opts)
	if err != nil {
		t.Fatalf("Render with local file access failed: %v", err)
	}
	if !strings.HasPrefix(string(data), "%PDF-1.4") || !strings.HasPrefix(localTarget, "file://") {
		t.Fatalf("unexpected local render output=%q target=%q", string(data), localTarget)
	}
}

func TestChromiumRenderer_ConcurrencyAndErrorPaths(t *testing.T) {
	t.Parallel()

	// Exercise defaultChromiumRunner directly using /usr/bin/true (fast, zero-side-effect POSIX binary)
	if _, err := defaultChromiumRunner(context.Background(), "/usr/bin/true", nil); err != nil {
		t.Fatalf("defaultChromiumRunner with /usr/bin/true failed: %v", err)
	}

	// Context already canceled
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	r := NewChromiumRenderer(WithChromiumBinary("/usr/bin/chromium"))
	if _, err := r.Render("<p>x</p>", Options{Context: canceledCtx}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// Semaphore blocked -> context timeout
	fullSem := make(chan struct{}, 1)
	fullSem <- struct{}{}
	timeoutCtx, cancelTimeout := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancelTimeout()
	if _, err := r.Render("<p>x</p>", Options{Context: timeoutCtx, Semaphore: fullSem}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded on full semaphore, got %v", err)
	}

	// Missing binary (auto-discovery failure)
	rMissing := NewChromiumRenderer()
	rMissing.findBinary = func() (string, error) { return "", ErrChromiumNotFound }
	if _, err := rMissing.Render("<p>x</p>", DefaultOptions()); !errors.Is(err, ErrNoRendererAvailable) || !errors.Is(err, ErrChromiumNotFound) {
		t.Fatalf("expected wrapped ErrNoRendererAvailable + ErrChromiumNotFound, got %v", err)
	}

	// Auto-discovery success + MkdirTemp error
	simErr := errors.New("simulated error")
	rTmpErr := NewChromiumRenderer()
	rTmpErr.findBinary = func() (string, error) { return "/usr/bin/chromium", nil }
	rTmpErr.mkdirTemp = func(string, string) (string, error) { return "", simErr }
	if _, err := rTmpErr.Render("<p>x</p>", DefaultOptions()); !errors.Is(err, simErr) {
		t.Fatalf("expected mkdirTemp simErr, got %v", err)
	}

	// WriteFile error when EnableLocalFileAccess=true (mkdirTemp returns nonexistent dir)
	rWriteErr := NewChromiumRenderer(WithChromiumBinary("/usr/bin/chromium"))
	rWriteErr.mkdirTemp = func(string, string) (string, error) {
		return filepath.Join(t.TempDir(), "does-not-exist", "nested"), nil
	}
	if _, err := rWriteErr.Render("<p>x</p>", Options{EnableLocalFileAccess: true}); err == nil {
		t.Fatalf("expected write html error")
	}

	// Loopback listener error
	rListenErr := NewChromiumRenderer(WithChromiumBinary("/usr/bin/chromium"))
	rListenErr.listenLoopback = func(context.Context) (net.Listener, error) { return nil, simErr }
	if _, err := rListenErr.Render("<p>x</p>", DefaultOptions()); !errors.Is(err, simErr) {
		t.Fatalf("expected listenLoopback simErr, got %v", err)
	}

	// Runner error with stderr output & without stderr output
	rExecWithOut := NewChromiumRenderer(
		WithChromiumBinary("/usr/bin/chromium"),
		WithChromiumCommandRunner(func(context.Context, string, []string) ([]byte, error) {
			return []byte("GPU crash"), simErr
		}),
	)
	if _, err := rExecWithOut.Render("<p>x</p>", DefaultOptions()); !errors.Is(err, simErr) || !strings.Contains(err.Error(), "GPU crash") {
		t.Fatalf("expected stderr included in error, got %v", err)
	}

	rExecNoOut := NewChromiumRenderer(
		WithChromiumBinary("/usr/bin/chromium"),
		WithChromiumCommandRunner(func(context.Context, string, []string) ([]byte, error) {
			return nil, simErr
		}),
	)
	if _, err := rExecNoOut.Render("<p>x</p>", DefaultOptions()); !errors.Is(err, simErr) {
		t.Fatalf("expected simErr without output, got %v", err)
	}

	// Runner succeeds but fails to create output.pdf
	rMissingOut := NewChromiumRenderer(
		WithChromiumBinary("/usr/bin/chromium"),
		WithChromiumCommandRunner(func(context.Context, string, []string) ([]byte, error) {
			return nil, nil
		}),
	)
	if _, err := rMissingOut.Render("<p>x</p>", DefaultOptions()); err == nil {
		t.Fatalf("expected missing output.pdf read error")
	}

	// Runner creates non-PDF garbage
	rInvalidOut := NewChromiumRenderer(
		WithChromiumBinary("/usr/bin/chromium"),
		WithChromiumCommandRunner(func(_ context.Context, _ string, args []string) ([]byte, error) {
			for _, a := range args {
				if strings.HasPrefix(a, "--print-to-pdf=") {
					_ = os.WriteFile(strings.TrimPrefix(a, "--print-to-pdf="), []byte("NOT-PDF"), 0o600)
				}
			}
			return nil, nil
		}),
	)
	if _, err := rInvalidOut.Render("<p>x</p>", DefaultOptions()); !errors.Is(err, ErrInvalidPDFOutput) {
		t.Fatalf("expected ErrInvalidPDFOutput, got %v", err)
	}

	// Bounded concurrency test under -race
	var active, maxActive int32
	rConc := NewChromiumRenderer(
		WithChromiumBinary("/usr/bin/chromium"),
		WithChromiumMaxConcurrency(2),
		WithChromiumCommandRunner(func(_ context.Context, _ string, args []string) ([]byte, error) {
			cur := atomic.AddInt32(&active, 1)
			for {
				prev := atomic.LoadInt32(&maxActive)
				if cur <= prev || atomic.CompareAndSwapInt32(&maxActive, prev, cur) {
					break
				}
			}
			time.Sleep(15 * time.Millisecond)
			for _, a := range args {
				if strings.HasPrefix(a, "--print-to-pdf=") {
					_ = os.WriteFile(strings.TrimPrefix(a, "--print-to-pdf="), []byte("%PDF-1.4 ok"), 0o600)
				}
			}
			atomic.AddInt32(&active, -1)
			return nil, nil
		}),
	)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := rConc.Render("<p>concurrent</p>", DefaultOptions()); err != nil {
				t.Errorf("concurrent render failed: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&maxActive); got > 2 || got == 0 {
		t.Fatalf("expected 1 <= maxActive <= 2, got %d", got)
	}
}
