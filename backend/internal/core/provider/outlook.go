package provider

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"email-campaign-system/internal/models"
	"email-campaign-system/pkg/logger"
)

const (
	OutlookSMTPHost         = "smtp-mail.outlook.com"
	OutlookSMTPPort         = 587
	OutlookMaxMessageSize   = 35 * 1024 * 1024
	OutlookDefaultDailyLim  = 300
	OutlookDefaultHourlyLim = 50
)

var (
	ErrOutlookAuthFailed       = errors.New("outlook authentication failed")
	ErrOutlookRateLimitHit     = errors.New("outlook rate limit exceeded")
	ErrOutlookMessageRejected  = errors.New("outlook message rejected")
	ErrOutlookTemporaryFailure = errors.New("outlook temporary failure")
	ErrOutlookAccountLocked    = errors.New("outlook account locked")
)

type OutlookProvider struct {
	config       *ProviderConfig
	log          logger.Logger
	smtpProvider Provider
	mu           sync.RWMutex
	health       *ProviderHealth
	rateLimiter  *OutlookRateLimiter
	lastSend     time.Time
	messageCount int64
	errorCount   int64
	retryConfig  *OutlookRetryConfig
}

type OutlookRateLimiter struct {
	hourlyLimit   int
	dailyLimit    int
	hourlySent    int
	dailySent     int
	lastHourReset time.Time
	lastDayReset  time.Time
	mu            sync.Mutex
}

type OutlookRetryConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	BackoffFactor  float64
}

func NewOutlookProvider(config *ProviderConfig, log logger.Logger) (Provider, error) {
	if config == nil {
		return nil, ErrInvalidConfig
	}

	outlookConfig := buildOutlookConfig(config)

	smtpProvider, err := NewSMTPProviderImpl(outlookConfig, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create outlook smtp provider: %w", err)
	}

	provider := &OutlookProvider{
		config:       outlookConfig,
		log:          log,
		smtpProvider: smtpProvider,
		health: &ProviderHealth{
			Status:    HealthStatusUnknown,
			LastCheck: time.Now(),
		},
		rateLimiter: &OutlookRateLimiter{
			hourlyLimit:   outlookConfig.RateLimitPerHour,
			dailyLimit:    outlookConfig.RateLimitPerDay,
			lastHourReset: time.Now(),
			lastDayReset:  time.Now(),
		},
		lastSend: time.Now(),
		retryConfig: &OutlookRetryConfig{
			MaxRetries:     3,
			InitialBackoff: 3 * time.Second,
			MaxBackoff:     60 * time.Second,
			BackoffFactor:  2.0,
		},
	}

	return provider, nil
}

func NewHotmailProvider(config *ProviderConfig, log logger.Logger) (Provider, error) {
	return NewOutlookProvider(config, log)
}

func buildOutlookConfig(config *ProviderConfig) *ProviderConfig {
	outlookConfig := &ProviderConfig{
		Type:     ProviderTypeOutlook,
		Host:     OutlookSMTPHost,
		Port:     OutlookSMTPPort,
		Username: config.Username,
		Password: config.Password,
		TLSConfig: &TLSConfig{
			Enabled:            true,
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		},
		TimeoutConfig: &TimeoutConfig{
			Connect: 30 * time.Second,
			Send:    90 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
		},
		MaxMessageSize:   OutlookMaxMessageSize,
		RateLimitPerDay:  OutlookDefaultDailyLim,
		RateLimitPerHour: OutlookDefaultHourlyLim,
	}

	if config.RateLimitPerDay > 0 {
		outlookConfig.RateLimitPerDay = config.RateLimitPerDay
	}

	if config.RateLimitPerHour > 0 {
		outlookConfig.RateLimitPerHour = config.RateLimitPerHour
	}

	if config.ConnectionPool != nil {
		outlookConfig.ConnectionPool = config.ConnectionPool
	}

	return outlookConfig
}

