package models

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type AttachmentFormat string

const (
	AttachmentFormatPDF  AttachmentFormat = "pdf"
	AttachmentFormatJPG  AttachmentFormat = "jpg"
	AttachmentFormatJPEG AttachmentFormat = "jpeg"
	AttachmentFormatPNG  AttachmentFormat = "png"
	AttachmentFormatWebP AttachmentFormat = "webp"
	AttachmentFormatHEIC AttachmentFormat = "heic"
	AttachmentFormatHEIF AttachmentFormat = "heif"
)

type AttachmentStatus string

const (
	AttachmentStatusPending   AttachmentStatus = "pending"
	AttachmentStatusGenerating AttachmentStatus = "generating"
	AttachmentStatusReady     AttachmentStatus = "ready"
	AttachmentStatusFailed    AttachmentStatus = "failed"
	AttachmentStatusCached    AttachmentStatus = "cached"
)

type AttachmentSource string

const (
	AttachmentSourceFile      AttachmentSource = "file"
	AttachmentSourceGenerated AttachmentSource = "generated"
	AttachmentSourceConverted AttachmentSource = "converted"
	AttachmentSourceTemplate  AttachmentSource = "template"
	AttachmentSourceURL       AttachmentSource = "url"
)

type ConversionBackend string

const (
	ConversionBackendChromedp   ConversionBackend = "chromedp"
	ConversionBackendWeasyPrint ConversionBackend = "weasyprint"
	ConversionBackendImgkit     ConversionBackend = "imgkit"
	ConversionBackendPdfkit     ConversionBackend = "pdfkit"
	ConversionBackendWkhtmltopdf ConversionBackend = "wkhtmltopdf"
)

type Attachment struct {
	ID                  string                 `json:"id"`
	TenantID            string                 `json:"tenant_id"`
	CampaignID          string                 `json:"campaign_id"`
	TemplateID          string                 `json:"template_id"`
	Name                string                 `json:"name"`
	Filename            string                 `json:"filename"`
	OriginalFilename    string                 `json:"original_filename"`
	Format              AttachmentFormat       `json:"format"`
	Status              AttachmentStatus       `json:"status"`
	Source              AttachmentSource       `json:"source"`
	ContentType         string                 `json:"content_type"`
	Size                int64                  `json:"size"`
	Hash                string                 `json:"hash"`
	FilePath            string                 `json:"file_path"`
	CachePath           string                 `json:"cache_path"`
	IsCached            bool                   `json:"is_cached"`
	IsInline            bool                   `json:"is_inline"`
	ContentID           string                 `json:"content_id"`
	ConversionConfig    *ConversionConfig      `json:"conversion_config,omitempty"`
	RotationConfig      *AttachmentRotationConfig `json:"rotation_config,omitempty"`
	PersonalizationData map[string]interface{} `json:"personalization_data"`
	SourceHTML          string                 `json:"source_html,omitempty"`
	SourceTemplateID    string                 `json:"source_template_id,omitempty"`
	GeneratedFrom       string                 `json:"generated_from,omitempty"`
	Width               int                    `json:"width,omitempty"`
	Height              int                    `json:"height,omitempty"`
	Quality             int                    `json:"quality,omitempty"`
	DPI                 int                    `json:"dpi,omitempty"`
	LastError           string                 `json:"last_error,omitempty"`
	GenerationTimeMs    int64                  `json:"generation_time_ms"`
	CacheHitCount       int64                  `json:"cache_hit_count"`
	UsageCount          int64                  `json:"usage_count"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
	GeneratedAt         *time.Time             `json:"generated_at,omitempty"`
	LastUsedAt          *time.Time             `json:"last_used_at,omitempty"`
	ExpiresAt           *time.Time             `json:"expires_at,omitempty"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
}

type ConversionConfig struct {
	Backend           ConversionBackend      `json:"backend"`
	SourceFormat      string                 `json:"source_format"`
	TargetFormat      AttachmentFormat       `json:"target_format"`
	PageSize          string                 `json:"page_size"`
	Orientation       string                 `json:"orientation"`
	MarginTop         int                    `json:"margin_top"`
	MarginRight       int                    `json:"margin_right"`
	MarginBottom      int                    `json:"margin_bottom"`
	MarginLeft        int                    `json:"margin_left"`
	DPI               int                    `json:"dpi"`
	Quality           int                    `json:"quality"`
	Width             int                    `json:"width"`
	Height            int                    `json:"height"`
	EnableJavaScript  bool                   `json:"enable_javascript"`
	WaitForLoad       bool                   `json:"wait_for_load"`
	WaitTimeMs        int                    `json:"wait_time_ms"`
	Timeout           int                    `json:"timeout"`
	CustomCSS         string                 `json:"custom_css,omitempty"`
	CustomOptions     map[string]interface{} `json:"custom_options,omitempty"`
}

