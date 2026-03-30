package models

import (
        "errors"
        "fmt"
        "net/mail"
        "regexp"
        "strings"
        "time"
)

type RecipientStatus string

const (
        RecipientStatusPending    RecipientStatus = "pending"
        RecipientStatusQueued     RecipientStatus = "queued"
        RecipientStatusSending    RecipientStatus = "sending"
        RecipientStatusSent       RecipientStatus = "sent"
        RecipientStatusDelivered  RecipientStatus = "delivered"
        RecipientStatusFailed     RecipientStatus = "failed"
        RecipientStatusBounced    RecipientStatus = "bounced"
        RecipientStatusHardBounce RecipientStatus = "hard_bounce"
        RecipientStatusSoftBounce RecipientStatus = "soft_bounce"
        RecipientStatusComplaint  RecipientStatus = "complaint"
        RecipientStatusUnsubscribed RecipientStatus = "unsubscribed"
        RecipientStatusSkipped    RecipientStatus = "skipped"
        RecipientStatusInvalid    RecipientStatus = "invalid"
)

type RecipientSource string

const (
        RecipientSourceManual   RecipientSource = "manual"
        RecipientSourceCSV      RecipientSource = "csv"
        RecipientSourceTXT      RecipientSource = "txt"
        RecipientSourceAPI      RecipientSource = "api"
        RecipientSourceImport   RecipientSource = "import"
)

type Recipient struct {
        ID                string            `json:"id" db:"id"`
        TenantID          string            `json:"tenant_id" db:"tenant_id"`
        CampaignID        string            `json:"campaign_id" db:"campaign_id"`
        ListID            string            `json:"list_id" db:"recipient_list_id"`
        Email             string            `json:"email" db:"email"`
        FirstName         string            `json:"first_name" db:"first_name"`
        LastName          string            `json:"last_name" db:"last_name"`
        FullName          string            `json:"full_name" db:"full_name"`
        Status            RecipientStatus   `json:"status" db:"status"`
        Source            RecipientSource   `json:"source" db:"source"`
        IsValid           bool              `json:"is_valid" db:"is_valid"`
        IsBlacklisted     bool              `json:"is_blacklisted" db:"is_blacklisted"`
        IsDuplicate       bool              `json:"is_duplicate" db:"is_duplicate"`
        Tags              []string          `json:"tags" db:"tags"`
        CustomFields      map[string]string `json:"custom_fields"`
        PersonalizationData map[string]interface{} `json:"personalization_data"`
        ValidationResult  *EmailValidation  `json:"validation_result,omitempty"`
        DeliveryInfo      DeliveryInfo      `json:"delivery_info"`
        Stats             RecipientStats    `json:"stats"`
        AccountID         string            `json:"account_id" db:"account_id"`
        TemplateID        string            `json:"template_id" db:"template_id"`
        SubjectUsed       string            `json:"subject_used" db:"subject_used"`
        SenderNameUsed    string            `json:"sender_name_used" db:"sender_name_used"`
        MessageID         string            `json:"message_id" db:"message_id"`
        Priority          int               `json:"priority" db:"priority"`
        RetryCount        int               `json:"retry_count" db:"retry_count"`
        MaxRetries        int               `json:"max_retries" db:"max_retries"`
        LastError         string            `json:"last_error" db:"last_error"`
        LastErrorAt       *time.Time        `json:"last_error_at" db:"last_error_at"`
        ScheduledAt       *time.Time        `json:"scheduled_at" db:"scheduled_at"`
        QueuedAt          *time.Time        `json:"queued_at" db:"queued_at"`
        SentAt            *time.Time        `json:"sent_at" db:"sent_at"`
        DeliveredAt       *time.Time        `json:"delivered_at" db:"delivered_at"`
        BouncedAt         *time.Time        `json:"bounced_at" db:"bounced_at"`
        FailedAt          *time.Time        `json:"failed_at" db:"failed_at"`
        UnsubscribedAt    *time.Time        `json:"unsubscribed_at" db:"unsubscribed_at"`
        CreatedAt         time.Time         `json:"created_at" db:"created_at"`
        UpdatedAt         time.Time         `json:"updated_at" db:"updated_at"`
        ImportedAt        *time.Time        `json:"imported_at" db:"imported_at"`
        ImportBatchID     string            `json:"import_batch_id" db:"import_batch_id"`
        Metadata          map[string]interface{} `json:"metadata,omitempty"`
}

