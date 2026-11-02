---
name: go-app-kit-pdf
description: "Compile HTML templates into in-memory PDF documents and Indian GST B2B tax invoices using github.com/umesh0492/go-app-kit/pdf. Use when generating GST invoices, payment receipts, bounding wkhtmltopdf subprocess concurrency, or mocking PDF rendering in unit tests."
---

# `go-app-kit/pdf` — HTML-to-PDF & GST Tax Invoice Subskill

Use this skill when generating PDF documents, GST tax invoices, or payment receipts with `github.com/umesh0492/go-app-kit/pdf`.

## 1. Architecture & Renderer Abstraction

- **Production Rendering**: Uses `WkhtmlRenderer` (`NewWkhtmlRenderer(maxConcurrent)`), which invokes the host `wkhtmltopdf` binary via `stdin`/`stdout` pipes with **zero temporary disk files** and a struct-encapsulated semaphore to bound concurrent subprocesses.
- **Test / CI Rendering (Without `wkhtmltopdf` Installed)**: Use `pdf.WithRenderer(customRenderer)` to inject a mock or stub `pdf.Renderer` in unit tests and CI environments where `wkhtmltopdf` is absent.

---

## 2. Generating a Compliant Indian GST B2B Tax Invoice

Combine `go-fintech-india` (for exact paise arithmetic and words conversion) with `pdf.GSTInvoiceTemplate`:

```go
import (
	"context"
	"github.com/umesh0492/go-app-kit/pdf"
	fintech "github.com/umesh0492/go-fintech-india"
)

func RenderGSTInvoice(ctx context.Context, data pdf.InvoiceData) ([]byte, error) {
	// Ensure GSTINs are validated via go-fintech-india before rendering
	if err := fintech.ValidateGSTIN(data.SellerGSTIN); err != nil {
		return nil, err
	}
	if data.BuyerGSTIN != "" {
		if err := fintech.ValidateGSTIN(data.BuyerGSTIN); err != nil {
			return nil, err
		}
	}

	// Render in-memory A4 Portrait PDF
	return pdf.GenerateFromTemplate(
		pdf.GSTInvoiceTemplate,
		data,
		pdf.WithPageSize("A4"),
		pdf.WithOrientation("Portrait"),
		pdf.WithMargins(10, 10, 10, 10),
		pdf.WithDPI(300),
	)
}
```

---

## 3. Bounded Concurrency & Custom Renderer Pattern

To prevent `wkhtmltopdf` memory spikes under heavy load, instantiate a shared `WkhtmlRenderer` with a bounded concurrency limit (e.g., 4 concurrent renders per pod):

```go
renderer := pdf.NewWkhtmlRenderer(4) // max 4 concurrent wkhtmltopdf subprocesses

pdfBytes, err := pdf.GenerateFromTemplate(
	pdf.ReceiptTemplate,
	receiptData,
	pdf.WithRenderer(renderer),
	pdf.WithTitle("Payment Receipt"),
)
```

### Mocking in Unit Tests
```go
type stubRenderer struct{}

func (s stubRenderer) RenderHTML(ctx context.Context, html string, opts pdf.Options) ([]byte, error) {
	return []byte("%PDF-1.4 mock-bytes"), nil
}

// Pass pdf.WithRenderer(stubRenderer{}) in tests so tests never require host wkhtmltopdf.
```
