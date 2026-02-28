package formats

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"strings"
	"sync"
	"time"

	"github.com/chai2010/webp"
	"github.com/chromedp/chromedp"
)

var (
	ErrWebPEncoding = errors.New("webp: encoding failed")
	ErrInvalidWebPQuality = errors.New("webp: invalid quality value")
)

type WebPGenerator struct {
	mu      sync.RWMutex
	config  *WebPConfig
	pool    chan context.Context
	ctx     context.Context
	cancel  context.CancelFunc
	closed  bool
}

type WebPConfig struct {
	Width             int
	Height            int
	Quality           float32
	Lossless          bool
	ViewportWidth     int
	ViewportHeight    int
	DeviceScaleFactor float64
	Timeout           time.Duration
	PoolSize          int
	FullPage          bool
	BackgroundColor   string
	Method            CompressionMethod
	WaitForLoad       bool
	WaitTime          time.Duration
	ExactQuality      bool
	AlphaQuality      int
}

type CompressionMethod int

const (
	MethodLossy    CompressionMethod = 0
	MethodLossless CompressionMethod = 1
	MethodNear     CompressionMethod = 2
)

type WebPRequest struct {
	HTML              string
	Config            *WebPConfig
	Context           context.Context
	CustomCSS         string
	OmitBackground    bool
	JavaScriptEnabled bool
	Metadata          *WebPMetadata
}

type WebPMetadata struct {
	Title       string
	Author      string
	Description string
	Copyright   string
}

type WebPOptions struct {
	Quality      float32
	Lossless     bool
	Exact        bool
	Method       int
	AlphaQuality int
}

func NewWebPGenerator(cfg *WebPConfig) (*WebPGenerator, error) {
	if cfg == nil {
		cfg = DefaultWebPConfig()
	}

	if err := ValidateWebPConfig(cfg); err != nil {
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
		chromedp.Flag("force-color-profile", "srgb"),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)

	wg := &WebPGenerator{
		config: cfg,
		pool:   make(chan context.Context, cfg.PoolSize),
		ctx:    allocCtx,
		cancel: allocCancel,
	}

	for i := 0; i < cfg.PoolSize; i++ {
		browserCtx, _ := chromedp.NewContext(allocCtx)
		wg.pool <- browserCtx
	}

	return wg, nil
}

func DefaultWebPConfig() *WebPConfig {
	return &WebPConfig{
		Width:             1024,
		Height:            768,
		Quality:           80.0,
		Lossless:          false,
		ViewportWidth:     1024,
		ViewportHeight:    768,
		DeviceScaleFactor: 1.0,
		Timeout:           30 * time.Second,
		PoolSize:          4,
		FullPage:          true,
		Method:            MethodLossy,
		WaitForLoad:       true,
		WaitTime:          500 * time.Millisecond,
		ExactQuality:      false,
		AlphaQuality:      100,
	}
}

func (wg *WebPGenerator) Generate(req *WebPRequest) ([]byte, error) {
	wg.mu.RLock()
	closed := wg.closed
	wg.mu.RUnlock()

	if closed {
		return nil, errors.New("webp: generator closed")
	}

	if req.HTML == "" {
		return nil, errors.New("webp: empty HTML content")
	}

	if req.Config == nil {
		req.Config = wg.config
	}

	if req.Context == nil {
		ctx, cancel := context.WithTimeout(context.Background(), req.Config.Timeout)
		defer cancel()
		req.Context = ctx
	}

	var browserCtx context.Context
	select {
	case browserCtx = <-wg.pool:
		defer func() {
			select {
			case wg.pool <- browserCtx:
			default:
			}
		}()
	case <-req.Context.Done():
		return nil, errors.New("webp: generation timeout")
	}

	html := req.HTML
	
	if req.CustomCSS != "" {
		html = injectWebPCSS(html, req.CustomCSS)
	}

	html = optimizeHTMLForWebP(html, req.Config)

	var screenshotBuf []byte

	tasks := chromedp.Tasks{
		chromedp.Navigate("about:blank"),
		chromedp.EmulateViewport(
			int64(req.Config.ViewportWidth),
			int64(req.Config.ViewportHeight),
			chromedp.EmulateScale(req.Config.DeviceScaleFactor),
		),
	}

	if req.Config.BackgroundColor != "" {
		tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
			css := fmt.Sprintf("body { background-color: %s !important; }", req.Config.BackgroundColor)
			return chromedp.Run(ctx, chromedp.Evaluate(
				fmt.Sprintf(`document.head.insertAdjacentHTML('beforeend', '<style>%s</style>')`, css),
				nil,
			))
		}))
	}

	tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.Run(ctx, chromedp.Navigate("data:text/html,"+html))
	}))

	if req.Config.WaitForLoad {
		tasks = append(tasks, chromedp.Sleep(req.Config.WaitTime))
	}

	if req.Config.FullPage {
		tasks = append(tasks, chromedp.FullScreenshot(&screenshotBuf, 100))
	} else {
		tasks = append(tasks, chromedp.CaptureScreenshot(&screenshotBuf))
	}

	if err := chromedp.Run(browserCtx, tasks); err != nil {
		return nil, fmt.Errorf("webp: generation failed: %w", err)
	}

	return wg.convertToWebP(screenshotBuf, req.Config)
}

