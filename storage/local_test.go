package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type localFailReader struct{}

func (localFailReader) Read([]byte) (int, error) {
	return 0, errors.New("simulated local read failure")
}

func TestNewLocalStore_OptionsAndErrors(t *testing.T) {
	t.Parallel()

	if _, err := NewLocalStore("   "); !errors.Is(err, ErrInvalidRootDir) {
		t.Fatalf("expected ErrInvalidRootDir, got %v", err)
	}

	dir := t.TempDir()
	if _, err := NewLocalStore(dir, WithLocalMaxObjectBytes(0)); !errors.Is(err, ErrObjectTooLarge) {
		t.Fatalf("expected ErrObjectTooLarge, got %v", err)
	}
	if _, err := NewLocalStore(dir, WithLocalBaseURL("ftp://invalid")); !errors.Is(err, ErrInvalidBaseURL) {
		t.Fatalf("expected ErrInvalidBaseURL, got %v", err)
	}
	if _, err := NewLocalStore(dir, WithLocalSigningSecret(nil)); !errors.Is(err, ErrEmptySigningSecret) {
		t.Fatalf("expected ErrEmptySigningSecret, got %v", err)
	}
	if _, err := NewLocalStore(dir, WithFilePerm(0)); !errors.Is(err, ErrInvalidFileMode) {
		t.Fatalf("expected ErrInvalidFileMode on file perm 0, got %v", err)
	}
	if _, err := NewLocalStore(dir, WithDirPerm(0)); !errors.Is(err, ErrInvalidFileMode) {
		t.Fatalf("expected ErrInvalidFileMode on dir perm 0, got %v", err)
	}

	fixed := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	store, err := NewLocalStore(
		dir,
		nil,
		WithLocalMaxObjectBytes(1024),
		WithLocalBaseURL("https://local.abeta.dev/"),
		WithLocalSigningSecret([]byte("local-secret-key")),
		WithLocalClock(nil),
		WithLocalClock(func() time.Time { return fixed }),
		WithFilePerm(0o600),
		WithDirPerm(0o700),
	)
	if err != nil {
		t.Fatalf("NewLocalStore failed: %v", err)
	}
	if store.RootDir() == "" {
		t.Fatalf("expected non-empty RootDir")
	}

	// Filesystem init failure paths (abs, objects MkdirAll, meta MkdirAll)
	simErr := errors.New("simulated fs init error")
	if _, err := NewLocalStore(dir, func(s *LocalStore) error {
		s.fs.abs = func(string) (string, error) { return "", simErr }
		return nil
	}); !errors.Is(err, simErr) {
		t.Fatalf("expected abs error, got %v", err)
	}
	if _, err := NewLocalStore(dir, func(s *LocalStore) error {
		s.fs.mkdirAll = func(string, os.FileMode) error { return simErr }
		return nil
	}); !errors.Is(err, simErr) {
		t.Fatalf("expected objects mkdirAll error, got %v", err)
	}
	calls := 0
	if _, err := NewLocalStore(dir, func(s *LocalStore) error {
		s.fs.mkdirAll = func(p string, m os.FileMode) error {
			calls++
			if calls == 2 {
				return simErr
			}
			return os.MkdirAll(p, m)
		}
		return nil
	}); !errors.Is(err, simErr) {
		t.Fatalf("expected meta mkdirAll error, got %v", err)
	}
}

