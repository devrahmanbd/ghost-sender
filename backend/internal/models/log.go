package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"
)

type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
	LogLevelFatal LogLevel = "fatal"
)

type LogType string

const (
	LogTypeCampaign    LogType = "campaign"
	LogTypeEmail       LogType = "email"
	LogTypeSystem      LogType = "system"
	LogTypePerformance LogType = "performance"
	LogTypeSecurity    LogType = "security"
	LogTypeDebug       LogType = "debug"
	LogTypeAPI         LogType = "api"
	LogTypeDatabase    LogType = "database"
	LogTypeAccount     LogType = "account"
	LogTypeTemplate    LogType = "template"
	LogTypeProxy       LogType = "proxy"
)

type LogEntry struct {
	ID             string                 `json:"id"`
	TenantID       string                 `json:"tenant_id"`
	SessionID      string                 `json:"session_id"`
	CampaignID     string                 `json:"campaign_id,omitempty"`
	AccountID      string                 `json:"account_id,omitempty"`
	RecipientID    string                 `json:"recipient_id,omitempty"`
	EmailID        string                 `json:"email_id,omitempty"`
	TemplateID     string                 `json:"template_id,omitempty"`
	ProxyID        string                 `json:"proxy_id,omitempty"`
	Level          LogLevel               `json:"level"`
	Type           LogType                `json:"type"`
	Message        string                 `json:"message"`
	Details        string                 `json:"details,omitempty"`
	ErrorCode      string                 `json:"error_code,omitempty"`
	ErrorMessage   string                 `json:"error_message,omitempty"`
	StackTrace     string                 `json:"stack_trace,omitempty"`
	Component      string                 `json:"component"`
	Function       string                 `json:"function,omitempty"`
	File           string                 `json:"file,omitempty"`
	Line           int                    `json:"line,omitempty"`
	Duration       int64                  `json:"duration,omitempty"`
	Timestamp      time.Time              `json:"timestamp"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	Tags           []string               `json:"tags,omitempty"`
	UserAgent      string                 `json:"user_agent,omitempty"`
	IPAddress      string                 `json:"ip_address,omitempty"`
	RequestID      string                 `json:"request_id,omitempty"`
	CorrelationID  string                 `json:"correlation_id,omitempty"`
	Success        bool                   `json:"success"`
	Retryable      bool                   `json:"retryable"`
	RetryCount     int                    `json:"retry_count,omitempty"`
	ExpiresAt      *time.Time             `json:"expires_at,omitempty"`
}

type EmailLogEntry struct {
	LogEntry
	RecipientEmail string        `json:"recipient_email"`
	Subject        string        `json:"subject,omitempty"`
	FromAddress    string        `json:"from_address"`
	ToAddress      string        `json:"to_address"`
	SMTPResponse   string        `json:"smtp_response,omitempty"`
	MessageID      string        `json:"message_id,omitempty"`
	SendAttempt    int           `json:"send_attempt"`
	SendStatus     EmailStatus   `json:"send_status"`
	FailureReason  string        `json:"failure_reason,omitempty"`
	SentAt         *time.Time    `json:"sent_at,omitempty"`
}

type PerformanceLogEntry struct {
	LogEntry
	Operation       string                 `json:"operation"`
	DurationMs      int64                  `json:"duration_ms"`
	CPUUsage        float64                `json:"cpu_usage,omitempty"`
	MemoryUsageMB   float64                `json:"memory_usage_mb,omitempty"`
	ThroughputPerSec float64               `json:"throughput_per_sec,omitempty"`
	ItemsProcessed  int64                  `json:"items_processed,omitempty"`
	ErrorsCount     int64                  `json:"errors_count,omitempty"`
	Metrics         map[string]interface{} `json:"metrics,omitempty"`
}

type CampaignLogEntry struct {
	LogEntry
	CampaignName   string         `json:"campaign_name"`
	CampaignStatus CampaignStatus `json:"campaign_status"`
	Action         string         `json:"action"`
	EmailsSent     int64          `json:"emails_sent"`
	EmailsFailed   int64          `json:"emails_failed"`
	Progress       float64        `json:"progress"`
}

type SystemLogEntry struct {
	LogEntry
	SystemComponent string  `json:"system_component"`
	Action          string  `json:"action"`
	ResourceType    string  `json:"resource_type,omitempty"`
	ResourceID      string  `json:"resource_id,omitempty"`
	CPUPercent      float64 `json:"cpu_percent,omitempty"`
	MemoryPercent   float64 `json:"memory_percent,omitempty"`
	DiskPercent     float64 `json:"disk_percent,omitempty"`
}

func NewLogEntry(level LogLevel, logType LogType, message string, tenantID string) *LogEntry {
	return &LogEntry{
		ID:        generateLogID(),
		TenantID:  tenantID,
		Level:     level,
		Type:      logType,
		Message:   message,
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
		Tags:      []string{},
		Success:   level != LogLevelError && level != LogLevelFatal,
	}
}

func NewEmailLog(level LogLevel, message string, tenantID, campaignID, accountID, recipientEmail string) *EmailLogEntry {
	entry := &EmailLogEntry{
		LogEntry: LogEntry{
			ID:         generateLogID(),
			TenantID:   tenantID,
			CampaignID: campaignID,
			AccountID:  accountID,
			Level:      level,
			Type:       LogTypeEmail,
			Message:    message,
			Timestamp:  time.Now(),
			Metadata:   make(map[string]interface{}),
			Success:    level != LogLevelError && level != LogLevelFatal,
		},
		RecipientEmail: recipientEmail,
		SendAttempt:    1,
	}
	return entry
}

func NewPerformanceLog(operation string, durationMs int64, tenantID string) *PerformanceLogEntry {
	entry := &PerformanceLogEntry{
		LogEntry: LogEntry{
			ID:        generateLogID(),
			TenantID:  tenantID,
			Level:     LogLevelInfo,
			Type:      LogTypePerformance,
			Message:   fmt.Sprintf("Performance metric for %s", operation),
			Timestamp: time.Now(),
			Metadata:  make(map[string]interface{}),
			Success:   true,
		},
		Operation:  operation,
		DurationMs: durationMs,
		Metrics:    make(map[string]interface{}),
	}
	return entry
}

func NewCampaignLog(level LogLevel, campaignID, campaignName, action string, tenantID string) *CampaignLogEntry {
	entry := &CampaignLogEntry{
		LogEntry: LogEntry{
			ID:         generateLogID(),
			TenantID:   tenantID,
			CampaignID: campaignID,
			Level:      level,
			Type:       LogTypeCampaign,
			Message:    fmt.Sprintf("Campaign %s: %s", campaignName, action),
			Timestamp:  time.Now(),
			Metadata:   make(map[string]interface{}),
			Success:    level != LogLevelError && level != LogLevelFatal,
		},
		CampaignName: campaignName,
		Action:       action,
	}
	return entry
}

func NewSystemLog(level LogLevel, component, action, message string, tenantID string) *SystemLogEntry {
	entry := &SystemLogEntry{
		LogEntry: LogEntry{
			ID:        generateLogID(),
			TenantID:  tenantID,
			Level:     level,
			Type:      LogTypeSystem,
			Message:   message,
			Component: component,
			Timestamp: time.Now(),
			Metadata:  make(map[string]interface{}),
			Success:   level != LogLevelError && level != LogLevelFatal,
		},
		SystemComponent: component,
		Action:          action,
	}
	return entry
}

func (l *LogEntry) Validate() error {
	if l.TenantID == "" {
		return errors.New("tenant ID is required")
	}
	if l.Message == "" {
		return errors.New("message is required")
	}
	if !l.Level.IsValid() {
		return fmt.Errorf("invalid log level: %s", l.Level)
	}
	if !l.Type.IsValid() {
		return fmt.Errorf("invalid log type: %s", l.Type)
	}
	return nil
}

func (l *LogEntry) WithSession(sessionID string) *LogEntry {
	l.SessionID = sessionID
	return l
}

func (l *LogEntry) WithCampaign(campaignID string) *LogEntry {
	l.CampaignID = campaignID
	return l
}

func (l *LogEntry) WithAccount(accountID string) *LogEntry {
	l.AccountID = accountID
	return l
}

func (l *LogEntry) WithRecipient(recipientID string) *LogEntry {
	l.RecipientID = recipientID
	return l
}

func (l *LogEntry) WithEmail(emailID string) *LogEntry {
	l.EmailID = emailID
	return l
}

func (l *LogEntry) WithTemplate(templateID string) *LogEntry {
	l.TemplateID = templateID
	return l
}

func (l *LogEntry) WithProxy(proxyID string) *LogEntry {
	l.ProxyID = proxyID
	return l
}

func (l *LogEntry) WithError(err error) *LogEntry {
	if err != nil {
		l.Level = LogLevelError
		l.ErrorMessage = err.Error()
		l.Success = false
	}
	return l
}

func (l *LogEntry) WithStackTrace() *LogEntry {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	l.StackTrace = string(buf[:n])
	return l
}

func (l *LogEntry) WithDetails(details string) *LogEntry {
	l.Details = details
	return l
}

func (l *LogEntry) WithDuration(duration time.Duration) *LogEntry {
	l.Duration = duration.Milliseconds()
	return l
}

func (l *LogEntry) WithComponent(component string) *LogEntry {
	l.Component = component
	return l
}

func (l *LogEntry) WithMetadata(key string, value interface{}) *LogEntry {
	if l.Metadata == nil {
		l.Metadata = make(map[string]interface{})
	}
	l.Metadata[key] = value
	return l
}

func (l *LogEntry) WithMetadataMap(metadata map[string]interface{}) *LogEntry {
	if l.Metadata == nil {
		l.Metadata = make(map[string]interface{})
	}
	for k, v := range metadata {
		l.Metadata[k] = v
	}
	return l
}

func (l *LogEntry) WithTag(tag string) *LogEntry {
	if l.Tags == nil {
		l.Tags = []string{}
	}
	l.Tags = append(l.Tags, tag)
	return l
}

func (l *LogEntry) WithTags(tags []string) *LogEntry {
	l.Tags = append(l.Tags, tags...)
	return l
}

func (l *LogEntry) WithRequest(requestID, userAgent, ipAddress string) *LogEntry {
	l.RequestID = requestID
	l.UserAgent = userAgent
	l.IPAddress = ipAddress
	return l
}

func (l *LogEntry) WithCorrelation(correlationID string) *LogEntry {
	l.CorrelationID = correlationID
	return l
}

func (l *LogEntry) MarkAsRetryable(retryable bool) *LogEntry {
	l.Retryable = retryable
	return l
}

func (l *LogEntry) IncrementRetry() *LogEntry {
	l.RetryCount++
	return l
}

func (l *LogEntry) SetExpiration(duration time.Duration) *LogEntry {
	expiresAt := time.Now().Add(duration)
	l.ExpiresAt = &expiresAt
	return l
}

func (l *LogEntry) CaptureCallerInfo() *LogEntry {
	_, file, line, ok := runtime.Caller(2)
	if ok {
		l.File = file
		l.Line = line
		parts := strings.Split(file, "/")
		if len(parts) > 0 {
			l.Function = parts[len(parts)-1]
		}
	}
	return l
}

func (l *LogEntry) IsError() bool {
	return l.Level == LogLevelError || l.Level == LogLevelFatal
}

func (l *LogEntry) IsWarning() bool {
	return l.Level == LogLevelWarn
}

func (l *LogEntry) IsInfo() bool {
	return l.Level == LogLevelInfo
}

func (l *LogEntry) IsDebug() bool {
	return l.Level == LogLevelDebug
}

func (l *LogEntry) GetAge() time.Duration {
	return time.Since(l.Timestamp)
}

func (l *LogEntry) IsExpired() bool {
	if l.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*l.ExpiresAt)
}

func (l *LogEntry) ToJSON() (string, error) {
	data, err := json.Marshal(l)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (l *LogEntry) FormatMessage() string {
	timestamp := l.Timestamp.Format("2006-01-02 15:04:05.000")
	levelStr := strings.ToUpper(string(l.Level))
	
	msg := fmt.Sprintf("[%s] [%s] [%s] %s", timestamp, levelStr, l.Type, l.Message)
	
	if l.Component != "" {
		msg = fmt.Sprintf("%s [%s]", msg, l.Component)
	}
	
	if l.CampaignID != "" {
		msg = fmt.Sprintf("%s [campaign:%s]", msg, l.CampaignID)
	}
	
	if l.ErrorMessage != "" {
		msg = fmt.Sprintf("%s - Error: %s", msg, l.ErrorMessage)
	}
	
	return msg
}

func (l *LogEntry) Clone() *LogEntry {
	clone := *l
	clone.ID = generateLogID()
	clone.Timestamp = time.Now()
	
	if l.Metadata != nil {
		clone.Metadata = make(map[string]interface{})
		for k, v := range l.Metadata {
			clone.Metadata[k] = v
		}
	}
	
	if l.Tags != nil {
		clone.Tags = make([]string, len(l.Tags))
		copy(clone.Tags, l.Tags)
	}
	
	return &clone
}

func (ll LogLevel) IsValid() bool {
	switch ll {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError, LogLevelFatal:
		return true
	}
	return false
}

func (ll LogLevel) GetNumericValue() int {
	switch ll {
	case LogLevelDebug:
		return 10
	case LogLevelInfo:
		return 20
	case LogLevelWarn:
		return 30
	case LogLevelError:
		return 40
	case LogLevelFatal:
		return 50
	default:
		return 0
	}
}

func (lt LogType) IsValid() bool {
	switch lt {
	case LogTypeCampaign, LogTypeEmail, LogTypeSystem, LogTypePerformance,
		LogTypeSecurity, LogTypeDebug, LogTypeAPI, LogTypeDatabase,
		LogTypeAccount, LogTypeTemplate, LogTypeProxy:
		return true
	}
	return false
}

func (e *EmailLogEntry) MarkAsSent(messageID, smtpResponse string) {
	e.Success = true
	e.SendStatus = EmailStatusSent
	e.MessageID = messageID
	e.SMTPResponse = smtpResponse
	now := time.Now()
	e.SentAt = &now
	e.Level = LogLevelInfo
	e.Message = fmt.Sprintf("Email sent successfully to %s", e.RecipientEmail)
}

func (e *EmailLogEntry) MarkAsFailed(reason, smtpResponse string) {
	e.Success = false
	e.SendStatus = EmailStatusFailed
	e.FailureReason = reason
	e.SMTPResponse = smtpResponse
	e.Level = LogLevelError
	e.Message = fmt.Sprintf("Email failed to send to %s: %s", e.RecipientEmail, reason)
}

func (p *PerformanceLogEntry) SetMetrics(cpuUsage, memoryUsageMB, throughput float64) {
	p.CPUUsage = cpuUsage
	p.MemoryUsageMB = memoryUsageMB
	p.ThroughputPerSec = throughput
}

func (p *PerformanceLogEntry) AddMetric(key string, value interface{}) {
	if p.Metrics == nil {
		p.Metrics = make(map[string]interface{})
	}
	p.Metrics[key] = value
}

func (c *CampaignLogEntry) UpdateStats(emailsSent, emailsFailed int64, progress float64) {
	c.EmailsSent = emailsSent
	c.EmailsFailed = emailsFailed
	c.Progress = progress
}

func (s *SystemLogEntry) SetResourceMetrics(cpu, memory, disk float64) {
	s.CPUPercent = cpu
	s.MemoryPercent = memory
	s.DiskPercent = disk
}

func generateLogID() string {
	return fmt.Sprintf("log_%d", time.Now().UnixNano())
}

type LogFilter struct {
	TenantID      string     `json:"tenant_id"`
	SessionID     string     `json:"session_id,omitempty"`
	CampaignID    string     `json:"campaign_id,omitempty"`
	AccountID     string     `json:"account_id,omitempty"`
	Level         LogLevel   `json:"level,omitempty"`
	Type          LogType    `json:"type,omitempty"`
	Component     string     `json:"component,omitempty"`
	StartTime     *time.Time `json:"start_time,omitempty"`
	EndTime       *time.Time `json:"end_time,omitempty"`
	SearchText    string     `json:"search_text,omitempty"`
	Tags          []string   `json:"tags,omitempty"`
	OnlyErrors    bool       `json:"only_errors,omitempty"`
	OnlySuccesses bool       `json:"only_successes,omitempty"`
	Limit         int        `json:"limit,omitempty"`
	Offset        int        `json:"offset,omitempty"`
}

func (l *LogEntry) MatchesFilter(filter *LogFilter) bool {
	if filter.TenantID != "" && l.TenantID != filter.TenantID {
		return false
	}
	if filter.SessionID != "" && l.SessionID != filter.SessionID {
		return false
	}
	if filter.CampaignID != "" && l.CampaignID != filter.CampaignID {
		return false
	}
	if filter.AccountID != "" && l.AccountID != filter.AccountID {
		return false
	}
	if filter.Level != "" && l.Level != filter.Level {
		return false
	}
	if filter.Type != "" && l.Type != filter.Type {
		return false
	}
	if filter.Component != "" && l.Component != filter.Component {
		return false
	}
	if filter.StartTime != nil && l.Timestamp.Before(*filter.StartTime) {
		return false
	}
	if filter.EndTime != nil && l.Timestamp.After(*filter.EndTime) {
		return false
	}
	if filter.OnlyErrors && !l.IsError() {
		return false
	}
	if filter.OnlySuccesses && !l.Success {
		return false
	}
	if filter.SearchText != "" {
		searchLower := strings.ToLower(filter.SearchText)
		if !strings.Contains(strings.ToLower(l.Message), searchLower) &&
			!strings.Contains(strings.ToLower(l.Details), searchLower) {
			return false
		}
	}
	if len(filter.Tags) > 0 {
		hasTag := false
		for _, filterTag := range filter.Tags {
			for _, logTag := range l.Tags {
				if filterTag == logTag {
					hasTag = true
					break
				}
			}
			if hasTag {
				break
			}
		}
		if !hasTag {
			return false
		}
	}
	return true
}

func (l *LogEntry) GetSeverityColor() string {
	switch l.Level {
	case LogLevelDebug:
		return "gray"
	case LogLevelInfo:
		return "blue"
	case LogLevelWarn:
		return "yellow"
	case LogLevelError:
		return "red"
	case LogLevelFatal:
		return "darkred"
	default:
		return "black"
	}
}
