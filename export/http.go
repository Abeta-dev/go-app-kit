// SPDX-License-Identifier: MIT

package export

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
)

var filenameReplacer = strings.NewReplacer(
	"\r", "",
	"\n", "",
	"\"", "",
	"\\", "",
)

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

// StreamHTTP writes records as an attachment CSV using the provided http.ResponseWriter
// with UTF-8 Byte Order Mark (BOM) and Windows CRLF line endings for maximum spreadsheet compatibility.
// Compatible with standard http.ResponseWriter as well as Gin (c.Writer), Echo, Chi, and Fiber adapters.
func StreamHTTP[T any](w http.ResponseWriter, filename string, columns []Column[T], items []T) error {
	safeFilename := sanitizeFilename(filename)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", safeFilename))
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	streamer := NewCSVStreamer[T](
		w,
		columns,
		WithBOM(true),
		WithCRLF(true),
	)

	if err := streamer.WriteHeader(); err != nil {
		return fmt.Errorf("failed to write CSV headers: %w", err)
	}

	if err := streamer.WriteRows(items); err != nil {
		return fmt.Errorf("failed to stream CSV rows: %w", err)
	}

	return streamer.Flush()
}