func (p *OutlookProvider) Send(ctx context.Context, message *models.Email) (string, error) {
	if message == nil {
		return "", errors.New("message cannot be nil")
	}

	if err := p.checkRateLimit(); err != nil {
		return "", err
	}

	if err := p.validateMessage(message); err != nil {
		return "", fmt.Errorf("message validation failed: %w", err)
	}

	var lastErr error
	backoff := p.retryConfig.InitialBackoff

	for attempt := 0; attempt <= p.retryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			if p.log != nil {
				(p.log).Debug(fmt.Sprintf("retrying outlook send: attempt=%d, backoff=%v", attempt, backoff))
			}
			time.Sleep(backoff)
			backoff = time.Duration(float64(backoff) * p.retryConfig.BackoffFactor)
			if backoff > p.retryConfig.MaxBackoff {
				backoff = p.retryConfig.MaxBackoff
			}
		}

		startTime := time.Now()

		messageID, err := p.smtpProvider.Send(ctx, message)
		if err == nil {
			p.incrementMessageCount()
			p.updateHealth(true, time.Since(startTime))
			p.lastSend = time.Now()

			if p.log != nil {
				(p.log).Debug(fmt.Sprintf("email sent via outlook: to=%s, duration=%v, attempt=%d", message.To.Address, time.Since(startTime), attempt+1))
			}

			return messageID, nil
		}

		lastErr = err
		p.incrementErrorCount()
		p.updateHealth(false, time.Since(startTime))

		categorizedErr := p.categorizeError(err)

		if categorizedErr == ErrOutlookAccountLocked {
			if p.log != nil {
				(p.log).Error(fmt.Sprintf("outlook account locked: error=%v, username=%s", err, p.config.Username))
			}
			return "", categorizedErr
		}

		if !p.isRetryableError(err) {
			if p.log != nil {
				(p.log).Error(fmt.Sprintf("non-retryable outlook error: error=%v, to=%s", err, message.To.Address))
			}
			return "", categorizedErr
		}

		if p.log != nil {
			(p.log).Warn(fmt.Sprintf("retryable outlook error: error=%v, attempt=%d, max_retries=%d", err, attempt+1, p.retryConfig.MaxRetries))
		}
	}

	return "", fmt.Errorf("outlook send failed after %d attempts: %w", p.retryConfig.MaxRetries+1, lastErr)
}

func (p *OutlookProvider) validateMessage(message *models.Email) error {
	if message.To.Address == "" {
		return errors.New("recipient email is required")
	}

	if message.From.Address == "" {
		return errors.New("sender email is required")
	}

	if message.Subject == "" {
		return errors.New("subject is required")
	}

	if message.HTMLBody == "" && message.PlainTextBody == "" {
		return errors.New("message body is required")
	}

	totalSize := int64(len(message.HTMLBody) + len(message.PlainTextBody))
	for _, att := range message.Attachments {
		totalSize += int64(len(att.Content))
	}

	if totalSize > p.config.MaxMessageSize {
		return fmt.Errorf("message size %d exceeds outlook limit %d", totalSize, p.config.MaxMessageSize)
	}

	return nil
}

func (p *OutlookProvider) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	retryablePatterns := []string{
		"temporary failure",
		"try again",
		"timeout",
		"connection reset",
		"connection refused",
		"broken pipe",
		"network",
		"421",
		"450",
		"451",
		"452",
	}

	nonRetryablePatterns := []string{
		"locked",
		"suspended",
		"disabled",
		"blocked",
	}

	for _, pattern := range nonRetryablePatterns {
		if strings.Contains(errStr, pattern) {
			return false
		}
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

func (p *OutlookProvider) categorizeError(err error) error {
	if err == nil {
		return nil
	}

	errStr := strings.ToLower(err.Error())

	if strings.Contains(errStr, "locked") || strings.Contains(errStr, "suspended") ||
		strings.Contains(errStr, "disabled") {
		return ErrOutlookAccountLocked
	}

	if strings.Contains(errStr, "authentication") || strings.Contains(errStr, "auth") ||
		strings.Contains(errStr, "535") {
		return ErrOutlookAuthFailed
	}

	if strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "too many") ||
		strings.Contains(errStr, "554") {
		return ErrOutlookRateLimitHit
	}

	if strings.Contains(errStr, "rejected") || strings.Contains(errStr, "blocked") ||
		strings.Contains(errStr, "spam") || strings.Contains(errStr, "550") ||
		strings.Contains(errStr, "551") {
		return ErrOutlookMessageRejected
	}

	if strings.Contains(errStr, "temporary") || strings.Contains(errStr, "421") ||
		strings.Contains(errStr, "450") || strings.Contains(errStr, "451") ||
		strings.Contains(errStr, "452") {
		return ErrOutlookTemporaryFailure
	}

	return err
}

func (p *OutlookProvider) TestConnection(ctx context.Context) error {
	if p.log != nil {
		(p.log).Debug("testing outlook connection")
	}

	if p.smtpProvider == nil {
		return errors.New("outlook smtp provider not initialized")
	}

	startTime := time.Now()
	err := p.smtpProvider.TestConnection(ctx)

	if err != nil {
		p.updateHealth(false, time.Since(startTime))
		return fmt.Errorf("outlook connection test failed: %w", err)
	}

	p.updateHealth(true, time.Since(startTime))
	if p.log != nil {
		(p.log).Info("outlook connection test successful")
	}

	return nil
}

