package storage_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abeta-dev/go-app-kit/storage"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("simulated read failure")
}

func TestValidateKey(t *testing.T) {
	t.Parallel()

	validKeys := []string{
		"invoices/2026/INV-001.pdf",
		"exports/gst-report.csv",
		"single-segment.txt",
		"a/b/c/d.json",
	}
	for _, k := range validKeys {
		if err := storage.ValidateKey(k); err != nil {
			t.Fatalf("expected valid key %q, got error: %v", k, err)
		}
	}

	invalidKeys := []string{
		"",
		"/leading-slash.pdf",
		"trailing-slash.pdf/",
		"double//slash.pdf",
		"../secret.pdf",
		"invoices/../secret.pdf",
		"invoices/./current.pdf",
		`invoices\windows\path.pdf`,
		"invoices/null\x00byte.pdf",
	}
	for _, k := range invalidKeys {
		if err := storage.ValidateKey(k); !errors.Is(err, storage.ErrInvalidKey) {
			t.Fatalf("expected ErrInvalidKey for %q, got %v", k, err)
		}
	}
}

func TestNewMemoryStore_OptionsValidation(t *testing.T) {
	t.Parallel()

	if _, err := storage.NewMemoryStore(storage.WithMaxObjectBytes(0)); !errors.Is(err, storage.ErrObjectTooLarge) {
		t.Fatalf("expected ErrObjectTooLarge for limit 0, got %v", err)
	}
	if _, err := storage.NewMemoryStore(storage.WithBaseURL("")); !errors.Is(err, storage.ErrInvalidBaseURL) {
		t.Fatalf("expected ErrInvalidBaseURL for empty baseURL, got %v", err)
	}
	if _, err := storage.NewMemoryStore(storage.WithBaseURL("ftp://files.local")); !errors.Is(err, storage.ErrInvalidBaseURL) {
		t.Fatalf("expected ErrInvalidBaseURL for non-http scheme, got %v", err)
	}
	if _, err := storage.NewMemoryStore(storage.WithBaseURL("https://")); !errors.Is(err, storage.ErrInvalidBaseURL) {
		t.Fatalf("expected ErrInvalidBaseURL for missing host, got %v", err)
	}
	if _, err := storage.NewMemoryStore(storage.WithSigningSecret(nil)); !errors.Is(err, storage.ErrEmptySigningSecret) {
		t.Fatalf("expected ErrEmptySigningSecret for nil secret, got %v", err)
	}

	fixedTime := time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC)
	store, err := storage.NewMemoryStore(
		nil,
		storage.WithMaxObjectBytes(1024),
		storage.WithBaseURL("https://cdn.abeta.dev/"),
		storage.WithSigningSecret([]byte("top-secret-key")),
		storage.WithClock(nil),
		storage.WithClock(func() time.Time { return fixedTime }),
	)
	if err != nil || store == nil {
		t.Fatalf("expected valid MemoryStore, got err=%v", err)
	}
}

