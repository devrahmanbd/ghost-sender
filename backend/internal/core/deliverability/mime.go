package deliverability

import (
    "bufio"
    "bytes"
    "crypto/rand"
    "encoding/base64"
    "encoding/hex"
    "fmt"
    "io"
    "mime"
    "mime/multipart"
    "mime/quotedprintable"
    "net/textproto"
    "os"
    "path/filepath"
    "strings"
    "time"

    "email-campaign-system/internal/models"
)

type MIMEBuilder struct {
    headers     *EmailHeaders
    htmlContent string
    textContent string
    attachments []*models.Attachment
    inlineFiles []*models.Attachment
    boundary    string
    config      *MIMEConfig
}

type MIMEConfig struct {
    EnableMultipart      bool
    EnableAlternative    bool
    PreferredEncoding    EncodingType
    ChunkSize            int
    AutoGenerateText     bool
    MaxLineLength        int
    UseQuotedPrintable   bool
    EnableInlineImages   bool
}

type EncodingType string

const (
    EncodingBase64          EncodingType = "base64"
    EncodingQuotedPrintable EncodingType = "quoted-printable"
    Encoding7Bit            EncodingType = "7bit"
    Encoding8Bit            EncodingType = "8bit"
    EncodingBinary          EncodingType = "binary"
)

type MIMEPart struct {
    ContentType             string
    ContentTransferEncoding string
    Content                 []byte
    Headers                 map[string]string
    Boundary                string
    Parts                   []*MIMEPart
}

type MIMEMessage struct {
    Headers map[string]string
    Body    string
    Raw     []byte
    Size    int64
}

func NewMIMEBuilder() *MIMEBuilder {
    return &MIMEBuilder{
        config:      DefaultMIMEConfig(),
        attachments: make([]*models.Attachment, 0),
        inlineFiles: make([]*models.Attachment, 0),
    }
}

func DefaultMIMEConfig() *MIMEConfig {
    return &MIMEConfig{
        EnableMultipart:      true,
        EnableAlternative:    true,
        PreferredEncoding:    EncodingBase64,
        ChunkSize:            76,
        AutoGenerateText:     true,
        MaxLineLength:        998,
        UseQuotedPrintable:   false,
        EnableInlineImages:   true,
    }
}

func (mb *MIMEBuilder) WithHeaders(headers *EmailHeaders) *MIMEBuilder {
    mb.headers = headers
    return mb
}

func (mb *MIMEBuilder) WithHTML(html string) *MIMEBuilder {
    mb.htmlContent = html
    return mb
}

func (mb *MIMEBuilder) WithText(text string) *MIMEBuilder {
    mb.textContent = text
    return mb
}

func (mb *MIMEBuilder) WithAttachment(attachment *models.Attachment) *MIMEBuilder {
    mb.attachments = append(mb.attachments, attachment)
    return mb
}

func (mb *MIMEBuilder) WithInlineFile(file *models.Attachment) *MIMEBuilder {
    mb.inlineFiles = append(mb.inlineFiles, file)
    return mb
}

func (mb *MIMEBuilder) WithConfig(config *MIMEConfig) *MIMEBuilder {
    mb.config = config
    return mb
}

