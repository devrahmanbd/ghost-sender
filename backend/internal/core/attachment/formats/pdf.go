package formats

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

var (
	ErrInvalidPageSize   = errors.New("pdf: invalid page size")
	ErrInvalidOrientation = errors.New("pdf: invalid orientation")
	ErrGenerationTimeout = errors.New("pdf: generation timeout")
)

type PageSize string

const (
	PageSizeLetter PageSize = "Letter"
	PageSizeA4     PageSize = "A4"
	PageSizeA3     PageSize = "A3"
	PageSizeLegal  PageSize = "Legal"
	PageSizeTabloid PageSize = "Tabloid"
	PageSizeCustom PageSize = "Custom"
)

type Orientation string

const (
	OrientationPortrait  Orientation = "portrait"
	OrientationLandscape Orientation = "landscape"
)

type PDFGenerator struct {
	mu      sync.RWMutex
	config  *PDFConfig
	pool    chan context.Context
	ctx     context.Context
	cancel  context.CancelFunc
	closed  bool
}

type PDFConfig struct {
	PageSize          PageSize
	Orientation       Orientation
	MarginTop         float64
	MarginBottom      float64
	MarginLeft        float64
	MarginRight       float64
	PrintBackground   bool
	DisplayHeaderFooter bool
	HeaderTemplate    string
	FooterTemplate    string
	Scale             float64
	PaperWidth        float64
	PaperHeight       float64
	PreferCSSPageSize bool
	Timeout           time.Duration
	PoolSize          int
	WaitForLoad       bool
	WaitTime          time.Duration
}

type PDFRequest struct {
	HTML              string
	Config            *PDFConfig
	Context           context.Context
	Metadata          *PDFMetadata
	Watermark         string
	CustomCSS         string
	JavaScriptEnabled bool
}

type PDFMetadata struct {
	Title    string
	Author   string
	Subject  string
	Keywords string
	Creator  string
	Producer string
}

func NewPDFGenerator(cfg *PDFConfig) (*PDFGenerator, error) {
	if cfg == nil {
		cfg = DefaultPDFConfig()
	}

	if err := ValidatePDFConfig(cfg); err != nil {
		return nil, err
	}

	if cfg.PoolSize <= 0 {
		cfg.PoolSize = 4
	}

	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.DisableGPU,
		chromedp.NoSandbox,
		chromedp.Headless,
		chromedp.Flag("disable-web-security", true),
		chromedp.Flag("disable-features", "IsolateOrigins,site-per-process"),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)

	pg := &PDFGenerator{
		config: cfg,
		pool:   make(chan context.Context, cfg.PoolSize),
		ctx:    allocCtx,
		cancel: allocCancel,
	}

	for i := 0; i < cfg.PoolSize; i++ {
		browserCtx, _ := chromedp.NewContext(allocCtx)
		pg.pool <- browserCtx
	}

	return pg, nil
}

func DefaultPDFConfig() *PDFConfig {
	return &PDFConfig{
		PageSize:          PageSizeA4,
		Orientation:       OrientationPortrait,
		MarginTop:         0.4,
		MarginBottom:      0.4,
		MarginLeft:        0.4,
		MarginRight:       0.4,
		PrintBackground:   true,
		DisplayHeaderFooter: false,
		Scale:             1.0,
		PreferCSSPageSize: false,
		Timeout:           30 * time.Second,
		PoolSize:          4,
		WaitForLoad:       true,
		WaitTime:          500 * time.Millisecond,
	}
}

func (pg *PDFGenerator) Generate(req *PDFRequest) ([]byte, error) {
	pg.mu.RLock()
	closed := pg.closed
	pg.mu.RUnlock()

	if closed {
		return nil, errors.New("pdf: generator closed")
	}

	if req.HTML == "" {
		return nil, errors.New("pdf: empty HTML content")
	}

	if req.Config == nil {
		req.Config = pg.config
	}

	if req.Context == nil {
		ctx, cancel := context.WithTimeout(context.Background(), req.Config.Timeout)
		defer cancel()
		req.Context = ctx
	}

	var browserCtx context.Context
	select {
	case browserCtx = <-pg.pool:
		defer func() {
			select {
			case pg.pool <- browserCtx:
			default:
			}
		}()
	case <-req.Context.Done():
		return nil, ErrGenerationTimeout
	}

	html := req.HTML
	
	if req.CustomCSS != "" {
		html = injectCSS(html, req.CustomCSS)
	}

	if req.Watermark != "" {
		html = addWatermark(html, req.Watermark)
	}

	html = optimizeHTMLForPDF(html)

	printParams := pg.buildPrintParams(req.Config)

	var pdfBuf []byte

	tasks := chromedp.Tasks{
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			frameTree, err := page.GetFrameTree().Do(ctx)
			if err != nil {
				return err
			}
			return page.SetDocumentContent(frameTree.Frame.ID, html).Do(ctx)
		}),
	}

	if req.Config.WaitForLoad {
		tasks = append(tasks, chromedp.Sleep(req.Config.WaitTime))
	}

	tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		pdfBuf, _, err = printParams.Do(ctx)
		return err
	}))

	if err := chromedp.Run(browserCtx, tasks); err != nil {
		return nil, fmt.Errorf("pdf: generation failed: %w", err)
	}

	if len(pdfBuf) < 4 || string(pdfBuf[:4]) != "%PDF" {
		return nil, errors.New("pdf: invalid PDF generated")
	}

	return pdfBuf, nil
}

