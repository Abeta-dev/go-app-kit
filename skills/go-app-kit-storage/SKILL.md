---
name: go-app-kit-storage
description: "Store generated PDF tax invoices and streaming CSV export artifacts with SHA-256 content digests, key traversal validation, size limits, and deterministic HMAC-SHA256 presigned GET/PUT URLs using github.com/abeta-dev/go-app-kit/storage. Use when building artifact archiving, document download links, or zero-leak object storage workflows."
---

# `go-app-kit/storage` — Object Storage & Presigned URL Subskill

Use this skill when archiving compiled `pdf` documents, storing `export` CSV reports, or generating HMAC-SHA256 presigned download/upload links with `github.com/abeta-dev/go-app-kit/storage`.

## 1. Security & Memory Guarantees

- **Strict Key Traversal Guard (`ValidateKey`)**: Rejects empty keys, leading or trailing slashes, empty path segments (`//`), `.` or `..` path traversal segments, Windows backslashes, and null bytes (`ErrInvalidKey`).
- **Bounded Memory Stream Reading (`WithMaxObjectBytes`)**: Enforces size limits via `io.LimitReader(body, maxObjectBytes+1)` and returns `ErrObjectTooLarge` without unbounded heap allocations.
- **SHA-256 Content Digest & Defensive Cloning**: Computes `ObjectMeta.SHA256Hex` on `Put` and defensively clones `Metadata` maps on `Put`, `Get`, and `Head`.
- **Constant-Time HMAC-SHA256 Presigned URLs (`PresignGet`, `PresignPut`, `VerifyPresignedURL`)**: Signs HTTP method, object key, content type (for `PUT`), and Unix expiration timestamp, verifying signatures with `hmac.Equal`.

---

## 2. Archiving PDF Invoices & Generating Presigned Download URLs

```go
import (
	"bytes"
	"context"
	"net/http"
	"time"

	"github.com/abeta-dev/go-app-kit/storage"
)

func ArchiveAndPresignInvoice(ctx context.Context, pdfBytes []byte) (storage.ObjectMeta, storage.PresignedURL, error) {
	store, err := storage.NewMemoryStore(
		storage.WithBaseURL("https://objects.enterprise.internal"),
		storage.WithSigningSecret([]byte("replace-with-32-byte-signing-secret")),
		storage.WithMaxObjectBytes(16*1024*1024), // 16 MiB
	)
	if err != nil {
		return storage.ObjectMeta{}, storage.PresignedURL{}, err
	}

	meta, err := store.Put(ctx, storage.PutInput{
		Key:         "invoices/2026/INV-2026-001.pdf",
		ContentType: "application/pdf",
		Body:        bytes.NewReader(pdfBytes),
		Metadata:    map[string]string{"invoice_id": "INV-2026-001"},
	})
	if err != nil {
		return storage.ObjectMeta{}, storage.PresignedURL{}, err
	}

	presigned, err := store.PresignGet(ctx, meta.Key, 15*time.Minute)
	if err != nil {
		return storage.ObjectMeta{}, storage.PresignedURL{}, err
	}

	// Verify presigned URL authenticity and TTL
	if err := store.VerifyPresignedURL(presigned.URL, http.MethodGet, time.Now().UTC()); err != nil {
		return storage.ObjectMeta{}, storage.PresignedURL{}, err
	}
	return meta, presigned, nil
}
```
