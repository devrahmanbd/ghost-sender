package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type NotificationType string

const (
	NotificationTypeCampaignStarted   NotificationType = "campaign_started"
	NotificationTypeCampaignCompleted NotificationType = "campaign_completed"
	NotificationTypeCampaignPaused    NotificationType = "campaign_paused"
	NotificationTypeCampaignResumed   NotificationType = "campaign_resumed"
	NotificationTypeCampaignFailed    NotificationType = "campaign_failed"
	NotificationTypeAccountSuspended  NotificationType = "account_suspended"
	NotificationTypeAccountRestored   NotificationType = "account_restored"
	NotificationTypeSystemAlert       NotificationType = "system_alert"
	NotificationTypeErrorAlert        NotificationType = "error_alert"
	NotificationTypeWarningAlert      NotificationType = "warning_alert"
	NotificationTypeProgressUpdate    NotificationType = "progress_update"
	NotificationTypeMilestone         NotificationType = "milestone"
	NotificationTypeTest              NotificationType = "test"
)

type NotificationChannel string

const (
	NotificationChannelTelegram NotificationChannel = "telegram"
	NotificationChannelEmail    NotificationChannel = "email"
	NotificationChannelWebhook  NotificationChannel = "webhook"
	NotificationChannelWebSocket NotificationChannel = "websocket"
)

type NotificationStatus string

const (
	NotificationStatusPending   NotificationStatus = "pending"
	NotificationStatusQueued    NotificationStatus = "queued"
	NotificationStatusSending   NotificationStatus = "sending"
	NotificationStatusSent      NotificationStatus = "sent"
	NotificationStatusFailed    NotificationStatus = "failed"
	NotificationStatusCancelled NotificationStatus = "cancelled"
)

type NotificationPriority string

const (
	NotificationPriorityLow      NotificationPriority = "low"
	NotificationPriorityNormal   NotificationPriority = "normal"
	NotificationPriorityHigh     NotificationPriority = "high"
	NotificationPriorityCritical NotificationPriority = "critical"
)

type NotificationFormat string

const (
	NotificationFormatPlain    NotificationFormat = "plain"
	NotificationFormatMarkdown NotificationFormat = "markdown"
	NotificationFormatHTML     NotificationFormat = "html"
)

