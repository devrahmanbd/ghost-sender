package models

import (
        "errors"
        "fmt"
        "regexp"
        "strings"
        "time"
        "github.com/google/uuid"
)

type TemplateType string

const (
        TemplateTypeHTML      TemplateType = "html"
        TemplateTypePlainText TemplateType = "plain_text"
        TemplateTypeMixed     TemplateType = "mixed"
)

type TemplateStatus string

const (
        TemplateStatusDraft    TemplateStatus = "draft"
        TemplateStatusActive   TemplateStatus = "active"
        TemplateStatusInactive TemplateStatus = "inactive"
        TemplateStatusArchived TemplateStatus = "archived"
)

type Template struct {
        ID                  string            `json:"id" db:"id"`
        TenantID            string            `json:"tenant_id" db:"tenant_id"`
        Name                string            `json:"name" db:"name"`
        Description         string            `json:"description" db:"description"`
        Type                TemplateType      `json:"type" db:"type"`
        Status              TemplateStatus    `json:"status" db:"status"`
        Version             int               `json:"version" db:"version"`
        IsDefault           bool              `json:"is_default" db:"is_default"`
        Tags                []string          `json:"tags" db:"tags"`
        HTMLContent         string            `json:"html_content" db:"html_content"`
        PlainTextContent    string            `json:"plain_text_content" db:"plain_text_content"`
        Subjects            []string          `json:"subjects"`
        SenderNames         []string          `json:"sender_names"`
        ReplyToEmail        string            `json:"reply_to_email" db:"reply_to_email"`
        ReplyToName         string            `json:"reply_to_name" db:"reply_to_name"`
        Variables           []TemplateVariable `json:"variables"`
        RequiredVariables   []string          `json:"required_variables"`
        OptionalVariables   []string          `json:"optional_variables"`
        CustomVariables     map[string][]string `json:"custom_variables"`
        SpamScore           float64           `json:"spam_score" db:"spam_score"`
        SpamAnalysis        *SpamAnalysis     `json:"spam_analysis,omitempty"`
        ValidationResult    *ValidationResult `json:"validation_result,omitempty"`
        RotationConfig      TemplateRotationConfig `json:"rotation_config"`
        TemplateAttachmentConfig    *TemplateAttachmentConfig `json:"attachment_config,omitempty"`
        PersonalizationConfig PersonalizationConfig `json:"personalization_config"`
        TrackingConfig      TrackingConfig    `json:"tracking_config"`
        Stats               TemplateStats     `json:"stats"`
        FilePath            string            `json:"file_path" db:"file_path"`
        FileSize            int64             `json:"file_size" db:"file_size"`
        FileHash            string            `json:"file_hash" db:"file_hash"`
        PreviewURL          string            `json:"preview_url" db:"preview_url"`
        ThumbnailURL        string            `json:"thumbnail_url" db:"thumbnail_url"`
        LastValidatedAt     *time.Time        `json:"last_validated_at" db:"last_validated_at"`
        LastUsedAt          *time.Time        `json:"last_used_at" db:"last_used_at"`
        CreatedAt           time.Time         `json:"created_at" db:"created_at"`
        UpdatedAt           time.Time         `json:"updated_at" db:"updated_at"`
        CreatedBy           string            `json:"created_by" db:"created_by"`
        UpdatedBy           string            `json:"updated_by" db:"updated_by"`
        Metadata            map[string]interface{} `json:"metadata,omitempty"`
}

type TemplateVariable struct {
        Name         string   `json:"name"`
        Type         string   `json:"type"`
        Description  string   `json:"description"`
        DefaultValue string   `json:"default_value"`
        IsRequired   bool     `json:"is_required"`
        Pattern      string   `json:"pattern,omitempty"`
        Examples     []string `json:"examples,omitempty"`
        Category     string   `json:"category"`
}

