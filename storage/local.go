package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultLocalFilePerm os.FileMode = 0o600
	defaultLocalDirPerm  os.FileMode = 0o700
)

var (
	// ErrInvalidRootDir is returned when NewLocalStore is called with an empty root directory.
	ErrInvalidRootDir = errors.New("storage: root directory must not be empty")
	// ErrInvalidFileMode is returned when a zero file or directory permission mode is supplied.
	ErrInvalidFileMode = errors.New("storage: permission mode must be non-zero")
)

type localFSOps struct {
	abs        func(string) (string, error)
	mkdirAll   func(string, os.FileMode) error
	createTemp func(string, string) (*os.File, error)
	chmodFile  func(*os.File, os.FileMode) error
	syncFile   func(*os.File) error
	closeFile  func(*os.File) error
	rename     func(string, string) error
	remove     func(string) error
	open       func(string) (*os.File, error)
	readFile   func(string) ([]byte, error)
}

func defaultLocalFSOps() localFSOps {
	return localFSOps{
		abs:        filepath.Abs,
		mkdirAll:   os.MkdirAll,
		createTemp: os.CreateTemp,
		chmodFile:  func(f *os.File, perm os.FileMode) error { return f.Chmod(perm) },
		syncFile:   func(f *os.File) error { return f.Sync() },
		closeFile:  func(f *os.File) error { return f.Close() },
		rename:     os.Rename,
		remove:     os.Remove,
		open:       os.Open,
		readFile:   os.ReadFile,
	}
}

// LocalOption configures a LocalStore instance.
type LocalOption func(*LocalStore) error

// WithLocalMaxObjectBytes sets the maximum allowed object size in bytes for LocalStore.
func WithLocalMaxObjectBytes(limit int64) LocalOption {
	return func(s *LocalStore) error {
		if limit <= 0 {
			return ErrObjectTooLarge
		}
		s.maxObjectBytes = limit
		return nil
	}
}

// WithLocalBaseURL configures the base HTTP/HTTPS endpoint used when constructing presigned URLs.
func WithLocalBaseURL(baseURL string) LocalOption {
	return func(s *LocalStore) error {
		validated, err := validateBaseURL(baseURL)
		if err != nil {
			return err
		}
		s.baseURL = validated
		return nil
	}
}

// WithLocalSigningSecret configures the HMAC-SHA256 secret key used to sign and verify presigned URLs.
func WithLocalSigningSecret(secret []byte) LocalOption {
	return func(s *LocalStore) error {
		if len(secret) == 0 {
			return ErrEmptySigningSecret
		}
		s.signingSecret = append([]byte(nil), secret...)
		return nil
	}
}

// WithLocalClock configures a deterministic clock source for timestamps and presigned URL expiration checks.
func WithLocalClock(nowFunc func() time.Time) LocalOption {
	return func(s *LocalStore) error {
		if nowFunc != nil {
			s.now = nowFunc
		}
		return nil
	}
}

// WithFilePerm sets the file permission mode applied to stored objects and metadata sidecars (default: 0600).
func WithFilePerm(perm os.FileMode) LocalOption {
	return func(s *LocalStore) error {
		if perm == 0 {
			return ErrInvalidFileMode
		}
		s.filePerm = perm
		return nil
	}
}

// WithDirPerm sets the directory permission mode applied when creating nested directories (default: 0700).
func WithDirPerm(perm os.FileMode) LocalOption {
	return func(s *LocalStore) error {
		if perm == 0 {
			return ErrInvalidFileMode
		}
		s.dirPerm = perm
		return nil
	}
}

// LocalStore is a thread-safe, local atomic filesystem implementation of Store.
// Every Put streams into a sibling temporary file with 0600 permissions, calls fsync,
// and atomically renames both the object payload and its JSON metadata sidecar into place.
type LocalStore struct {
	mu             sync.RWMutex
	rootDir        string
	objectsDir     string
	metaDir        string
	maxObjectBytes int64
	baseURL        string
	signingSecret  []byte
	now            func() time.Time
	filePerm       os.FileMode
	dirPerm        os.FileMode
	fs             localFSOps
}

