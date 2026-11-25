package pdf

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"math"
	"strings"
)

const (
	defaultPureGoFontSize   = 10.5
	defaultPureGoLineHeight = 1.4
	mmToPoints              = 72.0 / 25.4
)

// PureGoOption configures a PureGoRenderer instance.
type PureGoOption func(*PureGoRenderer)

// WithPureGoFontSize sets the base body font size in points (default: 10.5).
func WithPureGoFontSize(pt float64) PureGoOption {
	return func(r *PureGoRenderer) {
		if pt > 0 {
			r.fontSize = pt
		}
	}
}

// WithPureGoLineHeight sets the line-height multiplier relative to font size (default: 1.4).
func WithPureGoLineHeight(mult float64) PureGoOption {
	return func(r *PureGoRenderer) {
		if mult >= 1.0 {
			r.lineHeight = mult
		}
	}
}

// PureGoRenderer is a zero-dependency, pure-Go PDF 1.4 Renderer that compiles structured HTML
// text (headings, paragraphs, tables, lists, line breaks) into valid multi-page PDF bytes
// without requiring wkhtmltopdf, Chromium, CGO, or any external host binary.
type PureGoRenderer struct {
	fontSize   float64
	lineHeight float64
}

// NewPureGoRenderer constructs a PureGoRenderer with optional typography overrides.
func NewPureGoRenderer(opts ...PureGoOption) *PureGoRenderer {
	r := &PureGoRenderer{
		fontSize:   defaultPureGoFontSize,
		lineHeight: defaultPureGoLineHeight,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
	return r
}

type pdfTextBlock struct {
	text       string
	bold       bool
	scale      float64
	spaceAfter float64
}

type pdfStyledLine struct {
	text    string
	bold    bool
	size    float64
	leading float64
}

// Render parses rawHTML into readable text blocks and emits a standards-compliant PDF 1.4 document
// honoring Options.PageSize, Options.Orientation, Options.Margin*, Options.Title, and Options.Context.
func (r *PureGoRenderer) Render(rawHTML string, opts Options) ([]byte, error) {
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	pageW, pageH := resolvePageDimensions(opts.PageSize, opts.Orientation)
	marginLeft := float64(opts.MarginLeft) * mmToPoints
	marginRight := float64(opts.MarginRight) * mmToPoints
	marginTop := float64(opts.MarginTop) * mmToPoints
	marginBottom := float64(opts.MarginBottom) * mmToPoints

	usableW := pageW - marginLeft - marginRight
	if usableW < 72 {
		usableW = 72
		marginLeft = 36
	}
	usableH := pageH - marginTop - marginBottom
	if usableH < 72 {
		marginTop = 36
		marginBottom = 36
	}

	blocks := parseHTMLToBlocks(rawHTML)
	var lines []pdfStyledLine
	for _, blk := range blocks {
		size := r.fontSize * blk.scale
		leading := size * r.lineHeight
		wrapped := wrapPDFText(blk.text, usableW, size)
		for _, wline := range wrapped {
			lines = append(lines, pdfStyledLine{
				text:    wline,
				bold:    blk.bold,
				size:    size,
				leading: leading,
			})
		}
		if blk.spaceAfter > 0 {
			lines = append(lines, pdfStyledLine{
				text:    "",
				size:    size,
				leading: blk.spaceAfter,
			})
		}
	}

	pages := paginatePDFLines(lines, pageH, marginTop, marginBottom)
	return buildPDF14Document(pages, pageW, pageH, marginLeft, marginTop, opts.Title), nil
}

func resolvePageDimensions(pageSize, orientation string) (width, height float64) {
	switch strings.ToLower(strings.TrimSpace(pageSize)) {
	case "letter":
		width, height = 612.0, 792.0
	case "legal":
		width, height = 612.0, 1008.0
	default: // A4
		width, height = 595.28, 841.89
	}
	if strings.EqualFold(strings.TrimSpace(orientation), "landscape") {
		return height, width
	}
	return width, height
}

type htmlBlockParser struct {
	blocks   []pdfTextBlock
	current  strings.Builder
	bold     bool
	scale    float64
	inRow    bool
	rowCells int
}

func (p *htmlBlockParser) flush(spaceAfter float64) {
	text := normalizeWhitespace(html.UnescapeString(p.current.String()))
	p.current.Reset()
	if text == "" {
		return
	}
	p.blocks = append(p.blocks, pdfTextBlock{
		text:       text,
		bold:       p.bold,
		scale:      p.scale,
		spaceAfter: spaceAfter,
	})
}

func (p *htmlBlockParser) handleTag(rawTag string) {
	isClosing := strings.HasPrefix(rawTag, "/")
	if isClosing {
		rawTag = strings.TrimSpace(rawTag[1:])
	}
	tagName := strings.ToLower(extractTagName(rawTag))

	switch tagName {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		p.handleHeading(tagName, isClosing)
	case "tr", "td", "th":
		p.handleTableTag(tagName, isClosing)
	case "li":
		p.flush(2)
		if !isClosing {
			p.current.WriteString("- ")
		}
	case "p", "div", "section", "article", "header", "footer", "blockquote", "table":
		p.flush(4)
	case "br":
		p.flush(0)
	case "hr":
		p.flush(2)
		p.blocks = append(p.blocks, pdfTextBlock{
			text:       strings.Repeat("-", 60),
			scale:      0.9,
			spaceAfter: 4,
		})
	}
}

func (p *htmlBlockParser) handleHeading(tagName string, isClosing bool) {
	if isClosing {
		p.flush(6)
		p.bold = false
		p.scale = 1.0
		return
	}
	p.flush(4)
	p.bold = true
	p.scale = headingScale(tagName)
}

func (p *htmlBlockParser) handleTableTag(tagName string, isClosing bool) {
	if tagName == "tr" {
		if isClosing {
			p.flush(3)
			p.inRow = false
			p.rowCells = 0
			return
		}
		p.flush(2)
		p.inRow = true
		p.rowCells = 0
		return
	}
	if !isClosing && p.inRow {
		if p.rowCells > 0 {
			p.current.WriteString(" | ")
		}
		p.rowCells++
	}
}

func parseHTMLToBlocks(rawHTML string) []pdfTextBlock {
	cleaned := stripIgnoredHTMLSections(rawHTML)
	p := htmlBlockParser{scale: 1.0}

	for i := 0; i < len(cleaned); {
		if cleaned[i] != '<' {
			nextTag := strings.IndexByte(cleaned[i:], '<')
			if nextTag < 0 {
				p.current.WriteString(cleaned[i:])
				break
			}
			p.current.WriteString(cleaned[i : i+nextTag])
			i += nextTag
			continue
		}

		closeIdx := strings.IndexByte(cleaned[i:], '>')
		if closeIdx < 0 {
			p.current.WriteString(cleaned[i:])
			break
		}
		rawTag := strings.TrimSpace(cleaned[i+1 : i+closeIdx])
		i += closeIdx + 1
		p.handleTag(rawTag)
	}
	p.flush(0)
	return p.blocks
}

func stripIgnoredHTMLSections(raw string) string {
	s := raw
	for {
		start := strings.Index(s, "<!--")
		if start < 0 {
			break
		}
		end := strings.Index(s[start+4:], "-->")
		if end < 0 {
			s = s[:start]
			break
		}
		s = s[:start] + s[start+4+end+3:]
	}

	for _, tag := range []string{"style", "script", "head"} {
		openPat := "<" + tag
		closePat := "</" + tag + ">"
		for {
			lower := strings.ToLower(s)
			start := strings.Index(lower, openPat)
			if start < 0 {
				break
			}
			end := strings.Index(lower[start:], closePat)
			if end < 0 {
				s = s[:start]
				break
			}
			s = s[:start] + s[start+end+len(closePat):]
		}
	}
	return s
}

func extractTagName(rawTag string) string {
	for i := 0; i < len(rawTag); i++ {
		c := rawTag[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '/' {
			return rawTag[:i]
		}
	}
	return rawTag
}

func headingScale(tag string) float64 {
	switch tag {
	case "h1":
		return 1.75
	case "h2":
		return 1.45
	case "h3":
		return 1.25
	default:
		return 1.1
	}
}

func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func wrapPDFText(text string, maxWidthPt, fontSizePt float64) []string {
	sanitized := sanitizePDFText(text)
	maxChars := int(math.Floor(maxWidthPt / (fontSizePt * 0.54)))
	if maxChars < 12 {
		maxChars = 12
	}

	words := strings.Fields(sanitized)
	var lines []string
	var cur strings.Builder

	for _, word := range words {
		for len(word) > maxChars {
			if cur.Len() > 0 {
				lines = append(lines, cur.String())
				cur.Reset()
			}
			lines = append(lines, word[:maxChars])
			word = word[maxChars:]
		}

		switch {
		case cur.Len() == 0:
			cur.WriteString(word)
		case cur.Len()+1+len(word) <= maxChars:
			cur.WriteByte(' ')
			cur.WriteString(word)
		default:
			lines = append(lines, cur.String())
			cur.Reset()
			cur.WriteString(word)
		}
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return lines
}

func sanitizePDFText(s string) string {
	replacer := strings.NewReplacer(
		"₹", "INR ",
		"—", "-",
		"–", "-",
		"“", `"`,
		"”", `"`,
		"‘", "'",
		"’", "'",
		"•", "-",
		"\u00a0", " ",
	)
	replaced := replacer.Replace(s)
	var b strings.Builder
	b.Grow(len(replaced))
	for _, r := range replaced {
		if r >= 0x20 && r <= 0x7e {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	return normalizeWhitespace(b.String())
}

func escapePDFString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' || c == '(' || c == ')' {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	return b.String()
}

func paginatePDFLines(lines []pdfStyledLine, pageH, marginTop, marginBottom float64) [][]pdfStyledLine {
	if len(lines) == 0 {
		return [][]pdfStyledLine{{}}
	}

	var pages [][]pdfStyledLine
	var current []pdfStyledLine
	y := pageH - marginTop

	for _, line := range lines {
		if y-line.leading < marginBottom && len(current) > 0 {
			pages = append(pages, current)
			current = nil
			y = pageH - marginTop
		}
		current = append(current, line)
		y -= line.leading
	}
	pages = append(pages, current)
	return pages
}

func buildPDF14Document(pages [][]pdfStyledLine, pageW, pageH, marginLeft, marginTop float64, title string) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")

	// Fixed object layout:
	//   1: Catalog
	//   2: Pages
	//   3: Font /F1 (Helvetica)
	//   4: Font /F2 (Helvetica-Bold)
	//   5: Info
	//   6 + 2*i: Page i
	//   7 + 2*i: Content stream for Page i
	totalObjs := 5 + len(pages)*2
	offsets := make([]int, totalObjs+1)

	writeObj := func(id int, body string) {
		offsets[id] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", id, body)
	}

	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")

	var kids strings.Builder
	for i := range pages {
		if i > 0 {
			kids.WriteByte(' ')
		}
		fmt.Fprintf(&kids, "%d 0 R", 6+i*2)
	}
	writeObj(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), len(pages)))
	writeObj(3, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	writeObj(4, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>")

	infoBody := "<< /Producer (go-app-kit/pdf PureGoRenderer)"
	if trimmedTitle := sanitizePDFText(title); trimmedTitle != "" {
		infoBody += " /Title (" + escapePDFString(trimmedTitle) + ")"
	}
	infoBody += " >>"
	writeObj(5, infoBody)

	for i, pageLines := range pages {
		pageObjID := 6 + i*2
		contentObjID := 7 + i*2

		var stream bytes.Buffer
		y := pageH - marginTop
		for _, line := range pageLines {
			y -= line.leading
			if line.text == "" {
				continue
			}
			fontName := "F1"
			if line.bold {
				fontName = "F2"
			}
			fmt.Fprintf(&stream, "BT /%s %.2f Tf 1 0 0 1 %.2f %.2f Tm (%s) Tj ET\n",
				fontName, line.size, marginLeft, y, escapePDFString(line.text))
		}

		pageDict := fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents %d 0 R >>",
			pageW, pageH, contentObjID,
		)
		writeObj(pageObjID, pageDict)

		contentStr := stream.String()
		writeObj(contentObjID, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(contentStr), contentStr))
	}

	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", totalObjs+1)
	for id := 1; id <= totalObjs; id++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[id])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R /Info 5 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		totalObjs+1, xrefOffset)

	return buf.Bytes()
}
