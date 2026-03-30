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
	Office365SMTPHost        = "smtp.office365.com"
	Office365SMTPPort        = 587
	Office365MaxMessageSize  = 35 * 1024 * 1024
	Office365DefaultDailyLim = 10000
	Office365DefaultHrlyLim  = 1000
)

var (
	ErrOffice365AuthFailed       = errors.New("office365 authentication failed")
	ErrOffice365RateLimitHit     = errors.New("office365 rate limit exceeded")
	ErrOffice365MessageRejected  = errors.New("office365 message rejected")
	ErrOffice365TemporaryFailure = errors.New("office365 temporary failure")
	ErrOffice365AccountLocked    = errors.New("office365 account locked or disabled")
	ErrOffice365TLSRequired      = errors.New("office365 requires TLS — STARTTLS must be established before authentication")
)

type Office365Provider struct {
	config       *ProviderConfig
	log          logger.Logger
	smtpProvider Provider
	mu           sync.RWMutex
	health       *ProviderHealth
	rateLimiter  *Office365RateLimiter
	lastSend     time.Time
	messageCount int64
	errorCount   int64
	retryConfig  *Office365RetryConfig
}

type Office365RateLimiter struct {
	hourlyLimit   int
	dailyLimit    int
	hourlySent    int
	dailySent     int
	lastHourReset time.Time
	lastDayReset  time.Time
	mu            sync.Mutex
}

type Office365RetryConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	BackoffFactor  float64
}

func NewOffice365Provider(config *ProviderConfig, log logger.Logger) (Provider, error) {
	if config == nil {
		return nil, ErrInvalidConfig
	}

	office365Config := buildOffice365Config(config)

	smtpProvider, err := NewSMTPProviderImpl(office365Config, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create office365 smtp provider: %w", err)
	}

	provider := &Office365Provider{
		config:       office365Config,
		log:          log,
		smtpProvider: smtpProvider,
		health: &ProviderHealth{
			Status:    HealthStatusUnknown,
			LastCheck: time.Now(),
		},
		rateLimiter: &Office365RateLimiter{
			hourlyLimit:   office365Config.RateLimitPerHour,
			dailyLimit:    office365Config.RateLimitPerDay,
			lastHourReset: time.Now(),
			lastDayReset:  time.Now(),
		},
		lastSend: time.Now(),
		retryConfig: &Office365RetryConfig{
			MaxRetries:     3,
			InitialBackoff: 2 * time.Second,
			MaxBackoff:     60 * time.Second,
			BackoffFactor:  2.0,
		},
	}

	return provider, nil
}

func buildOffice365Config(config *ProviderConfig) *ProviderConfig {
	office365Config := &ProviderConfig{
		Type:     ProviderTypeOffice365,
		Host:     Office365SMTPHost,
		Port:     Office365SMTPPort,
		Username: config.Username,
		Password: config.Password,
		// Office 365 requires TLS 1.2+ and STARTTLS on port 587.
		// Authentication (PLAIN or LOGIN) must only be attempted after STARTTLS succeeds.
		TLSConfig: &TLSConfig{
			Enabled:            true,
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
			ServerName:         Office365SMTPHost,
		},
		TimeoutConfig: &TimeoutConfig{
			Connect: 30 * time.Second,
			Send:    90 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
		},
		RetryConfig: &RetryConfig{
			MaxRetries:   3,
			InitialDelay: 2 * time.Second,
			MaxDelay:     60 * time.Second,
			Multiplier:   2.0,
		},
		MaxMessageSize:   Office365MaxMessageSize,
		RateLimitPerDay:  Office365DefaultDailyLim,
		RateLimitPerHour: Office365DefaultHrlyLim,
		ConnectionPool: &ConnectionPoolConfig{
			MaxConnections: 5,
			MaxLifetime:    5 * time.Minute,
		},
	}

	if config.RateLimitPerDay > 0 {
		office365Config.RateLimitPerDay = config.RateLimitPerDay
	}
	if config.RateLimitPerHour > 0 {
		office365Config.RateLimitPerHour = config.RateLimitPerHour
	}
	if config.ConnectionPool != nil {
		office365Config.ConnectionPool = config.ConnectionPool
	}

	return office365Config
}

func (p *Office365Provider) Send(ctx context.Context, message *models.Email) (string, error) {
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
				(p.log).Debug(fmt.Sprintf("retrying office365 send: attempt=%d, backoff=%v", attempt, backoff))
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
				(p.log).Debug(fmt.Sprintf("email sent via office365: to=%s, duration=%v, attempt=%d",
					message.To.Address, time.Since(startTime), attempt+1))
			}
			return messageID, nil
		}

		lastErr = err
		p.incrementErrorCount()
		p.updateHealth(false, time.Since(startTime))

		categorizedErr := p.categorizeError(err)

		// Non-retryable errors
		if categorizedErr == ErrOffice365AccountLocked || categorizedErr == ErrOffice365AuthFailed {
			if p.log != nil {
				(p.log).Error(fmt.Sprintf("office365 non-retryable error: %v, username=%s", err, p.config.Username))
			}
			return "", categorizedErr
		}

		if !p.isRetryableError(err) {
			if p.log != nil {
				(p.log).Error(fmt.Sprintf("office365 non-retryable error: %v, to=%s", err, message.To.Address))
			}
			return "", categorizedErr
		}

		if p.log != nil {
			(p.log).Warn(fmt.Sprintf("office365 retryable error: %v, attempt=%d/%d", err, attempt+1, p.retryConfig.MaxRetries))
		}
	}

	return "", fmt.Errorf("office365 send failed after %d attempts: %w", p.retryConfig.MaxRetries+1, lastErr)
}

