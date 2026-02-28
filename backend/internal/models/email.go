package models

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type EmailPriority string

const (
	EmailPriorityHigh   EmailPriority = "high"
	EmailPriorityNormal EmailPriority = "normal"
	EmailPriorityLow    EmailPriority = "low"
)

type EmailStatus string

const (
	EmailStatusDraft     EmailStatus = "draft"
	EmailStatusQueued    EmailStatus = "queued"
	EmailStatusSending   EmailStatus = "sending"
	EmailStatusSent      EmailStatus = "sent"
	EmailStatusDelivered EmailStatus = "delivered"
	EmailStatusFailed    EmailStatus = "failed"
	EmailStatusBounced   EmailStatus = "bounced"
)

type Email struct {
	ID                string                 `json:"id"`
	CampaignID        string                 `json:"campaign_id"`
	RecipientID       string                 `json:"recipient_id"`
	AccountID         string                 `json:"account_id"`
	TemplateID        string                 `json:"template_id"`
	MessageID         string                 `json:"message_id"`
	Status            EmailStatus            `json:"status"`
	Priority          EmailPriority          `json:"priority"`
	From              EmailAddress           `json:"from"`
	To                EmailAddress           `json:"to"`
	ReplyTo           *EmailAddress          `json:"reply_to,omitempty"`
	CC                []EmailAddress         `json:"cc,omitempty"`
	BCC               []EmailAddress         `json:"bcc,omitempty"`
	Subject           string                 `json:"subject"`
	HTMLBody          string                 `json:"html_body"`
	PlainTextBody     string                 `json:"plain_text_body"`
	Headers           map[string]string      `json:"headers"`
	Attachments       []EmailAttachment      `json:"attachments"`
	InlineAttachments []EmailAttachment      `json:"inline_attachments"`
	PersonalizationData map[string]interface{} `json:"personalization_data"`
	TrackingEnabled   bool                   `json:"tracking_enabled"`
	TrackingPixelURL  string                 `json:"tracking_pixel_url,omitempty"`
	UnsubscribeURL    string                 `json:"unsubscribe_url,omitempty"`
	ListUnsubscribeHeaders []string          `json:"list_unsubscribe_headers,omitempty"`
	FeedbackID        string                 `json:"feedback_id,omitempty"`
	ReturnPath        string                 `json:"return_path,omitempty"`
	EncodedSize       int64                  `json:"encoded_size"`
	RawSize           int64                  `json:"raw_size"`
	Charset           string                 `json:"charset"`
	ContentType       string                 `json:"content_type"`
	MIMEVersion       string                 `json:"mime_version"`
	SendAttempts      int                    `json:"send_attempts"`
	MaxRetries        int                    `json:"max_retries"`
	LastError         string                 `json:"last_error,omitempty"`
	SMTPResponse      string                 `json:"smtp_response,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	QueuedAt          *time.Time             `json:"queued_at,omitempty"`
	SentAt            *time.Time             `json:"sent_at,omitempty"`
	DeliveredAt       *time.Time             `json:"delivered_at,omitempty"`
	FailedAt          *time.Time             `json:"failed_at,omitempty"`
	UpdatedAt         time.Time              `json:"updated_at"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
}

type EmailAddress struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
}

type EmailAttachment struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Content     []byte `json:"content,omitempty"`
	FilePath    string `json:"file_path,omitempty"`
	Size        int64  `json:"size"`
	Inline      bool   `json:"inline"`
	ContentID   string `json:"content_id,omitempty"`
	Encoding    string `json:"encoding"`
}

func NewEmail(from, to EmailAddress, subject string) *Email {
	now := time.Now()
	return &Email{
		ID:          generateEmailID(),
		Status:      EmailStatusDraft,
		Priority:    EmailPriorityNormal,
		From:        from,
		To:          to,
		Subject:     subject,
		Headers:     make(map[string]string),
		Attachments: []EmailAttachment{},
		InlineAttachments: []EmailAttachment{},
		PersonalizationData: make(map[string]interface{}),
		Charset:     "UTF-8",
		MIMEVersion: "1.0",
		MaxRetries:  3,
		CreatedAt:   now,
		UpdatedAt:   now,
		Metadata:    make(map[string]interface{}),
	}
}

func (e *Email) Validate() error {
	if e.From.Address == "" {
		return errors.New("from address is required")
	}
	if e.To.Address == "" {
		return errors.New("to address is required")
	}
	if e.Subject == "" {
		return errors.New("subject is required")
	}
	if e.HTMLBody == "" && e.PlainTextBody == "" {
		return errors.New("email must have HTML or plain text body")
	}
	if !e.Priority.IsValid() {
		return fmt.Errorf("invalid priority: %s", e.Priority)
	}
	if !e.Status.IsValid() {
		return fmt.Errorf("invalid status: %s", e.Status)
	}
	return nil
}