type EmailValidation struct {
        IsValid           bool      `json:"is_valid"`
        IsFormatValid     bool      `json:"is_format_valid"`
        IsDomainValid     bool      `json:"is_domain_valid"`
        HasMXRecord       bool      `json:"has_mx_record"`
        IsDisposable      bool      `json:"is_disposable"`
        IsRoleAccount     bool      `json:"is_role_account"`
        IsFreeProvider    bool      `json:"is_free_provider"`
        Domain            string    `json:"domain"`
        Username          string    `json:"username"`
        SuggestedEmail    string    `json:"suggested_email,omitempty"`
        ValidationErrors  []string  `json:"validation_errors,omitempty"`
        ValidatedAt       time.Time `json:"validated_at"`
}

type DeliveryInfo struct {
        AttemptCount       int       `json:"attempt_count"`
        LastAttemptAt      *time.Time `json:"last_attempt_at,omitempty"`
        ResponseCode       int       `json:"response_code"`
        ResponseMessage    string    `json:"response_message"`
        SMTPResponse       string    `json:"smtp_response"`
        BounceType         string    `json:"bounce_type,omitempty"`
        BounceReason       string    `json:"bounce_reason,omitempty"`
        ComplaintType      string    `json:"complaint_type,omitempty"`
        ComplaintFeedback  string    `json:"complaint_feedback,omitempty"`
        UnsubscribeMethod  string    `json:"unsubscribe_method,omitempty"`
        UnsubscribeReason  string    `json:"unsubscribe_reason,omitempty"`
        IPAddress          string    `json:"ip_address,omitempty"`
        UserAgent          string    `json:"user_agent,omitempty"`
}

type RecipientStats struct {
        OpenCount          int       `json:"open_count"`
        ClickCount         int       `json:"click_count"`
        UniqueOpens        int       `json:"unique_opens"`
        UniqueClicks       int       `json:"unique_clicks"`
        FirstOpenedAt      *time.Time `json:"first_opened_at,omitempty"`
        LastOpenedAt       *time.Time `json:"last_opened_at,omitempty"`
        FirstClickedAt     *time.Time `json:"first_clicked_at,omitempty"`
        LastClickedAt      *time.Time `json:"last_clicked_at,omitempty"`
        LinksClicked       []string  `json:"links_clicked,omitempty"`
        OpenLocations      []string  `json:"open_locations,omitempty"`
        OpenDevices        []string  `json:"open_devices,omitempty"`
}

func NewRecipient(email, tenantID string) (*Recipient, error) {
        if err := validateEmailAddress(email); err != nil {
                return nil, err
        }

        now := time.Now()
        recipient := &Recipient{
                ID:          generateRecipientID(),
                TenantID:    tenantID,
                Email:       strings.ToLower(strings.TrimSpace(email)),
                Status:      RecipientStatusPending,
                Source:      RecipientSourceManual,
                IsValid:     true,
                Tags:        []string{},
                CustomFields: make(map[string]string),
                PersonalizationData: make(map[string]interface{}),
                Priority:    100,
                MaxRetries:  3,
                CreatedAt:   now,
                UpdatedAt:   now,
                Metadata:    make(map[string]interface{}),
        }

        recipient.extractNameFromEmail()
        
        return recipient, nil
}

func NewRecipientFromCSV(email string, fields map[string]string, tenantID string) (*Recipient, error) {
        recipient, err := NewRecipient(email, tenantID)
        if err != nil {
                return nil, err
        }

        recipient.Source = RecipientSourceCSV
        now := time.Now()
        recipient.ImportedAt = &now

        if firstName, ok := fields["first_name"]; ok {
                recipient.FirstName = firstName
        }
        if lastName, ok := fields["last_name"]; ok {
                recipient.LastName = lastName
        }
        if fullName, ok := fields["full_name"]; ok {
                recipient.FullName = fullName
        }

        for key, value := range fields {
                if key != "email" && key != "first_name" && key != "last_name" && key != "full_name" {
                        recipient.CustomFields[key] = value
                }
        }

        if recipient.FullName == "" && recipient.FirstName != "" {
                if recipient.LastName != "" {
                        recipient.FullName = recipient.FirstName + " " + recipient.LastName
                } else {
                        recipient.FullName = recipient.FirstName
                }
        }

        return recipient, nil
}

