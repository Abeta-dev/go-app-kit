package pdf_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	wkhtml "github.com/SebastiaanKlippert/go-wkhtmltopdf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umesh0492/go-app-kit/pdf"
)

type mockRenderer struct {
	renderFunc   func(html string, opts pdf.Options) ([]byte, error)
	called       bool
	capturedHTML string
	capturedOpts pdf.Options
}

func (m *mockRenderer) Render(html string, opts pdf.Options) ([]byte, error) {
	m.called = true
	m.capturedHTML = html
	m.capturedOpts = opts
	if m.renderFunc != nil {
		return m.renderFunc(html, opts)
	}
	return []byte("MOCK_PDF_CONTENT"), nil
}

func TestGenerate_CustomRenderer(t *testing.T) {
	mock := &mockRenderer{}

	buf, err := pdf.Generate("<html><body>Test Document</body></html>",
		pdf.WithRenderer(mock),
		pdf.WithPageSize("Letter"),
		pdf.WithOrientation("Landscape"),
		pdf.WithDPI(150),
		pdf.WithMargins(15, 15, 20, 20),
		pdf.WithTitle("Test Document"),
	)
	require.NoError(t, err)
	require.NotNil(t, buf)
	assert.Equal(t, "MOCK_PDF_CONTENT", buf.String())
	assert.True(t, mock.called)
	assert.Equal(t, "Letter", mock.capturedOpts.PageSize)
	assert.Equal(t, "Landscape", mock.capturedOpts.Orientation)
	assert.Equal(t, uint(150), mock.capturedOpts.DPI)
	assert.Equal(t, uint(15), mock.capturedOpts.MarginTop)
	assert.Equal(t, uint(15), mock.capturedOpts.MarginBottom)
	assert.Equal(t, uint(20), mock.capturedOpts.MarginLeft)
	assert.Equal(t, uint(20), mock.capturedOpts.MarginRight)
	assert.Equal(t, "Test Document", mock.capturedOpts.Title)
	assert.False(t, mock.capturedOpts.EnableLocalFileAccess)
}

func TestGenerate_CustomRenderer_Error(t *testing.T) {
	expectedErr := errors.New("custom engine render failure")
	mock := &mockRenderer{
		renderFunc: func(html string, opts pdf.Options) ([]byte, error) {
			return nil, expectedErr
		},
	}

	buf, err := pdf.Generate("<html></html>", pdf.WithRenderer(mock))
	require.Error(t, err)
	assert.Nil(t, buf)
	assert.ErrorIs(t, err, expectedErr)
}

func TestGenerate_LocalFileAccess(t *testing.T) {
	t.Run("Default disables local file access", func(t *testing.T) {
		mock := &mockRenderer{}
		_, err := pdf.Generate("<html></html>", pdf.WithRenderer(mock))
		require.NoError(t, err)
		assert.False(t, mock.capturedOpts.EnableLocalFileAccess)
	})

	t.Run("WithLocalFileAccess(true) enables local file access", func(t *testing.T) {
		mock := &mockRenderer{}
		_, err := pdf.Generate("<html></html>", pdf.WithRenderer(mock), pdf.WithLocalFileAccess(true))
		require.NoError(t, err)
		assert.True(t, mock.capturedOpts.EnableLocalFileAccess)
	})
}

func TestGenerate_NoRendererAvailable(t *testing.T) {
	// Verifies graceful handling without C-binary dependencies on host system.
	origEnv := os.Getenv("WKHTMLTOPDF_PATH")
	os.Setenv("WKHTMLTOPDF_PATH", "/nonexistent_binary_location")
	defer func() {
		os.Setenv("WKHTMLTOPDF_PATH", origEnv)
		wkhtml.SetPath("")
	}()
	wkhtml.SetPath("")

	buf, err := pdf.Generate("<html><body>No host binary</body></html>")
	require.Error(t, err)
	assert.Nil(t, buf)
	assert.ErrorIs(t, err, pdf.ErrNoRendererAvailable)
}

func TestWkhtmlRenderer_Direct_NoRendererAvailable(t *testing.T) {
	origEnv := os.Getenv("WKHTMLTOPDF_PATH")
	os.Setenv("WKHTMLTOPDF_PATH", "/nonexistent_binary_location")
	defer func() {
		os.Setenv("WKHTMLTOPDF_PATH", origEnv)
		wkhtml.SetPath("")
	}()
	wkhtml.SetPath("")

	r := &pdf.WkhtmlRenderer{}
	bytes, err := r.Render("<html></html>", pdf.DefaultOptions())
	require.Error(t, err)
	assert.Nil(t, bytes)
	assert.ErrorIs(t, err, pdf.ErrNoRendererAvailable)
}

