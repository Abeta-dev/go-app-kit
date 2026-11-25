// Package storage provides a zero-leak object storage abstraction and
// deterministic HMAC-SHA256 presigned URL engine for archiving generated PDF
// tax invoices, compliance documents, and streaming CSV export artifacts.
package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultMaxObjectBytes int64 = 64 * 1024 * 1024 // 64 MiB

var (
	// ErrInvalidKey is returned when an object key is empty, starts or ends with a slash,
	// contains path traversal segments (".."), backslashes, or null bytes.
	ErrInvalidKey = errors.New("storage: invalid object key")
	// ErrObjectNotFound is returned when the requested object key does not exist.
	ErrObjectNotFound = errors.New("storage: object not found")
	// ErrObjectTooLarge is returned when an object payload exceeds the configured maximum byte limit.
	ErrObjectTooLarge = errors.New("storage: object exceeds maximum allowed size")
	// ErrEmptyContentType is returned when an upload or presigned PUT omits a MIME content type.
	ErrEmptyContentType = errors.New("storage: content type must not be empty")
	// ErrInvalidExpiry is returned when a presigned URL TTL is non-positive or the URL has expired.
	ErrInvalidExpiry = errors.New("storage: invalid or expired presigned URL TTL")
	// ErrInvalidBaseURL is returned when the configured base URL is empty or lacks a valid scheme and host.
	ErrInvalidBaseURL = errors.New("storage: invalid base URL")
	// ErrEmptySigningSecret is returned when the configured HMAC signing secret is empty.
	ErrEmptySigningSecret = errors.New("storage: signing secret must not be empty")
)

// ObjectMeta describes the immutable metadata and SHA-256 content digest of a stored object.
type ObjectMeta struct {
	Key         string
	ContentType string
	SizeBytes   int64
	SHA256Hex   string
	CreatedAt   time.Time
	Metadata    map[string]string
}

// PutInput encapsulates the parameters for storing an object.
type PutInput struct {
	Key         string
	ContentType string
	Body        io.Reader
	Metadata    map[string]string
}

// PresignedURL represents a time-bounded, HMAC-SHA256-authenticated object download or upload URL.
type PresignedURL struct {
	URL       string
	Method    string
	ExpiresAt time.Time
	Signature string
}

// Store defines the contract for enterprise object storage and presigned URL generation.
type Store interface {
	Put(ctx context.Context, in PutInput) (ObjectMeta, error)
	Get(ctx context.Context, key string) (io.ReadCloser, ObjectMeta, error)
	Head(ctx context.Context, key string) (ObjectMeta, error)
	Delete(ctx context.Context, key string) error
	PresignGet(ctx context.Context, key string, ttl time.Duration) (PresignedURL, error)
	PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (PresignedURL, error)
}

// ValidateKey enforces strict object key hygiene, rejecting empty keys,
// leading or trailing slashes, empty segments, "." or ".." path traversal segments,
// backslashes, and null bytes.
func ValidateKey(key string) error {
	if key == "" || strings.HasPrefix(key, "/") || strings.HasSuffix(key, "/") {
		return ErrInvalidKey
	}
	if strings.Contains(key, "\\") || strings.ContainsRune(key, '\x00') {
		return ErrInvalidKey
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return ErrInvalidKey
		}
	}
	return nil
}

// Option configures a MemoryStore instance.
type Option func(*MemoryStore) error

// WithMaxObjectBytes sets the maximum allowed object size in bytes.
func WithMaxObjectBytes(limit int64) Option {
	return func(s *MemoryStore) error {
		if limit <= 0 {
			return ErrObjectTooLarge
		}
		s.maxObjectBytes = limit
		return nil
	}
}

// WithBaseURL configures the base HTTP/HTTPS endpoint used when constructing presigned URLs.
func WithBaseURL(baseURL string) Option {
	return func(s *MemoryStore) error {
		validated, err := validateBaseURL(baseURL)
		if err != nil {
			return err
		}
		s.baseURL = validated
		return nil
	}
}

// WithSigningSecret configures the HMAC-SHA256 secret key used to sign and verify presigned URLs.
func WithSigningSecret(secret []byte) Option {
	return func(s *MemoryStore) error {
		if len(secret) == 0 {
			return ErrEmptySigningSecret
		}
		s.signingSecret = append([]byte(nil), secret...)
		return nil
	}
}

// WithClock configures a deterministic clock source for timestamps and presigned URL expiration checks.
func WithClock(nowFunc func() time.Time) Option {
	return func(s *MemoryStore) error {
		if nowFunc != nil {
			s.now = nowFunc
		}
		return nil
	}
}

