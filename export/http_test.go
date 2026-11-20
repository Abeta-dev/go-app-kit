// SPDX-License-Identifier: MIT

package export_test

import (
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abeta-dev/go-app-kit/export"
	"github.com/andybalholm/brotli"
)

type sampleRecord struct {
	ID   string
	Name string
	Role string
}

type failingResponseWriter struct {
	header        http.Header
	writesAllowed int
}

func (f *failingResponseWriter) Header() http.Header {
	if f.header == nil {
		f.header = make(http.Header)
	}
	return f.header
}

func (f *failingResponseWriter) Write(p []byte) (int, error) {
	if f.writesAllowed <= 0 {
		return 0, errors.New("simulated response write failure")
	}
	f.writesAllowed--
	return len(p), nil
}

func (f *failingResponseWriter) WriteHeader(statusCode int) {}

func TestStreamHTTP(t *testing.T) {
	rec := httptest.NewRecorder()
	cols := []export.Column[sampleRecord]{
		{Header: "User ID", Extractor: func(r sampleRecord) string { return r.ID }},
		{Header: "Full Name", Extractor: func(r sampleRecord) string { return r.Name }},
		{Header: "System Role", Extractor: func(r sampleRecord) string { return r.Role }},
	}
	items := []sampleRecord{
		{ID: "usr-1", Name: "Alice", Role: "Admin"},
		{ID: "usr-2", Name: "Bob", Role: "Member"},
	}

	err := export.StreamHTTP(rec, "users.csv", cols, items)
	if err != nil {
		t.Fatalf("unexpected error streaming CSV: %v", err)
	}

	if rec.Header().Get("Content-Type") != "text/csv; charset=utf-8" {
		t.Fatalf("unexpected content type: %s", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("expected X-Content-Type-Options: nosniff, got %s", rec.Header().Get("X-Content-Type-Options"))
	}
	if rec.Header().Get("Content-Disposition") != `attachment; filename="users.csv"` {
		t.Fatalf("unexpected content disposition: %s", rec.Header().Get("Content-Disposition"))
	}

	body := rec.Body.String()
	// Check BOM
	if !strings.HasPrefix(body, "\xef\xbb\xbf") {
		t.Fatal("expected UTF-8 BOM prefix")
	}
	if !strings.Contains(body, "User ID,Full Name,System Role\r\n") {
		t.Fatalf("unexpected header row in CSV body: %s", body)
	}
	if !strings.Contains(body, "usr-1,Alice,Admin\r\n") || !strings.Contains(body, "usr-2,Bob,Member\r\n") {
		t.Fatalf("unexpected data rows in CSV body: %s", body)
	}
}

func TestStreamHTTP_FilenameSanitizationAndErrors(t *testing.T) {
	cols := []export.Column[sampleRecord]{
		{Header: "ID", Extractor: func(r sampleRecord) string { return r.ID }},
	}

	cases := []struct {
		input    string
		expected string
	}{
		{"../../etc/passwd.csv", `attachment; filename="passwd.csv"`},
		{"invoices\r\nX-Injected: true.csv", `attachment; filename="invoicesX-Injected: true.csv"`},
		{`bad"quote\slash.csv`, `attachment; filename="badquoteslash.csv"`},
		{"", `attachment; filename="export.csv"`},
		{"   ", `attachment; filename="export.csv"`},
		{".", `attachment; filename="export.csv"`},
		{"/", `attachment; filename="export.csv"`},
	}

	for _, tc := range cases {
		rec := httptest.NewRecorder()
		if err := export.StreamHTTP(rec, tc.input, cols, nil); err != nil {
			t.Fatalf("StreamHTTP(%q) failed: %v", tc.input, err)
		}
		if got := rec.Header().Get("Content-Disposition"); got != tc.expected {
			t.Errorf("StreamHTTP(%q) Content-Disposition = %q, want %q", tc.input, got, tc.expected)
		}
	}

	// Error branch 1: WriteHeader fails (0 writes allowed, fails on BOM)
	w1 := &failingResponseWriter{writesAllowed: 0}
	if err := export.StreamHTTP(w1, "test.csv", cols, []sampleRecord{{ID: "1"}}); err == nil {
		t.Fatalf("expected error when WriteHeader fails")
	}

	// Error branch 2: WriteRows fails (BOM + Header succeed = 2 writes, row > 4096 bytes triggers 3rd write)
	w2 := &failingResponseWriter{writesAllowed: 2}
	if err := export.StreamHTTP(w2, "test.csv", cols, []sampleRecord{{ID: strings.Repeat("A", 5000)}}); err == nil {
		t.Fatalf("expected error when WriteRows fails")
	}
}

func TestNegotiateEncoding(t *testing.T) {
	cases := []struct {
		header   string
		expected string
	}{
		{"", ""},
		{"   ", ""},
		{"identity", ""},
		{"deflate", ""},
		{"br", "br"},
		{"gzip", "gzip"},
		{"x-gzip", "gzip"},
		{"gzip, deflate, br", "br"},
		{"gzip, , br", "br"},
		{"gzip;q=1.0, br;q=1.0", "br"},
		{"gzip;q=0.9, br;q=0.8", "gzip"},
		{"br;q=0.8, gzip;q=0.5", "br"},
		{"br;Q=0.9, gzip;q=0.4", "br"},
		{"br;level=4;q=0.8, gzip;q=0.5", "br"},
		{"br;q=0, gzip;q=0.5", "gzip"},
		{"br;q=0, gzip;q=0", ""},
		{"*;q=0.5", "br"},
		{"br;q=0, *;q=0.5", "gzip"},
		{"gzip;q=0, *;q=0.5", "br"},
		{"*;q=0", ""},
		{"br;q=invalid, gzip;q=0.5", "gzip"},
		{"br;q=NaN, gzip;q=0.5", "gzip"},
		{"br;q=1.5, gzip;q=-0.2", ""},
	}

	for _, tc := range cases {
		if got := export.NegotiateEncoding(tc.header); got != tc.expected {
			t.Errorf("NegotiateEncoding(%q) = %q, want %q", tc.header, got, tc.expected)
		}
	}
}

func TestStreamHTTPCompressed_BrotliAndGzipAndFallback(t *testing.T) {
	cols := []export.Column[sampleRecord]{
		{Header: "User ID", Extractor: func(r sampleRecord) string { return r.ID }},
		{Header: "Full Name", Extractor: func(r sampleRecord) string { return r.Name }},
	}
	items := []sampleRecord{
		{ID: "usr-1", Name: "Alice"},
		{ID: "usr-2", Name: "Bob"},
	}

	// 1. Brotli negotiation
	recBr := httptest.NewRecorder()
	reqBr := httptest.NewRequest(http.MethodGet, "/export", nil)
	reqBr.Header.Set("Accept-Encoding", "gzip, deflate, br")

	if err := export.StreamHTTPCompressed(recBr, reqBr, "report.csv", cols, items); err != nil {
		t.Fatalf("StreamHTTPCompressed(br) failed: %v", err)
	}
	if got := recBr.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("Content-Encoding = %q, want \"br\"", got)
	}
	if got := recBr.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("Vary = %q, want \"Accept-Encoding\"", got)
	}
	if !recBr.Flushed {
		t.Fatal("expected ResponseRecorder to be flushed")
	}
	brBytes, err := io.ReadAll(brotli.NewReader(recBr.Body))
	if err != nil {
		t.Fatalf("brotli decompress failed: %v", err)
	}
	brCSV := string(brBytes)
	if !strings.HasPrefix(brCSV, "\xef\xbb\xbf") || !strings.Contains(brCSV, "usr-1,Alice\r\n") {
		t.Fatalf("unexpected decompressed brotli CSV: %q", brCSV)
	}

	// 2. Gzip negotiation with pre-existing Vary header deduplication
	recGz := httptest.NewRecorder()
	recGz.Header().Add("Vary", "Origin, Accept-Encoding")
	reqGz := httptest.NewRequest(http.MethodGet, "/export", nil)
	reqGz.Header.Set("Accept-Encoding", "gzip;q=0.9, br;q=0.4")

	if err := export.StreamHTTPCompressed(recGz, reqGz, "report.csv", cols, items); err != nil {
		t.Fatalf("StreamHTTPCompressed(gzip) failed: %v", err)
	}
	if got := recGz.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want \"gzip\"", got)
	}
	gzReader, err := gzip.NewReader(recGz.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader failed: %v", err)
	}
	gzBytes, err := io.ReadAll(gzReader)
	if err != nil {
		t.Fatalf("gzip read failed: %v", err)
	}
	_ = gzReader.Close()
	gzCSV := string(gzBytes)
	if !strings.HasPrefix(gzCSV, "\xef\xbb\xbf") || !strings.Contains(gzCSV, "usr-2,Bob\r\n") {
		t.Fatalf("unexpected decompressed gzip CSV: %q", gzCSV)
	}

	// 3. Fallback when r == nil
	recNil := httptest.NewRecorder()
	if err := export.StreamHTTPCompressed(recNil, nil, "report.csv", cols, items); err != nil {
		t.Fatalf("StreamHTTPCompressed(nil request) failed: %v", err)
	}
	if got := recNil.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("expected empty Content-Encoding for nil request, got %q", got)
	}
	if !strings.Contains(recNil.Body.String(), "usr-1,Alice\r\n") {
		t.Fatalf("unexpected uncompressed CSV body: %q", recNil.Body.String())
	}

	// 4. Non-Flusher ResponseWriter succeeds cleanly
	nonFlusher := &failingResponseWriter{writesAllowed: 10}
	if err := export.StreamHTTPCompressed(nonFlusher, nil, "report.csv", cols, items); err != nil {
		t.Fatalf("StreamHTTPCompressed(non-flusher) failed: %v", err)
	}
}

