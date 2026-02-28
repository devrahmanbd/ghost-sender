package attachment

import (
    "context"
    "errors"
    "fmt"
    "image"
    "image/jpeg"
    "image/png"
    "io"
    "os"
    "strings"
    "sync"
    "time"

    "github.com/chromedp/cdproto/page"
    "github.com/chromedp/chromedp"
)

var (
    ErrBackendNotSupported = errors.New("converter: backend not supported")
    ErrConversionFailed    = errors.New("converter: conversion failed")
    ErrInvalidHTML         = errors.New("converter: invalid HTML")
    ErrFormatNotSupported  = errors.New("converter: format not supported")
)

type Backend string

const (
    BackendChromedp    Backend = "chromedp"
    BackendWkhtmltopdf Backend = "wkhtmltopdf"
    BackendWeasyPrint  Backend = "weasyprint"
)

type ConverterConfig struct {
    Backend          Backend
    ChromedpTimeout  time.Duration
    ChromedpPoolSize int
    TempDir          string
    PDFOptions       *PDFOptions
    ImageOptions     *ImageOptions
}

type PDFOptions struct {
    PageSize        string
    Orientation     string
    MarginTop       float64
    MarginBottom    float64
    MarginLeft      float64
    MarginRight     float64
    PrintBackground bool
    Scale           float64
}

type ImageOptions struct {
    Width   int
    Height  int
    Quality int
    Scale   float64
}

type MultiConverter struct {
    mu        sync.RWMutex
    config    *ConverterConfig
    backends  map[Backend]Converter
    primary   Converter
    fallbacks []Converter
    closed    bool
}

func NewMultiConverter(cfg *ConverterConfig) (*MultiConverter, error) {
    if cfg == nil {
        cfg = &ConverterConfig{
            Backend:          BackendChromedp,
            ChromedpTimeout:  30 * time.Second,
            ChromedpPoolSize: 4,
        }
    }

    if cfg.PDFOptions == nil {
        cfg.PDFOptions = &PDFOptions{
            PageSize:        "A4",
            Orientation:     "portrait",
            MarginTop:       0.4,
            MarginBottom:    0.4,
            MarginLeft:      0.4,
            MarginRight:     0.4,
            PrintBackground: true,
            Scale:           1.0,
        }
    }

    if cfg.ImageOptions == nil {
        cfg.ImageOptions = &ImageOptions{
            Width:   1024,
            Height:  768,
            Quality: 90,
            Scale:   1.0,
        }
    }

    mc := &MultiConverter{
        config:   cfg,
        backends: make(map[Backend]Converter),
    }

    primary, err := mc.createBackend(cfg.Backend)
    if err != nil {
        return nil, fmt.Errorf("create primary backend: %w", err)
    }
    mc.primary = primary
    mc.backends[cfg.Backend] = primary

    return mc, nil
}

func (mc *MultiConverter) createBackend(backend Backend) (Converter, error) {
    switch backend {
    case BackendChromedp:
        return NewChromedpConverter(mc.config)
    case BackendWkhtmltopdf:
        return nil, fmt.Errorf("%w: wkhtmltopdf", ErrBackendNotSupported)
    case BackendWeasyPrint:
        return nil, fmt.Errorf("%w: weasyprint", ErrBackendNotSupported)
    default:
        return nil, fmt.Errorf("%w: %s", ErrBackendNotSupported, backend)
    }
}

func (mc *MultiConverter) Convert(req *ConversionRequest) ([]byte, error) {
    mc.mu.RLock()
    closed := mc.closed
    primary := mc.primary
    mc.mu.RUnlock()

    if closed {
        return nil, errors.New("converter: closed")
    }

    if req.HTML == "" {
        return nil, ErrInvalidHTML
    }

    if req.Context == nil {
        ctx, cancel := context.WithTimeout(context.Background(), mc.config.ChromedpTimeout)
        defer cancel()
        req.Context = ctx
    }

    data, err := primary.Convert(req)
    if err == nil {
        return data, nil
    }

    for _, fallback := range mc.fallbacks {
        data, err = fallback.Convert(req)
        if err == nil {
            return data, nil
        }
    }

    return nil, fmt.Errorf("%w: all backends failed", ErrConversionFailed)
}