type Notification struct {
	ID             string                 `json:"id"`
	TenantID       string                 `json:"tenant_id"`
	Type           NotificationType       `json:"type"`
	Channel        NotificationChannel    `json:"channel"`
	Status         NotificationStatus     `json:"status"`
	Priority       NotificationPriority   `json:"priority"`
	Format         NotificationFormat     `json:"format"`
	Title          string                 `json:"title"`
	Message        string                 `json:"message"`
	ShortMessage   string                 `json:"short_message,omitempty"`
	CampaignID     string                 `json:"campaign_id,omitempty"`
	CampaignName   string                 `json:"campaign_name,omitempty"`
	RecipientID    string                 `json:"recipient_id,omitempty"`
	Recipient      string                 `json:"recipient"`
	Data           map[string]interface{} `json:"data,omitempty"`
	Attachments    []NotificationAttachment `json:"attachments,omitempty"`
	Buttons        []NotificationButton   `json:"buttons,omitempty"`
	InlineKeyboard [][]NotificationButton `json:"inline_keyboard,omitempty"`
	SendAttempts   int                    `json:"send_attempts"`
	MaxRetries     int                    `json:"max_retries"`
	LastError      string                 `json:"last_error,omitempty"`
	Response       string                 `json:"response,omitempty"`
	MessageID      string                 `json:"message_id,omitempty"`
	ChatID         string                 `json:"chat_id,omitempty"`
	WebhookURL     string                 `json:"webhook_url,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	QueuedAt       *time.Time             `json:"queued_at,omitempty"`
	SentAt         *time.Time             `json:"sent_at,omitempty"`
	FailedAt       *time.Time             `json:"failed_at,omitempty"`
	ScheduledFor   *time.Time             `json:"scheduled_for,omitempty"`
	ExpiresAt      *time.Time             `json:"expires_at,omitempty"`
	UpdatedAt      time.Time              `json:"updated_at"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

type NotificationAttachment struct {
	Type        string `json:"type"`
	URL         string `json:"url,omitempty"`
	FilePath    string `json:"file_path,omitempty"`
	Filename    string `json:"filename"`
	Caption     string `json:"caption,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

type NotificationButton struct {
	Text         string `json:"text"`
	URL          string `json:"url,omitempty"`
	CallbackData string `json:"callback_data,omitempty"`
	Action       string `json:"action,omitempty"`
}

type TelegramConfig struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	BotToken      string    `json:"bot_token"`
	ChatID        string    `json:"chat_id"`
	Username      string    `json:"username,omitempty"`
	Enabled       bool      `json:"enabled"`
	ParseMode     string    `json:"parse_mode"`
	WebhookURL    string    `json:"webhook_url,omitempty"`
	WebhookSecret string    `json:"webhook_secret,omitempty"`
	LastTestAt    *time.Time `json:"last_test_at,omitempty"`
	LastTestStatus string    `json:"last_test_status,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type NotificationTemplate struct {
	ID          string                 `json:"id"`
	TenantID    string                 `json:"tenant_id"`
	Name        string                 `json:"name"`
	Type        NotificationType       `json:"type"`
	Channel     NotificationChannel    `json:"channel"`
	Format      NotificationFormat     `json:"format"`
	Title       string                 `json:"title"`
	Body        string                 `json:"body"`
	Variables   []string               `json:"variables"`
	IsDefault   bool                   `json:"is_default"`
	Enabled     bool                   `json:"enabled"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

func NewNotification(notifType NotificationType, channel NotificationChannel, title, message, recipient, tenantID string) *Notification {
	now := time.Now()
	return &Notification{
		ID:         generateNotificationID(),
		TenantID:   tenantID,
		Type:       notifType,
		Channel:    channel,
		Status:     NotificationStatusPending,
		Priority:   NotificationPriorityNormal,
		Format:     NotificationFormatMarkdown,
		Title:      title,
		Message:    message,
		Recipient:  recipient,
		MaxRetries: 3,
		CreatedAt:  now,
		UpdatedAt:  now,
		Data:       make(map[string]interface{}),
		Metadata:   make(map[string]interface{}),
	}
}

func NewTelegramNotification(notifType NotificationType, title, message, chatID, tenantID string) *Notification {
	notif := NewNotification(notifType, NotificationChannelTelegram, title, message, chatID, tenantID)
	notif.ChatID = chatID
	notif.Format = NotificationFormatMarkdown
	return notif
}

func (n *Notification) Validate() error {
	if n.TenantID == "" {
		return errors.New("tenant ID is required")
	}
	if n.Recipient == "" && n.ChatID == "" && n.WebhookURL == "" {
		return errors.New("recipient, chat ID, or webhook URL is required")
	}
	if n.Message == "" {
		return errors.New("message is required")
	}
	if !n.Type.IsValid() {
		return fmt.Errorf("invalid notification type: %s", n.Type)
	}
	if !n.Channel.IsValid() {
		return fmt.Errorf("invalid notification channel: %s", n.Channel)
	}
	if !n.Status.IsValid() {
		return fmt.Errorf("invalid notification status: %s", n.Status)
	}
	if !n.Priority.IsValid() {
		return fmt.Errorf("invalid notification priority: %s", n.Priority)
	}
	return nil
}

func (n *Notification) SetPriority(priority NotificationPriority) error {
	if !priority.IsValid() {
		return fmt.Errorf("invalid priority: %s", priority)
	}
	n.Priority = priority
	n.UpdatedAt = time.Now()
	return nil
}

func (n *Notification) SetFormat(format NotificationFormat) error {
	if !format.IsValid() {
		return fmt.Errorf("invalid format: %s", format)
	}
	n.Format = format
	n.UpdatedAt = time.Now()
	return nil
}

func (n *Notification) AddData(key string, value interface{}) {
	if n.Data == nil {
		n.Data = make(map[string]interface{})
	}
	n.Data[key] = value
	n.UpdatedAt = time.Now()
}

func (n *Notification) AddAttachment(attachment NotificationAttachment) {
	n.Attachments = append(n.Attachments, attachment)
	n.UpdatedAt = time.Now()
}

func (n *Notification) AddButton(button NotificationButton) {
	n.Buttons = append(n.Buttons, button)
	n.UpdatedAt = time.Now()
}

func (n *Notification) AddInlineKeyboardRow(buttons []NotificationButton) {
	n.InlineKeyboard = append(n.InlineKeyboard, buttons)
	n.UpdatedAt = time.Now()
}

func (n *Notification) MarkAsQueued() {
	n.Status = NotificationStatusQueued
	now := time.Now()
	n.QueuedAt = &now
	n.UpdatedAt = now
}

func (n *Notification) MarkAsSending() {
	n.Status = NotificationStatusSending
	n.SendAttempts++
	n.UpdatedAt = time.Now()
}

func (n *Notification) MarkAsSent(messageID, response string) {
	n.Status = NotificationStatusSent
	n.MessageID = messageID
	n.Response = response
	now := time.Now()
	n.SentAt = &now
	n.UpdatedAt = now
}

func (n *Notification) MarkAsFailed(errorMsg string) {
	n.Status = NotificationStatusFailed
	n.LastError = errorMsg
	now := time.Now()
	n.FailedAt = &now
	n.UpdatedAt = now
}

func (n *Notification) MarkAsCancelled() {
	n.Status = NotificationStatusCancelled
	n.UpdatedAt = time.Now()
}

func (n *Notification) CanRetry() bool {
	return n.Status == NotificationStatusFailed && n.SendAttempts < n.MaxRetries
}

func (n *Notification) GetRetryDelay() time.Duration {
	baseDelay := 5 * time.Second
	return baseDelay * time.Duration(1<<uint(n.SendAttempts))
}

func (n *Notification) ShouldRetry() bool {
	if !n.CanRetry() {
		return false
	}
	
	if n.FailedAt == nil {
		return true
	}
	
	retryDelay := n.GetRetryDelay()
	return time.Since(*n.FailedAt) >= retryDelay
}

func (n *Notification) IsExpired() bool {
	if n.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*n.ExpiresAt)
}

func (n *Notification) SetExpiration(duration time.Duration) {
	expiresAt := time.Now().Add(duration)
	n.ExpiresAt = &expiresAt
	n.UpdatedAt = time.Now()
}

func (n *Notification) SetSchedule(scheduledFor time.Time) {
	n.ScheduledFor = &scheduledFor
	n.UpdatedAt = time.Now()
}

func (n *Notification) IsScheduled() bool {
	return n.ScheduledFor != nil && time.Now().Before(*n.ScheduledFor)
}

func (n *Notification) ShouldSend() bool {
	if n.IsExpired() {
		return false
	}
	if n.IsScheduled() {
		return false
	}
	if n.Status == NotificationStatusSent || n.Status == NotificationStatusCancelled {
		return false
	}
	return true
}

func (n *Notification) IsPending() bool {
	return n.Status == NotificationStatusPending || n.Status == NotificationStatusQueued
}

func (n *Notification) IsSent() bool {
	return n.Status == NotificationStatusSent
}

func (n *Notification) IsFailed() bool {
	return n.Status == NotificationStatusFailed
}

func (n *Notification) FormatMessage(data map[string]interface{}) string {
	message := n.Message
	
	for key, value := range data {
		placeholder := fmt.Sprintf("{%s}", key)
		valueStr := fmt.Sprintf("%v", value)
		message = replaceAll(message, placeholder, valueStr)
	}
	
	if n.Data != nil {
		for key, value := range n.Data {
			placeholder := fmt.Sprintf("{%s}", key)
			valueStr := fmt.Sprintf("%v", value)
			message = replaceAll(message, placeholder, valueStr)
		}
	}
	
	return message
}

func (n *Notification) FormatTitle(data map[string]interface{}) string {
	title := n.Title
	
	for key, value := range data {
		placeholder := fmt.Sprintf("{%s}", key)
		valueStr := fmt.Sprintf("%v", value)
		title = replaceAll(title, placeholder, valueStr)
	}
	
	return title
}

func (n *Notification) GetFormattedMessage(data map[string]interface{}) string {
	switch n.Format {
	case NotificationFormatMarkdown:
		return n.FormatMessage(data)
	case NotificationFormatHTML:
		return n.FormatMessage(data)
	default:
		return n.FormatMessage(data)
	}
}

func (n *Notification) ToJSON() (string, error) {
	data, err := json.Marshal(n)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (n *Notification) Clone() *Notification {
	clone := *n
	clone.ID = generateNotificationID()
	clone.Status = NotificationStatusPending
	clone.SendAttempts = 0
	now := time.Now()
	clone.CreatedAt = now
	clone.UpdatedAt = now
	clone.QueuedAt = nil
	clone.SentAt = nil
	clone.FailedAt = nil
	
	if n.Data != nil {
		clone.Data = make(map[string]interface{})
		for k, v := range n.Data {
			clone.Data[k] = v
		}
	}
	
	if n.Metadata != nil {
		clone.Metadata = make(map[string]interface{})
		for k, v := range n.Metadata {
			clone.Metadata[k] = v
		}
	}
	
	return &clone
}

func (nt NotificationType) IsValid() bool {
	switch nt {
	case NotificationTypeCampaignStarted, NotificationTypeCampaignCompleted,
		NotificationTypeCampaignPaused, NotificationTypeCampaignResumed,
		NotificationTypeCampaignFailed, NotificationTypeAccountSuspended,
		NotificationTypeAccountRestored, NotificationTypeSystemAlert,
		NotificationTypeErrorAlert, NotificationTypeWarningAlert,
		NotificationTypeProgressUpdate, NotificationTypeMilestone,
		NotificationTypeTest:
		return true
	}
	return false
}

func (nc NotificationChannel) IsValid() bool {
	switch nc {
	case NotificationChannelTelegram, NotificationChannelEmail,
		NotificationChannelWebhook, NotificationChannelWebSocket:
		return true
	}
	return false
}

func (ns NotificationStatus) IsValid() bool {
	switch ns {
	case NotificationStatusPending, NotificationStatusQueued,
		NotificationStatusSending, NotificationStatusSent,
		NotificationStatusFailed, NotificationStatusCancelled:
		return true
	}
	return false
}

func (np NotificationPriority) IsValid() bool {
	switch np {
	case NotificationPriorityLow, NotificationPriorityNormal,
		NotificationPriorityHigh, NotificationPriorityCritical:
		return true
	}
	return false
}

func (nf NotificationFormat) IsValid() bool {
	switch nf {
	case NotificationFormatPlain, NotificationFormatMarkdown, NotificationFormatHTML:
		return true
	}
	return false
}

func NewTelegramConfig(botToken, chatID, tenantID string) *TelegramConfig {
	now := time.Now()
	return &TelegramConfig{
		ID:        generateTelegramConfigID(),
		TenantID:  tenantID,
		BotToken:  botToken,
		ChatID:    chatID,
		Enabled:   true,
		ParseMode: "Markdown",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (tc *TelegramConfig) Validate() error {
	if tc.TenantID == "" {
		return errors.New("tenant ID is required")
	}
	if tc.BotToken == "" {
		return errors.New("bot token is required")
	}
	if tc.ChatID == "" {
		return errors.New("chat ID is required")
	}
	return nil
}

func (tc *TelegramConfig) MarkTestSuccess() {
	now := time.Now()
	tc.LastTestAt = &now
	tc.LastTestStatus = "success"
	tc.UpdatedAt = now
}

func (tc *TelegramConfig) MarkTestFailed(reason string) {
	now := time.Now()
	tc.LastTestAt = &now
	tc.LastTestStatus = fmt.Sprintf("failed: %s", reason)
	tc.UpdatedAt = now
}

func (tc *TelegramConfig) Enable() {
	tc.Enabled = true
	tc.UpdatedAt = time.Now()
}

func (tc *TelegramConfig) Disable() {
	tc.Enabled = false
	tc.UpdatedAt = time.Now()
}

func NewNotificationTemplate(name string, notifType NotificationType, channel NotificationChannel, tenantID string) *NotificationTemplate {
	now := time.Now()
	return &NotificationTemplate{
		ID:        generateNotificationTemplateID(),
		TenantID:  tenantID,
		Name:      name,
		Type:      notifType,
		Channel:   channel,
		Format:    NotificationFormatMarkdown,
		Variables: []string{},
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  make(map[string]interface{}),
	}
}

func (nt *NotificationTemplate) Validate() error {
	if nt.Name == "" {
		return errors.New("template name is required")
	}
	if nt.Body == "" {
		return errors.New("template body is required")
	}
	if !nt.Type.IsValid() {
		return fmt.Errorf("invalid notification type: %s", nt.Type)
	}
	if !nt.Channel.IsValid() {
		return fmt.Errorf("invalid notification channel: %s", nt.Channel)
	}
	return nil
}

func (nt *NotificationTemplate) Render(data map[string]interface{}) (string, string) {
	title := nt.Title
	body := nt.Body
	
	for key, value := range data {
		placeholder := fmt.Sprintf("{%s}", key)
		valueStr := fmt.Sprintf("%v", value)
		title = replaceAll(title, placeholder, valueStr)
		body = replaceAll(body, placeholder, valueStr)
	}
	
	return title, body
}

func generateNotificationID() string {
	return fmt.Sprintf("notif_%d", time.Now().UnixNano())
}

func generateTelegramConfigID() string {
	return fmt.Sprintf("tg_cfg_%d", time.Now().UnixNano())
}

func generateNotificationTemplateID() string {
	return fmt.Sprintf("notif_tpl_%d", time.Now().UnixNano())
}

func replaceAll(s, old, new string) string {
	result := s
	for i := 0; i < 100; i++ {
		if len(result) == 0 || len(old) == 0 {
			break
		}
		idx := indexOf(result, old)
		if idx == -1 {
			break
		}
		result = result[:idx] + new + result[idx+len(old):]
	}
	return result
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

type NotificationEvent struct {
	Type       NotificationType       `json:"type"`
	CampaignID string                 `json:"campaign_id,omitempty"`
	Data       map[string]interface{} `json:"data"`
	Timestamp  time.Time              `json:"timestamp"`
}

func NewNotificationEvent(notifType NotificationType) *NotificationEvent {
	return &NotificationEvent{
		Type:      notifType,
		Data:      make(map[string]interface{}),
		Timestamp: time.Now(),
	}
}

func (ne *NotificationEvent) WithCampaign(campaignID string) *NotificationEvent {
	ne.CampaignID = campaignID
	return ne
}

func (ne *NotificationEvent) WithData(key string, value interface{}) *NotificationEvent {
	ne.Data[key] = value
	return ne
}