func (mb *MIMEBuilder) Build() (*MIMEMessage, error) {
    mb.boundary = generateBoundary()

    if mb.config.AutoGenerateText && mb.textContent == "" && mb.htmlContent != "" {
        mb.textContent = htmlToText(mb.htmlContent)
    }

    hasAttachments := len(mb.attachments) > 0
    hasInline := len(mb.inlineFiles) > 0
    hasHTML := mb.htmlContent != ""
    hasText := mb.textContent != ""

    var buffer bytes.Buffer

    if mb.headers != nil {
        headers := mb.headers.ToMap()
        for key, value := range headers {
            if key != "Content-Type" && key != "Content-Transfer-Encoding" {
                buffer.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
            }
        }
    }

    if !hasAttachments && !hasInline {
        if hasHTML && hasText {
            buffer.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", mb.boundary))
            buffer.WriteString("MIME-Version: 1.0\r\n")
            buffer.WriteString("\r\n")
            buffer.WriteString(mb.buildAlternativePart())
        } else if hasHTML {
            buffer.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
            buffer.WriteString(fmt.Sprintf("Content-Transfer-Encoding: %s\r\n", mb.config.PreferredEncoding))
            buffer.WriteString("\r\n")
            buffer.WriteString(mb.encodeContent([]byte(mb.htmlContent)))
        } else if hasText {
            buffer.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
            buffer.WriteString(fmt.Sprintf("Content-Transfer-Encoding: %s\r\n", mb.config.PreferredEncoding))
            buffer.WriteString("\r\n")
            buffer.WriteString(mb.encodeContent([]byte(mb.textContent)))
        }
    } else {
        mixedBoundary := generateBoundary()
        buffer.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n", mixedBoundary))
        buffer.WriteString("MIME-Version: 1.0\r\n")
        buffer.WriteString("\r\n")

        if hasHTML && hasText {
            buffer.WriteString(fmt.Sprintf("--%s\r\n", mixedBoundary))
            buffer.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", mb.boundary))
            buffer.WriteString("\r\n")
            buffer.WriteString(mb.buildAlternativePart())
        } else if hasHTML {
            buffer.WriteString(fmt.Sprintf("--%s\r\n", mixedBoundary))
            buffer.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
            buffer.WriteString(fmt.Sprintf("Content-Transfer-Encoding: %s\r\n", mb.config.PreferredEncoding))
            buffer.WriteString("\r\n")
            buffer.WriteString(mb.encodeContent([]byte(mb.htmlContent)))
            buffer.WriteString("\r\n")
        } else if hasText {
            buffer.WriteString(fmt.Sprintf("--%s\r\n", mixedBoundary))
            buffer.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
            buffer.WriteString(fmt.Sprintf("Content-Transfer-Encoding: %s\r\n", mb.config.PreferredEncoding))
            buffer.WriteString("\r\n")
            buffer.WriteString(mb.encodeContent([]byte(mb.textContent)))
            buffer.WriteString("\r\n")
        }

        for _, inline := range mb.inlineFiles {
            buffer.WriteString(fmt.Sprintf("--%s\r\n", mixedBoundary))
            part, err := mb.buildAttachmentPart(inline, true)
            if err != nil {
                return nil, fmt.Errorf("failed to build inline attachment: %w", err)
            }
            buffer.WriteString(part)
        }

        for _, attachment := range mb.attachments {
            buffer.WriteString(fmt.Sprintf("--%s\r\n", mixedBoundary))
            part, err := mb.buildAttachmentPart(attachment, false)
            if err != nil {
                return nil, fmt.Errorf("failed to build attachment: %w", err)
            }
            buffer.WriteString(part)
        }

        buffer.WriteString(fmt.Sprintf("--%s--\r\n", mixedBoundary))
    }

    raw := buffer.Bytes()
    return &MIMEMessage{
        Headers: mb.headers.ToMap(),
        Body:    buffer.String(),
        Raw:     raw,
        Size:    int64(len(raw)),
    }, nil
}

func (mb *MIMEBuilder) buildAlternativePart() string {
    var buffer bytes.Buffer

    buffer.WriteString(fmt.Sprintf("--%s\r\n", mb.boundary))
    buffer.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
    buffer.WriteString(fmt.Sprintf("Content-Transfer-Encoding: %s\r\n", mb.config.PreferredEncoding))
    buffer.WriteString("\r\n")
    buffer.WriteString(mb.encodeContent([]byte(mb.textContent)))
    buffer.WriteString("\r\n")

    buffer.WriteString(fmt.Sprintf("--%s\r\n", mb.boundary))
    buffer.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
    buffer.WriteString(fmt.Sprintf("Content-Transfer-Encoding: %s\r\n", mb.config.PreferredEncoding))
    buffer.WriteString("\r\n")
    buffer.WriteString(mb.encodeContent([]byte(mb.htmlContent)))
    buffer.WriteString("\r\n")

    buffer.WriteString(fmt.Sprintf("--%s--\r\n", mb.boundary))

    return buffer.String()
}

