package pdf

import (
	"context"
	"strings"
	"testing"

	wkhtml "github.com/SebastiaanKlippert/go-wkhtmltopdf"
)

// MockPDFGeneratorTarget is an alias for internal pdfGenerator exposed for testing.
type MockPDFGeneratorTarget = pdfGenerator

// SetGeneratorFactoryForTesting replaces the generator factory for unit tests with mutex synchronization.
func SetGeneratorFactoryForTesting(fn func(opts Options) (pdfGenerator, error)) func() {
	generatorFactoryMu.Lock()
	orig := generatorFactory
	generatorFactory = fn
	generatorFactoryMu.Unlock()
	return func() {
		generatorFactoryMu.Lock()
		generatorFactory = orig
		generatorFactoryMu.Unlock()
	}
}

// MockPDFGenerator is an in-memory generator mock for testing WkhtmlRenderer.
type MockPDFGenerator struct {
	AddPageFunc       func(*wkhtml.PageReader)
	CreateFunc        func() error
	CreateContextFunc func(context.Context) error
	BytesFunc         func() []byte
}

func (m *MockPDFGenerator) AddPage(p *wkhtml.PageReader) {
	if m.AddPageFunc != nil {
		m.AddPageFunc(p)
	}
}

func (m *MockPDFGenerator) Create() error {
	if m.CreateFunc != nil {
		return m.CreateFunc()
	}
	return nil
}

func (m *MockPDFGenerator) CreateContext(ctx context.Context) error {
	if m.CreateContextFunc != nil {
		return m.CreateContextFunc(ctx)
	}
	if m.CreateFunc != nil {
		return m.CreateFunc()
	}
	return nil
}

func (m *MockPDFGenerator) Bytes() []byte {
	if m.BytesFunc != nil {
		return m.BytesFunc()
	}
	return []byte("%PDF-1.4 Mock Wkhtml")
}

func TestWkhtmlGenerator_Direct(t *testing.T) {
	// If wkhtmltopdf is not installed, we can still construct wkhtmlGenerator with an empty or simulated PDFGenerator
	page := wkhtml.NewPageReader(strings.NewReader("<html><body>test</body></html>"))
	gen := &wkhtmlGenerator{PDFGenerator: &wkhtml.PDFGenerator{}}
	gen.AddPage(page)
	_ = gen.Bytes()
	_ = gen.Create()
	_ = gen.CreateContext(context.Background())
}

func TestOptions_ConcurrencyAndContext(t *testing.T) {
	opts := DefaultOptions()

	WithMaxConcurrency(5)(&opts)
	if opts.MaxConcurrency != 5 {
		t.Errorf("expected MaxConcurrency 5, got %d", opts.MaxConcurrency)
	}
	WithMaxConcurrency(-1)(&opts)
	if opts.MaxConcurrency != 5 {
		t.Errorf("expected MaxConcurrency to remain 5, got %d", opts.MaxConcurrency)
	}

	ctx := context.WithValue(context.Background(), struct{}{}, "val")
	WithContext(ctx)(&opts)
	if opts.Context != ctx {
		t.Errorf("expected context to be set")
	}
	WithContext(nil)(&opts)
	if opts.Context != ctx {
		t.Errorf("expected context to remain unchanged on nil")
	}

	sem := make(chan struct{}, 3)
	WithSemaphore(sem)(&opts)
	if opts.Semaphore != sem {
		t.Errorf("expected semaphore to be set")
	}
}

func TestDefaultGenerator_Options(t *testing.T) {
	opts := DefaultOptions()
	opts.Title = "Statutory Invoice"
	opts.DPI = 150
	opts.PageSize = "Letter"
	opts.Orientation = "Landscape"
	opts.MarginTop = 15
	opts.MarginBottom = 15
	opts.MarginLeft = 20
	opts.MarginRight = 20

	gen, err := defaultGenerator(opts)
	if err != nil {
		// On environments without wkhtmltopdf binary, verify proper error wrapping
		if !strings.Contains(err.Error(), ErrNoRendererAvailable.Error()) {
			t.Errorf("expected ErrNoRendererAvailable wrapped, got: %v", err)
		}
		if gen != nil {
			t.Errorf("expected nil generator on error")
		}
		return
	}
	if gen == nil {
		t.Errorf("expected non-nil generator")
	}
}