func TestWkhtmlRenderer_Success(t *testing.T) {
	var addedPage *wkhtml.PageReader
	mock := &pdf.MockPDFGenerator{
		AddPageFunc: func(p *wkhtml.PageReader) {
			addedPage = p
		},
		CreateFunc: func() error { return nil },
		BytesFunc:  func() []byte { return []byte("PDF_BYTES_SUCCESS") },
	}
	reset := pdf.SetGeneratorFactoryForTesting(func(opts pdf.Options) (pdf.MockPDFGeneratorTarget, error) {
		return mock, nil
	})
	defer reset()

	r := &pdf.WkhtmlRenderer{}
	bytes, err := r.Render("<html><body>Test</body></html>", pdf.DefaultOptions())
	require.NoError(t, err)
	assert.Equal(t, "PDF_BYTES_SUCCESS", string(bytes))
	assert.NotNil(t, addedPage)
}

func TestWkhtmlRenderer_CreateError(t *testing.T) {
	expectedErr := errors.New("wkhtml execution failure")
	mock := &pdf.MockPDFGenerator{
		CreateFunc: func() error { return expectedErr },
	}
	reset := pdf.SetGeneratorFactoryForTesting(func(opts pdf.Options) (pdf.MockPDFGeneratorTarget, error) {
		return mock, nil
	})
	defer reset()

	r := &pdf.WkhtmlRenderer{}
	bytes, err := r.Render("<html></html>", pdf.DefaultOptions())
	require.Error(t, err)
	assert.Nil(t, bytes)
	assert.Contains(t, err.Error(), "failed to render pdf")
}

func TestWkhtmlRenderer_LocalFileAccess(t *testing.T) {
	var capturedPage *wkhtml.PageReader
	mock := &pdf.MockPDFGenerator{
		AddPageFunc: func(p *wkhtml.PageReader) {
			capturedPage = p
		},
	}
	reset := pdf.SetGeneratorFactoryForTesting(func(opts pdf.Options) (pdf.MockPDFGeneratorTarget, error) {
		return mock, nil
	})
	defer reset()

	r := &pdf.WkhtmlRenderer{}
	opts := pdf.DefaultOptions()
	pdf.WithLocalFileAccess(true)(&opts)
	_, err := r.Render("<html></html>", opts)
	require.NoError(t, err)
	require.NotNil(t, capturedPage)
	assert.Contains(t, capturedPage.Args(), "--enable-local-file-access")
}

func TestDefaultOptions(t *testing.T) {
	opts := pdf.DefaultOptions()
	assert.Equal(t, "A4", opts.PageSize)
	assert.Equal(t, "Portrait", opts.Orientation)
	assert.Equal(t, uint(300), opts.DPI)
	assert.Equal(t, uint(10), opts.MarginTop)
	assert.Equal(t, uint(10), opts.MarginBottom)
	assert.Equal(t, uint(10), opts.MarginLeft)
	assert.Equal(t, uint(10), opts.MarginRight)
	assert.False(t, opts.EnableLocalFileAccess)
}

func TestRenderTemplate(t *testing.T) {
	tmpl := "Hello, {{.Name | upper}}! Your role is {{.Role | lower}}."
	data := map[string]string{
		"Name": "John",
		"Role": "ADMIN",
	}

	rendered, err := pdf.RenderTemplate(tmpl, data)
	require.NoError(t, err)
	expected := "Hello, JOHN! Your role is admin."
	assert.Equal(t, expected, rendered)

	// Bad template syntax
	_, err = pdf.RenderTemplate("{{.Unclosed", data)
	assert.Error(t, err)

	// Execution error
	_, err = pdf.RenderTemplate("{{len .Missing}}", data)
	assert.Error(t, err)
}

func TestGenerateFromTemplate(t *testing.T) {
	mock := &mockRenderer{
		renderFunc: func(html string, opts pdf.Options) ([]byte, error) {
			assert.Contains(t, html, "Invoice")
			return []byte("PDF_FROM_TEMPLATE"), nil
		},
	}

	tmpl := "<h1>{{.Title}}</h1>"
	buf, err := pdf.GenerateFromTemplate(tmpl, map[string]string{"Title": "Invoice"}, pdf.WithRenderer(mock))
	require.NoError(t, err)
	require.NotNil(t, buf)
	assert.Equal(t, "PDF_FROM_TEMPLATE", buf.String())

	// Failed template syntax
	_, err = pdf.GenerateFromTemplate("{{.Bad", nil, pdf.WithRenderer(mock))
	assert.Error(t, err)
}