// ✅ Fixed: Read attachment data from file and return string with error handling
func (mb *MIMEBuilder) buildAttachmentPart(attachment *models.Attachment, inline bool) (string, error) {
    var buffer bytes.Buffer

    // ✅ Read attachment data from FilePath
    data, err := mb.readAttachmentData(attachment)
    if err != nil {
        return "", fmt.Errorf("failed to read attachment data: %w", err)
    }

    contentType := attachment.ContentType
    if contentType == "" {
        contentType = detectContentType(attachment.Filename)
    }

    buffer.WriteString(fmt.Sprintf("Content-Type: %s; name=\"%s\"\r\n", contentType, encodeHeaderValue(attachment.Filename)))
    buffer.WriteString("Content-Transfer-Encoding: base64\r\n")

    if inline {
        buffer.WriteString(fmt.Sprintf("Content-Disposition: inline; filename=\"%s\"\r\n", encodeHeaderValue(attachment.Filename)))
        if attachment.ContentID != "" {
            buffer.WriteString(fmt.Sprintf("Content-ID: <%s>\r\n", attachment.ContentID))
        }
    } else {
        buffer.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n", encodeHeaderValue(attachment.Filename)))
    }

    buffer.WriteString("\r\n")
    buffer.WriteString(base64EncodeChunked(data, mb.config.ChunkSize))
    buffer.WriteString("\r\n")

    return buffer.String(), nil
}

// ✅ Helper function to read attachment data from file
func (mb *MIMEBuilder) readAttachmentData(attachment *models.Attachment) ([]byte, error) {
    // Try to get the full path (cached or regular)
    filePath := attachment.GetFullPath()
    
    if filePath == "" {
        return nil, fmt.Errorf("attachment has no file path")
    }

    // Read the file
    data, err := os.ReadFile(filePath)
    if err != nil {
        return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
    }

    return data, nil
}

func (mb *MIMEBuilder) encodeContent(content []byte) string {
    switch mb.config.PreferredEncoding {
    case EncodingBase64:
        return base64EncodeChunked(content, mb.config.ChunkSize)
    case EncodingQuotedPrintable:
        return quotedPrintableEncode(content)
    case Encoding7Bit, Encoding8Bit:
        return string(content)
    default:
        return base64EncodeChunked(content, mb.config.ChunkSize)
    }
}

func generateBoundary() string {
    timestamp := time.Now().UnixNano()
    randomBytes := make([]byte, 16)
    rand.Read(randomBytes)
    return fmt.Sprintf("----=_Part_%d_%s", timestamp, hex.EncodeToString(randomBytes))
}

func base64EncodeChunked(data []byte, chunkSize int) string {
    encoded := base64.StdEncoding.EncodeToString(data)
    
    if chunkSize <= 0 {
        chunkSize = 76
    }

    var buffer bytes.Buffer
    for i := 0; i < len(encoded); i += chunkSize {
        end := i + chunkSize
        if end > len(encoded) {
            end = len(encoded)
        }
        buffer.WriteString(encoded[i:end])
        if end < len(encoded) {
            buffer.WriteString("\r\n")
        }
    }

    return buffer.String()
}

func quotedPrintableEncode(data []byte) string {
    var buffer bytes.Buffer
    writer := quotedprintable.NewWriter(&buffer)
    writer.Write(data)
    writer.Close()
    return buffer.String()
}

func htmlToText(html string) string {
    text := html
    text = strings.ReplaceAll(text, "<br>", "\n")
    text = strings.ReplaceAll(text, "<br/>", "\n")
    text = strings.ReplaceAll(text, "<br />", "\n")
    text = strings.ReplaceAll(text, "</p>", "\n\n")
    text = strings.ReplaceAll(text, "</div>", "\n")
    
    text = stripHTMLTags(text)
    text = strings.TrimSpace(text)
    
    return text
}

func stripHTMLTags(html string) string {
    var result strings.Builder
    inTag := false
    
    for _, char := range html {
        if char == '<' {
            inTag = true
            continue
        }
        if char == '>' {
            inTag = false
            continue
        }
        if !inTag {
            result.WriteRune(char)
        }
    }
    
    return result.String()
}

func detectContentType(filename string) string {
    ext := strings.ToLower(filepath.Ext(filename))
    
    contentTypes := map[string]string{
        ".txt":  "text/plain",
        ".html": "text/html",
        ".htm":  "text/html",
        ".css":  "text/css",
        ".js":   "application/javascript",
        ".json": "application/json",
        ".xml":  "application/xml",
        ".pdf":  "application/pdf",
        ".zip":  "application/zip",
        ".doc":  "application/msword",
        ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
        ".xls":  "application/vnd.ms-excel",
        ".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
        ".ppt":  "application/vnd.ms-powerpoint",
        ".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
        ".jpg":  "image/jpeg",
        ".jpeg": "image/jpeg",
        ".png":  "image/png",
        ".gif":  "image/gif",
        ".bmp":  "image/bmp",
        ".svg":  "image/svg+xml",
        ".webp": "image/webp",
        ".ico":  "image/x-icon",
        ".mp3":  "audio/mpeg",
        ".wav":  "audio/wav",
        ".ogg":  "audio/ogg",
        ".mp4":  "video/mp4",
        ".avi":  "video/x-msvideo",
        ".mov":  "video/quicktime",
        ".wmv":  "video/x-ms-wmv",
    }
    
    if ct, ok := contentTypes[ext]; ok {
        return ct
    }
    
    return "application/octet-stream"
}