type mockTestRenderer struct {
	renderFunc func(html string, opts Options) ([]byte, error)
}

func (m *mockTestRenderer) Render(html string, opts Options) ([]byte, error) {
	if m.renderFunc != nil {
		return m.renderFunc(html, opts)
	}
	return []byte("%PDF-1.4 Mock Test Renderer"), nil
}

func TestGenerateFromTemplateWithContext_Branches(t *testing.T) {
	ctx := context.Background()

	// 1. Invalid template syntax error
	_, err := GenerateFromTemplateWithContext(ctx, "{{.Bad", nil)
	if err == nil {
		t.Errorf("expected template syntax error, got nil")
	}

	// 2. Successful execution with custom renderer
	mock := &mockTestRenderer{
		renderFunc: func(html string, opts Options) ([]byte, error) {
			return []byte("%PDF-1.4 Template Output"), nil
		},
	}
	buf, err := GenerateFromTemplateWithContext(ctx, "<b>{{.Title}}</b>", map[string]string{"Title": "Invoice"}, WithRenderer(mock))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Template Output") {
		t.Errorf("expected rendered content in buffer, got: %s", buf.String())
	}
}

func TestWkhtmlRenderer_GetSemaphore_Branches(t *testing.T) {
	// 1. Uninitialized renderer with non-positive MaxConcurrency -> DefaultMaxConcurrency
	r1 := &WkhtmlRenderer{}
	opts1 := DefaultOptions()
	opts1.MaxConcurrency = 0
	sem1 := r1.getSemaphore(opts1)
	if cap(sem1) != DefaultMaxConcurrency {
		t.Errorf("expected default semaphore cap %d, got %d", DefaultMaxConcurrency, cap(sem1))
	}

	// 2. Uninitialized renderer with positive MaxConcurrency
	r2 := &WkhtmlRenderer{}
	opts2 := DefaultOptions()
	opts2.MaxConcurrency = 8
	sem2 := r2.getSemaphore(opts2)
	if cap(sem2) != 8 {
		t.Errorf("expected semaphore cap 8, got %d", cap(sem2))
	}

	// 3. Renderer with custom Semaphore field
	customSem := make(chan struct{}, 3)
	r3 := &WkhtmlRenderer{Semaphore: customSem}
	if got := r3.getSemaphore(opts1); got != customSem {
		t.Errorf("expected custom renderer semaphore")
	}
}

type legacyGeneratorWithoutCreateContext struct{}

func (l *legacyGeneratorWithoutCreateContext) AddPage(p *wkhtml.PageReader) {}
func (l *legacyGeneratorWithoutCreateContext) Create() error                { return nil }
func (l *legacyGeneratorWithoutCreateContext) Bytes() []byte                { return []byte("%PDF-1.4 Legacy") }

func TestWkhtmlRenderer_Render_LegacyGeneratorAndConcurrency(t *testing.T) {
	restore := SetGeneratorFactoryForTesting(func(opts Options) (pdfGenerator, error) {
		return &legacyGeneratorWithoutCreateContext{}, nil
	})
	defer restore()

	// 1. Renders via legacy Create() (non-CreateContext)
	r := NewWkhtmlRenderer(2)
	data, err := r.Render("<html><body>legacy</body></html>", DefaultOptions())
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if string(data) != "%PDF-1.4 Legacy" {
		t.Errorf("unexpected output: %s", string(data))
	}

	// 2. Generate with custom concurrency != DefaultMaxConcurrency without custom renderer
	buf, err := Generate("<html><body>concurrency test</body></html>", WithMaxConcurrency(4))
	if err != nil {
		t.Fatalf("Generate with custom concurrency failed: %v", err)
	}
	if string(buf.Bytes()) != "%PDF-1.4 Legacy" {
		t.Errorf("unexpected buffer: %s", buf.String())
	}
}