func TestGSTInvoiceTemplate_Render(t *testing.T) {
	require.NotEmpty(t, pdf.GSTInvoiceTemplate)

	data := map[string]any{
		"InvoiceNumber": "INV-2026-001",
		"InvoiceDate":   "01-Apr-2026",
		"DueDate":       "15-Apr-2026",
		"PlaceOfSupply": "Maharashtra (27)",
		"ReverseCharge": false,
		"IsInterState":  false,
		"PONumber":      "PO-9988",
		"PODate":        "28-Mar-2026",
		"PaymentTerms":  "Net 15",
		"FinancialYear": "FY 2026-27",
		"Supplier": map[string]string{
			"Name":      "Acme Cloud Technologies Pvt Ltd",
			"Address":   "Bandra Kurla Complex, Mumbai",
			"GSTIN":     "27AAAPZ1234F1Z5",
			"StateName": "Maharashtra",
			"StateCode": "27",
			"PAN":       "AAAPZ1234F",
			"Email":     "billing@acmecloud.in",
		},
		"Customer": map[string]string{
			"Name":      "Bharat Retail Solutions Ltd",
			"Address":   "Koramangala, Bengaluru",
			"GSTIN":     "29AAAPZ5678F1Z9",
			"StateName": "Karnataka",
			"StateCode": "29",
			"PAN":       "AAAPZ5678F",
		},
		"Items": []map[string]any{
			{
				"Index":         1,
				"Description":   "Cloud Infrastructure Hosting",
				"HSN":           "998313",
				"Quantity":      1,
				"UnitPrice":     "50,000.00",
				"TaxableAmount": "50,000.00",
				"CGSTRate":      9,
				"CGSTAmount":    "4,500.00",
				"SGSTRate":      9,
				"SGSTAmount":    "4,500.00",
				"TotalAmount":   "59,000.00",
			},
		},
		"SubTotal":      "50,000.00",
		"TotalCGST":     "4,500.00",
		"TotalSGST":     "4,500.00",
		"GrandTotal":    "59,000.00",
		"AmountInWords": "Fifty Nine Thousand Rupees Only",
		"BankDetails": map[string]string{
			"BankName":      "HDFC Bank",
			"AccountNumber": "50200012345678",
			"IFSC":          "HDFC0000001",
			"Branch":        "BKC Mumbai",
		},
	}

	rendered, err := pdf.RenderTemplate(pdf.GSTInvoiceTemplate, data)
	require.NoError(t, err)
	assert.Greater(t, len(rendered), 100)
}

func TestReceiptTemplate_Render(t *testing.T) {
	require.NotEmpty(t, pdf.ReceiptTemplate)

	data := map[string]any{
		"ReceiptNumber":        "REC-1002",
		"Amount":               "15,000.00",
		"AmountInWords":        "Fifteen Thousand Rupees Only",
		"PaymentDate":          "05-Apr-2026",
		"PaymentMode":          "UPI / NetBanking",
		"TransactionRef":       "UPI-309812739182",
		"CustomerName":         "Rohan Sharma",
		"CustomerEmail":        "rohan@example.com",
		"InvoiceReference":     "INV-2026-001",
		"Status":               "SUCCESS",
		"MerchantName":         "Acme Cloud Technologies",
		"Notes":                "Subscription renewal for Q1 2026",
		"MerchantSupportEmail": "support@acmecloud.in",
	}

	rendered, err := pdf.RenderTemplate(pdf.ReceiptTemplate, data)
	require.NoError(t, err)
	assert.Greater(t, len(rendered), 100)
}

func TestGenerateWithContext_Success(t *testing.T) {
	var capturedCtx context.Context
	mock := &pdf.MockPDFGenerator{
		CreateContextFunc: func(ctx context.Context) error {
			capturedCtx = ctx
			return nil
		},
		BytesFunc: func() []byte { return []byte("PDF_WITH_CONTEXT") },
	}
	reset := pdf.SetGeneratorFactoryForTesting(func(opts pdf.Options) (pdf.MockPDFGeneratorTarget, error) {
		return mock, nil
	})
	defer reset()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	buf, err := pdf.GenerateWithContext(ctx, "<html><body>Context Success</body></html>")
	require.NoError(t, err)
	require.NotNil(t, buf)
	assert.Equal(t, "PDF_WITH_CONTEXT", buf.String())
	assert.NotNil(t, capturedCtx)
}