type SpamAnalysis struct {
        OverallScore       float64            `json:"overall_score"`
        IsSpammy           bool               `json:"is_spammy"`
        Warnings           []string           `json:"warnings"`
        Errors             []string           `json:"errors"`
        ContentScore       float64            `json:"content_score"`
        LinkScore          float64            `json:"link_score"`
        ImageScore         float64            `json:"image_score"`
        HTMLScore          float64            `json:"html_score"`
        SubjectScore       float64            `json:"subject_score"`
        SpamWords          []string           `json:"spam_words"`
        SpamPhrases        []string           `json:"spam_phrases"`
        ExcessiveLinks     bool               `json:"excessive_links"`
        ExcessiveImages    bool               `json:"excessive_images"`
        SuspiciousURLs     []string           `json:"suspicious_urls"`
        MissingUnsubscribe bool               `json:"missing_unsubscribe"`
        Recommendations    []string           `json:"recommendations"`
        AnalyzedAt         time.Time          `json:"analyzed_at"`
}

type ValidationResult struct {
        IsValid            bool      `json:"is_valid"`
        Errors             []string  `json:"errors"`
        Warnings           []string  `json:"warnings"`
        HTMLValid          bool      `json:"html_valid"`
        HasValidStructure  bool      `json:"has_valid_structure"`
        HasRequiredElements bool     `json:"has_required_elements"`
        MissingVariables   []string  `json:"missing_variables"`
        UnusedVariables    []string  `json:"unused_variables"`
        BrokenLinks        []string  `json:"broken_links"`
        MissingImages      []string  `json:"missing_images"`
        ValidatedAt        time.Time `json:"validated_at"`
}

type TemplateRotationConfig struct {
        EnableRotation       bool             `json:"enable_rotation"`
        SubjectRotation      RotationStrategy `json:"subject_rotation"`
        SenderNameRotation   RotationStrategy `json:"sender_name_rotation"`
        UnlimitedRotation    bool             `json:"unlimited_rotation"`
        RotationLimit        int              `json:"rotation_limit"`
        CurrentSubjectIndex  int              `json:"current_subject_index"`
        CurrentSenderIndex   int              `json:"current_sender_index"`
        SubjectWeights       []float64        `json:"subject_weights,omitempty"`
        SenderNameWeights    []float64        `json:"sender_name_weights,omitempty"`
}

type TemplateAttachmentConfig struct {
        EnableAttachments    bool     `json:"enable_attachments"`
        AttachmentPaths      []string `json:"attachment_paths"`
        ConvertHTMLToPDF     bool     `json:"convert_html_to_pdf"`
        AttachmentFormats    []string `json:"attachment_formats"`
        EnableFormatRotation bool     `json:"enable_format_rotation"`
        MaxAttachmentSize    int64    `json:"max_attachment_size"`
        AttachmentNames      []string `json:"attachment_names,omitempty"`
}

type PersonalizationConfig struct {
        EnablePersonalization bool              `json:"enable_personalization"`
        ExtractNameFromEmail  bool              `json:"extract_name_from_email"`
        FallbackName          string            `json:"fallback_name"`
        CustomFieldMapping    map[string]string `json:"custom_field_mapping"`
        DateFormat            string            `json:"date_format"`
        TimeZone              string            `json:"time_zone"`
        EnableTimeBasedContent bool             `json:"enable_time_based_content"`
}

type TrackingConfig struct {
        EnableOpenTracking   bool   `json:"enable_open_tracking"`
        EnableClickTracking  bool   `json:"enable_click_tracking"`
        TrackingDomain       string `json:"tracking_domain,omitempty"`
        OpenTrackingPixel    string `json:"open_tracking_pixel,omitempty"`
        ClickTrackingPrefix  string `json:"click_tracking_prefix,omitempty"`
}

type TemplateStats struct {
        TimesUsed          int64     `json:"times_used"`
        TotalSent          int64     `json:"total_sent"`
        TotalDelivered     int64     `json:"total_delivered"`
        TotalOpens         int64     `json:"total_opens"`
        TotalClicks        int64     `json:"total_clicks"`
        TotalBounces       int64     `json:"total_bounces"`
        TotalComplaints    int64     `json:"total_complaints"`
        AverageOpenRate    float64   `json:"average_open_rate"`
        AverageClickRate   float64   `json:"average_click_rate"`
        AverageBounceRate  float64   `json:"average_bounce_rate"`
        AverageComplaintRate float64 `json:"average_complaint_rate"`
        LastUsedAt         *time.Time `json:"last_used_at,omitempty"`
        LastUpdatedAt      time.Time `json:"last_updated_at"`
}