func (mc *MultiConverter) AddFallback(backend Backend) error {
    mc.mu.Lock()
    defer mc.mu.Unlock()

    if mc.closed {
        return errors.New("converter: closed")
    }

    if conv, exists := mc.backends[backend]; exists {
        mc.fallbacks = append(mc.fallbacks, conv)
        return nil
    }

    conv, err := mc.createBackend(backend)
    if err != nil {
        return err
    }

    mc.backends[backend] = conv
    mc.fallbacks = append(mc.fallbacks, conv)
    return nil
}

func (mc *MultiConverter) Close() error {
    mc.mu.Lock()
    defer mc.mu.Unlock()

    if mc.closed {
        return nil
    }
    mc.closed = true

    for _, conv := range mc.backends {
        _ = conv.Close()
    }

    return nil
}

type ChromedpConverter struct {
    mu     sync.RWMutex
    config *ConverterConfig
    ctx    context.Context
    cancel context.CancelFunc
    pool   chan context.Context
    closed bool
}

func NewChromedpConverter(cfg *ConverterConfig) (Converter, error) {  // ← FIXED: Return Converter interface
    if cfg.ChromedpPoolSize <= 0 {
        cfg.ChromedpPoolSize = 4
    }

    opts := append(chromedp.DefaultExecAllocatorOptions[:],
        chromedp.DisableGPU,
        chromedp.NoSandbox,
        chromedp.Headless,
    )

    ctx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)

    cc := &ChromedpConverter{
        config: cfg,
        ctx:    ctx,
        cancel: cancel,
        pool:   make(chan context.Context, cfg.ChromedpPoolSize),
    }

    for i := 0; i < cfg.ChromedpPoolSize; i++ {
        browserCtx, _ := chromedp.NewContext(ctx)
        cc.pool <- browserCtx
    }

    return cc, nil
}

func (cc *ChromedpConverter) Convert(req *ConversionRequest) ([]byte, error) {
    cc.mu.RLock()
    closed := cc.closed
    cc.mu.RUnlock()

    if closed {
        return nil, errors.New("converter: closed")
    }

    var browserCtx context.Context
    select {
    case browserCtx = <-cc.pool:
        defer func() {
            select {
            case cc.pool <- browserCtx:
            default:
            }
        }()
    case <-req.Context.Done():
        return nil, req.Context.Err()
    }

    switch req.Format {
    case FormatPDF:
        return cc.convertToPDF(browserCtx, req)
    case FormatJPG, FormatPNG:  // ← REMOVED WebP support (no encoder available)
        return cc.convertToImage(browserCtx, req)
    default:
        return nil, ErrFormatNotSupported
    }
}

func (cc *ChromedpConverter) convertToPDF(browserCtx context.Context, req *ConversionRequest) ([]byte, error) {
    opts := cc.config.PDFOptions
    
    printParams := page.PrintToPDF().
        WithPrintBackground(opts.PrintBackground).
        WithScale(opts.Scale).
        WithMarginTop(opts.MarginTop).
        WithMarginBottom(opts.MarginBottom).
        WithMarginLeft(opts.MarginLeft).
        WithMarginRight(opts.MarginRight)

    if opts.Orientation == "landscape" {
        printParams = printParams.WithLandscape(true)
    }

    var pdfBuf []byte
    
    err := chromedp.Run(browserCtx,
        chromedp.Navigate("about:blank"),
        chromedp.ActionFunc(func(ctx context.Context) error {
            frameTree, err := page.GetFrameTree().Do(ctx)
            if err != nil {
                return err
            }
            return page.SetDocumentContent(frameTree.Frame.ID, req.HTML).Do(ctx)
        }),
        chromedp.ActionFunc(func(ctx context.Context) error {
            var err error
            pdfBuf, _, err = printParams.Do(ctx)
            return err
        }),
    )

    if err != nil {
        return nil, fmt.Errorf("%w: %v", ErrConversionFailed, err)
    }

    return pdfBuf, nil
}

