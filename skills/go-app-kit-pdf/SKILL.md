---
name: go-app-kit-pdf
description: "Compile HTML templates into in-memory PDF documents and Indian GST B2B tax invoices using github.com/abeta-dev/go-app-kit/pdf. Use when generating GST invoices, payment receipts, bounding wkhtmltopdf subprocess concurrency, or mocking PDF rendering in unit tests."
---

# `go-app-kit/pdf` — HTML-to-PDF & GST Tax Invoice Subskill

Use this skill when generating PDF documents, GST tax invoices, or payment receipts with `github.com/abeta-dev/go-app-kit/pdf`.

## 1. Architecture & Renderer Abstraction

- **Modern Headless Chromium (`NewChromiumRenderer`)**: Uses Chrome/Chromium `--headless=new --print-to-pdf=...` with CSS `@page` injection, bounded concurrency (`WithChromiumMaxConcurrency`), container support (`WithChromiumNoSandbox(true)`), and ephemeral loopback HTTP serving when `EnableLocalFileAccess` is `false` so local `file://` SSRF is structurally blocked.
- **Zero-Dependency Pure-Go PDF 1.4 (`NewPureGoRenderer`)**: Compiles structured HTML (headings, paragraphs, tables, lists, line breaks) into standards-compliant multi-page `%PDF-1.4` documents with zero host browser binaries and zero CGO.
- **Legacy `WkhtmlRenderer` (`NewWkhtmlRenderer`)**: Deprecated since `v0.8.0` (remove in `>= v0.10.0`) under the strict `N + 2` deprecation contract.
- **Test / CI Rendering**: Use `pdf.NewPureGoRenderer()` or `pdf.WithChromiumCommandRunner(...)` / `pdf.WithRenderer(stub)` for deterministic tests without host browser binaries.

---

## 2. Generating a Compliant Indian GST B2B Tax Invoice

Combine `go-fintech-india` (for exact paise arithmetic and words conversion) with `pdf.GSTInvoiceTemplate`:

```go
import (
	"bytes"
	"context"
	"github.com/abeta-dev/go-app-kit/pdf"
	fintech "github.com/abeta-dev/go-fintech-india"
)

func RenderGSTInvoice(ctx context.Context, sellerGSTIN, buyerGSTIN string, data map[string]any) (*bytes.Buffer, error) {
	// Ensure GSTINs are validated via go-fintech-india before rendering
	if err := fintech.ValidateGSTIN(sellerGSTIN); err != nil {
		return nil, err
	}
	if buyerGSTIN != "" {
		if err := fintech.ValidateGSTIN(buyerGSTIN); err != nil {
			return nil, err
		}
	}

	// Render in-memory A4 Portrait PDF (*bytes.Buffer)
	return pdf.GenerateFromTemplateWithContext(
		ctx,
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

## 3. Bounded Concurrency & Modern Renderer Selection

Use `NewChromiumRenderer` (with bounded concurrency) for full CSS/Flexbox rendering, or `NewPureGoRenderer` for zero-binary environments:

```go
renderer := pdf.NewChromiumRenderer(
	pdf.WithChromiumMaxConcurrency(4),
	pdf.WithChromiumNoSandbox(true), // when running in Linux containers
)

pdfBuf, err := pdf.GenerateFromTemplate(
	pdf.ReceiptTemplate,
	receiptData,
	pdf.WithRenderer(renderer), // or pdf.WithRenderer(pdf.NewPureGoRenderer())
	pdf.WithTitle("Payment Receipt"),
)
```

### Mocking in Unit Tests
```go
type stubRenderer struct{}

func (s stubRenderer) Render(html string, opts pdf.Options) ([]byte, error) {
	return []byte("%PDF-1.4 mock-bytes"), nil
}

// Pass pdf.WithRenderer(stubRenderer{}) in tests so tests never require host wkhtmltopdf.
```