func (r *Recipient) Validate() error {
        if r.Email == "" {
                return errors.New("email is required")
        }
        if err := validateEmailAddress(r.Email); err != nil {
                return err
        }
        if r.TenantID == "" {
                return errors.New("tenant ID is required")
        }
        if !r.Status.IsValid() {
                return fmt.Errorf("invalid status: %s", r.Status)
        }
        if r.Priority < 0 || r.Priority > 1000 {
                return errors.New("priority must be between 0 and 1000")
        }
        return nil
}

func (r *Recipient) IsPending() bool {
        return r.Status == RecipientStatusPending || r.Status == RecipientStatusQueued
}

func (r *Recipient) IsSent() bool {
        return r.Status == RecipientStatusSent || r.Status == RecipientStatusDelivered
}

func (r *Recipient) IsFailed() bool {
        return r.Status == RecipientStatusFailed
}

func (r *Recipient) IsBounced() bool {
        return r.Status == RecipientStatusBounced || 
                   r.Status == RecipientStatusHardBounce || 
                   r.Status == RecipientStatusSoftBounce
}

func (r *Recipient) CanRetry() bool {
        if !r.IsFailed() {
                return false
        }
        return r.RetryCount < r.MaxRetries
}

func (r *Recipient) MarkAsQueued() {
        r.Status = RecipientStatusQueued
        now := time.Now()
        r.QueuedAt = &now
        r.UpdatedAt = now
}

func (r *Recipient) MarkAsSending() {
        r.Status = RecipientStatusSending
        r.DeliveryInfo.AttemptCount++
        now := time.Now()
        r.DeliveryInfo.LastAttemptAt = &now
        r.UpdatedAt = now
}

func (r *Recipient) MarkAsSent(accountID, templateID, messageID string) {
        r.Status = RecipientStatusSent
        r.AccountID = accountID
        r.TemplateID = templateID
        r.MessageID = messageID
        now := time.Now()
        r.SentAt = &now
        r.UpdatedAt = now
}

func (r *Recipient) MarkAsDelivered() {
        r.Status = RecipientStatusDelivered
        now := time.Now()
        r.DeliveredAt = &now
        r.UpdatedAt = now
}

func (r *Recipient) MarkAsFailed(errorMsg string) {
        r.Status = RecipientStatusFailed
        r.LastError = errorMsg
        r.RetryCount++
        now := time.Now()
        r.LastErrorAt = &now
        r.FailedAt = &now
        r.UpdatedAt = now
}

func (r *Recipient) MarkAsBounced(bounceType, bounceReason string) {
        if bounceType == "hard" {
                r.Status = RecipientStatusHardBounce
        } else if bounceType == "soft" {
                r.Status = RecipientStatusSoftBounce
        } else {
                r.Status = RecipientStatusBounced
        }
        
        r.DeliveryInfo.BounceType = bounceType
        r.DeliveryInfo.BounceReason = bounceReason
        
        now := time.Now()
        r.BouncedAt = &now
        r.UpdatedAt = now
}

func (r *Recipient) MarkAsComplaint(complaintType, feedback string) {
        r.Status = RecipientStatusComplaint
        r.DeliveryInfo.ComplaintType = complaintType
        r.DeliveryInfo.ComplaintFeedback = feedback
        r.UpdatedAt = time.Now()
}

func (r *Recipient) MarkAsUnsubscribed(method, reason string) {
        r.Status = RecipientStatusUnsubscribed
        r.DeliveryInfo.UnsubscribeMethod = method
        r.DeliveryInfo.UnsubscribeReason = reason
        now := time.Now()
        r.UnsubscribedAt = &now
        r.UpdatedAt = now
}

func (r *Recipient) MarkAsSkipped(reason string) {
        r.Status = RecipientStatusSkipped
        r.LastError = reason
        r.UpdatedAt = time.Now()
}

func (r *Recipient) MarkAsInvalid(reason string) {
        r.Status = RecipientStatusInvalid
        r.IsValid = false
        r.LastError = reason
        r.UpdatedAt = time.Now()
}

func (r *Recipient) RecordOpen(ipAddress, userAgent string) {
        r.Stats.OpenCount++
        
        now := time.Now()
        if r.Stats.FirstOpenedAt == nil {
                r.Stats.FirstOpenedAt = &now
                r.Stats.UniqueOpens = 1
        }
        r.Stats.LastOpenedAt = &now
        
        if ipAddress != "" {
                r.DeliveryInfo.IPAddress = ipAddress
                if !contains(r.Stats.OpenLocations, ipAddress) {
                        r.Stats.OpenLocations = append(r.Stats.OpenLocations, ipAddress)
                }
        }
        
        if userAgent != "" {
                r.DeliveryInfo.UserAgent = userAgent
                if !contains(r.Stats.OpenDevices, userAgent) {
                        r.Stats.OpenDevices = append(r.Stats.OpenDevices, userAgent)
                }
        }
        
        r.UpdatedAt = now
}