func (pg *PDFGenerator) GenerateBatch(requests []*PDFRequest) ([][]byte, []error) {
	results := make([][]byte, len(requests))
	errs := make([]error, len(requests))
	
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, pg.config.PoolSize)

	for i, req := range requests {
		wg.Add(1)
		go func(idx int, request *PDFRequest) {
			defer wg.Done()
			
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			pdf, err := pg.Generate(request)
			results[idx] = pdf
			errs[idx] = err
		}(i, req)
	}

	wg.Wait()
	return results, errs
}

func (pg *PDFGenerator) buildPrintParams(cfg *PDFConfig) *page.PrintToPDFParams {
	params := page.PrintToPDF().
		WithPrintBackground(cfg.PrintBackground).
		WithScale(cfg.Scale).
		WithMarginTop(cfg.MarginTop).
		WithMarginBottom(cfg.MarginBottom).
		WithMarginLeft(cfg.MarginLeft).
		WithMarginRight(cfg.MarginRight).
		WithPreferCSSPageSize(cfg.PreferCSSPageSize)

	if cfg.Orientation == OrientationLandscape {
		params = params.WithLandscape(true)
	}

	if cfg.DisplayHeaderFooter {
		params = params.WithDisplayHeaderFooter(true)
		if cfg.HeaderTemplate != "" {
			params = params.WithHeaderTemplate(cfg.HeaderTemplate)
		}
		if cfg.FooterTemplate != "" {
			params = params.WithFooterTemplate(cfg.FooterTemplate)
		}
	}

	if cfg.PageSize == PageSizeCustom && cfg.PaperWidth > 0 && cfg.PaperHeight > 0 {
		params = params.WithPaperWidth(cfg.PaperWidth).WithPaperHeight(cfg.PaperHeight)
	}

	return params
}

func (pg *PDFGenerator) Close() error {
	pg.mu.Lock()
	defer pg.mu.Unlock()

	if pg.closed {
		return nil
	}
	pg.closed = true

	close(pg.pool)
	
	if pg.cancel != nil {
		pg.cancel()
	}

	return nil
}

func ValidatePDFConfig(cfg *PDFConfig) error {
	if cfg == nil {
		return errors.New("pdf: nil config")
	}

	switch cfg.PageSize {
	case PageSizeLetter, PageSizeA4, PageSizeA3, PageSizeLegal, PageSizeTabloid, PageSizeCustom:
	default:
		return ErrInvalidPageSize
	}

	switch cfg.Orientation {
	case OrientationPortrait, OrientationLandscape:
	default:
		return ErrInvalidOrientation
	}

	if cfg.Scale <= 0 || cfg.Scale > 2 {
		return errors.New("pdf: scale must be between 0 and 2")
	}

	if cfg.MarginTop < 0 || cfg.MarginBottom < 0 || cfg.MarginLeft < 0 || cfg.MarginRight < 0 {
		return errors.New("pdf: margins must be non-negative")
	}

	if cfg.PageSize == PageSizeCustom {
		if cfg.PaperWidth <= 0 || cfg.PaperHeight <= 0 {
			return errors.New("pdf: custom page size requires width and height")
		}
	}

	return nil
}

func GetPageDimensions(pageSize PageSize) (width, height float64) {
	switch pageSize {
	case PageSizeLetter:
		return 8.5, 11
	case PageSizeA4:
		return 8.27, 11.69
	case PageSizeA3:
		return 11.69, 16.54
	case PageSizeLegal:
		return 8.5, 14
	case PageSizeTabloid:
		return 11, 17
	default:
		return 8.27, 11.69
	}
}