type TemplateVersion struct {
        ID               string    `json:"id" db:"id"`
        TemplateID       string    `json:"template_id" db:"template_id"`
        Version          int       `json:"version" db:"version"`
        HTMLContent      string    `json:"html_content" db:"html_content"`
        PlainTextContent string    `json:"plain_text_content" db:"plain_text_content"`
        Subjects         []string  `json:"subjects"`
        SenderNames      []string  `json:"sender_names"`
        ChangeLog        string    `json:"change_log" db:"change_log"`
        CreatedBy        string    `json:"created_by" db:"created_by"`
        CreatedAt        time.Time `json:"created_at" db:"created_at"`
}

func NewTemplate(name string, tenantID, createdBy string) *Template {
        now := time.Now()
        return &Template{
                ID:                generateTemplateID(),
                TenantID:          tenantID,
                Name:              name,
                Type:              TemplateTypeHTML,
                Status:            TemplateStatusDraft,
                Version:           1,
                Tags:              []string{},
                Subjects:          []string{},
                SenderNames:       []string{},
                Variables:         []TemplateVariable{},
                RequiredVariables: []string{},
                OptionalVariables: []string{},
                CustomVariables:   make(map[string][]string),
                SpamScore:         0,
                RotationConfig:    DefaultTemplateRotationConfig(),
                PersonalizationConfig: DefaultPersonalizationConfig(),
                TrackingConfig:    DefaultTrackingConfig(),
                Stats:             TemplateStats{LastUpdatedAt: now},
                CreatedAt:         now,
                UpdatedAt:         now,
                CreatedBy:         createdBy,
                UpdatedBy:         createdBy,
                Metadata:          make(map[string]interface{}),
        }
}

func DefaultTemplateRotationConfig() TemplateRotationConfig {
        return TemplateRotationConfig{
                EnableRotation:      false,
                SubjectRotation:     RotationStrategySequential,
                SenderNameRotation:  RotationStrategySequential,
                UnlimitedRotation:   true,
                RotationLimit:       0,
                CurrentSubjectIndex: 0,
                CurrentSenderIndex:  0,
        }
}

func DefaultPersonalizationConfig() PersonalizationConfig {
        return PersonalizationConfig{
                EnablePersonalization: true,
                ExtractNameFromEmail:  true,
                FallbackName:          "there",
                CustomFieldMapping:    make(map[string]string),
                DateFormat:            "2006-01-02",
                TimeZone:              "UTC",
                EnableTimeBasedContent: false,
        }
}

func DefaultTrackingConfig() TrackingConfig {
        return TrackingConfig{
                EnableOpenTracking:  true,
                EnableClickTracking: true,
        }
}
func (t *Template) Validate() error {
        if t.Name == "" {
                return errors.New("template name is required")
        }
        if t.TenantID == "" {
                return errors.New("tenant ID is required")
        }
        if !t.Type.IsValid() {
                return fmt.Errorf("invalid template type: %s", t.Type)
        }
        if !t.Status.IsValid() {
                return fmt.Errorf("invalid template status: %s", t.Status)
        }
        if t.HTMLContent == "" && t.PlainTextContent == "" {
                return errors.New("template must have HTML or plain text content")
        }
        if len(t.Subjects) == 0 {
                return errors.New("template must have at least one subject line")
        }
        if len(t.SenderNames) == 0 {
                return errors.New("template must have at least one sender name")
        }
        return nil
}

func (t *Template) IsActive() bool {
        return t.Status == TemplateStatusActive
}

func (t *Template) IsDraft() bool {
        return t.Status == TemplateStatusDraft
}

func (t *Template) Activate() error {
        if t.Status == TemplateStatusActive {
                return errors.New("template is already active")
        }
        if err := t.Validate(); err != nil {
                return fmt.Errorf("cannot activate invalid template: %w", err)
        }
        t.Status = TemplateStatusActive
        t.UpdatedAt = time.Now()
        return nil
}

func (t *Template) Deactivate() error {
        if t.Status == TemplateStatusInactive {
                return errors.New("template is already inactive")
        }
        t.Status = TemplateStatusInactive
        t.UpdatedAt = time.Now()
        return nil
}