func (cc *ChromedpConverter) convertToImage(browserCtx context.Context, req *ConversionRequest) ([]byte, error) {
    opts := cc.config.ImageOptions
    
    var screenshotBuf []byte
    
    err := chromedp.Run(browserCtx,
        chromedp.Navigate("about:blank"),
        chromedp.ActionFunc(func(ctx context.Context) error {
            frameTree, err := page.GetFrameTree().Do(ctx)
            if err != nil {
                return err
            }
            return page.SetDocumentContent(frameTree.Frame.ID, req.HTML).Do(ctx)
        }),
        chromedp.EmulateViewport(int64(opts.Width), int64(opts.Height)),
        chromedp.FullScreenshot(&screenshotBuf, opts.Quality),
    )

    if err != nil {
        return nil, fmt.Errorf("%w: %v", ErrConversionFailed, err)
    }

    return cc.convertImageFormat(screenshotBuf, req.Format, opts.Quality)
}

func (cc *ChromedpConverter) convertImageFormat(data []byte, format Format, quality int) ([]byte, error) {
    if format == FormatPNG {
        return data, nil
    }

    img, _, err := image.Decode(strings.NewReader(string(data)))
    if err != nil {
        return nil, fmt.Errorf("decode image: %w", err)
    }

    tmpFile, err := os.CreateTemp(cc.config.TempDir, fmt.Sprintf("convert-*.%s", format))
    if err != nil {
        return nil, fmt.Errorf("create temp file: %w", err)
    }
    defer os.Remove(tmpFile.Name())
    defer tmpFile.Close()

    switch format {
    case FormatJPG:
        err = jpeg.Encode(tmpFile, img, &jpeg.Options{Quality: quality})
    case FormatPNG:
        err = png.Encode(tmpFile, img)
    default:
        return nil, ErrFormatNotSupported  // ← REMOVED WebP encoding
    }

    if err != nil {
        return nil, fmt.Errorf("encode image: %w", err)
    }

    tmpFile.Seek(0, 0)
    result, err := io.ReadAll(tmpFile)
    if err != nil {
        return nil, fmt.Errorf("read result: %w", err)
    }

    return result, nil
}

func (cc *ChromedpConverter) Close() error {
    cc.mu.Lock()
    defer cc.mu.Unlock()

    if cc.closed {
        return nil
    }
    cc.closed = true

    close(cc.pool)
    
    if cc.cancel != nil {
        cc.cancel()
    }

    return nil
}

type ConverterFactory struct {
    mu        sync.RWMutex
    builders  map[Backend]ConverterBuilder
    instances map[Backend]Converter
}

type ConverterBuilder func(cfg *ConverterConfig) (Converter, error)

var defaultFactory = &ConverterFactory{
    builders:  make(map[Backend]ConverterBuilder),
    instances: make(map[Backend]Converter),
}

func init() {
    RegisterBackend(BackendChromedp, NewChromedpConverter)  // ← FIXED: Now works since return type is Converter
}

func RegisterBackend(backend Backend, builder ConverterBuilder) {
    defaultFactory.mu.Lock()
    defer defaultFactory.mu.Unlock()
    defaultFactory.builders[backend] = builder
}

func GetConverter(backend Backend, cfg *ConverterConfig) (Converter, error) {
    defaultFactory.mu.Lock()
    defer defaultFactory.mu.Unlock()

    if conv, exists := defaultFactory.instances[backend]; exists {
        return conv, nil
    }

    builder, exists := defaultFactory.builders[backend]
    if !exists {
        return nil, fmt.Errorf("%w: %s", ErrBackendNotSupported, backend)
    }

    conv, err := builder(cfg)
    if err != nil {
        return nil, err
    }

    defaultFactory.instances[backend] = conv
    return conv, nil
}

func ValidateHTML(html string) error {
    if html == "" {
        return ErrInvalidHTML
    }

    html = strings.TrimSpace(html)
    
    if !strings.Contains(html, "<") || !strings.Contains(html, ">") {
        return ErrInvalidHTML
    }

    return nil
}

func EstimateConversionTime(format Format, htmlSize int) time.Duration {
    baseTime := 2 * time.Second
    
    switch format {
    case FormatPDF:
        baseTime = 3 * time.Second
    case FormatJPG, FormatPNG:
        baseTime = 2 * time.Second
    case FormatWebP:
        baseTime = 2500 * time.Millisecond
    }

    sizeMultiplier := float64(htmlSize) / 100000.0
    if sizeMultiplier < 1 {
        sizeMultiplier = 1
    }

    return time.Duration(float64(baseTime) * sizeMultiplier)
}