func TestGenerateWithContext_Cancellation(t *testing.T) {
	mock := &pdf.MockPDFGenerator{
		CreateContextFunc: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
	reset := pdf.SetGeneratorFactoryForTesting(func(opts pdf.Options) (pdf.MockPDFGeneratorTarget, error) {
		return mock, nil
	})
	defer reset()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	buf, err := pdf.GenerateWithContext(ctx, "<html><body>Canceled</body></html>")
	require.Error(t, err)
	assert.Nil(t, buf)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestGenerateFromTemplateWithContext(t *testing.T) {
	mock := &pdf.MockPDFGenerator{
		CreateContextFunc: func(ctx context.Context) error {
			return nil
		},
		BytesFunc: func() []byte { return []byte("TEMPLATE_PDF_WITH_CTX") },
	}
	reset := pdf.SetGeneratorFactoryForTesting(func(opts pdf.Options) (pdf.MockPDFGeneratorTarget, error) {
		return mock, nil
	})
	defer reset()

	ctx := context.Background()
	buf, err := pdf.GenerateFromTemplateWithContext(ctx, "<h1>{{.Title}}</h1>", map[string]string{"Title": "Invoice"})
	require.NoError(t, err)
	require.NotNil(t, buf)
	assert.Equal(t, "TEMPLATE_PDF_WITH_CTX", buf.String())
}

func TestWkhtmlRenderer_ConcurrencyThrottling(t *testing.T) {
	var currentActive int32
	var maxActive int32

	mock := &pdf.MockPDFGenerator{
		CreateContextFunc: func(ctx context.Context) error {
			active := atomic.AddInt32(&currentActive, 1)
			for {
				oldMax := atomic.LoadInt32(&maxActive)
				if active <= oldMax || atomic.CompareAndSwapInt32(&maxActive, oldMax, active) {
					break
				}
			}
			time.Sleep(30 * time.Millisecond)
			atomic.AddInt32(&currentActive, -1)
			return nil
		},
		BytesFunc: func() []byte { return []byte("CONCURRENT_PDF") },
	}
	reset := pdf.SetGeneratorFactoryForTesting(func(opts pdf.Options) (pdf.MockPDFGeneratorTarget, error) {
		return mock, nil
	})
	defer reset()

	// Use custom semaphore bounded to 2 concurrent executions
	sem := make(chan struct{}, 2)
	concurrencyOpt := pdf.WithSemaphore(sem)

	const totalTasks = 6
	var wg sync.WaitGroup
	errs := make([]error, totalTasks)

	for i := 0; i < totalTasks; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = pdf.Generate("<html><body>Throttle Test</body></html>", concurrencyOpt)
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "task %d should succeed", i)
	}

	observedMax := atomic.LoadInt32(&maxActive)
	assert.LessOrEqual(t, observedMax, int32(2), "concurrent executions must never exceed semaphore bound of 2")
	assert.Greater(t, observedMax, int32(0), "at least one task must have executed")
}

func TestWkhtmlRenderer_SemaphoreContextCancellation(t *testing.T) {
	// Semaphore filled with 1 token to block incoming caller
	sem := make(chan struct{}, 1)
	sem <- struct{}{} // fill semaphore

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	buf, err := pdf.GenerateWithContext(ctx, "<html><body>Timeout Test</body></html>", pdf.WithSemaphore(sem))
	require.Error(t, err)
	assert.Nil(t, buf)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestNewWkhtmlRenderer_EncapsulatedSemaphore(t *testing.T) {
	var currentActive int32
	var maxActive int32

	mock := &pdf.MockPDFGenerator{
		CreateContextFunc: func(ctx context.Context) error {
			active := atomic.AddInt32(&currentActive, 1)
			for {
				oldMax := atomic.LoadInt32(&maxActive)
				if active <= oldMax || atomic.CompareAndSwapInt32(&maxActive, oldMax, active) {
					break
				}
			}
			time.Sleep(30 * time.Millisecond)
			atomic.AddInt32(&currentActive, -1)
			return nil
		},
		BytesFunc: func() []byte { return []byte("ENCAPSULATED_PDF") },
	}
	reset := pdf.SetGeneratorFactoryForTesting(func(opts pdf.Options) (pdf.MockPDFGeneratorTarget, error) {
		return mock, nil
	})
	defer reset()

	renderer := pdf.NewWkhtmlRenderer(2)

	const totalTasks = 6
	var wg sync.WaitGroup
	errs := make([]error, totalTasks)

	for i := 0; i < totalTasks; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = renderer.Render("<html><body>Encapsulated Test</body></html>", pdf.DefaultOptions())
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "task %d should succeed", i)
	}

	observedMax := atomic.LoadInt32(&maxActive)
	assert.LessOrEqual(t, observedMax, int32(2), "concurrent executions must never exceed encapsulated semaphore bound of 2")
	assert.Greater(t, observedMax, int32(0), "at least one task must have executed")
}