func (r *Recipient) RecordClick(link, ipAddress, userAgent string) {
        r.Stats.ClickCount++
        
        now := time.Now()
        if r.Stats.FirstClickedAt == nil {
                r.Stats.FirstClickedAt = &now
                r.Stats.UniqueClicks = 1
        }
        r.Stats.LastClickedAt = &now
        
        if link != "" && !contains(r.Stats.LinksClicked, link) {
                r.Stats.LinksClicked = append(r.Stats.LinksClicked, link)
        }
        
        if ipAddress != "" {
                r.DeliveryInfo.IPAddress = ipAddress
        }
        
        if userAgent != "" {
                r.DeliveryInfo.UserAgent = userAgent
        }
        
        r.UpdatedAt = now
}

func (r *Recipient) extractNameFromEmail() {
        if r.Email == "" {
                return
        }

        parts := strings.Split(r.Email, "@")
        if len(parts) != 2 {
                return
        }

        username := parts[0]
        
        username = strings.ReplaceAll(username, ".", " ")
        username = strings.ReplaceAll(username, "_", " ")
        username = strings.ReplaceAll(username, "-", " ")
        username = strings.ReplaceAll(username, "+", " ")
        
        re := regexp.MustCompile(`\d+`)
        username = re.ReplaceAllString(username, "")
        
        username = strings.TrimSpace(username)
        
        if username == "" {
                return
        }

        words := strings.Fields(username)
        if len(words) == 0 {
                return
        }

        for i, word := range words {
                if len(word) > 0 {
                        words[i] = strings.ToUpper(string(word[0])) + strings.ToLower(word[1:])
                }
        }

        if r.FirstName == "" && len(words) > 0 {
                r.FirstName = words[0]
        }

        if r.LastName == "" && len(words) > 1 {
                r.LastName = words[len(words)-1]
        }

        if r.FullName == "" {
                r.FullName = strings.Join(words, " ")
        }
}

// GetPersonalizationData — function: GetPersonalizationData, receiver: *Recipient
func (r *Recipient) GetPersonalizationData() map[string]interface{} {
    // Always recompute — do NOT cache in r.PersonalizationData to avoid stale values
    data := make(map[string]interface{})

    data["EMAIL"]      = r.Email
    data["FIRST_NAME"] = r.FirstName
    data["LAST_NAME"]  = r.LastName
    data["FULL_NAME"]  = r.FullName  // ← now populated by toModelRecipient Fix 2
    data["NAME"]       = r.getPreferredName()

    for key, value := range r.CustomFields {
        data[strings.ToUpper(key)] = value
    }
    return data
}


func (r *Recipient) getPreferredName() string {
        if r.FirstName != "" {
                return r.FirstName
        }
        if r.FullName != "" {
                return r.FullName
        }
        if r.LastName != "" {
                return r.LastName
        }
        return "there"
}

func (r *Recipient) SetCustomField(key, value string) {
        if r.CustomFields == nil {
                r.CustomFields = make(map[string]string)
        }
        r.CustomFields[key] = value
        r.UpdatedAt = time.Now()
}

func (r *Recipient) GetCustomField(key string) (string, bool) {
        if r.CustomFields == nil {
                return "", false
        }
        value, exists := r.CustomFields[key]
        return value, exists
}

func (r *Recipient) SetValidationResult(result *EmailValidation) {
        r.ValidationResult = result
        r.IsValid = result.IsValid
        r.UpdatedAt = time.Now()
}

func (r *Recipient) GetDomain() string {
        parts := strings.Split(r.Email, "@")
        if len(parts) == 2 {
                return parts[1]
        }
        return ""
}

func (r *Recipient) GetUsername() string {
        parts := strings.Split(r.Email, "@")
        if len(parts) == 2 {
                return parts[0]
        }
        return ""
}

func (r *Recipient) Clone() *Recipient {
        clone := *r
        clone.ID = generateRecipientID()
        clone.Status = RecipientStatusPending
        clone.RetryCount = 0
        now := time.Now()
        clone.CreatedAt = now
        clone.UpdatedAt = now
        clone.SentAt = nil
        clone.DeliveredAt = nil
        clone.BouncedAt = nil
        clone.FailedAt = nil
        clone.LastErrorAt = nil
        clone.Stats = RecipientStats{}
        return &clone
}