type AttachmentRotationConfig struct {
	EnableRotation    bool             `json:"enable_rotation"`
	RotationStrategy  RotationStrategy `json:"rotation_strategy"`
	FormatRotation    []AttachmentFormat `json:"format_rotation"`
	CurrentIndex      int              `json:"current_index"`
	RotationCount     int64            `json:"rotation_count"`
}

func NewAttachment(filename string, format AttachmentFormat, tenantID string) *Attachment {
	now := time.Now()
	return &Attachment{
		ID:                  generateAttachmentID(),
		TenantID:            tenantID,
		Filename:            filename,
		OriginalFilename:    filename,
		Format:              format,
		Status:              AttachmentStatusPending,
		Source:              AttachmentSourceFile,
		ContentType:         getContentType(format),
		PersonalizationData: make(map[string]interface{}),
		Quality:             90,
		DPI:                 300,
		CreatedAt:           now,
		UpdatedAt:           now,
		Metadata:            make(map[string]interface{}),
	}
}

func NewAttachmentFromHTML(html, filename string, format AttachmentFormat, tenantID string) *Attachment {
	attachment := NewAttachment(filename, format, tenantID)
	attachment.Source = AttachmentSourceGenerated
	attachment.SourceHTML = html
	attachment.ConversionConfig = DefaultConversionConfig(format)
	return attachment
}

func DefaultConversionConfig(format AttachmentFormat) *ConversionConfig {
	return &ConversionConfig{
		Backend:          ConversionBackendChromedp,
		TargetFormat:     format,
		PageSize:         "A4",
		Orientation:      "portrait",
		MarginTop:        10,
		MarginRight:      10,
		MarginBottom:     10,
		MarginLeft:       10,
		DPI:              300,
		Quality:          90,
		EnableJavaScript: false,
		WaitForLoad:      true,
		WaitTimeMs:       1000,
		Timeout:          30,
		CustomOptions:    make(map[string]interface{}),
	}
}

func (a *Attachment) Validate() error {
	if a.Filename == "" {
		return errors.New("filename is required")
	}
	if a.TenantID == "" {
		return errors.New("tenant ID is required")
	}
	if !a.Format.IsValid() {
		return fmt.Errorf("invalid format: %s", a.Format)
	}
	if !a.Status.IsValid() {
		return fmt.Errorf("invalid status: %s", a.Status)
	}
	if !a.Source.IsValid() {
		return fmt.Errorf("invalid source: %s", a.Source)
	}
	if a.Size < 0 {
		return errors.New("size cannot be negative")
	}
	return nil
}

func (a *Attachment) GenerateHash(content []byte) string {
	hasher := sha256.New()
	hasher.Write(content)
	
	if a.PersonalizationData != nil && len(a.PersonalizationData) > 0 {
		for key, value := range a.PersonalizationData {
			hasher.Write([]byte(fmt.Sprintf("%s:%v", key, value)))
		}
	}
	
	a.Hash = hex.EncodeToString(hasher.Sum(nil))
	return a.Hash
}

func (a *Attachment) GetCacheKey() string {
	if a.Hash == "" {
		return ""
	}
	return fmt.Sprintf("attachment_%s_%s", a.Format, a.Hash)
}

func (a *Attachment) MarkAsGenerated(filePath string, size int64, generationTimeMs int64) {
	a.Status = AttachmentStatusReady
	a.FilePath = filePath
	a.Size = size
	a.GenerationTimeMs = generationTimeMs
	now := time.Now()
	a.GeneratedAt = &now
	a.UpdatedAt = now
}

func (a *Attachment) MarkAsCached(cachePath string) {
	a.IsCached = true
	a.Status = AttachmentStatusCached
	a.CachePath = cachePath
	a.CacheHitCount++
	a.UpdatedAt = time.Now()
}

func (a *Attachment) MarkAsFailed(errorMsg string) {
	a.Status = AttachmentStatusFailed
	a.LastError = errorMsg
	a.UpdatedAt = time.Now()
}

func (a *Attachment) MarkAsGenerating() {
	a.Status = AttachmentStatusGenerating
	a.UpdatedAt = time.Now()
}

func (a *Attachment) IncrementUsage() {
	a.UsageCount++
	now := time.Now()
	a.LastUsedAt = &now
	a.UpdatedAt = now
}

func (a *Attachment) GetFileExtension() string {
	return fmt.Sprintf(".%s", a.Format)
}