func TestLocalStore_PutGetHeadDeleteAtomicAndPermissions(t *testing.T) {
	t.Parallel()

	fixed := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	store, err := NewLocalStore(
		root,
		WithLocalMaxObjectBytes(64),
		WithLocalClock(func() time.Time { return fixed }),
	)
	if err != nil {
		t.Fatalf("NewLocalStore failed: %v", err)
	}

	// Verify Store interface satisfaction
	var _ Store = store

	ctx := context.Background()
	inputMeta := map[string]string{"invoice_id": "INV-2026-099", "tenant": "acme"}
	payload := []byte("%PDF-1.4 atomic invoice payload")

	meta, err := store.Put(ctx, PutInput{
		Key:         "invoices/2026/INV-2026-099.pdf",
		ContentType: "application/pdf",
		Body:        bytes.NewReader(payload),
		Metadata:    inputMeta,
	})
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Mutating caller metadata must not mutate persisted metadata
	inputMeta["invoice_id"] = "MUTATED"
	meta.Metadata["invoice_id"] = "MUTATED-RETURN"

	if meta.SizeBytes != int64(len(payload)) || len(meta.SHA256Hex) != 64 || !meta.CreatedAt.Equal(fixed) {
		t.Fatalf("unexpected ObjectMeta: %+v", meta)
	}

	// Verify 0600 permissions on both object file and metadata sidecar, and zero leftover temp files
	objPath, metaPath := store.objectPaths("invoices/2026/INV-2026-099.pdf")
	for _, p := range []string{objPath, metaPath} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %q failed: %v", p, err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("expected 0600 permissions on %q, got %04o", p, perm)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(objPath))
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "INV-2026-099.pdf" {
		t.Fatalf("expected zero leftover temp files, got %v", entries)
	}

	headMeta, err := store.Head(ctx, "invoices/2026/INV-2026-099.pdf")
	if err != nil {
		t.Fatalf("Head failed: %v", err)
	}
	if headMeta.Metadata["invoice_id"] != "INV-2026-099" {
		t.Fatalf("expected persisted metadata invoice_id=INV-2026-099, got %q", headMeta.Metadata["invoice_id"])
	}

	rc, getMeta, err := store.Get(ctx, "invoices/2026/INV-2026-099.pdf")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	data, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if !bytes.Equal(data, payload) || getMeta.SHA256Hex != headMeta.SHA256Hex {
		t.Fatalf("payload or digest mismatch")
	}

	// Atomic overwrite with nil body
	emptyMeta, err := store.Put(ctx, PutInput{
		Key:         "invoices/2026/INV-2026-099.pdf",
		ContentType: "application/pdf",
		Body:        nil,
	})
	if err != nil || emptyMeta.SizeBytes != 0 {
		t.Fatalf("expected atomic overwrite to 0 bytes, got meta=%+v err=%v", emptyMeta, err)
	}

	// Delete removes both payload and sidecar
	if err := store.Delete(ctx, "invoices/2026/INV-2026-099.pdf"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if err := store.Delete(ctx, "invoices/2026/INV-2026-099.pdf"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("expected ErrObjectNotFound on second Delete, got %v", err)
	}
	if _, err := store.Head(ctx, "invoices/2026/INV-2026-099.pdf"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("expected ErrObjectNotFound on Head, got %v", err)
	}
	if _, _, err := store.Get(ctx, "invoices/2026/INV-2026-099.pdf"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("expected ErrObjectNotFound on Get, got %v", err)
	}
}

func TestLocalStore_PresignVerifyAndConcurrency(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2026, 4, 15, 14, 0, 0, 0, time.UTC)
	store, err := NewLocalStore(
		t.TempDir(),
		WithLocalBaseURL("https://local-objects.abeta.dev"),
		WithLocalSigningSecret([]byte("local-hmac-key")),
		WithLocalClock(func() time.Time { return baseTime }),
	)
	if err != nil {
		t.Fatalf("NewLocalStore failed: %v", err)
	}

	ctx := context.Background()
	getURL, err := store.PresignGet(ctx, "reports/q1.csv", 10*time.Minute)
	if err != nil {
		t.Fatalf("PresignGet failed: %v", err)
	}
	if err := store.VerifyPresignedURL(getURL.URL, http.MethodGet, baseTime.Add(2*time.Minute)); err != nil {
		t.Fatalf("VerifyPresignedURL GET failed: %v", err)
	}

	putURL, err := store.PresignPut(ctx, "reports/q1.csv", "text/csv", 10*time.Minute)
	if err != nil {
		t.Fatalf("PresignPut failed: %v", err)
	}
	if err := store.VerifyPresignedURL(putURL.URL, http.MethodPut, baseTime.Add(2*time.Minute)); err != nil {
		t.Fatalf("VerifyPresignedURL PUT failed: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := "concurrent/doc.pdf"
			_, _ = store.Put(ctx, PutInput{
				Key:         key,
				ContentType: "application/pdf",
				Body:        strings.NewReader("%PDF-1.4 ok"),
			})
			_, _ = store.Head(ctx, key)
			if rc, _, err := store.Get(ctx, key); err == nil {
				_, _ = io.ReadAll(rc)
				_ = rc.Close()
			}
		}()
	}
	wg.Wait()
}