type storedObject struct {
	meta ObjectMeta
	data []byte
}

// MemoryStore is a thread-safe, zero-leak in-memory implementation of Store with
// SHA-256 content addressing, size limits, and HMAC-SHA256 presigned URL support.
type MemoryStore struct {
	mu             sync.RWMutex
	objects        map[string]storedObject
	maxObjectBytes int64
	baseURL        string
	signingSecret  []byte
	now            func() time.Time
}

// NewMemoryStore initializes a new MemoryStore with the provided functional options.
func NewMemoryStore(opts ...Option) (*MemoryStore, error) {
	store := &MemoryStore{
		objects:        make(map[string]storedObject),
		maxObjectBytes: defaultMaxObjectBytes,
		baseURL:        "https://storage.local",
		signingSecret:  []byte("go-app-kit-default-storage-secret"),
		now:            time.Now,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(store); err != nil {
			return nil, err
		}
	}
	return store, nil
}

// Put validates the key and content type, streams the body up to maxObjectBytes,
// computes the SHA-256 hex digest, and stores an immutable copy in memory.
func (s *MemoryStore) Put(ctx context.Context, in PutInput) (ObjectMeta, error) {
	if err := ctx.Err(); err != nil {
		return ObjectMeta{}, err
	}
	if err := ValidateKey(in.Key); err != nil {
		return ObjectMeta{}, err
	}
	contentType := strings.TrimSpace(in.ContentType)
	if contentType == "" {
		return ObjectMeta{}, ErrEmptyContentType
	}

	var data []byte
	if in.Body != nil {
		limited := io.LimitReader(in.Body, s.maxObjectBytes+1)
		buf, err := io.ReadAll(limited)
		if err != nil {
			return ObjectMeta{}, err
		}
		if int64(len(buf)) > s.maxObjectBytes {
			return ObjectMeta{}, ErrObjectTooLarge
		}
		data = buf
	}

	sum := sha256.Sum256(data)
	meta := ObjectMeta{
		Key:         in.Key,
		ContentType: contentType,
		SizeBytes:   int64(len(data)),
		SHA256Hex:   hex.EncodeToString(sum[:]),
		CreatedAt:   s.now().UTC(),
		Metadata:    cloneMetadata(in.Metadata),
	}

	s.mu.Lock()
	s.objects[in.Key] = storedObject{
		meta: meta,
		data: data,
	}
	s.mu.Unlock()

	return cloneObjectMeta(meta), nil
}

// Get retrieves an object reader and its cloned metadata by key.
func (s *MemoryStore) Get(ctx context.Context, key string) (io.ReadCloser, ObjectMeta, error) {
	if err := ctx.Err(); err != nil {
		return nil, ObjectMeta{}, err
	}
	if err := ValidateKey(key); err != nil {
		return nil, ObjectMeta{}, err
	}

	s.mu.RLock()
	obj, ok := s.objects[key]
	s.mu.RUnlock()
	if !ok {
		return nil, ObjectMeta{}, ErrObjectNotFound
	}

	bufCopy := append([]byte(nil), obj.data...)
	return io.NopCloser(bytes.NewReader(bufCopy)), cloneObjectMeta(obj.meta), nil
}

// Head returns the object's cloned metadata without copying its payload bytes.
func (s *MemoryStore) Head(ctx context.Context, key string) (ObjectMeta, error) {
	if err := ctx.Err(); err != nil {
		return ObjectMeta{}, err
	}
	if err := ValidateKey(key); err != nil {
		return ObjectMeta{}, err
	}

	s.mu.RLock()
	obj, ok := s.objects[key]
	s.mu.RUnlock()
	if !ok {
		return ObjectMeta{}, ErrObjectNotFound
	}

	return cloneObjectMeta(obj.meta), nil
}

// Delete removes an object from the store by key.
func (s *MemoryStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateKey(key); err != nil {
		return err
	}

	s.mu.Lock()
	_, ok := s.objects[key]
	if ok {
		delete(s.objects, key)
	}
	s.mu.Unlock()

	if !ok {
		return ErrObjectNotFound
	}
	return nil
}

// PresignGet generates a deterministic HMAC-SHA256 presigned GET URL for downloading an object.
func (s *MemoryStore) PresignGet(ctx context.Context, key string, ttl time.Duration) (PresignedURL, error) {
	return presignGet(ctx, s.baseURL, s.signingSecret, s.now, key, ttl)
}