func (a *Attachment) GetFullPath() string {
	if a.IsCached && a.CachePath != "" {
		return a.CachePath
	}
	return a.FilePath
}

func (a *Attachment) SetPersonalization(data map[string]interface{}) {
	if a.PersonalizationData == nil {
		a.PersonalizationData = make(map[string]interface{})
	}
	for key, value := range data {
		a.PersonalizationData[key] = value
	}
	a.UpdatedAt = time.Now()
}

func (a *Attachment) ApplyPersonalization(html string) string {
	result := html
	for key, value := range a.PersonalizationData {
		placeholder := fmt.Sprintf("{%s}", key)
		valueStr := fmt.Sprintf("%v", value)
		result = strings.ReplaceAll(result, placeholder, valueStr)
	}
	return result
}

func (a *Attachment) ConvertToFormat(targetFormat AttachmentFormat) error {
	if !targetFormat.IsValid() {
		return fmt.Errorf("invalid target format: %s", targetFormat)
	}
	
	if a.ConversionConfig == nil {
		a.ConversionConfig = DefaultConversionConfig(targetFormat)
	}
	
	a.ConversionConfig.TargetFormat = targetFormat
	a.ConversionConfig.SourceFormat = string(a.Format)
	a.Format = targetFormat
	a.ContentType = getContentType(targetFormat)
	a.Status = AttachmentStatusPending
	a.UpdatedAt = time.Now()
	
	return nil
}

func (a *Attachment) Clone() *Attachment {
	clone := *a
	clone.ID = generateAttachmentID()
	clone.Status = AttachmentStatusPending
	clone.IsCached = false
	clone.UsageCount = 0
	clone.CacheHitCount = 0
	now := time.Now()
	clone.CreatedAt = now
	clone.UpdatedAt = now
	clone.GeneratedAt = nil
	clone.LastUsedAt = nil
	
	if a.PersonalizationData != nil {
		clone.PersonalizationData = make(map[string]interface{})
		for k, v := range a.PersonalizationData {
			clone.PersonalizationData[k] = v
		}
	}
	
	if a.ConversionConfig != nil {
		config := *a.ConversionConfig
		clone.ConversionConfig = &config
	}
	
	return &clone
}

func (a *Attachment) IsReady() bool {
	return a.Status == AttachmentStatusReady || a.Status == AttachmentStatusCached
}

func (a *Attachment) IsGenerating() bool {
	return a.Status == AttachmentStatusGenerating
}

func (a *Attachment) IsFailed() bool {
	return a.Status == AttachmentStatusFailed
}

func (a *Attachment) NeedsGeneration() bool {
	return a.Status == AttachmentStatusPending && a.Source != AttachmentSourceFile
}

func (a *Attachment) IsExpired() bool {
	if a.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*a.ExpiresAt)
}

func (a *Attachment) SetExpiration(duration time.Duration) {
	expiresAt := time.Now().Add(duration)
	a.ExpiresAt = &expiresAt
	a.UpdatedAt = time.Now()
}

func (a *Attachment) GetSizeInMB() float64 {
	return float64(a.Size) / (1024 * 1024)
}

func (a *Attachment) GetSizeInKB() float64 {
	return float64(a.Size) / 1024
}

func (a *Attachment) IsImage() bool {
	switch a.Format {
	case AttachmentFormatJPG, AttachmentFormatJPEG, AttachmentFormatPNG,
		AttachmentFormatWebP, AttachmentFormatHEIC, AttachmentFormatHEIF:
		return true
	}
	return false
}

func (a *Attachment) IsPDF() bool {
	return a.Format == AttachmentFormatPDF
}

func (a *Attachment) SupportsConversion() bool {
	return a.Source == AttachmentSourceGenerated || 
		   a.Source == AttachmentSourceTemplate ||
		   a.SourceHTML != ""
}

func (a *Attachment) SetDimensions(width, height int) {
	a.Width = width
	a.Height = height
	a.UpdatedAt = time.Now()
}

func (a *Attachment) SetQuality(quality int) error {
	if quality < 1 || quality > 100 {
		return errors.New("quality must be between 1 and 100")
	}
	a.Quality = quality
	if a.ConversionConfig != nil {
		a.ConversionConfig.Quality = quality
	}
	a.UpdatedAt = time.Now()
	return nil
}

func (a *Attachment) SetDPI(dpi int) error {
	if dpi < 72 || dpi > 600 {
		return errors.New("DPI must be between 72 and 600")
	}
	a.DPI = dpi
	if a.ConversionConfig != nil {
		a.ConversionConfig.DPI = dpi
	}
	a.UpdatedAt = time.Now()
	return nil
}

