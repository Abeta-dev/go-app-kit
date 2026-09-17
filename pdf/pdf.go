// Package pdf provides in-memory HTML-to-PDF compilation via wkhtmltopdf
// with production-ready options, template helpers, and document templates.
package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"strings"
	"sync"

	wkhtml "github.com/SebastiaanKlippert/go-wkhtmltopdf"
)

// ErrNoRendererAvailable is returned when no PDF rendering engine is available
// on the host system and no custom Renderer has been configured.
var ErrNoRendererAvailable = errors.New("pdf: no renderer available: wkhtmltopdf not found in PATH and no custom Renderer configured")

// Renderer defines an abstract interface for compiling HTML into PDF bytes.
// This decouples document compilation from any specific engine (e.g. wkhtmltopdf, headless Chrome, Gotenberg, or test mocks).
type Renderer interface {
	Render(html string, opts Options) ([]byte, error)
}

// DefaultMaxConcurrency is the default upper bound for simultaneous wkhtmltopdf subprocesses.
const DefaultMaxConcurrency = 10

// WkhtmlRenderer is the default Renderer implementation backed by wkhtmltopdf.
// Concurrency is bounded per renderer instance via an encapsulated semaphore channel.
type WkhtmlRenderer struct {
	Semaphore chan struct{}
	sem       chan struct{}
	semOnce   sync.Once
}

// NewWkhtmlRenderer constructs a WkhtmlRenderer with an encapsulated semaphore of capacity maxConcurrency.
// If maxConcurrency <= 0, DefaultMaxConcurrency (10) is used.
func NewWkhtmlRenderer(maxConcurrency ...int) *WkhtmlRenderer {
	limit := DefaultMaxConcurrency
	if len(maxConcurrency) > 0 && maxConcurrency[0] > 0 {
		limit = maxConcurrency[0]
	}
	r := &WkhtmlRenderer{
		sem: make(chan struct{}, limit),
	}
	r.semOnce.Do(func() {})
	return r
}

func (w *WkhtmlRenderer) getSemaphore(opts Options) chan struct{} {
	if w.Semaphore != nil {
		return w.Semaphore
	}
	if opts.Semaphore != nil {
		return opts.Semaphore
	}
	if w.sem != nil {
		return w.sem
	}
	w.semOnce.Do(func() {
		limit := opts.MaxConcurrency
		if limit <= 0 {
			limit = DefaultMaxConcurrency
		}
		w.sem = make(chan struct{}, limit)
	})
	return w.sem
}

type pdfGenerator interface {
	AddPage(*wkhtml.PageReader)
	Create() error
	Bytes() []byte
}

type wkhtmlGenerator struct {
	*wkhtml.PDFGenerator
}

func (w *wkhtmlGenerator) AddPage(p *wkhtml.PageReader) {
	w.PDFGenerator.AddPage(p)
}

func (w *wkhtmlGenerator) Create() error {
	return w.PDFGenerator.Create()
}

func (w *wkhtmlGenerator) CreateContext(ctx context.Context) error {
	return w.PDFGenerator.CreateContext(ctx)
}

func (w *wkhtmlGenerator) Bytes() []byte {
	return w.PDFGenerator.Bytes()
}

var (
	generatorFactoryMu sync.RWMutex
	generatorFactory   = defaultGenerator
)

func getGeneratorFactory() func(opts Options) (pdfGenerator, error) {
	generatorFactoryMu.RLock()
	defer generatorFactoryMu.RUnlock()
	return generatorFactory
}

// Render compiles HTML into PDF bytes using wkhtmltopdf with concurrency bounding and context cancellation.
func (w *WkhtmlRenderer) Render(html string, opts Options) ([]byte, error) {
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	sem := w.getSemaphore(opts)

	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	factory := getGeneratorFactory()
	pdfg, err := factory(opts)
	if err != nil {
		return nil, err
	}

	page := wkhtml.NewPageReader(bytes.NewBufferString(html))
	page.EnableLocalFileAccess.Set(opts.EnableLocalFileAccess)
	page.Encoding.Set("UTF-8")

	pdfg.AddPage(page)

	if cp, ok := pdfg.(interface{ CreateContext(context.Context) error }); ok {
		err = cp.CreateContext(ctx)
	} else {
		err = pdfg.Create()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to render pdf: %w", err)
	}

	return pdfg.Bytes(), nil
}

// Options configures PDF rendering properties.
type Options struct {
	PageSize              string // "A4", "Letter", etc. (Default: "A4")
	Orientation           string // "Portrait", "Landscape" (Default: "Portrait")
	DPI                   uint   // DPI resolution (Default: 300)
	MarginTop             uint   // Margins in mm (Default: 10)
	MarginBottom          uint   // Margins in mm (Default: 10)
	MarginLeft            uint   // Margins in mm (Default: 10)
	MarginRight           uint   // Margins in mm (Default: 10)
	Title                 string // Document title
	EnableLocalFileAccess bool   // Default: false (prevents file:/// exfiltration)
	MaxConcurrency        int    // Max simultaneous wkhtmltopdf executions (Default: 10)
	Context               context.Context
	Semaphore             chan struct{}
	renderer              Renderer
}

// Option modifies Options.
type Option func(*Options)