func (wg *WebPGenerator) convertToWebP(data []byte, cfg *WebPConfig) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("webp: decode failed: %w", err)
	}

	if cfg.Width > 0 && cfg.Height > 0 {
		img = resizeWebPImage(img, cfg.Width, cfg.Height)
	}

	var buf bytes.Buffer

	opts := &webp.Options{
		Lossless: cfg.Lossless,
		Quality:  cfg.Quality,
		Exact:    cfg.ExactQuality,
	}

	if err := webp.Encode(&buf, img, opts); err != nil {
		return nil, fmt.Errorf("webp: encode failed: %w", err)
	}

	return buf.Bytes(), nil
}

func (wg *WebPGenerator) GenerateBatch(requests []*WebPRequest) ([][]byte, []error) {
	results := make([][]byte, len(requests))
	errs := make([]error, len(requests))
	
	var wgSync sync.WaitGroup
	semaphore := make(chan struct{}, wg.config.PoolSize)

	for i, req := range requests {
		wgSync.Add(1)
		go func(idx int, request *WebPRequest) {
			defer wgSync.Done()
			
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			webp, err := wg.Generate(request)
			results[idx] = webp
			errs[idx] = err
		}(i, req)
	}

	wgSync.Wait()
	return results, errs
}

func (wg *WebPGenerator) Close() error {
	wg.mu.Lock()
	defer wg.mu.Unlock()

	if wg.closed {
		return nil
	}
	wg.closed = true

	close(wg.pool)
	
	if wg.cancel != nil {
		wg.cancel()
	}

	return nil
}

func ValidateWebPConfig(cfg *WebPConfig) error {
	if cfg == nil {
		return errors.New("webp: nil config")
	}

	if cfg.ViewportWidth <= 0 || cfg.ViewportHeight <= 0 {
		return errors.New("webp: invalid viewport dimensions")
	}

	if cfg.Width < 0 || cfg.Height < 0 {
		return errors.New("webp: invalid dimensions")
	}

	if cfg.Quality < 0 || cfg.Quality > 100 {
		return ErrInvalidWebPQuality
	}

	if cfg.DeviceScaleFactor <= 0 || cfg.DeviceScaleFactor > 3 {
		return errors.New("webp: device scale factor must be between 0 and 3")
	}

	if cfg.AlphaQuality < 0 || cfg.AlphaQuality > 100 {
		return errors.New("webp: alpha quality must be between 0 and 100")
	}

	return nil
}

func injectWebPCSS(html, css string) string {
	style := fmt.Sprintf("<style>%s</style>", css)
	
	if strings.Contains(html, "</head>") {
		return strings.Replace(html, "</head>", style+"</head>", 1)
	}
	
	if strings.Contains(html, "<html>") {
		return strings.Replace(html, "<html>", "<html><head>"+style+"</head>", 1)
	}
	
	return style + html
}

func optimizeHTMLForWebP(html string, cfg *WebPConfig) string {
	if !strings.Contains(html, "<!DOCTYPE") {
		html = "<!DOCTYPE html>" + html
	}

	webpCSS := `
		<style>
		* {
			-webkit-font-smoothing: antialiased;
			-moz-osx-font-smoothing: grayscale;
		}
		body {
			margin: 0;
			padding: 0;
		}
		img {
			max-width: 100%;
			height: auto;
		}
		</style>
	`
	
	html = injectWebPCSS(html, webpCSS)

	return html
}

func resizeWebPImage(src image.Image, width, height int) image.Image {
	bounds := src.Bounds()
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()

	if srcWidth == width && srcHeight == height {
		return src
	}

	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	xRatio := float64(srcWidth) / float64(width)
	yRatio := float64(srcHeight) / float64(height)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			srcX := int(float64(x) * xRatio)
			srcY := int(float64(y) * yRatio)
			
			if srcX >= srcWidth {
				srcX = srcWidth - 1
			}
			if srcY >= srcHeight {
				srcY = srcHeight - 1
			}

			dst.Set(x, y, src.At(srcX, srcY))
		}
	}

	return dst
}

func ConvertToWebP(data []byte, quality float32, lossless bool) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("webp: decode failed: %w", err)
	}

	var buf bytes.Buffer
	opts := &webp.Options{
		Lossless: lossless,
		Quality:  quality,
	}

	if err := webp.Encode(&buf, img, opts); err != nil {
		return nil, fmt.Errorf("webp: encode failed: %w", err)
	}

	return buf.Bytes(), nil
}

