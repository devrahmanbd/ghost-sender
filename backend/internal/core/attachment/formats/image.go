package formats

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

var (
	ErrInvalidDimensions = errors.New("image: invalid dimensions")
	ErrInvalidQuality    = errors.New("image: invalid quality")
	ErrUnsupportedFormat = errors.New("image: unsupported format")
)

type ImageFormat string

const (
	FormatJPEG ImageFormat = "jpeg"
	FormatJPG  ImageFormat = "jpg"
	FormatPNG  ImageFormat = "png"
)

type ImageGenerator struct {
	mu      sync.RWMutex
	config  *ImageConfig
	pool    chan context.Context
	ctx     context.Context
	cancel  context.CancelFunc
	closed  bool
}

type ImageConfig struct {
	Width             int
	Height            int
	Quality           int
	Format            ImageFormat
	ViewportWidth     int
	ViewportHeight    int
	DeviceScaleFactor float64
	Timeout           time.Duration
	PoolSize          int
	FullPage          bool
	BackgroundColor   string
	Compression       CompressionLevel
	OptimizePNG       bool
	WaitForLoad       bool
	WaitTime          time.Duration
}

type CompressionLevel int

const (
	CompressionNone    CompressionLevel = 0
	CompressionDefault CompressionLevel = 1
	CompressionBest    CompressionLevel = 2
)

type ImageRequest struct {
	HTML              string
	Config            *ImageConfig
	Context           context.Context
	CustomCSS         string
	ClipRect          *ClipRect
	OmitBackground    bool
	JavaScriptEnabled bool
}

type ClipRect struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

func NewImageGenerator(cfg *ImageConfig) (*ImageGenerator, error) {
	if cfg == nil {
		cfg = DefaultImageConfig()
	}

	if err := ValidateImageConfig(cfg); err != nil {
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

	ig := &ImageGenerator{
		config: cfg,
		pool:   make(chan context.Context, cfg.PoolSize),
		ctx:    allocCtx,
		cancel: allocCancel,
	}

	for i := 0; i < cfg.PoolSize; i++ {
		browserCtx, _ := chromedp.NewContext(allocCtx)
		ig.pool <- browserCtx
	}

	return ig, nil
}

func DefaultImageConfig() *ImageConfig {
	return &ImageConfig{
		Width:             1024,
		Height:            768,
		Quality:           90,
		Format:            FormatPNG,
		ViewportWidth:     1024,
		ViewportHeight:    768,
		DeviceScaleFactor: 1.0,
		Timeout:           30 * time.Second,
		PoolSize:          4,
		FullPage:          true,
		Compression:       CompressionDefault,
		OptimizePNG:       true,
		WaitForLoad:       true,
		WaitTime:          500 * time.Millisecond,
	}
}

func (ig *ImageGenerator) Generate(req *ImageRequest) ([]byte, error) {
	ig.mu.RLock()
	closed := ig.closed
	ig.mu.RUnlock()

	if closed {
		return nil, errors.New("image: generator closed")
	}

	if req.HTML == "" {
		return nil, errors.New("image: empty HTML content")
	}

	if req.Config == nil {
		req.Config = ig.config
	}

	if req.Context == nil {
		ctx, cancel := context.WithTimeout(context.Background(), req.Config.Timeout)
		defer cancel()
		req.Context = ctx
	}

	var browserCtx context.Context
	select {
	case browserCtx = <-ig.pool:
		defer func() {
			select {
			case ig.pool <- browserCtx:
			default:
			}
		}()
	case <-req.Context.Done():
		return nil, errors.New("image: generation timeout")
	}

	html := req.HTML
	
	if req.CustomCSS != "" {
		html = injectCSSForImage(html, req.CustomCSS)
	}

	html = optimizeHTMLForImage(html, req.Config)

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
		tasks = append(tasks, chromedp.FullScreenshot(&screenshotBuf, req.Config.Quality))
	} else {
		tasks = append(tasks, chromedp.CaptureScreenshot(&screenshotBuf))
	}

	if err := chromedp.Run(browserCtx, tasks); err != nil {
		return nil, fmt.Errorf("image: generation failed: %w", err)
	}

	return ig.processImage(screenshotBuf, req.Config)
}

