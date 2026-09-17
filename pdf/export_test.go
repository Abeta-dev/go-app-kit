package pdf

import (
	"context"

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