func (p *OutlookProvider) Validate() error {
	if p.config == nil {
		return ErrInvalidConfig
	}

	if p.config.Username == "" {
		return fmt.Errorf("outlook username is required")
	}

	if p.config.Password == "" {
		return fmt.Errorf("outlook app password is required")
	}

	emailLower := strings.ToLower(p.config.Username)
	validDomains := []string{"@outlook.com", "@hotmail.com", "@live.com", "@msn.com"}

	isValid := false
	for _, domain := range validDomains {
		if strings.Contains(emailLower, domain) {
			isValid = true
			break
		}
	}

	if !isValid {
		return fmt.Errorf("invalid outlook/hotmail email address")
	}

	return nil
}

func (p *OutlookProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.smtpProvider != nil {
		if err := p.smtpProvider.Close(); err != nil {
			if p.log != nil {
				(p.log).Error(fmt.Sprintf("error closing outlook smtp provider: %v", err))
			}
		}
	}

	p.health.IsConnected = false
	if p.log != nil {
		(p.log).Info("outlook provider closed")
	}

	return nil
}

func (p *OutlookProvider) Name() string {
	return fmt.Sprintf("Outlook (%s)", p.config.Username)
}

func (p *OutlookProvider) Type() ProviderType {
	return ProviderTypeOutlook
}

func (p *OutlookProvider) SupportedFeatures() []Feature {
	return []Feature{
		FeatureTLS,
		FeatureSTARTTLS,
		FeatureHTMLBody,
		FeatureTextBody,
		FeatureAttachments,
		FeatureConnectionPool,
	}
}

func (p *OutlookProvider) GetConfig() *ProviderConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config
}

func (p *OutlookProvider) checkRateLimit() error {
	p.rateLimiter.mu.Lock()
	defer p.rateLimiter.mu.Unlock()

	now := time.Now()

	if now.Sub(p.rateLimiter.lastHourReset) >= time.Hour {
		p.rateLimiter.hourlySent = 0
		p.rateLimiter.lastHourReset = now
	}

	if now.Sub(p.rateLimiter.lastDayReset) >= 24*time.Hour {
		p.rateLimiter.dailySent = 0
		p.rateLimiter.lastDayReset = now
	}

	if p.rateLimiter.hourlySent >= p.rateLimiter.hourlyLimit {
		return fmt.Errorf("%w: hourly limit %d/%d", ErrOutlookRateLimitHit,
			p.rateLimiter.hourlySent, p.rateLimiter.hourlyLimit)
	}

	if p.rateLimiter.dailySent >= p.rateLimiter.dailyLimit {
		return fmt.Errorf("%w: daily limit %d/%d", ErrOutlookRateLimitHit,
			p.rateLimiter.dailySent, p.rateLimiter.dailyLimit)
	}

	p.rateLimiter.hourlySent++
	p.rateLimiter.dailySent++

	return nil
}

func (p *OutlookProvider) updateHealth(success bool, responseTime time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()

	if success {
		p.health.LastSuccess = now
		p.health.ConsecutiveFails = 0
		p.health.Status = HealthStatusHealthy
		p.health.Message = "operational"
		p.health.IsConnected = true
	} else {
		p.health.LastFailure = now
		p.health.ConsecutiveFails++

		if p.health.ConsecutiveFails >= 5 {
			p.health.Status = HealthStatusUnhealthy
			p.health.Message = "multiple consecutive failures"
		} else if p.health.ConsecutiveFails >= 3 {
			p.health.Status = HealthStatusDegraded
			p.health.Message = "experiencing issues"
		}
	}

	p.health.LastCheck = now

	if responseTime > 0 {
		if p.health.AvgResponseTime == 0 {
			p.health.AvgResponseTime = responseTime
		} else {
			p.health.AvgResponseTime = (p.health.AvgResponseTime + responseTime) / 2
		}
	}

	total := p.health.TotalSent + p.health.TotalFailed
	if total > 0 {
		p.health.ErrorRate = float64(p.health.TotalFailed) / float64(total) * 100
	}
}

func (p *OutlookProvider) incrementMessageCount() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messageCount++
	p.health.TotalSent++
}

func (p *OutlookProvider) incrementErrorCount() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.errorCount++
	p.health.TotalFailed++
}

func (p *OutlookProvider) GetHealth() *ProviderHealth {
	p.mu.RLock()
	defer p.mu.RUnlock()

	health := *p.health
	return &health
}

func (p *OutlookProvider) GetStats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	p.rateLimiter.mu.Lock()
	hourlySent := p.rateLimiter.hourlySent
	dailySent := p.rateLimiter.dailySent
	p.rateLimiter.mu.Unlock()

	return map[string]interface{}{
		"total_sent":        p.messageCount,
		"total_failed":      p.errorCount,
		"last_send_time":    p.lastSend,
		"avg_response_time": p.health.AvgResponseTime,
		"hourly_sent":       hourlySent,
		"daily_sent":        dailySent,
	}
}
