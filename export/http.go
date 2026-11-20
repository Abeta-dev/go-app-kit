// SPDX-License-Identifier: MIT

package export

import (
	"compress/gzip"
	"fmt"
	"io"
	"math"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
)

var filenameReplacer = strings.NewReplacer(
	"\r", "",
	"\n", "",
	"\"", "",
	"\\", "",
)

var brotliWriterPool = sync.Pool{
	New: func() any {
		return brotli.NewWriterLevel(io.Discard, brotli.DefaultCompression)
	},
}

var gzipWriterPool = sync.Pool{
	New: func() any {
		return gzip.NewWriter(io.Discard)
	},
}

type pooledCompressor interface {
	io.WriteCloser
	Reset(w io.Writer)
}

func sanitizeFilename(filename string) string {
	cleaned := filenameReplacer.Replace(strings.TrimSpace(filename))
	if cleaned == "" {
		return "export.csv"
	}
	base := filepath.Base(cleaned)
	if base == "" || base == "." || base == "/" {
		return "export.csv"
	}
	return base
}

func parseQuality(params []string) (float64, bool) {
	q := 1.0
	for _, raw := range params {
		param := strings.TrimSpace(raw)
		if len(param) >= 2 && (param[:2] == "q=" || param[:2] == "Q=") {
			val, err := strconv.ParseFloat(param[2:], 64)
			if err != nil || math.IsNaN(val) || val < 0 || val > 1 {
				return 0, false
			}
			q = val
		}
	}
	return q, true
}

// NegotiateEncoding parses an RFC 9110 Accept-Encoding header value with q= weights
// and returns "br", "gzip", or "" (identity/uncompressed). When both "br" and "gzip"
// have equal positive quality weights, "br" is preferred.
func NegotiateEncoding(acceptEncoding string) string {
	if strings.TrimSpace(acceptEncoding) == "" {
		return ""
	}

	brQ, gzipQ, wildcardQ := -1.0, -1.0, -1.0

	for _, part := range strings.Split(acceptEncoding, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		subparts := strings.Split(part, ";")
		coding := strings.ToLower(strings.TrimSpace(subparts[0]))
		q, ok := parseQuality(subparts[1:])
		if !ok {
			continue
		}
		switch coding {
		case "br":
			brQ = q
		case "gzip", "x-gzip":
			gzipQ = q
		case "*":
			wildcardQ = q
		}
	}

	if brQ < 0 {
		brQ = wildcardQ
	}
	if gzipQ < 0 {
		gzipQ = wildcardQ
	}

	if brQ <= 0 && gzipQ <= 0 {
		return ""
	}
	if brQ >= gzipQ {
		return "br"
	}
	return "gzip"
}

func setCSVDownloadHeaders(w http.ResponseWriter, filename string) {
	safeFilename := sanitizeFilename(filename)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", safeFilename))
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func addVaryAcceptEncoding(h http.Header) {
	for _, v := range h.Values("Vary") {
		for _, token := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "Accept-Encoding") {
				return
			}
		}
	}
	h.Add("Vary", "Accept-Encoding")
}

func streamCSV[T any](w io.Writer, columns []Column[T], items []T, opts ...CSVOption) error {
	allOpts := make([]CSVOption, 0, 2+len(opts))
	allOpts = append(allOpts, WithBOM(true), WithCRLF(true))
	allOpts = append(allOpts, opts...)

	streamer := NewCSVStreamer[T](w, columns, allOpts...)

	if err := streamer.WriteHeader(); err != nil {
		return fmt.Errorf("failed to write CSV headers: %w", err)
	}

	if err := streamer.WriteRows(items); err != nil {
		return fmt.Errorf("failed to stream CSV rows: %w", err)
	}

	return streamer.Flush()
}

func streamWithPooledCompressor[T any](w http.ResponseWriter, pool *sync.Pool, encoding string, columns []Column[T], items []T, opts ...CSVOption) error {
	w.Header().Set("Content-Encoding", encoding)
	cw := pool.Get().(pooledCompressor)
	cw.Reset(w)
	defer func() {
		cw.Reset(io.Discard)
		pool.Put(cw)
	}()
	if err := streamCSV(cw, columns, items, opts...); err != nil {
		return err
	}
	if err := cw.Close(); err != nil {
		return fmt.Errorf("failed to close %s writer: %w", encoding, err)
	}
	return nil
}

// StreamHTTP writes records as an attachment CSV using the provided http.ResponseWriter
// with UTF-8 Byte Order Mark (BOM) and Windows CRLF line endings for maximum spreadsheet compatibility.
// Compatible with standard http.ResponseWriter as well as Gin (c.Writer), Echo, Chi, and Fiber adapters.
func StreamHTTP[T any](w http.ResponseWriter, filename string, columns []Column[T], items []T, opts ...CSVOption) error {
	setCSVDownloadHeaders(w, filename)
	return streamCSV(w, columns, items, opts...)
}

// StreamHTTPCompressed negotiates RFC 9110 Accept-Encoding ("br" or "gzip") from r
// and streams records as a compressed (or uncompressed fallback) CSV attachment using
// pooled Brotli and Gzip writers. Appends Vary: Accept-Encoding alongside standard CSV download headers.
func StreamHTTPCompressed[T any](w http.ResponseWriter, r *http.Request, filename string, columns []Column[T], items []T, opts ...CSVOption) error {
	setCSVDownloadHeaders(w, filename)
	addVaryAcceptEncoding(w.Header())

	var acceptEncoding string
	if r != nil {
		acceptEncoding = r.Header.Get("Accept-Encoding")
	}

	var err error
	switch NegotiateEncoding(acceptEncoding) {
	case "br":
		err = streamWithPooledCompressor(w, &brotliWriterPool, "br", columns, items, opts...)
	case "gzip":
		err = streamWithPooledCompressor(w, &gzipWriterPool, "gzip", columns, items, opts...)
	default:
		err = streamCSV(w, columns, items, opts...)
	}
	if err != nil {
		return err
	}

	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}
