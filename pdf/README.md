# `pdf`

In-memory HTML-to-PDF document compilation with responsive layout options, template helpers, and embedded production billing templates.

---

## When to Use

- **Automated Billing & Invoicing**: Compiling B2B GST tax invoices, SaaS monthly billing statements, or point-of-sale payment receipts dynamically from JSON data.
- **Transactional Attachment Generation**: Creating PDF documents in memory to attach directly to transactional emails or store in S3/blob storage without writing temporary files to disk.
- **Document Standardization**: Applying unified page sizes (A4, Letter), DPI, millimeter margins, and document title metadata across enterprise services.

---

## Why It Is Written Like That

1. **Modern Pluggable Backends (`Renderer`)**: Ships two first-class replacements for archived upstream `wkhtmltopdf` behind the unified `Renderer` interface:
   - **`NewChromiumRenderer`**: Modern headless Chrome/Chromium (`--headless=new --print-to-pdf=...`) with automatic binary discovery (`FindChromiumBinary`), CSS `@page` margin/size/orientation injection, bounded concurrency semaphore, and ephemeral loopback HTTP serving when `EnableLocalFileAccess` is `false` (blocking `file://` SSRF).
   - **`NewPureGoRenderer`**: Zero external binary, zero CGO, pure-Go PDF 1.4 compiler supporting headings, paragraphs, tables, lists, word-wrapping, multi-page pagination, and document title metadata.
   - **Legacy `NewWkhtmlRenderer`**: Retained for backward compatibility and deprecated since `v0.8.0` (remove in `>= v0.10.0`) under the strict `N + 2` deprecation contract.
2. **Embedded Production Templates (`go:embed`)**: Bundles standard GST tax invoice and payment receipt templates directly into the compiled Go binary so downstream services don't need external HTML asset deployment.
3. **Functional Option Pattern**: Uses composable options (`WithPageSize`, `WithOrientation`, `WithMargins`, `WithDPI`, `WithTitle`, `WithRenderer`) with safe production defaults (A4 Portrait, 10mm margins, 300 DPI).

---

## Alternatives Evaluated

- **Pure Go PDF Canvas Engines (e.g. `gofpdf`, `unidoc`)**: Require imperative, coordinate-based low-level drawing commands (`pdf.CellFormat(x, y, ...)`). Complex responsive layouts, tax tables, and CSS styling take days to hand-code and maintain.
- **Headless Chrome (Puppeteer / Playwright)**: Requires running full Chromium instances consuming 300MB–1GB RAM per worker, significantly increasing container image sizes and operational cold-start latency.
- **Cloud Document APIs (e.g. DocRaptor, PDFShift)**: Introduce recurring per-document costs, outbound network dependencies, and transmission of sensitive customer PII to third parties.

---

## Comparison Table

| Alternative | Pros | Cons | Why We Chose `go-app-kit/pdf` |
|---|---|---|---|
| **Low-Level Go PDF Libraries (`gofpdf`)** | Zero external binary requirements | Imperative coordinate layout; impossible to use standard HTML/CSS templates | `go-app-kit/pdf` uses standard HTML/CSS templates designers can edit. |
| **Headless Chrome (Playwright/CDP)** | Full modern CSS/Flexbox support | High memory footprint (>500MB/instance), heavy container images | `go-app-kit/pdf` uses lightweight `wkhtmltopdf` with fast sub-second in-memory compilation. |
| **SaaS Document APIs** | Zero infrastructure management | Per-call API fees, data privacy risks with PII, network failure point | `go-app-kit/pdf` runs fully self-hosted and zero-cost within your VPC. |

---

## Usage Example

```go
package main

import (
    "os"
    "github.com/abeta-dev/go-app-kit/pdf"
)

func main() {
    data := map[string]any{
        "InvoiceNumber": "INV-2026-001",
        "GrandTotal":    "59,000.00",
        "AmountInWords": "Fifty Nine Thousand Rupees Only",
        // ... see templates/gst_invoice.html for full data schema
    }

    // Compile embedded GST invoice template via headless Chromium or pure-Go PDF 1.4
    buf, err := pdf.GenerateFromTemplate(pdf.GSTInvoiceTemplate, data,
        pdf.WithRenderer(pdf.NewPureGoRenderer()), // or pdf.NewChromiumRenderer()
        pdf.WithPageSize("A4"),
        pdf.WithOrientation("Portrait"),
        pdf.WithMargins(10, 10, 10, 10),
    )
    if err != nil {
        panic(err)
    }

    // Save or stream to S3 / HTTP Response
    _ = os.WriteFile("invoice.pdf", buf.Bytes(), 0644)
}
```

---

## Known Limitations

- **Chromium Backend**: `NewChromiumRenderer()` requires a local Chrome or Chromium binary (`CHROMIUM_PATH`, `CHROME_PATH`, or `$PATH`); if missing, `ErrNoRendererAvailable` wrapping `ErrChromiumNotFound` is returned.
- **Pure-Go Backend**: `NewPureGoRenderer()` requires zero external binaries and outputs valid PDF 1.4 text/table documents using standard Type1 Helvetica fonts, without executing JavaScript or CSS Flexbox layouts.
- **Legacy `wkhtmltopdf`**: Deprecated since `v0.8.0` (remove in `>= v0.10.0`). Migrate to `NewChromiumRenderer()` or `NewPureGoRenderer()`.