func (e *Email) GenerateMessageID(domain string) string {
	if domain == "" {
		domain = "localhost"
	}
	
	timestamp := time.Now().UnixNano()
	randomBytes := make([]byte, 16)
	rand.Read(randomBytes)
	randomStr := hex.EncodeToString(randomBytes)
	
	e.MessageID = fmt.Sprintf("<%d.%s@%s>", timestamp, randomStr, domain)
	return e.MessageID
}

func (e *Email) AddAttachment(attachment EmailAttachment) {
	attachment.ID = generateAttachmentID()
	attachment.Inline = false
	
	if attachment.Encoding == "" {
		attachment.Encoding = "base64"
	}
	
	e.Attachments = append(e.Attachments, attachment)
	e.RawSize += attachment.Size
	e.UpdatedAt = time.Now()
}

func (e *Email) AddInlineAttachment(attachment EmailAttachment) {
	attachment.ID = generateAttachmentID()
	attachment.Inline = true
	
	if attachment.ContentID == "" {
		attachment.ContentID = fmt.Sprintf("inline_%s", attachment.ID)
	}
	
	if attachment.Encoding == "" {
		attachment.Encoding = "base64"
	}
	
	e.InlineAttachments = append(e.InlineAttachments, attachment)
	e.RawSize += attachment.Size
	e.UpdatedAt = time.Now()
}

func (e *Email) AddHeader(key, value string) {
	if e.Headers == nil {
		e.Headers = make(map[string]string)
	}
	e.Headers[key] = value
	e.UpdatedAt = time.Now()
}

func (e *Email) SetPriority(priority EmailPriority) error {
	if !priority.IsValid() {
		return fmt.Errorf("invalid priority: %s", priority)
	}
	e.Priority = priority
	
	switch priority {
	case EmailPriorityHigh:
		e.AddHeader("X-Priority", "1")
		e.AddHeader("Importance", "high")
	case EmailPriorityLow:
		e.AddHeader("X-Priority", "5")
		e.AddHeader("Importance", "low")
	default:
		e.AddHeader("X-Priority", "3")
		e.AddHeader("Importance", "normal")
	}
	
	e.UpdatedAt = time.Now()
	return nil
}

func (e *Email) SetFeedbackID(campaignID, accountID string) {
	e.FeedbackID = fmt.Sprintf("%s:%s:%s", campaignID, accountID, e.ID)
	e.AddHeader("Feedback-ID", e.FeedbackID)
	e.UpdatedAt = time.Now()
}

