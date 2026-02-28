package notification

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"
	"text/template"
	"time"

	"email-campaign-system/internal/models"
)

var (
	ErrTemplateNotFound     = errors.New("template not found")
	ErrInvalidTemplate      = errors.New("invalid template")
	ErrTemplateRenderFailed = errors.New("template render failed")
)

type TemplateManager struct {
	mu             sync.RWMutex
	templates      map[EventType]*NotificationTemplate
	customTemplates map[string]*NotificationTemplate
	defaultFormat   TemplateFormat
	functions      template.FuncMap
	cache          *TemplateCache
}

type NotificationTemplate struct {
	ID          string
	Name        string
	EventType   EventType
	Format      TemplateFormat
	Subject     string
	Body        string
	Variables   []string
	compiled    *template.Template
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type TemplateFormat string

const (
	FormatPlainText TemplateFormat = "plain"
	FormatMarkdown  TemplateFormat = "markdown"
	FormatHTML      TemplateFormat = "html"
)

type TemplateCache struct {
	mu    sync.RWMutex
	items map[string]*CachedTemplate
}

type CachedTemplate struct {
	Content   string
	ExpiresAt time.Time
}

type TemplateData struct {
	Campaign    *CampaignData
	Account     *AccountData
	Stats       *StatsData
	Error       *ErrorData
	System      *SystemData
	Timestamp   time.Time
	Custom      map[string]interface{}
}

type CampaignData struct {
	ID          string
	Name        string
	Status      string
	TotalEmails int
	SentEmails  int
	FailedEmails int
	Progress    float64
	StartTime   time.Time
	EndTime     time.Time
	Duration    time.Duration
}

type AccountData struct {
	ID          string
	Email       string
	Provider    string
	Status      string
	HealthScore float64
	SentCount   int
	FailedCount int
}

type StatsData struct {
	SentCount    int64
	FailedCount  int64
	SuccessRate  float64
	Throughput   float64
	AverageDelay time.Duration
}

type ErrorData struct {
	Message    string
	Code       string
	Severity   string
	StackTrace string
	Context    map[string]interface{}
}

type SystemData struct {
	CPUUsage    float64
	MemoryUsage float64
	DiskUsage   float64
	Uptime      time.Duration
	Version     string
}

func NewTemplateManager() *TemplateManager {
	tm := &TemplateManager{
		templates:       make(map[EventType]*NotificationTemplate),
		customTemplates: make(map[string]*NotificationTemplate),
		defaultFormat:   FormatHTML,
		functions:       defaultTemplateFunctions(),
		cache:           NewTemplateCache(),
	}

	tm.registerBuiltInTemplates()
	return tm
}

func NewTemplateCache() *TemplateCache {
	return &TemplateCache{
		items: make(map[string]*CachedTemplate),
	}
}

func (tm *TemplateManager) registerBuiltInTemplates() {
	tm.RegisterTemplate(EventCampaignStarted, campaignStartedTemplate())
	tm.RegisterTemplate(EventCampaignPaused, campaignPausedTemplate())
	tm.RegisterTemplate(EventCampaignResumed, campaignResumedTemplate())
	tm.RegisterTemplate(EventCampaignCompleted, campaignCompletedTemplate())
	tm.RegisterTemplate(EventCampaignFailed, campaignFailedTemplate())
	tm.RegisterTemplate(EventAccountSuspended, accountSuspendedTemplate())
	tm.RegisterTemplate(EventAccountRestored, accountRestoredTemplate())
	tm.RegisterTemplate(EventSendSuccess, sendSuccessTemplate())
	tm.RegisterTemplate(EventSendFailed, sendFailedTemplate())
	tm.RegisterTemplate(EventQuotaReached, quotaReachedTemplate())
	tm.RegisterTemplate(EventSystemError, systemErrorTemplate())
	tm.RegisterTemplate(EventSystemWarning, systemWarningTemplate())
}

func (tm *TemplateManager) RegisterTemplate(eventType EventType, tpl *NotificationTemplate) error {
	if tpl == nil {
		return ErrInvalidTemplate
	}

	if err := tm.validateTemplate(tpl); err != nil {
		return err
	}

	if err := tm.compileTemplate(tpl); err != nil {
		return err
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	tpl.EventType = eventType
	tpl.CreatedAt = time.Now()
	tm.templates[eventType] = tpl

	return nil
}

func (tm *TemplateManager) RegisterCustomTemplate(name string, tpl *NotificationTemplate) error {
	if name == "" {
		return errors.New("template name is required")
	}

	if err := tm.validateTemplate(tpl); err != nil {
		return err
	}

	if err := tm.compileTemplate(tpl); err != nil {
		return err
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	tpl.Name = name
	tpl.CreatedAt = time.Now()
	tm.customTemplates[name] = tpl

	return nil
}

func (tm *TemplateManager) GetTemplate(eventType EventType) *NotificationTemplate {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tpl, exists := tm.templates[eventType]; exists {
		return tpl
	}

	return tm.getDefaultTemplate(eventType)
}

func (tm *TemplateManager) GetCustomTemplate(name string) (*NotificationTemplate, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tpl, exists := tm.customTemplates[name]
	if !exists {
		return nil, ErrTemplateNotFound
	}

	return tpl, nil
}

func (tm *TemplateManager) RenderTemplate(eventType EventType, data *TemplateData) (string, error) {
	tpl := tm.GetTemplate(eventType)
	if tpl == nil {
		return "", ErrTemplateNotFound
	}

	cacheKey := tm.getCacheKey(eventType, data)
	if cached := tm.cache.Get(cacheKey); cached != "" {
		return cached, nil
	}

	rendered, err := tm.render(tpl, data)
	if err != nil {
		return "", err
	}

	tm.cache.Set(cacheKey, rendered, 5*time.Minute)

	return rendered, nil
}

func (tm *TemplateManager) render(tpl *NotificationTemplate, data *TemplateData) (string, error) {
	if tpl.compiled == nil {
		if err := tm.compileTemplate(tpl); err != nil {
			return "", err
		}
	}

	var buf bytes.Buffer
	if err := tpl.compiled.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("%w: %v", ErrTemplateRenderFailed, err)
	}

	return buf.String(), nil
}

func (tm *TemplateManager) compileTemplate(tpl *NotificationTemplate) error {
	tmpl, err := template.New(tpl.ID).Funcs(tm.functions).Parse(tpl.Body)
	if err != nil {
		return fmt.Errorf("failed to compile template: %w", err)
	}

	tpl.compiled = tmpl
	return nil
}

func (tm *TemplateManager) validateTemplate(tpl *NotificationTemplate) error {
	if tpl.Body == "" {
		return errors.New("template body is required")
	}

	if tpl.ID == "" {
		tpl.ID = generateTemplateID()
	}

	return nil
}

func (tm *TemplateManager) getDefaultTemplate(eventType EventType) *NotificationTemplate {
	return &NotificationTemplate{
		ID:        generateTemplateID(),
		EventType: eventType,
		Format:    tm.defaultFormat,
		Subject:   string(eventType),
		Body:      "Event: {{.EventType}}",
		Variables: []string{},
	}
}

func (tm *TemplateManager) getCacheKey(eventType EventType, data *TemplateData) string {
	return fmt.Sprintf("%s_%d", eventType, data.Timestamp.Unix())
}

func (tc *TemplateCache) Get(key string) string {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	cached, exists := tc.items[key]
	if !exists {
		return ""
	}

	if time.Now().After(cached.ExpiresAt) {
		delete(tc.items, key)
		return ""
	}

	return cached.Content
}

func (tc *TemplateCache) Set(key, content string, ttl time.Duration) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.items[key] = &CachedTemplate{
		Content:   content,
		ExpiresAt: time.Now().Add(ttl),
	}
}

func (tc *TemplateCache) Clear() {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.items = make(map[string]*CachedTemplate)
}

func defaultTemplateFunctions() template.FuncMap {
	return template.FuncMap{
		"formatTime": func(t time.Time, format string) string {
			return t.Format(format)
		},
		"formatDuration": func(d time.Duration) string {
			return d.String()
		},
		"percentage": func(value, total float64) string {
			if total == 0 {
				return "0%"
			}
			return fmt.Sprintf("%.2f%%", (value/total)*100)
		},
		"humanize": func(n int64) string {
			if n < 1000 {
				return fmt.Sprintf("%d", n)
			}
			if n < 1000000 {
				return fmt.Sprintf("%.1fK", float64(n)/1000)
			}
			return fmt.Sprintf("%.1fM", float64(n)/1000000)
		},
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"title": strings.Title,
	}
}

func campaignStartedTemplate() *NotificationTemplate {
	return &NotificationTemplate{
		ID:      "campaign_started",
		Name:    "Campaign Started",
		Format:  FormatHTML,
		Subject: "Campaign Started",
		Body: `<b>🚀 Campaign Started</b>

<b>Campaign:</b> {{.Campaign.Name}}
<b>Total Recipients:</b> {{humanize .Campaign.TotalEmails}}
<b>Started At:</b> {{formatTime .Timestamp "2006-01-02 15:04:05"}}

The campaign has been initiated and emails are being sent.`,
		Variables: []string{"Campaign.Name", "Campaign.TotalEmails", "Timestamp"},
	}
}

func campaignPausedTemplate() *NotificationTemplate {
	return &NotificationTemplate{
		ID:      "campaign_paused",
		Name:    "Campaign Paused",
		Format:  FormatHTML,
		Subject: "Campaign Paused",
		Body: `<b>⏸️ Campaign Paused</b>

<b>Campaign:</b> {{.Campaign.Name}}
<b>Progress:</b> {{percentage .Campaign.SentEmails .Campaign.TotalEmails}}
<b>Sent:</b> {{humanize .Campaign.SentEmails}} / {{humanize .Campaign.TotalEmails}}
<b>Paused At:</b> {{formatTime .Timestamp "2006-01-02 15:04:05"}}`,
		Variables: []string{"Campaign.Name", "Campaign.SentEmails", "Campaign.TotalEmails"},
	}
}

func campaignResumedTemplate() *NotificationTemplate {
	return &NotificationTemplate{
		ID:      "campaign_resumed",
		Name:    "Campaign Resumed",
		Format:  FormatHTML,
		Subject: "Campaign Resumed",
		Body: `<b>▶️ Campaign Resumed</b>

<b>Campaign:</b> {{.Campaign.Name}}
<b>Progress:</b> {{percentage .Campaign.SentEmails .Campaign.TotalEmails}}
<b>Remaining:</b> {{humanize (sub .Campaign.TotalEmails .Campaign.SentEmails)}}
<b>Resumed At:</b> {{formatTime .Timestamp "2006-01-02 15:04:05"}}`,
		Variables: []string{"Campaign.Name", "Campaign.SentEmails", "Campaign.TotalEmails"},
	}
}

func campaignCompletedTemplate() *NotificationTemplate {
	return &NotificationTemplate{
		ID:      "campaign_completed",
		Name:    "Campaign Completed",
		Format:  FormatHTML,
		Subject: "Campaign Completed",
		Body: `<b>✅ Campaign Completed Successfully</b>

<b>Campaign:</b> {{.Campaign.Name}}
<b>Total Sent:</b> {{humanize .Campaign.SentEmails}}
<b>Failed:</b> {{humanize .Campaign.FailedEmails}}
<b>Success Rate:</b> {{percentage (sub .Campaign.SentEmails .Campaign.FailedEmails) .Campaign.SentEmails}}
<b>Duration:</b> {{formatDuration .Campaign.Duration}}
<b>Completed At:</b> {{formatTime .Timestamp "2006-01-02 15:04:05"}}

Campaign has been completed successfully!`,
		Variables: []string{"Campaign.Name", "Campaign.SentEmails", "Campaign.FailedEmails", "Campaign.Duration"},
	}
}

func campaignFailedTemplate() *NotificationTemplate {
	return &NotificationTemplate{
		ID:      "campaign_failed",
		Name:    "Campaign Failed",
		Format:  FormatHTML,
		Subject: "Campaign Failed",
		Body: `<b>❌ Campaign Failed</b>

<b>Campaign:</b> {{.Campaign.Name}}
<b>Sent:</b> {{humanize .Campaign.SentEmails}} / {{humanize .Campaign.TotalEmails}}
<b>Failed:</b> {{humanize .Campaign.FailedEmails}}
<b>Error:</b> {{.Error.Message}}
<b>Failed At:</b> {{formatTime .Timestamp "2006-01-02 15:04:05"}}

Please check the logs for more details.`,
		Variables: []string{"Campaign.Name", "Campaign.SentEmails", "Campaign.FailedEmails", "Error.Message"},
	}
}

func accountSuspendedTemplate() *NotificationTemplate {
	return &NotificationTemplate{
		ID:      "account_suspended",
		Name:    "Account Suspended",
		Format:  FormatHTML,
		Subject: "Account Suspended",
		Body: `<b>⚠️ Account Suspended</b>

<b>Account:</b> {{.Account.Email}}
<b>Provider:</b> {{.Account.Provider}}
<b>Reason:</b> {{.Error.Message}}
<b>Health Score:</b> {{printf "%.2f" .Account.HealthScore}}
<b>Suspended At:</b> {{formatTime .Timestamp "2006-01-02 15:04:05"}}

This account has been automatically suspended due to issues.`,
		Variables: []string{"Account.Email", "Account.Provider", "Error.Message", "Account.HealthScore"},
	}
}

func accountRestoredTemplate() *NotificationTemplate {
	return &NotificationTemplate{
		ID:      "account_restored",
		Name:    "Account Restored",
		Format:  FormatHTML,
		Subject: "Account Restored",
		Body: `<b>✅ Account Restored</b>

<b>Account:</b> {{.Account.Email}}
<b>Provider:</b> {{.Account.Provider}}
<b>Health Score:</b> {{printf "%.2f" .Account.HealthScore}}
<b>Restored At:</b> {{formatTime .Timestamp "2006-01-02 15:04:05"}}

Account has been restored and is ready for use.`,
		Variables: []string{"Account.Email", "Account.Provider", "Account.HealthScore"},
	}
}

func sendSuccessTemplate() *NotificationTemplate {
	return &NotificationTemplate{
		ID:      "send_success",
		Name:    "Send Success",
		Format:  FormatHTML,
		Subject: "Email Sent Successfully",
		Body: `<b>✉️ Email Sent</b>

<b>Account:</b> {{.Account.Email}}
<b>Campaign:</b> {{.Campaign.Name}}
<b>Progress:</b> {{percentage .Campaign.SentEmails .Campaign.TotalEmails}}`,
		Variables: []string{"Account.Email", "Campaign.Name", "Campaign.SentEmails"},
	}
}

func sendFailedTemplate() *NotificationTemplate {
	return &NotificationTemplate{
		ID:      "send_failed",
		Name:    "Send Failed",
		Format:  FormatHTML,
		Subject: "Email Send Failed",
		Body: `<b>❌ Email Send Failed</b>

<b>Account:</b> {{.Account.Email}}
<b>Campaign:</b> {{.Campaign.Name}}
<b>Error:</b> {{.Error.Message}}
<b>Failed At:</b> {{formatTime .Timestamp "2006-01-02 15:04:05"}}`,
		Variables: []string{"Account.Email", "Campaign.Name", "Error.Message"},
	}
}

func quotaReachedTemplate() *NotificationTemplate {
	return &NotificationTemplate{
		ID:      "quota_reached",
		Name:    "Quota Reached",
		Format:  FormatHTML,
		Subject: "Quota Reached",
		Body: `<b>⚠️ Quota Reached</b>

<b>Account:</b> {{.Account.Email}}
<b>Sent Today:</b> {{humanize .Account.SentCount}}
<b>Campaign:</b> {{.Campaign.Name}}

Daily sending limit has been reached for this account.`,
		Variables: []string{"Account.Email", "Account.SentCount", "Campaign.Name"},
	}
}

func systemErrorTemplate() *NotificationTemplate {
	return &NotificationTemplate{
		ID:      "system_error",
		Name:    "System Error",
		Format:  FormatHTML,
		Subject: "System Error",
		Body: `<b>🚨 System Error</b>

<b>Error:</b> {{.Error.Message}}
<b>Severity:</b> {{upper .Error.Severity}}
<b>Code:</b> {{.Error.Code}}
<b>Time:</b> {{formatTime .Timestamp "2006-01-02 15:04:05"}}

Immediate attention required!`,
		Variables: []string{"Error.Message", "Error.Severity", "Error.Code"},
	}
}

func systemWarningTemplate() *NotificationTemplate {
	return &NotificationTemplate{
		ID:      "system_warning",
		Name:    "System Warning",
		Format:  FormatHTML,
		Subject: "System Warning",
		Body: `<b>⚠️ System Warning</b>

<b>Warning:</b> {{.Error.Message}}
<b>CPU Usage:</b> {{printf "%.2f%%" .System.CPUUsage}}
<b>Memory Usage:</b> {{printf "%.2f%%" .System.MemoryUsage}}
<b>Time:</b> {{formatTime .Timestamp "2006-01-02 15:04:05"}}`,
		Variables: []string{"Error.Message", "System.CPUUsage", "System.MemoryUsage"},
	}
}

func generateTemplateID() string {
	return fmt.Sprintf("tpl_%d", time.Now().UnixNano())
}

func NewTemplateData() *TemplateData {
	return &TemplateData{
		Timestamp: time.Now(),
		Custom:    make(map[string]interface{}),
	}
}

func (td *TemplateData) SetCampaign(campaign *models.Campaign, stats *models.CampaignStats) {
    if campaign == nil {
        return
    }

    td.Campaign = &CampaignData{
        ID:     campaign.ID,
        Name:   campaign.Name,
        Status: string(campaign.Status),
    }

    if stats != nil {
        td.Campaign.TotalEmails = int(campaign.TotalRecipients)
        td.Campaign.SentEmails = int(stats.TotalSent)
        td.Campaign.FailedEmails = int(stats.TotalFailed)
        
        // Calculate progress
        if campaign.TotalRecipients > 0 {
            td.Campaign.Progress = float64(stats.TotalSent) / float64(campaign.TotalRecipients) * 100
        }
        
        // Calculate duration
        if campaign.StartedAt != nil {
            if campaign.CompletedAt != nil {
                td.Campaign.Duration = campaign.CompletedAt.Sub(*campaign.StartedAt)
            } else {
                td.Campaign.Duration = time.Since(*campaign.StartedAt)
            }
        }
    }
}


func (td *TemplateData) SetAccount(account *models.Account) {
	if account == nil {
		return
	}

	td.Account = &AccountData{
		ID:       account.ID,
		Email:    account.Email,
		Provider: string(account.Provider),
		Status:   string(account.Status),
	}
}

func (td *TemplateData) SetError(err error, severity string) {
	if err == nil {
		return
	}

	td.Error = &ErrorData{
		Message:  err.Error(),
		Severity: severity,
		Context:  make(map[string]interface{}),
	}
}

func (td *TemplateData) SetCustom(key string, value interface{}) {
	td.Custom[key] = value
}

func sub(a, b int) int {
	return a - b
}

func add(a, b int) int {
	return a + b
}

func mul(a, b int) int {
	return a * b
}

func div(a, b int) int {
	if b == 0 {
		return 0
	}
	return a / b
}