func TestMemoryStore_PutGetHeadDelete(t *testing.T) {
	t.Parallel()

	fixedTime := time.Date(2026, 4, 12, 12, 30, 0, 0, time.UTC)
	store, err := storage.NewMemoryStore(
		storage.WithMaxObjectBytes(32),
		storage.WithClock(func() time.Time { return fixedTime }),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	inputMeta := map[string]string{"invoice_id": "INV-2026-001", "gstin": "27AAPFU0939F1ZV"}
	payload := []byte("%PDF-1.7 invoice content")

	meta, err := store.Put(ctx, storage.PutInput{
		Key:         "invoices/INV-2026-001.pdf",
		ContentType: "application/pdf",
		Body:        bytes.NewReader(payload),
		Metadata:    inputMeta,
	})
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Mutating caller metadata must not mutate stored metadata
	inputMeta["invoice_id"] = "TAMPERED"
	meta.Metadata["invoice_id"] = "TAMPERED-RETURN"

	if meta.SizeBytes != int64(len(payload)) {
		t.Fatalf("expected SizeBytes=%d, got %d", len(payload), meta.SizeBytes)
	}
	if meta.SHA256Hex == "" || len(meta.SHA256Hex) != 64 {
		t.Fatalf("expected 64-char SHA256Hex, got %q", meta.SHA256Hex)
	}
	if !meta.CreatedAt.Equal(fixedTime) {
		t.Fatalf("expected CreatedAt=%v, got %v", fixedTime, meta.CreatedAt)
	}

	headMeta, err := store.Head(ctx, "invoices/INV-2026-001.pdf")
	if err != nil {
		t.Fatalf("Head failed: %v", err)
	}
	if headMeta.Metadata["invoice_id"] != "INV-2026-001" {
		t.Fatalf("expected defensive metadata copy, got %q", headMeta.Metadata["invoice_id"])
	}

	rc, getMeta, err := store.Get(ctx, "invoices/INV-2026-001.pdf")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer func() { _ = rc.Close() }()

	readBytes, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if !bytes.Equal(readBytes, payload) {
		t.Fatalf("expected payload %q, got %q", payload, readBytes)
	}
	if getMeta.SHA256Hex != headMeta.SHA256Hex {
		t.Fatalf("expected matching SHA256Hex")
	}

	// Put with nil body and nil metadata
	emptyMeta, err := store.Put(ctx, storage.PutInput{
		Key:         "exports/empty.csv",
		ContentType: "text/csv",
		Body:        nil,
		Metadata:    nil,
	})
	if err != nil || emptyMeta.SizeBytes != 0 || emptyMeta.Metadata != nil {
		t.Fatalf("expected empty object put to succeed, got meta=%+v err=%v", emptyMeta, err)
	}

	if err := store.Delete(ctx, "invoices/INV-2026-001.pdf"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if err := store.Delete(ctx, "invoices/INV-2026-001.pdf"); !errors.Is(err, storage.ErrObjectNotFound) {
		t.Fatalf("expected ErrObjectNotFound on second Delete, got %v", err)
	}
	if _, err := store.Head(ctx, "invoices/INV-2026-001.pdf"); !errors.Is(err, storage.ErrObjectNotFound) {
		t.Fatalf("expected ErrObjectNotFound on Head after Delete, got %v", err)
	}
	if _, _, err := store.Get(ctx, "invoices/INV-2026-001.pdf"); !errors.Is(err, storage.ErrObjectNotFound) {
		t.Fatalf("expected ErrObjectNotFound on Get after Delete, got %v", err)
	}
}

func TestMemoryStore_ErrorPaths(t *testing.T) {
	t.Parallel()

	store, err := storage.NewMemoryStore(storage.WithMaxObjectBytes(8))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	// Canceled context across all methods
	if _, err := store.Put(canceledCtx, storage.PutInput{Key: "a/b.txt", ContentType: "text/plain"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled on Put, got %v", err)
	}
	if _, _, err := store.Get(canceledCtx, "a/b.txt"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled on Get, got %v", err)
	}
	if _, err := store.Head(canceledCtx, "a/b.txt"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled on Head, got %v", err)
	}
	if err := store.Delete(canceledCtx, "a/b.txt"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled on Delete, got %v", err)
	}
	if _, err := store.PresignGet(canceledCtx, "a/b.txt", time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled on PresignGet, got %v", err)
	}
	if _, err := store.PresignPut(canceledCtx, "a/b.txt", "text/plain", time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled on PresignPut, got %v", err)
	}

	// Invalid key across all methods
	if _, err := store.Put(ctx, storage.PutInput{Key: "../bad", ContentType: "text/plain"}); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("expected ErrInvalidKey on Put, got %v", err)
	}
	if _, _, err := store.Get(ctx, "../bad"); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("expected ErrInvalidKey on Get, got %v", err)
	}
	if _, err := store.Head(ctx, "../bad"); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("expected ErrInvalidKey on Head, got %v", err)
	}
	if err := store.Delete(ctx, "../bad"); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("expected ErrInvalidKey on Delete, got %v", err)
	}
	if _, err := store.PresignGet(ctx, "../bad", time.Minute); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("expected ErrInvalidKey on PresignGet, got %v", err)
	}
	if _, err := store.PresignPut(ctx, "../bad", "text/plain", time.Minute); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("expected ErrInvalidKey on PresignPut, got %v", err)
	}

	// Empty content type, read failure, and size limit exceeded
	if _, err := store.Put(ctx, storage.PutInput{Key: "a/b.txt", ContentType: "   "}); !errors.Is(err, storage.ErrEmptyContentType) {
		t.Fatalf("expected ErrEmptyContentType on Put, got %v", err)
	}
	if _, err := store.Put(ctx, storage.PutInput{Key: "a/b.txt", ContentType: "text/plain", Body: failingReader{}}); err == nil {
		t.Fatalf("expected reader failure on Put")
	}
	if _, err := store.Put(ctx, storage.PutInput{
		Key:         "a/b.txt",
		ContentType: "text/plain",
		Body:        strings.NewReader("0123456789"),
	}); !errors.Is(err, storage.ErrObjectTooLarge) {
		t.Fatalf("expected ErrObjectTooLarge on Put, got %v", err)
	}
}