// NewLocalStore initializes a LocalStore rooted at rootDir with strict 0700 directory
// and 0600 file permissions by default.
func NewLocalStore(rootDir string, opts ...LocalOption) (*LocalStore, error) {
	trimmed := strings.TrimSpace(rootDir)
	if trimmed == "" {
		return nil, ErrInvalidRootDir
	}

	store := &LocalStore{
		maxObjectBytes: defaultMaxObjectBytes,
		baseURL:        "https://storage.local",
		signingSecret:  []byte("go-app-kit-default-storage-secret"),
		now:            time.Now,
		filePerm:       defaultLocalFilePerm,
		dirPerm:        defaultLocalDirPerm,
		fs:             defaultLocalFSOps(),
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(store); err != nil {
			return nil, err
		}
	}

	absRoot, err := store.fs.abs(trimmed)
	if err != nil {
		return nil, err
	}
	store.rootDir = absRoot
	store.objectsDir = filepath.Join(absRoot, "objects")
	store.metaDir = filepath.Join(absRoot, ".meta")

	if err := store.fs.mkdirAll(store.objectsDir, store.dirPerm); err != nil {
		return nil, err
	}
	if err := store.fs.mkdirAll(store.metaDir, store.dirPerm); err != nil {
		return nil, err
	}
	return store, nil
}

// RootDir returns the absolute root directory backing this LocalStore.
func (s *LocalStore) RootDir() string {
	return s.rootDir
}

// Put streams the payload into a sibling temp file with owner-only permissions (0600 by default),
// computes the SHA-256 hex digest while enforcing maxObjectBytes, fsyncs, and atomically renames
// both the object file and its metadata sidecar into place.
func (s *LocalStore) Put(ctx context.Context, in PutInput) (ObjectMeta, error) {
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

	objPath, metaPath := s.objectPaths(in.Key)

	s.mu.Lock()
	defer s.mu.Unlock()

	sizeBytes, digestHex, err := s.writeAtomicStream(objPath, ".obj-*.tmp", s.maxObjectBytes, in.Body)
	if err != nil {
		return ObjectMeta{}, err
	}

	meta := ObjectMeta{
		Key:         in.Key,
		ContentType: contentType,
		SizeBytes:   sizeBytes,
		SHA256Hex:   digestHex,
		CreatedAt:   s.now().UTC(),
		Metadata:    cloneMetadata(in.Metadata),
	}

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return ObjectMeta{}, err
	}
	if _, _, err := s.writeAtomicStream(metaPath, ".meta-*.tmp", int64(len(metaJSON)), bytes.NewReader(metaJSON)); err != nil {
		return ObjectMeta{}, err
	}

	return cloneObjectMeta(meta), nil
}

// Get opens the stored object file and returns an io.ReadCloser alongside its cloned metadata.
func (s *LocalStore) Get(ctx context.Context, key string) (io.ReadCloser, ObjectMeta, error) {
	if err := ctx.Err(); err != nil {
		return nil, ObjectMeta{}, err
	}
	if err := ValidateKey(key); err != nil {
		return nil, ObjectMeta{}, err
	}

	objPath, metaPath := s.objectPaths(key)

	s.mu.RLock()
	defer s.mu.RUnlock()

	meta, err := s.readMetaFile(metaPath)
	if err != nil {
		return nil, ObjectMeta{}, err
	}

	f, err := s.fs.open(objPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ObjectMeta{}, ErrObjectNotFound
		}
		return nil, ObjectMeta{}, err
	}
	return f, cloneObjectMeta(meta), nil
}

// Head returns the stored object's cloned metadata from its sidecar file without reading object bytes.
func (s *LocalStore) Head(ctx context.Context, key string) (ObjectMeta, error) {
	if err := ctx.Err(); err != nil {
		return ObjectMeta{}, err
	}
	if err := ValidateKey(key); err != nil {
		return ObjectMeta{}, err
	}

	_, metaPath := s.objectPaths(key)

	s.mu.RLock()
	defer s.mu.RUnlock()

	meta, err := s.readMetaFile(metaPath)
	if err != nil {
		return ObjectMeta{}, err
	}
	return cloneObjectMeta(meta), nil
}

