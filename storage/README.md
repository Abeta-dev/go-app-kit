# `storage`

Zero-leak object storage abstraction and deterministic HMAC-SHA256 presigned URL engine for archiving generated `pdf` GST tax invoices, compliance audit exports, and streaming `export` CSV artifacts.

---

## When to Use

- **Durable Local Atomic Filesystem Archiving (`NewLocalStore`)**: Persisting sensitive local artifacts (desktop BFF state, offline PDFs, audit exports) using sibling temporary files, owner-only `0600` file / `0700` directory permissions, `fsync`, and atomic `os.Rename` with zero partial writes on failure.
- **Archiving Generated PDF Tax Invoices & Receipts**: Storing compiled `pdf.GenerateFromTemplate` buffers with immutable SHA-256 content digests (`SHA256Hex`), byte-size enforcement (`WithMaxObjectBytes` / `WithLocalMaxObjectBytes`), and defensive metadata cloning.
- **Time-Bounded Presigned Download & Upload Links**: Generating HMAC-SHA256-authenticated `GET` and `PUT` URLs (`PresignGet`, `PresignPut`, `VerifyPresignedURL`) with strict TTL and MIME `Content-Type` binding.
- **Deterministic In-Memory Integration Testing**: Testing invoice archiving and presigned download workflows without requiring external S3/MinIO containers or local filesystem temp files.

---

## Key Capabilities & Security Guarantees

1. **Local Atomic Write Protocol (`NewLocalStore`)**: Streams payloads to a sibling `.tmp` file with `0600` permissions (`WithFilePerm`), computes SHA-256 on the fly while enforcing `WithLocalMaxObjectBytes`, calls `fsync`, and atomically renames both the object file and its `.meta/*.json` sidecar into place.
2. **Path-Traversal & Key Hygiene (`ValidateKey`)**: Rejects empty keys, leading or trailing slashes, empty path segments (`//`), `.` or `..` traversal segments, Windows backslashes, and null bytes (`ErrInvalidKey`).
3. **Bounded Memory & Size Guard (`WithMaxObjectBytes`)**: Reads upload streams via `io.LimitReader(body, maxObjectBytes+1)` so oversized uploads fail fast with `ErrObjectTooLarge` without exhausting heap memory.
4. **Cryptographic Content Addressing (`ObjectMeta.SHA256Hex`)**: Automatically computes the hex-encoded SHA-256 digest of every stored object and defensively clones `Metadata` maps on `Put`, `Get`, and `Head`.
5. **Constant-Time HMAC-SHA256 Presigned URLs (`PresignGet`, `PresignPut`, `VerifyPresignedURL`)**: Binds HTTP method, object key, content type, and Unix expiration timestamp into a deterministic HMAC-SHA256 signature verified with `hmac.Equal`.

---

## Usage Example

```go
package main

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/abeta-dev/go-app-kit/storage"
)

func main() {
	ctx := context.Background()

	store, err := storage.NewMemoryStore(
		storage.WithBaseURL("https://files.abeta.dev"),
		storage.WithSigningSecret([]byte("replace-with-32-byte-hmac-key")),
		storage.WithMaxObjectBytes(10*1024*1024), // 10 MiB limit
	)
	if err != nil {
		panic(err)
	}

	meta, err := store.Put(ctx, storage.PutInput{
		Key:         "invoices/2026/INV-2026-001.pdf",
		ContentType: "application/pdf",
		Body:        bytes.NewReader([]byte("%PDF-1.7 GST Tax Invoice")),
		Metadata:    map[string]string{"gstin": "27AAPFU0939F1ZV"},
	})
	if err != nil {
		panic(err)
	}

	presigned, err := store.PresignGet(ctx, meta.Key, 15*time.Minute)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Stored %s (%d bytes, sha256=%s)\nDownload URL: %s\n",
		meta.Key, meta.SizeBytes, meta.SHA256Hex, presigned.URL)
}
```
