package attachment

import (
        "bytes"
        "context"
        "fmt"
        "image"
        "os"
        "os/exec"
        "strings"
        "sync"

        "github.com/chai2010/webp"
)

const (
        BackendNative Backend = "native"
)

type NativeConverter struct {
        mu     sync.RWMutex
        config *ConverterConfig
        closed bool
}

func NewNativeConverter(cfg *ConverterConfig) (Converter, error) {
        if cfg == nil {
                cfg = &ConverterConfig{
                        Backend: BackendNative,
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
                        Width:   1920,
                        Height:  0,
                        Quality: 95,
                        Scale:   2.0,
                }
        }
        if cfg.TempDir == "" {
                cfg.TempDir = os.TempDir()
        }

        return &NativeConverter{
                config: cfg,
        }, nil
}

func (c *NativeConverter) Convert(req *ConversionRequest) ([]byte, error) {
        c.mu.RLock()
        closed := c.closed
        c.mu.RUnlock()

        if closed {
                return nil, fmt.Errorf("converter: closed")
        }

        if req.HTML == "" {
                return nil, ErrInvalidHTML
        }

        if req.Context == nil {
                ctx, cancel := context.WithTimeout(context.Background(), 30000000000)
                defer cancel()
                req.Context = ctx
        }

        fmt.Printf("[WKHTML] Converting format=%s html_size=%d\n", req.Format, len(req.HTML))
        switch req.Format {
        case FormatPDF:
                return c.generatePDF(req)
        case FormatJPG:
                return c.generateImage(req, "jpg")
        case FormatPNG:
                return c.generateImage(req, "png")
        case FormatWebP:
                return c.generateImageWebP(req)
        case FormatHEIC:
                return c.generateImageHEIC(req, "heic")
        case FormatHEIF:
                return c.generateImageHEIC(req, "heif")
        default:
                return nil, fmt.Errorf("%w: %s", ErrFormatNotSupported, req.Format)
        }
}