func (af AttachmentFormat) IsValid() bool {
	switch af {
	case AttachmentFormatPDF, AttachmentFormatJPG, AttachmentFormatJPEG,
		AttachmentFormatPNG, AttachmentFormatWebP, AttachmentFormatHEIC,
		AttachmentFormatHEIF:
		return true
	}
	return false
}

func (as AttachmentStatus) IsValid() bool {
	switch as {
	case AttachmentStatusPending, AttachmentStatusGenerating,
		AttachmentStatusReady, AttachmentStatusFailed, AttachmentStatusCached:
		return true
	}
	return false
}

func (asrc AttachmentSource) IsValid() bool {
	switch asrc {
	case AttachmentSourceFile, AttachmentSourceGenerated,
		AttachmentSourceConverted, AttachmentSourceTemplate, AttachmentSourceURL:
		return true
	}
	return false
}

func (cb ConversionBackend) IsValid() bool {
	switch cb {
	case ConversionBackendChromedp, ConversionBackendWeasyPrint,
		ConversionBackendImgkit, ConversionBackendPdfkit, ConversionBackendWkhtmltopdf:
		return true
	}
	return false
}

func getContentType(format AttachmentFormat) string {
	switch format {
	case AttachmentFormatPDF:
		return "application/pdf"
	case AttachmentFormatJPG, AttachmentFormatJPEG:
		return "image/jpeg"
	case AttachmentFormatPNG:
		return "image/png"
	case AttachmentFormatWebP:
		return "image/webp"
	case AttachmentFormatHEIC:
		return "image/heic"
	case AttachmentFormatHEIF:
		return "image/heif"
	default:
		return "application/octet-stream"
	}
}

func DetectFormatFromFilename(filename string) AttachmentFormat {
	ext := strings.ToLower(filepath.Ext(filename))
	ext = strings.TrimPrefix(ext, ".")
	
	switch ext {
	case "pdf":
		return AttachmentFormatPDF
	case "jpg", "jpeg":
		return AttachmentFormatJPG
	case "png":
		return AttachmentFormatPNG
	case "webp":
		return AttachmentFormatWebP
	case "heic":
		return AttachmentFormatHEIC
	case "heif":
		return AttachmentFormatHEIF
	default:
		return AttachmentFormatPDF
	}
}

func (a *Attachment) GetNextRotationFormat() AttachmentFormat {
	if a.RotationConfig == nil || !a.RotationConfig.EnableRotation {
		return a.Format
	}
	
	if len(a.RotationConfig.FormatRotation) == 0 {
		return a.Format
	}
	
	currentIndex := a.RotationConfig.CurrentIndex
	if currentIndex >= len(a.RotationConfig.FormatRotation) {
		currentIndex = 0
	}
	
	nextFormat := a.RotationConfig.FormatRotation[currentIndex]
	
	a.RotationConfig.CurrentIndex = (currentIndex + 1) % len(a.RotationConfig.FormatRotation)
	a.RotationConfig.RotationCount++
	a.UpdatedAt = time.Now()
	
	return nextFormat
}

func (a *Attachment) EnableFormatRotation(formats []AttachmentFormat) {
	if a.RotationConfig == nil {
		a.RotationConfig = &AttachmentRotationConfig{}
	}
	
	a.RotationConfig.EnableRotation = true
	a.RotationConfig.FormatRotation = formats
	a.RotationConfig.CurrentIndex = 0
	a.UpdatedAt = time.Now()
}

func (a *Attachment) DisableFormatRotation() {
	if a.RotationConfig != nil {
		a.RotationConfig.EnableRotation = false
	}
	a.UpdatedAt = time.Now()
}

func (a *Attachment) GetCacheAge() time.Duration {
	if a.GeneratedAt == nil {
		return 0
	}
	return time.Since(*a.GeneratedAt)
}

func (a *Attachment) ShouldRegenerate(maxAge time.Duration) bool {
	if !a.IsReady() {
		return false
	}
	if a.IsExpired() {
		return true
	}
	if maxAge > 0 && a.GetCacheAge() > maxAge {
		return true
	}
	return false
}



func (a *Attachment) GetEstimatedGenerationTime() time.Duration {
	if a.GenerationTimeMs > 0 {
		return time.Duration(a.GenerationTimeMs) * time.Millisecond
	}
	
	baseTime := 1000 * time.Millisecond
	
	if a.IsImage() {
		return baseTime
	}
	if a.IsPDF() {
		return baseTime * 2
	}
	
	return baseTime
}

func (a *Attachment) GetDisplayName() string {
	if a.Name != "" {
		return a.Name
	}
	return a.Filename
}