func (e *Email) AddListUnsubscribe(httpURL, emailURL string) {
	var unsubscribeHeaders []string
	
	if httpURL != "" {
		unsubscribeHeaders = append(unsubscribeHeaders, fmt.Sprintf("<%s>", httpURL))
		e.AddHeader("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
	}
	
	if emailURL != "" {
		unsubscribeHeaders = append(unsubscribeHeaders, fmt.Sprintf("<mailto:%s>", emailURL))
	}
	
	if len(unsubscribeHeaders) > 0 {
		e.ListUnsubscribeHeaders = unsubscribeHeaders
		e.AddHeader("List-Unsubscribe", strings.Join(unsubscribeHeaders, ", "))
	}
	
	e.UpdatedAt = time.Now()
}

func (e *Email) AddTrackingPixel(trackingURL string) {
	e.TrackingEnabled = true
	e.TrackingPixelURL = trackingURL
	
	trackingPixel := fmt.Sprintf(`<img src="%s" width="1" height="1" alt="" style="display:none;" />`, trackingURL)
	
	if e.HTMLBody != "" {
		if strings.Contains(strings.ToLower(e.HTMLBody), "</body>") {
			e.HTMLBody = strings.Replace(e.HTMLBody, "</body>", trackingPixel+"</body>", 1)
		} else {
			e.HTMLBody += trackingPixel
		}
	}
	
	e.UpdatedAt = time.Now()
}

func (e *Email) ProcessLinksForTracking(trackingDomain string) {
	if !e.TrackingEnabled || trackingDomain == "" {
		return
	}
	
	e.UpdatedAt = time.Now()
}

func (e *Email) SetReplyTo(address, name string) {
	e.ReplyTo = &EmailAddress{
		Address: address,
		Name:    name,
	}
	e.UpdatedAt = time.Now()
}

func (e *Email) AddCC(address, name string) {
	if e.CC == nil {
		e.CC = []EmailAddress{}
	}
	e.CC = append(e.CC, EmailAddress{Address: address, Name: name})
	e.UpdatedAt = time.Now()
}

func (e *Email) AddBCC(address, name string) {
	if e.BCC == nil {
		e.BCC = []EmailAddress{}
	}
	e.BCC = append(e.BCC, EmailAddress{Address: address, Name: name})
	e.UpdatedAt = time.Now()
}

func (e *Email) MarkAsQueued() {
	e.Status = EmailStatusQueued
	now := time.Now()
	e.QueuedAt = &now
	e.UpdatedAt = now
}

func (e *Email) MarkAsSending() {
	e.Status = EmailStatusSending
	e.SendAttempts++
	e.UpdatedAt = time.Now()
}

func (e *Email) MarkAsSent(messageID, smtpResponse string) {
	e.Status = EmailStatusSent
	if messageID != "" {
		e.MessageID = messageID
	}
	e.SMTPResponse = smtpResponse
	now := time.Now()
	e.SentAt = &now
	e.UpdatedAt = now
}

func (e *Email) MarkAsDelivered() {
	e.Status = EmailStatusDelivered
	now := time.Now()
	e.DeliveredAt = &now
	e.UpdatedAt = now
}

func (e *Email) MarkAsFailed(errorMsg, smtpResponse string) {
	e.Status = EmailStatusFailed
	e.LastError = errorMsg
	e.SMTPResponse = smtpResponse
	now := time.Now()
	e.FailedAt = &now
	e.UpdatedAt = now
}

func (e *Email) MarkAsBounced(errorMsg string) {
	e.Status = EmailStatusBounced
	e.LastError = errorMsg
	e.UpdatedAt = time.Now()
}

func (e *Email) CanRetry() bool {
	return e.Status == EmailStatusFailed && e.SendAttempts < e.MaxRetries
}

func (e *Email) GetSize() int64 {
	size := int64(len(e.Subject))
	size += int64(len(e.HTMLBody))
	size += int64(len(e.PlainTextBody))
	
	for _, att := range e.Attachments {
		size += att.Size
	}
	
	for _, att := range e.InlineAttachments {
		size += att.Size
	}
	
	e.RawSize = size
	e.EncodedSize = int64(float64(size) * 1.37)
	
	return e.EncodedSize
}

func (e *Email) GetTotalAttachmentSize() int64 {
	var total int64
	for _, att := range e.Attachments {
		total += att.Size
	}
	for _, att := range e.InlineAttachments {
		total += att.Size
	}
	return total
}

func (e *Email) HasAttachments() bool {
	return len(e.Attachments) > 0 || len(e.InlineAttachments) > 0
}

func (e *Email) GetAttachmentCount() int {
	return len(e.Attachments) + len(e.InlineAttachments)
}

func (e *Email) BuildMIMEHeaders() map[string]string {
	headers := make(map[string]string)
	
	headers["MIME-Version"] = e.MIMEVersion
	headers["Message-ID"] = e.MessageID
	headers["Date"] = time.Now().Format(time.RFC1123Z)
	headers["From"] = e.From.FormatAddress()
	headers["To"] = e.To.FormatAddress()
	headers["Subject"] = e.Subject
	
	if e.ReplyTo != nil {
		headers["Reply-To"] = e.ReplyTo.FormatAddress()
	}
	
	if len(e.CC) > 0 {
		ccAddresses := make([]string, len(e.CC))
		for i, cc := range e.CC {
			ccAddresses[i] = cc.FormatAddress()
		}
		headers["Cc"] = strings.Join(ccAddresses, ", ")
	}
	
	if e.HTMLBody != "" && e.PlainTextBody != "" {
		e.ContentType = "multipart/alternative"
	} else if e.HasAttachments() {
		e.ContentType = "multipart/mixed"
	} else if e.HTMLBody != "" {
		e.ContentType = fmt.Sprintf("text/html; charset=%s", e.Charset)
	} else {
		e.ContentType = fmt.Sprintf("text/plain; charset=%s", e.Charset)
	}
	
	headers["Content-Type"] = e.ContentType
	
	for key, value := range e.Headers {
		headers[key] = value
	}
	
	return headers
}

func (e *Email) ApplyPersonalization(data map[string]interface{}) {
	if e.PersonalizationData == nil {
		e.PersonalizationData = make(map[string]interface{})
	}
	
	for key, value := range data {
		e.PersonalizationData[key] = value
	}
	
	e.Subject = e.replaceVariables(e.Subject, data)
	e.HTMLBody = e.replaceVariables(e.HTMLBody, data)
	e.PlainTextBody = e.replaceVariables(e.PlainTextBody, data)
	
	e.UpdatedAt = time.Now()
}

func (e *Email) replaceVariables(content string, data map[string]interface{}) string {
	result := content
	for key, value := range data {
		placeholder := fmt.Sprintf("{%s}", key)
		valueStr := fmt.Sprintf("%v", value)
		result = strings.ReplaceAll(result, placeholder, valueStr)
	}
	return result
}

func (e *Email) Clone() *Email {
	clone := *e
	clone.ID = generateEmailID()
	clone.Status = EmailStatusDraft
	clone.SendAttempts = 0
	now := time.Now()
	clone.CreatedAt = now
	clone.UpdatedAt = now
	clone.QueuedAt = nil
	clone.SentAt = nil
	clone.DeliveredAt = nil
	clone.FailedAt = nil
	
	clone.Headers = make(map[string]string)
	for k, v := range e.Headers {
		clone.Headers[k] = v
	}
	
	clone.PersonalizationData = make(map[string]interface{})
	for k, v := range e.PersonalizationData {
		clone.PersonalizationData[k] = v
	}
	
	return &clone
}

func (ea EmailAddress) FormatAddress() string {
	if ea.Name != "" {
		return fmt.Sprintf("%s <%s>", ea.Name, ea.Address)
	}
	return ea.Address
}

func (ep EmailPriority) IsValid() bool {
	switch ep {
	case EmailPriorityHigh, EmailPriorityNormal, EmailPriorityLow:
		return true
	}
	return false
}

func (es EmailStatus) IsValid() bool {
	switch es {
	case EmailStatusDraft, EmailStatusQueued, EmailStatusSending,
		EmailStatusSent, EmailStatusDelivered, EmailStatusFailed, EmailStatusBounced:
		return true
	}
	return false
}

func generateEmailID() string {
	return fmt.Sprintf("eml_%d", time.Now().UnixNano())
}

func generateAttachmentID() string {
	return fmt.Sprintf("att_%d", time.Now().UnixNano())
}

func (e *Email) IsDraft() bool {
	return e.Status == EmailStatusDraft
}

func (e *Email) IsQueued() bool {
	return e.Status == EmailStatusQueued
}

func (e *Email) IsSent() bool {
	return e.Status == EmailStatusSent || e.Status == EmailStatusDelivered
}

func (e *Email) IsFailed() bool {
	return e.Status == EmailStatusFailed || e.Status == EmailStatusBounced
}

func (e *Email) GetTimeSinceSent() time.Duration {
	if e.SentAt == nil {
		return 0
	}
	return time.Since(*e.SentAt)
}

func (e *Email) GetTimeInQueue() time.Duration {
	if e.QueuedAt == nil {
		return 0
	}
	if e.SentAt != nil {
		return e.SentAt.Sub(*e.QueuedAt)
	}
	return time.Since(*e.QueuedAt)
}

func (e *Email) SetReturnPath(returnPath string) {
	e.ReturnPath = returnPath
	e.AddHeader("Return-Path", returnPath)
	e.UpdatedAt = time.Now()
}

func (e *Email) GetRetryDelay() time.Duration {
	baseDelay := 30 * time.Second
	return baseDelay * time.Duration(1<<uint(e.SendAttempts))
}

func (e *Email) ShouldRetry() bool {
	if !e.CanRetry() {
		return false
	}
	
	if e.FailedAt == nil {
		return true
	}
	
	retryDelay := e.GetRetryDelay()
	return time.Since(*e.FailedAt) >= retryDelay
}

func (e *Email) RemoveAttachment(attachmentID string) bool {
	for i, att := range e.Attachments {
		if att.ID == attachmentID {
			e.Attachments = append(e.Attachments[:i], e.Attachments[i+1:]...)
			e.RawSize -= att.Size
			e.UpdatedAt = time.Now()
			return true
		}
	}
	
	for i, att := range e.InlineAttachments {
		if att.ID == attachmentID {
			e.InlineAttachments = append(e.InlineAttachments[:i], e.InlineAttachments[i+1:]...)
			e.RawSize -= att.Size
			e.UpdatedAt = time.Now()
			return true
		}
	}
	
	return false
}

func (e *Email) ClearAttachments() {
	e.Attachments = []EmailAttachment{}
	e.InlineAttachments = []EmailAttachment{}
	e.RawSize = int64(len(e.Subject) + len(e.HTMLBody) + len(e.PlainTextBody))
	e.UpdatedAt = time.Now()
}

func (e *Email) GetHeader(key string) (string, bool) {
	if e.Headers == nil {
		return "", false
	}
	value, exists := e.Headers[key]
	return value, exists
}

func (e *Email) RemoveHeader(key string) {
	if e.Headers != nil {
		delete(e.Headers, key)
		e.UpdatedAt = time.Now()
	}
}

func (e *Email) IsMultipart() bool {
	return strings.HasPrefix(e.ContentType, "multipart/")
}

func (e *Email) GetContentTypeBase() string {
	if idx := strings.Index(e.ContentType, ";"); idx > 0 {
		return strings.TrimSpace(e.ContentType[:idx])
	}
	return e.ContentType
}