func encodeHeaderValue(value string) string {
    if isASCII(value) {
        return value
    }
    return mime.QEncoding.Encode("UTF-8", value)
}

func isASCII(s string) bool {
    for _, r := range s {
        if r > 127 {
            return false
        }
    }
    return true
}

func CreateMultipartWriter(boundary string) *multipart.Writer {
    var buf bytes.Buffer
    writer := multipart.NewWriter(&buf)
    writer.SetBoundary(boundary)
    return writer
}

func AddTextPart(writer io.Writer, content string, encoding EncodingType) error {
    var encoded string
    switch encoding {
    case EncodingBase64:
        encoded = base64EncodeChunked([]byte(content), 76)
    case EncodingQuotedPrintable:
        encoded = quotedPrintableEncode([]byte(content))
    default:
        encoded = content
    }
    
    _, err := fmt.Fprintf(writer, "%s\r\n", encoded)
    return err
}

func AddHTMLPart(writer io.Writer, content string, encoding EncodingType) error {
    var encoded string
    switch encoding {
    case EncodingBase64:
        encoded = base64EncodeChunked([]byte(content), 76)
    case EncodingQuotedPrintable:
        encoded = quotedPrintableEncode([]byte(content))
    default:
        encoded = content
    }
    
    _, err := fmt.Fprintf(writer, "%s\r\n", encoded)
    return err
}

func CreateMIMEHeaders() textproto.MIMEHeader {
    return make(textproto.MIMEHeader)
}

func SetMIMEHeader(headers textproto.MIMEHeader, key, value string) {
    headers.Set(key, value)
}

func GetMIMEHeader(headers textproto.MIMEHeader, key string) string {
    return headers.Get(key)
}

func ParseMIMEMessage(data []byte) (*MIMEMessage, error) {
    reader := bytes.NewReader(data)
    tp := textproto.NewReader(bufio.NewReader(reader))
    
    headers, err := tp.ReadMIMEHeader()
    if err != nil {
        return nil, err
    }
    
    body, err := io.ReadAll(tp.R)
    if err != nil {
        return nil, err
    }
    
    headerMap := make(map[string]string)
    for key, values := range headers {
        if len(values) > 0 {
            headerMap[key] = values[0]
        }
    }
    
    return &MIMEMessage{
        Headers: headerMap,
        Body:    string(body),
        Raw:     data,
        Size:    int64(len(data)),
    }, nil
}

func ValidateMIMEStructure(message *MIMEMessage) error {
    if message.Headers == nil {
        return fmt.Errorf("missing headers")
    }
    
    contentType := message.Headers["Content-Type"]
    if contentType == "" {
        return fmt.Errorf("missing content-type header")
    }
    
    if message.Body == "" && len(message.Raw) == 0 {
        return fmt.Errorf("empty message body")
    }
    
    return nil
}

func ExtractBoundary(contentType string) string {
    parts := strings.Split(contentType, "boundary=")
    if len(parts) < 2 {
        return ""
    }
    
    boundary := strings.Trim(parts[1], "\"")
    return boundary
}

func SplitMIMEParts(body string, boundary string) []string {
    delimiter := "--" + boundary
    parts := strings.Split(body, delimiter)
    
    result := make([]string, 0)
    for _, part := range parts {
        trimmed := strings.TrimSpace(part)
        if trimmed != "" && trimmed != "--" {
            result = append(result, trimmed)
        }
    }
    
    return result
}

func (mm *MIMEMessage) String() string {
    return mm.Body
}

func (mm *MIMEMessage) Bytes() []byte {
    if len(mm.Raw) > 0 {
        return mm.Raw
    }
    return []byte(mm.Body)
}

func (mm *MIMEMessage) GetHeader(key string) string {
    return mm.Headers[key]
}

func (mm *MIMEMessage) SetHeader(key, value string) {
    if mm.Headers == nil {
        mm.Headers = make(map[string]string)
    }
    mm.Headers[key] = value
}
