---
name: go-app-kit-export
description: "Stream large CSV datasets with constant memory, Excel UTF-8 BOM support, and spreadsheet formula-injection escaping using github.com/Abeta-dev/go-app-kit/export. Use when building CSV download endpoints, streaming database cursors to HTTP responses, or preventing CSV injection attacks."
---

# `go-app-kit/export` — Streaming CSV Exporter Subskill

Use this skill when building CSV report downloads or streaming database query cursors with `github.com/Abeta-dev/go-app-kit/export`.

## 1. Security & Memory Guarantees

- **Spreadsheet Formula-Injection Defense**: Automatically escapes cell values starting with `=`, `+`, `-`, `@`, `\t`, or `\r` so malicious user input cannot execute formulas when opened in Microsoft Excel or Google Sheets.
- **Excel UTF-8 BOM Support**: Prepends `\xEF\xBB\xBF` when `WithBOM(true)` is enabled (enabled by default in `StreamHTTP`), ensuring `₹` and non-ASCII characters render cleanly in Excel.
- **Zero Full-Dataset Buffering**: Flushes rows in configurable batches (`WithFlushInterval(n)`).

---

## 2. One-Line HTTP Download Endpoint (`StreamHTTP[T]`)

For slices already loaded in memory, use `export.StreamHTTP` (sets `Content-Type: text/csv; charset=utf-8`, sanitized `Content-Disposition: attachment`, `X-Content-Type-Options: nosniff`, UTF-8 BOM, and CRLF line endings):

```go
import (
	"net/http"
	"github.com/Abeta-dev/go-app-kit/export"
	fintech "github.com/Abeta-dev/go-fintech-india"
)

var invoiceColumns = []export.Column[InvoiceRow]{
	{Header: "Invoice Number", Extractor: func(r InvoiceRow) string { return r.Number }},
	{Header: "Buyer GSTIN", Extractor: func(r InvoiceRow) string { return r.BuyerGSTIN }},
	{Header: "Amount (INR)", Extractor: func(r InvoiceRow) string { return fintech.NewMoney(r.AmountPaise).String() }},
}

func DownloadInvoicesCSV(w http.ResponseWriter, r *http.Request) {
	rows := fetchInvoices(r.Context())
	_ = export.StreamHTTP(w, "gst-invoices-fy26.csv", invoiceColumns, rows)
}
```

---

## 3. Cursor-Based Streaming for Million-Row Datasets (`NewCSVStreamer[T]`)

When streaming directly from a `pgx.Rows` database cursor without holding all rows in RAM:

```go
streamer := export.NewCSVStreamer(
	w,
	invoiceColumns,
	export.WithBOM(true),
	export.WithCRLF(true),
	export.WithFlushInterval(500),
)

for dbRows.Next() {
	var row InvoiceRow
	if err := dbRows.Scan(&row.Number, &row.BuyerGSTIN, &row.AmountPaise); err != nil {
		return err
	}
	if err := streamer.WriteRow(row); err != nil {
		return err
	}
}
return streamer.Close()
```