func ConvertFromWebP(data []byte) (image.Image, error) {
	img, err := webp.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("webp: decode failed: %w", err)
	}
	return img, nil
}

type WebPValidator struct{}

func (v *WebPValidator) ValidateWebP(data []byte) error {
	if len(data) < 12 {
		return errors.New("webp: data too small")
	}

	if string(data[0:4]) != "RIFF" {
		return errors.New("webp: invalid RIFF header")
	}

	if string(data[8:12]) != "WEBP" {
		return errors.New("webp: invalid WEBP signature")
	}

	return nil
}

func (v *WebPValidator) GetWebPInfo(data []byte) (width, height int, hasAlpha bool, err error) {
	img, err := webp.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, 0, false, fmt.Errorf("webp: decode failed: %w", err)
	}

	bounds := img.Bounds()
	width = bounds.Dx()
	height = bounds.Dy()

	_, hasAlpha = img.(*image.NRGBA)

	return width, height, hasAlpha, nil
}

func EstimateWebPSize(width, height int, quality float32, lossless bool) int64 {
	pixels := int64(width * height)
	
	if lossless {
		return pixels * 3
	}

	compressionRatio := 1.0 - (float64(quality) / 150.0)
	return int64(float64(pixels*3) * compressionRatio)
}

func OptimizeWebP(data []byte, targetQuality float32) ([]byte, error) {
	img, err := webp.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("webp: decode failed: %w", err)
	}

	var buf bytes.Buffer
	opts := &webp.Options{
		Lossless: false,
		Quality:  targetQuality,
	}

	if err := webp.Encode(&buf, img, opts); err != nil {
		return nil, fmt.Errorf("webp: encode failed: %w", err)
	}

	return buf.Bytes(), nil
}

func GetWebPMimeType() string {
	return "image/webp"
}

func GetWebPExtension() string {
	return ".webp"
}

func ParseCompressionMethod(method string) (CompressionMethod, error) {
	switch strings.ToLower(method) {
	case "lossy":
		return MethodLossy, nil
	case "lossless":
		return MethodLossless, nil
	case "near":
		return MethodNear, nil
	default:
		return MethodLossy, errors.New("webp: invalid compression method")
	}
}

func CalculateOptimalQuality(targetSize int64, width, height int) float32 {
	pixels := float64(width * height)
	bytesPerPixel := float64(targetSize) / pixels
	
	quality := (1.0 - (bytesPerPixel / 3.0)) * 150.0
	
	if quality < 0 {
		return 0
	}
	if quality > 100 {
		return 100
	}
	
	return float32(quality)
}

func CompareWebPQuality(original, compressed []byte) (float64, error) {
	imgOrig, err := webp.Decode(bytes.NewReader(original))
	if err != nil {
		return 0, fmt.Errorf("webp: decode original failed: %w", err)
	}

	imgComp, err := webp.Decode(bytes.NewReader(compressed))
	if err != nil {
		return 0, fmt.Errorf("webp: decode compressed failed: %w", err)
	}

	boundsOrig := imgOrig.Bounds()
	boundsComp := imgComp.Bounds()

	if boundsOrig != boundsComp {
		return 0, errors.New("webp: image dimensions mismatch")
	}

	var totalDiff float64
	var pixelCount int64

	for y := boundsOrig.Min.Y; y < boundsOrig.Max.Y; y++ {
		for x := boundsOrig.Min.X; x < boundsOrig.Max.X; x++ {
			r1, g1, b1, _ := imgOrig.At(x, y).RGBA()
			r2, g2, b2, _ := imgComp.At(x, y).RGBA()

			dr := float64(r1) - float64(r2)
			dg := float64(g1) - float64(g2)
			db := float64(b1) - float64(b2)

			totalDiff += (dr*dr + dg*dg + db*db)
			pixelCount++
		}
	}

	mse := totalDiff / float64(pixelCount)
	if mse == 0 {
		return 100.0, nil
	}

	maxVal := 65535.0 * 65535.0
	psnr := 10.0 * (20.0 - (10.0 * (mse / maxVal)))

	return psnr, nil
}

func BatchConvertToWebP(images [][]byte, quality float32, lossless bool) ([][]byte, []error) {
	results := make([][]byte, len(images))
	errs := make([]error, len(images))
	
	var wg sync.WaitGroup
	
	for i, imgData := range images {
		wg.Add(1)
		go func(idx int, data []byte) {
			defer wg.Done()
			webpData, err := ConvertToWebP(data, quality, lossless)
			results[idx] = webpData
			errs[idx] = err
		}(i, imgData)
	}
	
	wg.Wait()
	return results, errs
}

func GetCompressionRatio(original, compressed []byte) float64 {
	if len(original) == 0 {
		return 0
	}
	return float64(len(compressed)) / float64(len(original))
}