func (ig *ImageGenerator) processImage(data []byte, cfg *ImageConfig) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("image: decode failed: %w", err)
	}

	if cfg.Width > 0 && cfg.Height > 0 {
		img = resizeImage(img, cfg.Width, cfg.Height)
	}

	var buf bytes.Buffer

	switch cfg.Format {
	case FormatJPEG, FormatJPG:
		opts := &jpeg.Options{Quality: cfg.Quality}
		if err := jpeg.Encode(&buf, img, opts); err != nil {
			return nil, fmt.Errorf("image: jpeg encode failed: %w", err)
		}

	case FormatPNG:
		encoder := &png.Encoder{CompressionLevel: ig.getPNGCompression(cfg.Compression)}
		if err := encoder.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("image: png encode failed: %w", err)
		}

	default:
		return nil, ErrUnsupportedFormat
	}

	return buf.Bytes(), nil
}

func (ig *ImageGenerator) getPNGCompression(level CompressionLevel) png.CompressionLevel {
	switch level {
	case CompressionNone:
		return png.NoCompression
	case CompressionBest:
		return png.BestCompression
	default:
		return png.DefaultCompression
	}
}

func (ig *ImageGenerator) GenerateBatch(requests []*ImageRequest) ([][]byte, []error) {
	results := make([][]byte, len(requests))
	errs := make([]error, len(requests))
	
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, ig.config.PoolSize)

	for i, req := range requests {
		wg.Add(1)
		go func(idx int, request *ImageRequest) {
			defer wg.Done()
			
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			img, err := ig.Generate(request)
			results[idx] = img
			errs[idx] = err
		}(i, req)
	}

	wg.Wait()
	return results, errs
}

func (ig *ImageGenerator) Close() error {
	ig.mu.Lock()
	defer ig.mu.Unlock()

	if ig.closed {
		return nil
	}
	ig.closed = true

	close(ig.pool)
	
	if ig.cancel != nil {
		ig.cancel()
	}

	return nil
}

func ValidateImageConfig(cfg *ImageConfig) error {
	if cfg == nil {
		return errors.New("image: nil config")
	}

	if cfg.ViewportWidth <= 0 || cfg.ViewportHeight <= 0 {
		return ErrInvalidDimensions
	}

	if cfg.Width < 0 || cfg.Height < 0 {
		return ErrInvalidDimensions
	}

	if cfg.Quality < 0 || cfg.Quality > 100 {
		return ErrInvalidQuality
	}

	if cfg.DeviceScaleFactor <= 0 || cfg.DeviceScaleFactor > 3 {
		return errors.New("image: device scale factor must be between 0 and 3")
	}

	switch cfg.Format {
	case FormatJPEG, FormatJPG, FormatPNG:
	default:
		return ErrUnsupportedFormat
	}

	return nil
}

func injectCSSForImage(html, css string) string {
	style := fmt.Sprintf("<style>%s</style>", css)
	
	if strings.Contains(html, "</head>") {
		return strings.Replace(html, "</head>", style+"</head>", 1)
	}
	
	if strings.Contains(html, "<html>") {
		return strings.Replace(html, "<html>", "<html><head>"+style+"</head>", 1)
	}
	
	return style + html
}