func ParseFormat(s string) (Format, error) {
    s = strings.ToLower(strings.TrimSpace(s))
    switch s {
    case "pdf":
        return FormatPDF, nil
    case "jpg", "jpeg":
        return FormatJPG, nil
    case "png":
        return FormatPNG, nil
    case "webp":
        return FormatWebP, nil
    case "heic":
        return FormatHEIC, nil
    case "heif":
        return FormatHEIF, nil
    default:
        return "", fmt.Errorf("%w: %s", ErrFormatNotSupported, s)
    }
}

func GetMIMEType(format Format) string {
    switch format {
    case FormatPDF:
        return "application/pdf"
    case FormatJPG:
        return "image/jpeg"
    case FormatPNG:
        return "image/png"
    case FormatWebP:
        return "image/webp"
    case FormatHEIC:
        return "image/heic"
    case FormatHEIF:
        return "image/heif"
    default:
        return "application/octet-stream"
    }
}

func GetFileExtension(format Format) string {
    switch format {
    case FormatPDF:
        return ".pdf"
    case FormatJPG:
        return ".jpg"
    case FormatPNG:
        return ".png"
    case FormatWebP:
        return ".webp"
    case FormatHEIC:
        return ".heic"
    case FormatHEIF:
        return ".heif"
    default:
        return ".dat"
    }
}

type ConversionStats struct {
    TotalConversions uint64
    SuccessCount     uint64
    FailureCount     uint64
    AvgDuration      time.Duration
    ByFormat         map[Format]uint64
    ByBackend        map[Backend]uint64
}

type ConversionMetrics struct {
    mu    sync.RWMutex
    stats ConversionStats
}

func (m *ConversionMetrics) RecordConversion(format Format, backend Backend, duration time.Duration, success bool) {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.stats.TotalConversions++
    if success {
        m.stats.SuccessCount++
    } else {
        m.stats.FailureCount++
    }

    if m.stats.ByFormat == nil {
        m.stats.ByFormat = make(map[Format]uint64)
    }
    m.stats.ByFormat[format]++

    if m.stats.ByBackend == nil {
        m.stats.ByBackend = make(map[Backend]uint64)
    }
    m.stats.ByBackend[backend]++

    totalDur := time.Duration(m.stats.TotalConversions-1) * m.stats.AvgDuration
    m.stats.AvgDuration = (totalDur + duration) / time.Duration(m.stats.TotalConversions)
}

func (m *ConversionMetrics) GetStats() ConversionStats {
    m.mu.RLock()
    defer m.mu.RUnlock()

    snapshot := m.stats
    snapshot.ByFormat = make(map[Format]uint64)
    snapshot.ByBackend = make(map[Backend]uint64)

    for k, v := range m.stats.ByFormat {
        snapshot.ByFormat[k] = v
    }
    for k, v := range m.stats.ByBackend {
        snapshot.ByBackend[k] = v
    }

    return snapshot
}

func OptimizeHTML(html string) string {
    html = strings.TrimSpace(html)
    
    replacements := map[string]string{
        "\r\n": "\n",
        "\t":   " ",
    }
    
    for old, new := range replacements {
        html = strings.ReplaceAll(html, old, new)
    }

    lines := strings.Split(html, "\n")
    result := make([]string, 0, len(lines))
    
    for _, line := range lines {
        trimmed := strings.TrimSpace(line)
        if trimmed != "" {
            result = append(result, trimmed)
        }
    }

    return strings.Join(result, "\n")
}

func ValidateConversionResult(data []byte, format Format) error {
    if len(data) == 0 {
        return errors.New("empty conversion result")
    }

    switch format {
    case FormatPDF:
        if len(data) < 4 || string(data[:4]) != "%PDF" {
            return errors.New("invalid PDF data")
        }
    case FormatJPG:
        if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
            return errors.New("invalid JPEG data")
        }
    case FormatPNG:
        if len(data) < 8 {
            return errors.New("invalid PNG data")
        }
        pngSignature := []byte{137, 80, 78, 71, 13, 10, 26, 10}
        for i := 0; i < 8; i++ {
            if data[i] != pngSignature[i] {
                return errors.New("invalid PNG signature")
            }
        }
    }

    return nil
}
