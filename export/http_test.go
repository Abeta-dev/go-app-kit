// SPDX-License-Identifier: MIT

package export_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abeta-dev/go-app-kit/export"
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