func (p *Office365Provider) validateMessage(message *models.Email) error {
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
		return fmt.Errorf("message size %d exceeds office365 limit %d", totalSize, p.config.MaxMessageSize)
	}
	return nil
}

func (p *Office365Provider) isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	retryable := []string{"temporary", "try again", "timeout", "connection reset", "connection refused", "broken pipe", "421", "450", "451", "452"}
	nonRetryable := []string{"locked", "suspended", "disabled", "blocked", "authentication", "504", "535"}
	for _, p := range nonRetryable {
		if strings.Contains(errStr, p) {
			return false
		}
	}
	for _, p := range retryable {
		if strings.Contains(errStr, p) {
			return true
		}
	}
	return false
}

func (p *Office365Provider) categorizeError(err error) error {
	if err == nil {
		return nil
	}
	errStr := strings.ToLower(err.Error())
	if strings.Contains(errStr, "locked") || strings.Contains(errStr, "suspended") || strings.Contains(errStr, "disabled") {
		return ErrOffice365AccountLocked
	}
	if strings.Contains(errStr, "authentication") || strings.Contains(errStr, "auth") ||
		strings.Contains(errStr, "535") || strings.Contains(errStr, "504") {
		return ErrOffice365AuthFailed
	}
	if strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "too many") || strings.Contains(errStr, "554") {
		return ErrOffice365RateLimitHit
	}
	if strings.Contains(errStr, "rejected") || strings.Contains(errStr, "blocked") ||
		strings.Contains(errStr, "spam") || strings.Contains(errStr, "550") {
		return ErrOffice365MessageRejected
	}
	if strings.Contains(errStr, "temporary") || strings.Contains(errStr, "421") || strings.Contains(errStr, "450") {
		return ErrOffice365TemporaryFailure
	}
	return err
}

func (p *Office365Provider) TestConnection(ctx context.Context) error {
	if p.smtpProvider == nil {
		return errors.New("office365 smtp provider not initialized")
	}
	startTime := time.Now()
	err := p.smtpProvider.TestConnection(ctx)
	if err != nil {
		p.updateHealth(false, time.Since(startTime))
		return fmt.Errorf("office365 connection test failed: %w", err)
	}
	p.updateHealth(true, time.Since(startTime))
	if p.log != nil {
		(p.log).Info("office365 connection test successful")
	}
	return nil
}

func (p *Office365Provider) Validate() error {
	if p.config == nil {
		return ErrInvalidConfig
	}
	if p.config.Username == "" {
		return fmt.Errorf("office365 username is required")
	}
	if p.config.Password == "" {
		return fmt.Errorf("office365 password is required")
	}
	return nil
}

func (p *Office365Provider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.smtpProvider != nil {
		if err := p.smtpProvider.Close(); err != nil && p.log != nil {
			(p.log).Error(fmt.Sprintf("error closing office365 smtp provider: %v", err))
		}
	}
	p.health.IsConnected = false
	if p.log != nil {
		(p.log).Info("office365 provider closed")
	}
	return nil
}

func (p *Office365Provider) Name() string {
	return fmt.Sprintf("Office365 (%s)", p.config.Username)
}

func (p *Office365Provider) Type() ProviderType {
	return ProviderTypeOffice365
}

func (p *Office365Provider) SupportedFeatures() []Feature {
	return []Feature{
		FeatureTLS,
		FeatureSTARTTLS,
		FeatureHTMLBody,
		FeatureTextBody,
		FeatureAttachments,
		FeatureConnectionPool,
	}
}

func (p *Office365Provider) GetConfig() *ProviderConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config
}

func (p *Office365Provider) GetHealth() *ProviderHealth {
	p.mu.RLock()
	defer p.mu.RUnlock()
	health := *p.health
	return &health
}

func (p *Office365Provider) GetStats() map[string]interface{} {
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

func (p *Office365Provider) checkRateLimit() error {
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
		return fmt.Errorf("%w: hourly limit %d/%d", ErrOffice365RateLimitHit, p.rateLimiter.hourlySent, p.rateLimiter.hourlyLimit)
	}
	if p.rateLimiter.dailySent >= p.rateLimiter.dailyLimit {
		return fmt.Errorf("%w: daily limit %d/%d", ErrOffice365RateLimitHit, p.rateLimiter.dailySent, p.rateLimiter.dailyLimit)
	}
	p.rateLimiter.hourlySent++
	p.rateLimiter.dailySent++
	return nil
}

func (p *Office365Provider) updateHealth(success bool, responseTime time.Duration) {
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

func (p *Office365Provider) incrementMessageCount() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messageCount++
	p.health.TotalSent++
}

func (p *Office365Provider) incrementErrorCount() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.errorCount++
	p.health.TotalFailed++
}