// PresignPut generates a deterministic HMAC-SHA256 presigned PUT URL bound to key, contentType, and expiration.
func (s *MemoryStore) PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (PresignedURL, error) {
	return presignPut(ctx, s.baseURL, s.signingSecret, s.now, key, contentType, ttl)
}

// VerifyPresignedURL validates the base URL origin, object key, expiration timestamp,
// and constant-time HMAC-SHA256 signature of a presigned URL.
func (s *MemoryStore) VerifyPresignedURL(rawURL, method string, now time.Time) error {
	return verifyPresignedURL(s.baseURL, s.signingSecret, rawURL, method, now)
}

func presignGet(ctx context.Context, baseURL string, secret []byte, nowFn func() time.Time, key string, ttl time.Duration) (PresignedURL, error) {
	if err := ctx.Err(); err != nil {
		return PresignedURL{}, err
	}
	if err := ValidateKey(key); err != nil {
		return PresignedURL{}, err
	}
	if ttl <= 0 {
		return PresignedURL{}, ErrInvalidExpiry
	}

	expiresAt := nowFn().UTC().Add(ttl)
	expiresUnix := strconv.FormatInt(expiresAt.Unix(), 10)
	sig := computeHMACSignature(secret, http.MethodGet, key, "", expiresUnix)

	q := url.Values{}
	q.Set("expires", expiresUnix)
	q.Set("sig", sig)

	return PresignedURL{
		URL:       fmt.Sprintf("%s/%s?%s", baseURL, key, q.Encode()),
		Method:    http.MethodGet,
		ExpiresAt: expiresAt,
		Signature: sig,
	}, nil
}

func presignPut(ctx context.Context, baseURL string, secret []byte, nowFn func() time.Time, key, contentType string, ttl time.Duration) (PresignedURL, error) {
	if err := ctx.Err(); err != nil {
		return PresignedURL{}, err
	}
	if err := ValidateKey(key); err != nil {
		return PresignedURL{}, err
	}
	trimmedCT := strings.TrimSpace(contentType)
	if trimmedCT == "" {
		return PresignedURL{}, ErrEmptyContentType
	}
	if ttl <= 0 {
		return PresignedURL{}, ErrInvalidExpiry
	}

	expiresAt := nowFn().UTC().Add(ttl)
	expiresUnix := strconv.FormatInt(expiresAt.Unix(), 10)
	sig := computeHMACSignature(secret, http.MethodPut, key, trimmedCT, expiresUnix)

	q := url.Values{}
	q.Set("content_type", trimmedCT)
	q.Set("expires", expiresUnix)
	q.Set("sig", sig)

	return PresignedURL{
		URL:       fmt.Sprintf("%s/%s?%s", baseURL, key, q.Encode()),
		Method:    http.MethodPut,
		ExpiresAt: expiresAt,
		Signature: sig,
	}, nil
}

func verifyPresignedURL(baseURL string, secret []byte, rawURL, method string, now time.Time) error {
	prefix := baseURL + "/"
	if !strings.HasPrefix(rawURL, prefix) {
		return ErrInvalidBaseURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ErrInvalidBaseURL
	}

	rest := strings.TrimPrefix(rawURL, prefix)
	keyPart, _, _ := strings.Cut(rest, "?")
	if err := ValidateKey(keyPart); err != nil {
		return err
	}

	query := parsed.Query()
	expiresStr := query.Get("expires")
	sig := query.Get("sig")
	if expiresStr == "" || sig == "" {
		return ErrInvalidExpiry
	}

	expiresUnix, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil || now.UTC().Unix() > expiresUnix {
		return ErrInvalidExpiry
	}

	contentType := query.Get("content_type")
	if method == http.MethodPut && strings.TrimSpace(contentType) == "" {
		return ErrEmptyContentType
	}

	expectedSig := computeHMACSignature(secret, method, keyPart, contentType, expiresStr)
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return ErrEmptySigningSecret
	}
	return nil
}

func computeHMACSignature(secret []byte, method, key, contentType, expiresUnix string) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = io.WriteString(mac, method+"\n"+key+"\n"+contentType+"\n"+expiresUnix)
	return hex.EncodeToString(mac.Sum(nil))
}

func validateBaseURL(baseURL string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", ErrInvalidBaseURL
	}
	return trimmed, nil
}

func cloneMetadata(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneObjectMeta(meta ObjectMeta) ObjectMeta {
	meta.Metadata = cloneMetadata(meta.Metadata)
	return meta
}
