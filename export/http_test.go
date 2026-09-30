// SPDX-License-Identifier: MIT

package export_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/umesh0492/go-app-kit/export"
)

type sampleRecord struct {
	ID   string
	Name string
	Role string
}

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