func optimizeHTMLForImage(html string, cfg *ImageConfig) string {
	if !strings.Contains(html, "<!DOCTYPE") {
		html = "<!DOCTYPE html>" + html
	}

	imageCSS := `
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
	
	html = injectCSSForImage(html, imageCSS)

	return html
}

func resizeImage(src image.Image, width, height int) image.Image {
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

func EstimateImageSize(width, height int, format ImageFormat, quality int) int64 {
	pixels := int64(width * height)
	
	switch format {
	case FormatJPEG, FormatJPG:
		compressionRatio := 1.0 - (float64(quality) / 200.0)
		return int64(float64(pixels*3) * compressionRatio)
	case FormatPNG:
		return pixels * 4
	default:
		return pixels * 3
	}
}

func ParseImageFormat(s string) (ImageFormat, error) {
	switch strings.ToLower(s) {
	case "jpeg", "jpg":
		return FormatJPEG, nil
	case "png":
		return FormatPNG, nil
	default:
		return "", ErrUnsupportedFormat
	}
}

type ImageValidator struct{}

func (v *ImageValidator) ValidateImage(data []byte) error {
	if len(data) < 8 {
		return errors.New("image: data too small")
	}

	if data[0] == 0xFF && data[1] == 0xD8 {
		return nil
	}

	pngSignature := []byte{137, 80, 78, 71, 13, 10, 26, 10}
	if len(data) >= 8 {
		valid := true
		for i := 0; i < 8; i++ {
			if data[i] != pngSignature[i] {
				valid = false
				break
			}
		}
		if valid {
			return nil
		}
	}

	return errors.New("image: invalid image format")
}

func (v *ImageValidator) GetImageFormat(data []byte) (ImageFormat, error) {
	if len(data) < 8 {
		return "", errors.New("image: data too small")
	}

	if data[0] == 0xFF && data[1] == 0xD8 {
		return FormatJPEG, nil
	}

	pngSignature := []byte{137, 80, 78, 71, 13, 10, 26, 10}
	if len(data) >= 8 {
		valid := true
		for i := 0; i < 8; i++ {
			if data[i] != pngSignature[i] {
				valid = false
				break
			}
		}
		if valid {
			return FormatPNG, nil
		}
	}

	return "", errors.New("image: unknown format")
}

func (v *ImageValidator) GetImageDimensions(data []byte) (width, height int, err error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, 0, fmt.Errorf("image: decode failed: %w", err)
	}

	bounds := img.Bounds()
	return bounds.Dx(), bounds.Dy(), nil
}

func ConvertImageFormat(data []byte, targetFormat ImageFormat, quality int) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("image: decode failed: %w", err)
	}

	var buf bytes.Buffer

	switch targetFormat {
	case FormatJPEG, FormatJPG:
		opts := &jpeg.Options{Quality: quality}
		if err := jpeg.Encode(&buf, img, opts); err != nil {
			return nil, fmt.Errorf("image: jpeg encode failed: %w", err)
		}

	case FormatPNG:
		if err := png.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("image: png encode failed: %w", err)
		}

	default:
		return nil, ErrUnsupportedFormat
	}

	return buf.Bytes(), nil
}

func OptimizeJPEG(data []byte, quality int) ([]byte, error) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("image: decode failed: %w", err)
	}

	var buf bytes.Buffer
	opts := &jpeg.Options{Quality: quality}
	
	if err := jpeg.Encode(&buf, img, opts); err != nil {
		return nil, fmt.Errorf("image: encode failed: %w", err)
	}

	return buf.Bytes(), nil
}

func OptimizePNG(data []byte, compressionLevel CompressionLevel) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("image: decode failed: %w", err)
	}

	var buf bytes.Buffer
	encoder := &png.Encoder{}
	
	switch compressionLevel {
	case CompressionNone:
		encoder.CompressionLevel = png.NoCompression
	case CompressionBest:
		encoder.CompressionLevel = png.BestCompression
	default:
		encoder.CompressionLevel = png.DefaultCompression
	}
	
	if err := encoder.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("image: encode failed: %w", err)
	}

	return buf.Bytes(), nil
}

func GetMimeType(format ImageFormat) string {
	switch format {
	case FormatJPEG, FormatJPG:
		return "image/jpeg"
	case FormatPNG:
		return "image/png"
	default:
		return "application/octet-stream"
	}
}

func GetFileExtension(format ImageFormat) string {
	switch format {
	case FormatJPEG, FormatJPG:
		return ".jpg"
	case FormatPNG:
		return ".png"
	default:
		return ".dat"
	}
}