func (t *Template) Archive() error {
        t.Status = TemplateStatusArchived
        t.UpdatedAt = time.Now()
        return nil
}

func (t *Template) ExtractVariables() []string {
        re := regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)
        
        allMatches := make(map[string]bool)
        
        if t.HTMLContent != "" {
                matches := re.FindAllStringSubmatch(t.HTMLContent, -1)
                for _, match := range matches {
                        if len(match) > 1 {
                                allMatches[match[1]] = true
                        }
                }
        }
        
        if t.PlainTextContent != "" {
                matches := re.FindAllStringSubmatch(t.PlainTextContent, -1)
                for _, match := range matches {
                        if len(match) > 1 {
                                allMatches[match[1]] = true
                        }
                }
        }
        
        for _, subject := range t.Subjects {
                matches := re.FindAllStringSubmatch(subject, -1)
                for _, match := range matches {
                        if len(match) > 1 {
                                allMatches[match[1]] = true
                        }
                }
        }
        
        variables := make([]string, 0, len(allMatches))
        for variable := range allMatches {
                variables = append(variables, variable)
        }
        
        return variables
}

func (t *Template) HasVariable(variable string) bool {
        pattern := fmt.Sprintf("{%s}", variable)
        if strings.Contains(t.HTMLContent, pattern) {
                return true
        }
        if strings.Contains(t.PlainTextContent, pattern) {
                return true
        }
        for _, subject := range t.Subjects {
                if strings.Contains(subject, pattern) {
                        return true
                }
        }
        return false
}

func (t *Template) GetNextSubject() string {
        if len(t.Subjects) == 0 {
                return ""
        }
        
        if !t.RotationConfig.EnableRotation {
                return t.Subjects[0]
        }
        
        subject := t.Subjects[t.RotationConfig.CurrentSubjectIndex]
        
        t.RotationConfig.CurrentSubjectIndex++
        if t.RotationConfig.CurrentSubjectIndex >= len(t.Subjects) {
                t.RotationConfig.CurrentSubjectIndex = 0
        }
        
        t.UpdatedAt = time.Now()
        return subject
}

func (t *Template) GetNextSenderName() string {
        if len(t.SenderNames) == 0 {
                return ""
        }
        
        if !t.RotationConfig.EnableRotation {
                return t.SenderNames[0]
        }
        
        senderName := t.SenderNames[t.RotationConfig.CurrentSenderIndex]
        
        t.RotationConfig.CurrentSenderIndex++
        if t.RotationConfig.CurrentSenderIndex >= len(t.SenderNames) {
                t.RotationConfig.CurrentSenderIndex = 0
        }
        
        t.UpdatedAt = time.Now()
        return senderName
}

func (t *Template) IncrementUsage() {
        t.Stats.TimesUsed++
        now := time.Now()
        t.LastUsedAt = &now
        t.Stats.LastUsedAt = &now
        t.UpdatedAt = now
}

func (t *Template) RecordSent() {
        t.Stats.TotalSent++
        t.Stats.LastUpdatedAt = time.Now()
        t.UpdatedAt = time.Now()
}

func (t *Template) RecordDelivered() {
        t.Stats.TotalDelivered++
        t.calculateRates()
        t.Stats.LastUpdatedAt = time.Now()
        t.UpdatedAt = time.Now()
}

func (t *Template) RecordOpen() {
        t.Stats.TotalOpens++
        t.calculateRates()
        t.Stats.LastUpdatedAt = time.Now()
        t.UpdatedAt = time.Now()
}

func (t *Template) RecordClick() {
        t.Stats.TotalClicks++
        t.calculateRates()
        t.Stats.LastUpdatedAt = time.Now()
        t.UpdatedAt = time.Now()
}

func (t *Template) RecordBounce() {
        t.Stats.TotalBounces++
        t.calculateRates()
        t.Stats.LastUpdatedAt = time.Now()
        t.UpdatedAt = time.Now()
}

func (t *Template) RecordComplaint() {
        t.Stats.TotalComplaints++
        t.calculateRates()
        t.Stats.LastUpdatedAt = time.Now()
        t.UpdatedAt = time.Now()
}