// Delete removes both the object payload file and its metadata sidecar from disk.
func (s *LocalStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateKey(key); err != nil {
		return err
	}

	objPath, metaPath := s.objectPaths(key)

	s.mu.Lock()
	defer s.mu.Unlock()

	errObj := s.fs.remove(objPath)
	errMeta := s.fs.remove(metaPath)
	if errors.Is(errObj, os.ErrNotExist) && errors.Is(errMeta, os.ErrNotExist) {
		return ErrObjectNotFound
	}
	if errObj != nil && !errors.Is(errObj, os.ErrNotExist) {
		return errObj
	}
	if errMeta != nil && !errors.Is(errMeta, os.ErrNotExist) {
		return errMeta
	}
	return nil
}

// PresignGet generates a deterministic HMAC-SHA256 presigned GET URL for downloading an object.
func (s *LocalStore) PresignGet(ctx context.Context, key string, ttl time.Duration) (PresignedURL, error) {
	return presignGet(ctx, s.baseURL, s.signingSecret, s.now, key, ttl)
}

// PresignPut generates a deterministic HMAC-SHA256 presigned PUT URL bound to key, contentType, and expiration.
func (s *LocalStore) PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (PresignedURL, error) {
	return presignPut(ctx, s.baseURL, s.signingSecret, s.now, key, contentType, ttl)
}

// VerifyPresignedURL validates the base URL origin, object key, expiration timestamp,
// and constant-time HMAC-SHA256 signature of a presigned URL.
func (s *LocalStore) VerifyPresignedURL(rawURL, method string, now time.Time) error {
	return verifyPresignedURL(s.baseURL, s.signingSecret, rawURL, method, now)
}

func (s *LocalStore) objectPaths(key string) (objPath, metaPath string) {
	relPath := filepath.FromSlash(key)
	objPath = filepath.Join(s.objectsDir, relPath)
	metaPath = filepath.Join(s.metaDir, relPath+".json")
	return objPath, metaPath
}

func (s *LocalStore) readMetaFile(metaPath string) (ObjectMeta, error) {
	raw, err := s.fs.readFile(metaPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ObjectMeta{}, ErrObjectNotFound
		}
		return ObjectMeta{}, err
	}
	var meta ObjectMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return ObjectMeta{}, err
	}
	return meta, nil
}

func (s *LocalStore) writeAtomicStream(dstPath, tmpPattern string, maxBytes int64, body io.Reader) (sizeBytes int64, digestHex string, err error) {
	parentDir := filepath.Dir(dstPath)
	if err := s.fs.mkdirAll(parentDir, s.dirPerm); err != nil {
		return 0, "", err
	}

	tmp, err := s.fs.createTemp(parentDir, tmpPattern)
	if err != nil {
		return 0, "", err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = s.fs.closeFile(tmp)
			_ = s.fs.remove(tmpPath)
		}
	}()

	if err := s.fs.chmodFile(tmp, s.filePerm); err != nil {
		return 0, "", err
	}

	hasher := sha256.New()
	if body != nil {
		limited := io.LimitReader(body, maxBytes+1)
		n, copyErr := io.Copy(io.MultiWriter(tmp, hasher), limited)
		if copyErr != nil {
			return 0, "", copyErr
		}
		if n > maxBytes {
			return 0, "", ErrObjectTooLarge
		}
		sizeBytes = n
	}

	if err := s.fs.syncFile(tmp); err != nil {
		return 0, "", err
	}
	if err := s.fs.closeFile(tmp); err != nil {
		return 0, "", err
	}
	if err := s.fs.rename(tmpPath, dstPath); err != nil {
		return 0, "", err
	}
	cleanup = false
	return sizeBytes, hex.EncodeToString(hasher.Sum(nil)), nil
}