func (c *NativeConverter) generatePDF(req *ConversionRequest) ([]byte, error) {
        htmlFile, err := os.CreateTemp(c.config.TempDir, "convert-*.html")
        if err != nil {
                return nil, fmt.Errorf("converter: create temp html: %w", err)
        }
        defer os.Remove(htmlFile.Name())

        if _, err := htmlFile.WriteString(req.HTML); err != nil {
                htmlFile.Close()
                return nil, fmt.Errorf("converter: write html: %w", err)
        }
        htmlFile.Close()

        outFile, err := os.CreateTemp(c.config.TempDir, "convert-*.pdf")
        if err != nil {
                return nil, fmt.Errorf("converter: create temp output: %w", err)
        }
        outPath := outFile.Name()
        outFile.Close()
        defer os.Remove(outPath)

        opts := c.config.PDFOptions
        args := []string{
                "--quiet",
                "--page-size", opts.PageSize,
                "--margin-top", fmt.Sprintf("%.1fmm", opts.MarginTop*25.4),
                "--margin-bottom", fmt.Sprintf("%.1fmm", opts.MarginBottom*25.4),
                "--margin-left", fmt.Sprintf("%.1fmm", opts.MarginLeft*25.4),
                "--margin-right", fmt.Sprintf("%.1fmm", opts.MarginRight*25.4),
                "--encoding", "utf-8",
                "--no-stop-slow-scripts",
                "--disable-javascript",
                "--dpi", "300",
                "--image-quality", "100",
                "--enable-smart-shrinking",
        }
        if opts.PrintBackground {
                args = append(args, "--print-media-type")
        }
        if opts.Orientation == "landscape" {
                args = append(args, "--orientation", "Landscape")
        }
        args = append(args, htmlFile.Name(), outPath)

        cmd := exec.CommandContext(req.Context, "wkhtmltopdf", args...)
        cmd.Env = append(os.Environ(), "QT_QPA_PLATFORM=offscreen")
        var stderr bytes.Buffer
        cmd.Stderr = &stderr

        if err := cmd.Run(); err != nil {
                return nil, fmt.Errorf("converter: wkhtmltopdf failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
        }

        data, err := os.ReadFile(outPath)
        if err != nil {
                return nil, fmt.Errorf("converter: read pdf output: %w", err)
        }

        return data, nil
}

func (c *NativeConverter) generateImage(req *ConversionRequest, format string) ([]byte, error) {
        htmlFile, err := os.CreateTemp(c.config.TempDir, "convert-*.html")
        if err != nil {
                return nil, fmt.Errorf("converter: create temp html: %w", err)
        }
        defer os.Remove(htmlFile.Name())

        if _, err := htmlFile.WriteString(req.HTML); err != nil {
                htmlFile.Close()
                return nil, fmt.Errorf("converter: write html: %w", err)
        }
        htmlFile.Close()

        outFile, err := os.CreateTemp(c.config.TempDir, fmt.Sprintf("convert-*.%s", format))
        if err != nil {
                return nil, fmt.Errorf("converter: create temp output: %w", err)
        }
        outPath := outFile.Name()
        outFile.Close()
        defer os.Remove(outPath)

        opts := c.config.ImageOptions
        args := []string{
                "--quiet",
                "--width", fmt.Sprintf("%d", opts.Width),
                "--format", format,
                "--encoding", "utf-8",
                "--disable-javascript",
                "--zoom", fmt.Sprintf("%.1f", opts.Scale),
        }
        if opts.Height > 0 {
                args = append(args, "--height", fmt.Sprintf("%d", opts.Height))
        }
        if format == "jpg" || format == "jpeg" {
                args = append(args, "--quality", fmt.Sprintf("%d", opts.Quality))
        }
        args = append(args, htmlFile.Name(), outPath)

        cmd := exec.CommandContext(req.Context, "wkhtmltoimage", args...)
        cmd.Env = append(os.Environ(), "QT_QPA_PLATFORM=offscreen")
        var stderr bytes.Buffer
        cmd.Stderr = &stderr

        if err := cmd.Run(); err != nil {
                return nil, fmt.Errorf("converter: wkhtmltoimage failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
        }

        data, err := os.ReadFile(outPath)
        if err != nil {
                return nil, fmt.Errorf("converter: read image output: %w", err)
        }

        return data, nil
}

func (c *NativeConverter) generateImageWebP(req *ConversionRequest) ([]byte, error) {
        pngData, err := c.generateImage(req, "png")
        if err != nil {
                return nil, fmt.Errorf("converter: webp (png step): %w", err)
        }

        img, _, err := image.Decode(bytes.NewReader(pngData))
        if err != nil {
                return nil, fmt.Errorf("converter: webp decode png: %w", err)
        }

        quality := float32(c.config.ImageOptions.Quality)
        if quality <= 0 {
                quality = 90
        }

        var buf bytes.Buffer
        if err := webp.Encode(&buf, img, &webp.Options{Quality: quality}); err != nil {
                return nil, fmt.Errorf("converter: webp encode: %w", err)
        }

        return buf.Bytes(), nil
}

func (c *NativeConverter) Close() error {
        c.mu.Lock()
        defer c.mu.Unlock()
        c.closed = true
        return nil
}

func init() {
        RegisterBackend(BackendNative, NewNativeConverter)
}

func extractTextFromHTML(htmlContent string) []string {
        content := htmlContent

        headingTags := []struct {
                open   string
                close  string
                prefix string
        }{
                {"<h1", "</h1>", "H1:"},
                {"<h2", "</h2>", "H2:"},
                {"<h3", "</h3>", "H3:"},
        }
        for _, tag := range headingTags {
                for {
                        start := strings.Index(strings.ToLower(content), tag.open)
                        if start == -1 {
                                break
                        }
                        gtPos := strings.Index(content[start:], ">")
                        if gtPos == -1 {
                                break
                        }
                        endTag := strings.Index(strings.ToLower(content[start:]), tag.close)
                        if endTag == -1 {
                                break
                        }
                        inner := content[start+gtPos+1 : start+endTag]
                        inner = stripTags(inner)
                        content = content[:start] + "\n" + tag.prefix + inner + "\n" + content[start+endTag+len(tag.close):]
                }
        }

        boldTags := []struct {
                open  string
                close string
        }{
                {"<b>", "</b>"},
                {"<strong>", "</strong>"},
        }
        for _, tag := range boldTags {
                for {
                        start := strings.Index(strings.ToLower(content), tag.open)
                        if start == -1 {
                                break
                        }
                        endTag := strings.Index(strings.ToLower(content[start:]), tag.close)
                        if endTag == -1 {
                                break
                        }
                        inner := content[start+len(tag.open) : start+endTag]
                        inner = stripTags(inner)
                        content = content[:start] + "BOLD:" + inner + content[start+endTag+len(tag.close):]
                }
        }

        breakTags := []string{"<br>", "<br/>", "<br />", "<p>", "</p>", "<div>", "</div>", "<tr>", "</tr>", "<li>", "</li>"}
        for _, tag := range breakTags {
                content = strings.ReplaceAll(strings.ToLower(content), tag, "\n")
        }

        content = stripTags(content)

        content = strings.ReplaceAll(content, "&nbsp;", " ")
        content = strings.ReplaceAll(content, "&amp;", "&")
        content = strings.ReplaceAll(content, "&lt;", "<")
        content = strings.ReplaceAll(content, "&gt;", ">")
        content = strings.ReplaceAll(content, "&quot;", "\"")
        content = strings.ReplaceAll(content, "&#39;", "'")

        rawLines := strings.Split(content, "\n")
        var lines []string
        for _, line := range rawLines {
                trimmed := strings.TrimSpace(line)
                if trimmed != "" {
                        lines = append(lines, trimmed)
                }
        }

        return lines
}

func stripTags(s string) string {
        var result strings.Builder
        inTag := false
        for _, r := range s {
                if r == '<' {
                        inTag = true
                        continue
                }
                if r == '>' {
                        inTag = false
                        continue
                }
                if !inTag {
                        result.WriteRune(r)
                }
        }
        return result.String()
}

// generateImageHEIC renders HTML to a PNG first via wkhtmltoimage, then
// converts the PNG to HEIC or HEIF using ImageMagick 7 (magick).
func (c *NativeConverter) generateImageHEIC(req *ConversionRequest, ext string) ([]byte, error) {
        // Step 1: render HTML → PNG bytes using the existing wkhtmltoimage path.
        pngReq := &ConversionRequest{
                HTML:    req.HTML,
                Format:  FormatPNG,
                Context: req.Context,
        }
        pngData, err := c.generateImage(pngReq, "png")
        if err != nil {
                return nil, fmt.Errorf("converter: heic/heif png render: %w", err)
        }

        // Step 2: write PNG to a temp file so ImageMagick can read it.
        pngFile, err := os.CreateTemp(c.config.TempDir, "convert-*.png")
        if err != nil {
                return nil, fmt.Errorf("converter: heic create png temp: %w", err)
        }
        defer os.Remove(pngFile.Name())
        if _, err := pngFile.Write(pngData); err != nil {
                pngFile.Close()
                return nil, fmt.Errorf("converter: heic write png temp: %w", err)
        }
        pngFile.Close()

        // Step 3: create output temp file.
        outFile, err := os.CreateTemp(c.config.TempDir, "convert-*."+ext)
        if err != nil {
                return nil, fmt.Errorf("converter: heic create out temp: %w", err)
        }
        outPath := outFile.Name()
        outFile.Close()
        defer os.Remove(outPath)

        // Step 4: convert PNG → HEIC/HEIF via ImageMagick 7.
        cmd := exec.CommandContext(req.Context, "magick", pngFile.Name(), "-quality", "92", outPath)
        var stderr bytes.Buffer
        cmd.Stderr = &stderr
        if err := cmd.Run(); err != nil {
                return nil, fmt.Errorf("converter: magick %s: %w — %s", ext, err, stderr.String())
        }

        data, err := os.ReadFile(outPath)
        if err != nil {
                return nil, fmt.Errorf("converter: heic read output: %w", err)
        }
        if len(data) == 0 {
                return nil, fmt.Errorf("converter: magick %s produced empty output", ext)
        }
        return data, nil
}
