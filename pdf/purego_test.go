package pdf

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPureGoRenderer_OptionsAndValidPDFStructure(t *testing.T) {
	t.Parallel()

	r := NewPureGoRenderer(
		nil,
		WithPureGoFontSize(0),
		WithPureGoFontSize(11),
		WithPureGoLineHeight(0.5),
		WithPureGoLineHeight(1.35),
	)

	// Canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Render("<p>hi</p>", Options{Context: canceledCtx}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// Empty HTML with nil context yields a valid 1-page PDF 1.4
	emptyPDF, err := r.Render("   ", Options{})
	if err != nil {
		t.Fatalf("Render empty HTML failed: %v", err)
	}
	if !bytes.HasPrefix(emptyPDF, []byte("%PDF-1.4\n")) || !bytes.Contains(emptyPDF, []byte("/Count 1")) || !bytes.HasSuffix(emptyPDF, []byte("%%EOF\n")) {
		t.Fatalf("invalid empty PDF structure: %s", string(emptyPDF))
	}

	// Rich HTML with comments, style/script/head, headings, tables, lists, hr, br, entities, Unicode, escaping, long tokens
	richHTML := `<!DOCTYPE html>
	<html>
	<head><title>Ignored Head</title><style>body { color: red; }</style><script>alert(1)</script></head>
	<body>
		<!-- HTML comment stripped -->
		<h1>Tax Invoice (FY 2026-27)</h1>
		<h2>Supplier &amp; Buyer</h2>
		<h3>Line Items</h3>
		<h4>Notes &lt;Confidential&gt;</h4>
		<p>Amount: ₹59,000.00 — “Verified” ‘OK’ • Path C:\invoices\inv.pdf नमस्ते</p>
		<ul>
			<li>First item with line break<br>continued line</li>
			<li>Second item with SuperLongUnbreakableTokenExceedingSingleLineWidth1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ</li>
		</ul>
		<hr>
		<table>
			<tr><th>HSN</th><th>Description</th><th>Total</th></tr>
			<tr><td>998313</td><td>Cloud Hosting</td><td>₹59,000.00</td></tr>
		</table>
		<p>Trailing text without closing tag <unclosed
	`

	buf, err := Generate(
		richHTML,
		WithRenderer(r),
		WithPageSize("Letter"),
		WithOrientation("Landscape"),
		WithMargins(15, 15, 15, 15),
		WithTitle("GST Invoice (2026)"),
	)
	if err != nil {
		t.Fatalf("Generate rich HTML via PureGoRenderer failed: %v", err)
	}
	pdfStr := buf.String()
	for _, want := range []string{
		"%PDF-1.4\n",
		"/BaseFont /Helvetica",
		"/BaseFont /Helvetica-Bold",
		"/Title (GST Invoice \\(2026\\))",
		"INR 59,000.00",
		"998313 | Cloud Hosting | INR 59,000.00",
		"xref\n",
		"trailer\n",
		"startxref\n",
		"%%EOF\n",
	} {
		if !strings.Contains(pdfStr, want) {
			t.Fatalf("expected PDF output to contain %q", want)
		}
	}
}

func TestPureGoRenderer_PaginationDimensionsAndTemplates(t *testing.T) {
	t.Parallel()

	r := NewPureGoRenderer(WithPureGoFontSize(14), WithPureGoLineHeight(1.6))

	// Legal size + excessive margins clamping + multi-page overflow
	var bigHTML strings.Builder
	bigHTML.WriteString("<h1>Multi-Page Ledger</h1>")
	for i := 0; i < 60; i++ {
		bigHTML.WriteString("<p>Ledger entry row with enough descriptive content to force clean word wrapping and multi-page pagination across pages.</p>")
	}

	pdfBytes, err := r.Render(bigHTML.String(), Options{
		PageSize:     "Legal",
		Orientation:  "Portrait",
		MarginTop:    300, // exceeds page height -> clamped
		MarginBottom: 300,
		MarginLeft:   250, // exceeds page width -> clamped
		MarginRight:  250,
	})
	if err != nil {
		t.Fatalf("multi-page Render failed: %v", err)
	}
	if !bytes.Contains(pdfBytes, []byte("/Count ")) || bytes.Contains(pdfBytes, []byte("/Count 1 ")) {
		t.Fatalf("expected multi-page PDF (/Count > 1)")
	}

	// Unterminated comment & unterminated style tag handling + tiny maxChars clamp
	edgeHTML := "<!-- unclosed comment"
	if _, err := r.Render(edgeHTML, DefaultOptions()); err != nil {
		t.Fatalf("unclosed comment failed: %v", err)
	}
	edgeHTML2 := "<style>body{color:blue}"
	if _, err := r.Render(edgeHTML2, DefaultOptions()); err != nil {
		t.Fatalf("unclosed style failed: %v", err)
	}
	if lines := wrapPDFText("hello world", 10, 20); len(lines) == 0 {
		t.Fatalf("expected non-empty wrapped lines with narrow width clamp")
	}

	// End-to-end compile of embedded GSTInvoiceTemplate using zero-dep PureGoRenderer
	invoiceData := map[string]any{
		"InvoiceNumber": "INV-2026-042",
		"InvoiceDate":   "15-Apr-2026",
		"DueDate":       "30-Apr-2026",
		"PlaceOfSupply": "Karnataka (29)",
		"ReverseCharge": false,
		"IsInterState":  false,
		"Supplier": map[string]string{
			"Name":      "Abeta Cloud Pvt Ltd",
			"Address":   "Bengaluru",
			"GSTIN":     "29AACCG0527D1Z0",
			"StateName": "Karnataka",
			"StateCode": "29",
			"PAN":       "AACCG0527D",
			"Email":     "billing@abeta.dev",
		},
		"Customer": map[string]string{
			"Name":      "Demo Corp",
			"Address":   "Bengaluru",
			"GSTIN":     "29AACCG0527D1Z0",
			"StateName": "Karnataka",
			"StateCode": "29",
			"PAN":       "AACCG0527D",
		},
		"Items": []map[string]any{
			{
				"Index":         1,
				"Description":   "Enterprise Platform License",
				"HSN":           "998313",
				"Quantity":      1,
				"UnitPrice":     "10,000.00",
				"TaxableAmount": "10,000.00",
				"CGSTRate":      9,
				"CGSTAmount":    "900.00",
				"SGSTRate":      9,
				"SGSTAmount":    "900.00",
				"TotalAmount":   "11,800.00",
			},
		},
		"SubTotal":      "10,000.00",
		"TotalCGST":     "900.00",
		"TotalSGST":     "900.00",
		"GrandTotal":    "11,800.00",
		"AmountInWords": "Eleven Thousand Eight Hundred Rupees Only",
		"BankDetails": map[string]string{
			"BankName":      "HDFC Bank",
			"AccountNumber": "50200012345678",
			"IFSC":          "HDFC0000001",
			"Branch":        "Indiranagar",
		},
	}

	pdfBuf, err := GenerateFromTemplate(GSTInvoiceTemplate, invoiceData, WithRenderer(NewPureGoRenderer()), WithTitle("INV-2026-042"))
	if err != nil {
		t.Fatalf("GenerateFromTemplate with PureGoRenderer failed: %v", err)
	}
	if !bytes.HasPrefix(pdfBuf.Bytes(), []byte("%PDF-1.4\n")) || !strings.Contains(pdfBuf.String(), "INV-2026-042") {
		t.Fatalf("expected valid invoice PDF output")
	}
}