// DefaultOptions returns standard A4 portrait settings with 10mm margins.
// Note: EnableLocalFileAccess defaults to false to prevent local file inclusion.
func DefaultOptions() Options {
	return Options{
		PageSize:              "A4",
		Orientation:           "Portrait",
		DPI:                   300,
		MarginTop:             10,
		MarginBottom:          10,
		MarginLeft:            10,
		MarginRight:           10,
		EnableLocalFileAccess: false,
		MaxConcurrency:        DefaultMaxConcurrency,
		Context:               context.Background(),
	}
}

// WithContext sets the context for cancellation and timeout of the PDF generation process.
func WithContext(ctx context.Context) Option {
	return func(o *Options) {
		if ctx != nil {
			o.Context = ctx
		}
	}
}

// WithMaxConcurrency bounds the maximum number of simultaneous wkhtmltopdf processes (default: 10).
func WithMaxConcurrency(n int) Option {
	return func(o *Options) {
		if n > 0 {
			o.MaxConcurrency = n
		}
	}
}

// WithSemaphore configures a custom concurrency semaphore channel.
func WithSemaphore(sem chan struct{}) Option {
	return func(o *Options) {
		o.Semaphore = sem
	}
}

// WithPageSize sets the page size (e.g. "A4", "Letter").
func WithPageSize(size string) Option {
	return func(o *Options) { o.PageSize = size }
}

// WithOrientation sets page orientation ("Portrait" or "Landscape").
func WithOrientation(orientation string) Option {
	return func(o *Options) { o.Orientation = orientation }
}

// WithMargins sets top, bottom, left, and right margins in millimeters.
func WithMargins(top, bottom, left, right uint) Option {
	return func(o *Options) {
		o.MarginTop = top
		o.MarginBottom = bottom
		o.MarginLeft = left
		o.MarginRight = right
	}
}

// WithDPI sets rendering DPI.
func WithDPI(dpi uint) Option {
	return func(o *Options) { o.DPI = dpi }
}

// WithTitle sets document title metadata.
func WithTitle(title string) Option {
	return func(o *Options) { o.Title = title }
}

// WithLocalFileAccess controls whether local file access is permitted during rendering.
// By default, this is disabled (false) to prevent SSRF and arbitrary local file exfiltration (e.g. file:///etc/passwd).
func WithLocalFileAccess(enable bool) Option {
	return func(o *Options) { o.EnableLocalFileAccess = enable }
}

// WithRenderer configures a custom PDF rendering engine (e.g. headless Chrome, Gotenberg, or mock).
func WithRenderer(r Renderer) Option {
	return func(o *Options) {
		o.renderer = r
	}
}

// defaultGenerator is the unexported default constructor for wkhtml.PDFGenerator.
func defaultGenerator(opts Options) (pdfGenerator, error) {
	g, err := wkhtml.NewPDFGenerator()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoRendererAvailable, err)
	}
	g.Dpi.Set(opts.DPI)
	g.PageSize.Set(opts.PageSize)
	g.Orientation.Set(opts.Orientation)
	g.MarginTop.Set(opts.MarginTop)
	g.MarginBottom.Set(opts.MarginBottom)
	g.MarginLeft.Set(opts.MarginLeft)
	g.MarginRight.Set(opts.MarginRight)
	if opts.Title != "" {
		g.Title.Set(opts.Title)
	}
	return &wkhtmlGenerator{PDFGenerator: g}, nil
}

var defaultRenderer = NewWkhtmlRenderer(DefaultMaxConcurrency)

// Generate accepts a raw HTML string and optional configurations, compiling it into a PDF bytes buffer.
func Generate(html string, opts ...Option) (*bytes.Buffer, error) {
	config := DefaultOptions()
	for _, opt := range opts {
		opt(&config)
	}

	renderer := config.renderer
	if renderer == nil {
		if config.MaxConcurrency > 0 && config.MaxConcurrency != DefaultMaxConcurrency {
			renderer = NewWkhtmlRenderer(config.MaxConcurrency)
		} else {
			renderer = defaultRenderer
		}
	}

	data, err := renderer.Render(html, config)
	if err != nil {
		return nil, err
	}

	return bytes.NewBuffer(data), nil
}

// GenerateWithContext accepts a context, raw HTML string, and optional configurations,
// compiling it into a PDF bytes buffer with context cancellation support.
func GenerateWithContext(ctx context.Context, html string, opts ...Option) (*bytes.Buffer, error) {
	allOpts := make([]Option, 0, len(opts)+1)
	allOpts = append(allOpts, WithContext(ctx))
	allOpts = append(allOpts, opts...)
	return Generate(html, allOpts...)
}

// RenderTemplate evaluates an HTML Go template string against a data context.
func RenderTemplate(tmplStr string, data any) (string, error) {
	tmpl, err := template.New("pdf").Funcs(template.FuncMap{
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
	}).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// GenerateFromTemplate executes an HTML template with the given data context and renders it into a PDF buffer.
func GenerateFromTemplate(tmplStr string, data any, opts ...Option) (*bytes.Buffer, error) {
	rendered, err := RenderTemplate(tmplStr, data)
	if err != nil {
		return nil, err
	}
	return Generate(rendered, opts...)
}

// GenerateFromTemplateWithContext executes an HTML template with the given data context and
// renders it into a PDF buffer with context cancellation support.
func GenerateFromTemplateWithContext(ctx context.Context, tmplStr string, data any, opts ...Option) (*bytes.Buffer, error) {
	rendered, err := RenderTemplate(tmplStr, data)
	if err != nil {
		return nil, err
	}
	return GenerateWithContext(ctx, rendered, opts...)
}