func TestLocalStore_ErrorPathsAndRollback(t *testing.T) {
	t.Parallel()

	store, err := NewLocalStore(t.TempDir(), WithLocalMaxObjectBytes(8))
	if err != nil {
		t.Fatalf("NewLocalStore failed: %v", err)
	}

	ctx := context.Background()
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	// Context cancellation & invalid key across all CRUD methods
	if _, err := store.Put(canceledCtx, PutInput{Key: "a/b.txt", ContentType: "text/plain"}); !errors.Is(err, context.Canceled) {
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

	if _, err := store.Put(ctx, PutInput{Key: "../escape", ContentType: "text/plain"}); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("expected ErrInvalidKey on Put, got %v", err)
	}
	if _, _, err := store.Get(ctx, "../escape"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("expected ErrInvalidKey on Get, got %v", err)
	}
	if _, err := store.Head(ctx, "../escape"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("expected ErrInvalidKey on Head, got %v", err)
	}
	if err := store.Delete(ctx, "../escape"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("expected ErrInvalidKey on Delete, got %v", err)
	}

	// Empty content type, read failure, size limit exceeded (and zero orphaned temp files!)
	if _, err := store.Put(ctx, PutInput{Key: "a/b.txt", ContentType: " "}); !errors.Is(err, ErrEmptyContentType) {
		t.Fatalf("expected ErrEmptyContentType, got %v", err)
	}
	if _, err := store.Put(ctx, PutInput{Key: "a/b.txt", ContentType: "text/plain", Body: localFailReader{}}); err == nil {
		t.Fatalf("expected read error")
	}
	if _, err := store.Put(ctx, PutInput{Key: "a/b.txt", ContentType: "text/plain", Body: strings.NewReader("0123456789")}); !errors.Is(err, ErrObjectTooLarge) {
		t.Fatalf("expected ErrObjectTooLarge, got %v", err)
	}

	// Put valid object, then test missing payload on Get (meta exists, object deleted) and non-ErrNotExist errors
	if _, err := store.Put(ctx, PutInput{Key: "orphan/item.txt", ContentType: "text/plain", Body: strings.NewReader("ok")}); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	objPath, metaPath := store.objectPaths("orphan/item.txt")
	if err := os.Remove(objPath); err != nil {
		t.Fatalf("remove obj failed: %v", err)
	}
	if _, _, err := store.Get(ctx, "orphan/item.txt"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("expected ErrObjectNotFound when payload missing, got %v", err)
	}

	// Corrupt JSON metadata
	if err := os.WriteFile(metaPath, []byte("{bad-json"), 0o600); err != nil {
		t.Fatalf("write corrupt meta failed: %v", err)
	}
	if _, err := store.Head(ctx, "orphan/item.txt"); err == nil {
		t.Fatalf("expected JSON unmarshal error on corrupt metadata")
	}

	// Simulated FS errors for every atomic write & read/delete branch
	simErr := errors.New("simulated disk failure")
	baseFS := defaultLocalFSOps()

	store.fs = baseFS
	store.fs.readFile = func(string) ([]byte, error) { return nil, simErr }
	if _, err := store.Head(ctx, "orphan/item.txt"); !errors.Is(err, simErr) {
		t.Fatalf("expected simErr on readFile, got %v", err)
	}

	store.fs = baseFS
	// Put fresh item so meta exists, then fail os.Open with non-ErrNotExist error
	if _, err := store.Put(ctx, PutInput{Key: "x/y.txt", ContentType: "text/plain", Body: strings.NewReader("ok")}); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	store.fs.open = func(string) (*os.File, error) { return nil, simErr }
	if _, _, err := store.Get(ctx, "x/y.txt"); !errors.Is(err, simErr) {
		t.Fatalf("expected simErr on open, got %v", err)
	}

	// Delete error on object remove and meta remove
	store.fs = baseFS
	store.fs.remove = func(p string) error {
		if strings.Contains(p, "objects") {
			return simErr
		}
		return os.ErrNotExist
	}
	if err := store.Delete(ctx, "x/y.txt"); !errors.Is(err, simErr) {
		t.Fatalf("expected simErr on obj remove, got %v", err)
	}

	store.fs = baseFS
	store.fs.remove = func(p string) error {
		if strings.Contains(p, ".meta") {
			return simErr
		}
		return os.ErrNotExist
	}
	if err := store.Delete(ctx, "x/y.txt"); !errors.Is(err, simErr) {
		t.Fatalf("expected simErr on meta remove, got %v", err)
	}

	// writeAtomicStream failure branches: mkdirAll, createTemp, chmodFile, syncFile, closeFile, rename
	store.fs = baseFS
	store.fs.mkdirAll = func(string, os.FileMode) error { return simErr }
	if _, err := store.Put(ctx, PutInput{Key: "err/1.txt", ContentType: "text/plain", Body: strings.NewReader("ok")}); !errors.Is(err, simErr) {
		t.Fatalf("expected simErr on mkdirAll, got %v", err)
	}

	store.fs = baseFS
	store.fs.createTemp = func(string, string) (*os.File, error) { return nil, simErr }
	if _, err := store.Put(ctx, PutInput{Key: "err/2.txt", ContentType: "text/plain", Body: strings.NewReader("ok")}); !errors.Is(err, simErr) {
		t.Fatalf("expected simErr on createTemp, got %v", err)
	}

	store.fs = baseFS
	store.fs.chmodFile = func(*os.File, os.FileMode) error { return simErr }
	if _, err := store.Put(ctx, PutInput{Key: "err/3.txt", ContentType: "text/plain", Body: strings.NewReader("ok")}); !errors.Is(err, simErr) {
		t.Fatalf("expected simErr on chmodFile, got %v", err)
	}

	store.fs = baseFS
	store.fs.syncFile = func(*os.File) error { return simErr }
	if _, err := store.Put(ctx, PutInput{Key: "err/4.txt", ContentType: "text/plain", Body: strings.NewReader("ok")}); !errors.Is(err, simErr) {
		t.Fatalf("expected simErr on syncFile, got %v", err)
	}

	store.fs = baseFS
	store.fs.closeFile = func(f *os.File) error {
		_ = f.Close()
		return simErr
	}
	if _, err := store.Put(ctx, PutInput{Key: "err/5.txt", ContentType: "text/plain", Body: strings.NewReader("ok")}); !errors.Is(err, simErr) {
		t.Fatalf("expected simErr on closeFile, got %v", err)
	}

	store.fs = baseFS
	store.fs.rename = func(string, string) error { return simErr }
	if _, err := store.Put(ctx, PutInput{Key: "err/6.txt", ContentType: "text/plain", Body: strings.NewReader("ok")}); !errors.Is(err, simErr) {
		t.Fatalf("expected simErr on rename, got %v", err)
	}

	// json.Marshal failure (year 10000) and metadata write failure (second rename fails)
	store.fs = baseFS
	store.now = func() time.Time { return time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) }
	if _, err := store.Put(ctx, PutInput{Key: "err/7.txt", ContentType: "text/plain", Body: strings.NewReader("ok")}); err == nil {
		t.Fatalf("expected json.Marshal error on year 10000")
	}

	store.now = time.Now
	renameCalls := 0
	store.fs.rename = func(oldPath, newPath string) error {
		renameCalls++
		if renameCalls == 2 {
			return simErr
		}
		return os.Rename(oldPath, newPath)
	}
	if _, err := store.Put(ctx, PutInput{Key: "err/8.txt", ContentType: "text/plain", Body: strings.NewReader("ok")}); !errors.Is(err, simErr) {
		t.Fatalf("expected simErr on meta sidecar rename, got %v", err)
	}
}