func TestMemoryStore_PresignAndVerify(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2026, 4, 12, 15, 0, 0, 0, time.UTC)
	store, err := storage.NewMemoryStore(
		storage.WithBaseURL("https://objects.abeta.dev"),
		storage.WithSigningSecret([]byte("hmac-signing-key-2026")),
		storage.WithClock(func() time.Time { return baseTime }),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()

	if _, err := store.PresignGet(ctx, "invoices/INV-1.pdf", 0); !errors.Is(err, storage.ErrInvalidExpiry) {
		t.Fatalf("expected ErrInvalidExpiry on PresignGet with 0 TTL, got %v", err)
	}
	if _, err := store.PresignPut(ctx, "invoices/INV-1.pdf", "", time.Minute); !errors.Is(err, storage.ErrEmptyContentType) {
		t.Fatalf("expected ErrEmptyContentType on PresignPut, got %v", err)
	}
	if _, err := store.PresignPut(ctx, "invoices/INV-1.pdf", "application/pdf", -time.Second); !errors.Is(err, storage.ErrInvalidExpiry) {
		t.Fatalf("expected ErrInvalidExpiry on PresignPut with negative TTL, got %v", err)
	}

	getURL, err := store.PresignGet(ctx, "invoices/INV-1.pdf", 15*time.Minute)
	if err != nil {
		t.Fatalf("PresignGet failed: %v", err)
	}
	if getURL.Method != http.MethodGet || getURL.Signature == "" {
		t.Fatalf("unexpected PresignGet result: %+v", getURL)
	}
	if err := store.VerifyPresignedURL(getURL.URL, http.MethodGet, baseTime.Add(5*time.Minute)); err != nil {
		t.Fatalf("VerifyPresignedURL for GET failed: %v", err)
	}

	putURL, err := store.PresignPut(ctx, "invoices/INV-1.pdf", "application/pdf", 15*time.Minute)
	if err != nil {
		t.Fatalf("PresignPut failed: %v", err)
	}
	if putURL.Method != http.MethodPut || putURL.Signature == "" {
		t.Fatalf("unexpected PresignPut result: %+v", putURL)
	}
	if err := store.VerifyPresignedURL(putURL.URL, http.MethodPut, baseTime.Add(5*time.Minute)); err != nil {
		t.Fatalf("VerifyPresignedURL for PUT failed: %v", err)
	}

	// Verification failure cases
	if err := store.VerifyPresignedURL("https://evil.example.com/invoices/INV-1.pdf", http.MethodGet, baseTime); !errors.Is(err, storage.ErrInvalidBaseURL) {
		t.Fatalf("expected ErrInvalidBaseURL for wrong origin, got %v", err)
	}
	if err := store.VerifyPresignedURL("https://objects.abeta.dev/%zz", http.MethodGet, baseTime); !errors.Is(err, storage.ErrInvalidBaseURL) {
		t.Fatalf("expected ErrInvalidBaseURL for malformed URL escape, got %v", err)
	}
	if err := store.VerifyPresignedURL("https://objects.abeta.dev/../bad?expires=1&sig=abc", http.MethodGet, baseTime); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("expected ErrInvalidKey for traversal key in URL, got %v", err)
	}
	if err := store.VerifyPresignedURL("https://objects.abeta.dev/invoices/INV-1.pdf", http.MethodGet, baseTime); !errors.Is(err, storage.ErrInvalidExpiry) {
		t.Fatalf("expected ErrInvalidExpiry for missing query params, got %v", err)
	}
	if err := store.VerifyPresignedURL("https://objects.abeta.dev/invoices/INV-1.pdf?expires=notanint&sig=abc", http.MethodGet, baseTime); !errors.Is(err, storage.ErrInvalidExpiry) {
		t.Fatalf("expected ErrInvalidExpiry for non-numeric expires, got %v", err)
	}
	if err := store.VerifyPresignedURL(getURL.URL, http.MethodGet, baseTime.Add(20*time.Minute)); !errors.Is(err, storage.ErrInvalidExpiry) {
		t.Fatalf("expected ErrInvalidExpiry for expired URL, got %v", err)
	}
	if err := store.VerifyPresignedURL(getURL.URL, http.MethodPut, baseTime); !errors.Is(err, storage.ErrEmptyContentType) {
		t.Fatalf("expected ErrEmptyContentType when verifying GET URL as PUT without content_type, got %v", err)
	}
	tamperedURL := strings.Replace(getURL.URL, "sig="+getURL.Signature, "sig=00000000", 1)
	if err := store.VerifyPresignedURL(tamperedURL, http.MethodGet, baseTime); !errors.Is(err, storage.ErrEmptySigningSecret) {
		t.Fatalf("expected ErrEmptySigningSecret for tampered signature, got %v", err)
	}
}

func TestMemoryStore_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	store, err := storage.NewMemoryStore()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := "concurrent/object.pdf"
			_, _ = store.Put(ctx, storage.PutInput{
				Key:         key,
				ContentType: "application/pdf",
				Body:        strings.NewReader("payload"),
				Metadata:    map[string]string{"worker": "w"},
			})
			_, _ = store.Head(ctx, key)
			if rc, _, err := store.Get(ctx, key); err == nil {
				_ = rc.Close()
			}
			_, _ = store.PresignGet(ctx, key, time.Minute)
		}(i)
	}
	wg.Wait()
}