func (t *Template) calculateRates() {
        if t.Stats.TotalSent > 0 {
                t.Stats.AverageBounceRate = float64(t.Stats.TotalBounces) / float64(t.Stats.TotalSent)
                t.Stats.AverageComplaintRate = float64(t.Stats.TotalComplaints) / float64(t.Stats.TotalSent)
        }
        
        if t.Stats.TotalDelivered > 0 {
                t.Stats.AverageOpenRate = float64(t.Stats.TotalOpens) / float64(t.Stats.TotalDelivered)
                t.Stats.AverageClickRate = float64(t.Stats.TotalClicks) / float64(t.Stats.TotalDelivered)
        }
}

func (t *Template) CreateVersion(changeLog string, createdBy string) *TemplateVersion {
        return &TemplateVersion{
                ID:               generateVersionID(),
                TemplateID:       t.ID,
                Version:          t.Version,
                HTMLContent:      t.HTMLContent,
                PlainTextContent: t.PlainTextContent,
                Subjects:         t.Subjects,
                SenderNames:      t.SenderNames,
                ChangeLog:        changeLog,
                CreatedBy:        createdBy,
                CreatedAt:        time.Now(),
        }
}

func (t *Template) Clone() *Template {
        clone := *t
        clone.ID = generateTemplateID()
        clone.Name = t.Name + " (Copy)"
        clone.Status = TemplateStatusDraft
        clone.Version = 1
        now := time.Now()
        clone.CreatedAt = now
        clone.UpdatedAt = now
        clone.Stats = TemplateStats{LastUpdatedAt: now}
        clone.LastUsedAt = nil
        clone.LastValidatedAt = nil
        return &clone
}

func (t *Template) GetContentSize() int64 {
        size := int64(len(t.HTMLContent))
        size += int64(len(t.PlainTextContent))
        for _, subject := range t.Subjects {
                size += int64(len(subject))
        }
        return size
}

func (t *Template) HasSpamIssues() bool {
        if t.SpamAnalysis == nil {
                return false
        }
        return t.SpamAnalysis.IsSpammy || t.SpamScore > 5.0
}

func (t *Template) NeedsRevalidation() bool {
        if t.LastValidatedAt == nil {
                return true
        }
        return time.Since(*t.LastValidatedAt) > 24*time.Hour
}

func (tt TemplateType) IsValid() bool {
        switch tt {
        case TemplateTypeHTML, TemplateTypePlainText, TemplateTypeMixed:
                return true
        }
        return false
}

func (ts TemplateStatus) IsValid() bool {
        switch ts {
        case TemplateStatusDraft, TemplateStatusActive, TemplateStatusInactive, TemplateStatusArchived:
                return true
        }
        return false
}
// KEEP only these (edit the existing ones at ~line 537):
func generateTemplateID() string {
    return uuid.New().String()   // was: fmt.Sprintf("tpl-%d", time.Now().UnixNano())
}

func generateVersionID() string {
    return uuid.New().String()   // was: fmt.Sprintf("ver-%d", time.Now().UnixNano())
}
func (t *Template) GetWordCount() int {
        content := t.PlainTextContent
        if content == "" {
                content = stripHTML(t.HTMLContent)
        }
        words := strings.Fields(content)
        return len(words)
}

func stripHTML(html string) string {
        re := regexp.MustCompile("<[^>]*>")
        return re.ReplaceAllString(html, " ")
}

func (t *Template) GetLinkCount() int {
        re := regexp.MustCompile(`<a\s+[^>]*href=["'][^"']*["'][^>]*>`)
        matches := re.FindAllString(t.HTMLContent, -1)
        return len(matches)
}

func (t *Template) GetImageCount() int {
        re := regexp.MustCompile(`<img\s+[^>]*src=["'][^"']*["'][^>]*>`)
        matches := re.FindAllString(t.HTMLContent, -1)
        return len(matches)
}

func (t *Template) SetSpamScore(score float64, analysis *SpamAnalysis) {
        t.SpamScore = score
        t.SpamAnalysis = analysis
        t.UpdatedAt = time.Now()
}

func (t *Template) SetValidationResult(result *ValidationResult) {
        t.ValidationResult = result
        now := time.Now()
        t.LastValidatedAt = &now
        t.UpdatedAt = now
}