func TestStreamHTTPCompressed_Errors(t *testing.T) {
	cols := []export.Column[sampleRecord]{
		{Header: "ID", Extractor: func(r sampleRecord) string { return r.ID }},
	}
	items := []sampleRecord{{ID: "1"}}

	// 1. Brotli: streamCSV error via invalid delimiter option
	reqBr := httptest.NewRequest(http.MethodGet, "/export", nil)
	reqBr.Header.Set("Accept-Encoding", "br")
	if err := export.StreamHTTPCompressed(httptest.NewRecorder(), reqBr, "a.csv", cols, items, export.WithDelimiter('\n')); err == nil {
		t.Fatal("expected brotli streamCSV error with invalid delimiter")
	}

	// 2. Brotli: bw.Close() error when flushing compressed stream to failing writer
	wBrCloseErr := &failingResponseWriter{writesAllowed: 0}
	if err := export.StreamHTTPCompressed(wBrCloseErr, reqBr, "a.csv", cols, items); err == nil {
		t.Fatal("expected brotli Close error on failing response writer")
	}

	// 3. Gzip: streamCSV error when initial gzip header write fails
	reqGz := httptest.NewRequest(http.MethodGet, "/export", nil)
	reqGz.Header.Set("Accept-Encoding", "gzip")
	wGzStreamErr := &failingResponseWriter{writesAllowed: 0}
	if err := export.StreamHTTPCompressed(wGzStreamErr, reqGz, "a.csv", cols, items); err == nil {
		t.Fatal("expected gzip streamCSV error when writer allows 0 writes")
	}

	// 4. Gzip: gw.Close() error when gzip header succeeds (1 write) but deflate trailer fails
	wGzCloseErr := &failingResponseWriter{writesAllowed: 1}
	if err := export.StreamHTTPCompressed(wGzCloseErr, reqGz, "a.csv", cols, items); err == nil {
		t.Fatal("expected gzip Close error when writer fails on trailer flush")
	}

	// 5. Default (uncompressed): streamCSV error when writer allows 0 writes
	wPlainErr := &failingResponseWriter{writesAllowed: 0}
	if err := export.StreamHTTPCompressed(wPlainErr, nil, "a.csv", cols, items); err == nil {
		t.Fatal("expected uncompressed streamCSV error on failing response writer")
	}
}