func injectCSS(html, css string) string {
	style := fmt.Sprintf("<style>%s</style>", css)
	
	if strings.Contains(html, "</head>") {
		return strings.Replace(html, "</head>", style+"</head>", 1)
	}
	
	if strings.Contains(html, "<html>") {
		return strings.Replace(html, "<html>", "<html><head>"+style+"</head>", 1)
	}
	
	return style + html
}

func addWatermark(html, watermark string) string {
	watermarkCSS := `
		<style>
		.pdf-watermark {
			position: fixed;
			top: 50%;
			left: 50%;
			transform: translate(-50%, -50%) rotate(-45deg);
			font-size: 80px;
			color: rgba(0, 0, 0, 0.1);
			z-index: 9999;
			pointer-events: none;
			user-select: none;
			white-space: nowrap;
		}
		</style>
	`
	
	watermarkHTML := fmt.Sprintf(`<div class="pdf-watermark">%s</div>`, watermark)
	
	html = injectCSS(html, watermarkCSS)
	
	if strings.Contains(html, "<body>") {
		return strings.Replace(html, "<body>", "<body>"+watermarkHTML, 1)
	}
	
	return html + watermarkHTML
}

func optimizeHTMLForPDF(html string) string {
	optimizations := map[string]string{
		"@media print": "@media all",
	}

	for old, new := range optimizations {
		html = strings.ReplaceAll(html, old, new)
	}

	if !strings.Contains(html, "<!DOCTYPE") {
		html = "<!DOCTYPE html>" + html
	}

	pdfCSS := `
		<style>
		* {
			-webkit-print-color-adjust: exact !important;
			print-color-adjust: exact !important;
		}
		body {
			margin: 0;
			padding: 0;
		}
		img {
			max-width: 100%;
			height: auto;
		}
		@page {
			margin: 0;
		}
		</style>
	`
	
	html = injectCSS(html, pdfCSS)

	return html
}

func DefaultHeaderTemplate() string {
	return `
		<div style="font-size: 10px; text-align: center; width: 100%;">
			<span class="title"></span>
		</div>
	`
}

func DefaultFooterTemplate() string {
	return `
		<div style="font-size: 10px; text-align: center; width: 100%;">
			<span class="pageNumber"></span> / <span class="totalPages"></span>
		</div>
	`
}

func CreateHeaderFooter(header, footer string) (string, string) {
	if header == "" {
		header = DefaultHeaderTemplate()
	}
	if footer == "" {
		footer = DefaultFooterTemplate()
	}
	return header, footer
}

func EstimatePDFSize(htmlSize int) int64 {
	ratio := 1.5
	return int64(float64(htmlSize) * ratio)
}

func ParsePageSize(s string) (PageSize, error) {
	switch strings.ToLower(s) {
	case "letter":
		return PageSizeLetter, nil
	case "a4":
		return PageSizeA4, nil
	case "a3":
		return PageSizeA3, nil
	case "legal":
		return PageSizeLegal, nil
	case "tabloid":
		return PageSizeTabloid, nil
	case "custom":
		return PageSizeCustom, nil
	default:
		return "", ErrInvalidPageSize
	}
}

func ParseOrientation(s string) (Orientation, error) {
	switch strings.ToLower(s) {
	case "portrait":
		return OrientationPortrait, nil
	case "landscape":
		return OrientationLandscape, nil
	default:
		return "", ErrInvalidOrientation
	}
}

type PDFValidator struct{}

func (v *PDFValidator) ValidatePDF(data []byte) error {
	if len(data) < 4 {
		return errors.New("pdf: data too small")
	}

	if string(data[:4]) != "%PDF" {
		return errors.New("pdf: invalid PDF header")
	}

	if !strings.Contains(string(data), "%%EOF") {
		return errors.New("pdf: missing EOF marker")
	}

	return nil
}

func (v *PDFValidator) GetPDFVersion(data []byte) (string, error) {
	if len(data) < 8 {
		return "", errors.New("pdf: data too small")
	}

	header := string(data[:8])
	if !strings.HasPrefix(header, "%PDF-") {
		return "", errors.New("pdf: invalid PDF header")
	}

	return header[5:8], nil
}

func InchesToPoints(inches float64) float64 {
	return inches * 72
}

func PointsToInches(points float64) float64 {
	return points / 72
}

func MMToInches(mm float64) float64 {
	return mm / 25.4
}

func InchesToMM(inches float64) float64 {
	return inches * 25.4
}