func (r *Recipient) ResetForRetry() {
        r.Status = RecipientStatusPending
        r.LastError = ""
        r.LastErrorAt = nil
        r.UpdatedAt = time.Now()
}

func (r *Recipient) GetEngagementScore() float64 {
        score := 0.0
        
        if r.Stats.OpenCount > 0 {
                score += 10.0
        }
        if r.Stats.ClickCount > 0 {
                score += 20.0
        }
        
        score += float64(r.Stats.OpenCount) * 2.0
        score += float64(r.Stats.ClickCount) * 5.0
        
        if r.IsBounced() {
                score -= 50.0
        }
        if r.Status == RecipientStatusComplaint {
                score -= 100.0
        }
        
        if score < 0 {
                score = 0
        }
        if score > 100 {
                score = 100
        }
        
        return score
}

func (rs RecipientStatus) IsValid() bool {
        switch rs {
        case RecipientStatusPending, RecipientStatusQueued, RecipientStatusSending,
                RecipientStatusSent, RecipientStatusDelivered, RecipientStatusFailed,
                RecipientStatusBounced, RecipientStatusHardBounce, RecipientStatusSoftBounce,
                RecipientStatusComplaint, RecipientStatusUnsubscribed, RecipientStatusSkipped,
                RecipientStatusInvalid:
                return true
        }
        return false
}

func (rs RecipientSource) IsValid() bool {
        switch rs {
        case RecipientSourceManual, RecipientSourceCSV, RecipientSourceTXT,
                RecipientSourceAPI, RecipientSourceImport:
                return true
        }
        return false
}

func validateEmailAddress(email string) error {
        email = strings.TrimSpace(email)
        if email == "" {
                return errors.New("email address is required")
        }
        
        _, err := mail.ParseAddress(email)
        if err != nil {
                return fmt.Errorf("invalid email address format: %w", err)
        }
        
        if len(email) > 254 {
                return errors.New("email address is too long")
        }
        
        parts := strings.Split(email, "@")
        if len(parts) != 2 {
                return errors.New("email must contain exactly one @ symbol")
        }
        
        if len(parts[0]) == 0 || len(parts[0]) > 64 {
                return errors.New("email username must be between 1 and 64 characters")
        }
        
        if len(parts[1]) == 0 || len(parts[1]) > 255 {
                return errors.New("email domain must be between 1 and 255 characters")
        }
        
        return nil
}

func generateRecipientID() string {
        return fmt.Sprintf("rcp_%d", time.Now().UnixNano())
}

func contains(slice []string, item string) bool {
        for _, s := range slice {
                if s == item {
                        return true
                }
        }
        return false
}

func (r *Recipient) IsEngaged() bool {
        return r.Stats.OpenCount > 0 || r.Stats.ClickCount > 0
}

func (r *Recipient) HasOpened() bool {
        return r.Stats.OpenCount > 0
}

func (r *Recipient) HasClicked() bool {
        return r.Stats.ClickCount > 0
}

func (r *Recipient) GetTimeSinceSent() time.Duration {
        if r.SentAt == nil {
                return 0
        }
        return time.Since(*r.SentAt)
}

func (r *Recipient) GetDeliveryDuration() time.Duration {
        if r.SentAt == nil || r.DeliveredAt == nil {
                return 0
        }
        return r.DeliveredAt.Sub(*r.SentAt)
}

func (r *Recipient) ShouldBlacklist() bool {
        return r.Status == RecipientStatusHardBounce || 
                   r.Status == RecipientStatusComplaint ||
                   r.IsBlacklisted
}

func (r *Recipient) AddTag(tag string) {
        if r.Tags == nil {
                r.Tags = []string{}
        }
        if !contains(r.Tags, tag) {
                r.Tags = append(r.Tags, tag)
                r.UpdatedAt = time.Now()
        }
}

func (r *Recipient) RemoveTag(tag string) {
        if r.Tags == nil {
                return
        }
        for i, t := range r.Tags {
                if t == tag {
                        r.Tags = append(r.Tags[:i], r.Tags[i+1:]...)
                        r.UpdatedAt = time.Now()
                        break
                }
        }
}

func (r *Recipient) HasTag(tag string) bool {
        return contains(r.Tags, tag)
}
