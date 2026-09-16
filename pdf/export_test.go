package pdf

import wkhtml "github.com/SebastiaanKlippert/go-wkhtmltopdf"

// MockPDFGeneratorTarget is an alias for internal pdfGenerator exposed for testing.
type MockPDFGeneratorTarget = pdfGenerator

// SetGeneratorFactoryForTesting replaces the generator factory for unit tests.
func SetGeneratorFactoryForTesting(fn func(opts Options) (pdfGenerator, error)) func() {
	orig := generatorFactory
	generatorFactory = fn
	return func() {
		generatorFactory = orig
	}
}

// MockPDFGenerator is an in-memory generator mock for testing WkhtmlRenderer.
type MockPDFGenerator struct {
	AddPageFunc func(*wkhtml.PageReader)
	CreateFunc  func() error
	BytesFunc   func() []byte
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

func (m *MockPDFGenerator) Bytes() []byte {
	if m.BytesFunc != nil {
		return m.BytesFunc()
	}
	return []byte("%PDF-1.4 Mock Wkhtml")
}
