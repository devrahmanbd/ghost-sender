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
	YahooSMTPHost         = "smtp.mail.yahoo.com"
	YahooSMTPPort         = 587
	YahooMaxMessageSize   = 25 * 1024 * 1024
	YahooDefaultDailyLim  = 500
	YahooDefaultHourlyLim = 100
)

var (
	ErrYahooAuthFailed       = errors.New("yahoo authentication failed")
	ErrYahooRateLimitHit     = errors.New("yahoo rate limit exceeded")
	ErrYahooMessageRejected  = errors.New("yahoo message rejected")
	ErrYahooTemporaryFailure = errors.New("yahoo temporary failure")
)

type YahooProvider struct {
	config       *ProviderConfig
	log          logger.Logger
	smtpProvider Provider
	mu           sync.RWMutex
	health       *ProviderHealth
	rateLimiter  *YahooRateLimiter
	lastSend     time.Time
	messageCount int64
	errorCount   int64
	retryConfig  *YahooRetryConfig
}

type YahooRateLimiter struct {
	hourlyLimit   int
	dailyLimit    int
	hourlySent    int
	dailySent     int
	lastHourReset time.Time
	lastDayReset  time.Time
	mu            sync.Mutex
}

type YahooRetryConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	BackoffFactor  float64
}

func NewYahooProvider(config *ProviderConfig, log logger.Logger) (Provider, error) {
	if config == nil {
		return nil, ErrInvalidConfig
	}

	yahooConfig := buildYahooConfig(config)

	smtpProvider, err := NewSMTPProviderImpl(yahooConfig, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create yahoo smtp provider: %w", err)
	}

	provider := &YahooProvider{
		config:       yahooConfig,
		log:          log,
		smtpProvider: smtpProvider,
		health: &ProviderHealth{
			Status:    HealthStatusUnknown,
			LastCheck: time.Now(),
		},
		rateLimiter: &YahooRateLimiter{
			hourlyLimit:   yahooConfig.RateLimitPerHour,
			dailyLimit:    yahooConfig.RateLimitPerDay,
			lastHourReset: time.Now(),
			lastDayReset:  time.Now(),
		},
		lastSend: time.Now(),
		retryConfig: &YahooRetryConfig{
			MaxRetries:     3,
			InitialBackoff: 2 * time.Second,
			MaxBackoff:     30 * time.Second,
			BackoffFactor:  2.0,
		},
	}

	return provider, nil
}

func buildYahooConfig(config *ProviderConfig) *ProviderConfig {
	yahooConfig := &ProviderConfig{
		Type:     ProviderTypeYahoo,
		Host:     YahooSMTPHost,
		Port:     YahooSMTPPort,
		Username: config.Username,
		Password: config.Password,
		TLSConfig: &TLSConfig{
			Enabled:            true,
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		},
		TimeoutConfig: &TimeoutConfig{
			Connect: 30 * time.Second,
			Send:    60 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
		},
		MaxMessageSize:   YahooMaxMessageSize,
		RateLimitPerDay:  YahooDefaultDailyLim,
		RateLimitPerHour: YahooDefaultHourlyLim,
	}

	if config.RateLimitPerDay > 0 {
		yahooConfig.RateLimitPerDay = config.RateLimitPerDay
	}

	if config.RateLimitPerHour > 0 {
		yahooConfig.RateLimitPerHour = config.RateLimitPerHour
	}

	if config.ConnectionPool != nil {
		yahooConfig.ConnectionPool = config.ConnectionPool
	}

	return yahooConfig
}

func (p *YahooProvider) Send(ctx context.Context, message *models.Email) (string, error) {
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
				(p.log).Debug(fmt.Sprintf("retrying yahoo send: attempt=%d, backoff=%v", attempt, backoff))
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
				(p.log).Debug(fmt.Sprintf("email sent via yahoo: to=%s, duration=%v, attempt=%d", message.To.Address, time.Since(startTime), attempt+1))
			}

			return messageID, nil
		}

		lastErr = err
		p.incrementErrorCount()
		p.updateHealth(false, time.Since(startTime))

		if !p.isRetryableError(err) {
			if p.log != nil {
				(p.log).Error(fmt.Sprintf("non-retryable yahoo error: error=%v, to=%s", err, message.To.Address))
			}
			return "", p.categorizeError(err)
		}

		if p.log != nil {
			(p.log).Warn(fmt.Sprintf("retryable yahoo error: error=%v, attempt=%d, max_retries=%d", err, attempt+1, p.retryConfig.MaxRetries))
		}
	}

	return "", fmt.Errorf("yahoo send failed after %d attempts: %w", p.retryConfig.MaxRetries+1, lastErr)
}

func (p *YahooProvider) validateMessage(message *models.Email) error {
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
		return fmt.Errorf("message size %d exceeds yahoo limit %d", totalSize, p.config.MaxMessageSize)
	}

	return nil
}

func (p *YahooProvider) isRetryableError(err error) bool {
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

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

func (p *YahooProvider) categorizeError(err error) error {
	if err == nil {
		return nil
	}

	errStr := strings.ToLower(err.Error())

	if strings.Contains(errStr, "authentication") || strings.Contains(errStr, "auth") {
		return ErrYahooAuthFailed
	}

	if strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "too many") {
		return ErrYahooRateLimitHit
	}

	if strings.Contains(errStr, "rejected") || strings.Contains(errStr, "blocked") ||
		strings.Contains(errStr, "spam") || strings.Contains(errStr, "550") {
		return ErrYahooMessageRejected
	}

	if strings.Contains(errStr, "temporary") || strings.Contains(errStr, "421") ||
		strings.Contains(errStr, "450") || strings.Contains(errStr, "451") {
		return ErrYahooTemporaryFailure
	}

	return err
}

func (p *YahooProvider) TestConnection(ctx context.Context) error {
	if p.log != nil {
		(p.log).Debug("testing yahoo connection")
	}

	if p.smtpProvider == nil {
		return errors.New("yahoo smtp provider not initialized")
	}

	startTime := time.Now()
	err := p.smtpProvider.TestConnection(ctx)

	if err != nil {
		p.updateHealth(false, time.Since(startTime))
		return fmt.Errorf("yahoo connection test failed: %w", err)
	}

	p.updateHealth(true, time.Since(startTime))
	if p.log != nil {
		(p.log).Info("yahoo connection test successful")
	}

	return nil
}

func (p *YahooProvider) Validate() error {
	if p.config == nil {
		return ErrInvalidConfig
	}

	if p.config.Username == "" {
		return fmt.Errorf("yahoo username is required")
	}

	if p.config.Password == "" {
		return fmt.Errorf("yahoo app password is required")
	}

	if !strings.Contains(strings.ToLower(p.config.Username), "@yahoo") &&
		!strings.Contains(strings.ToLower(p.config.Username), "@ymail") &&
		!strings.Contains(strings.ToLower(p.config.Username), "@rocketmail") {
		return fmt.Errorf("invalid yahoo email address")
	}

	return nil
}

func (p *YahooProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.smtpProvider != nil {
		if err := p.smtpProvider.Close(); err != nil {
			if p.log != nil {
				(p.log).Error(fmt.Sprintf("error closing yahoo smtp provider: %v", err))
			}
		}
	}

	p.health.IsConnected = false
	if p.log != nil {
		(p.log).Info("yahoo provider closed")
	}

	return nil
}

func (p *YahooProvider) Name() string {
	return fmt.Sprintf("Yahoo (%s)", p.config.Username)
}

func (p *YahooProvider) Type() ProviderType {
	return ProviderTypeYahoo
}

func (p *YahooProvider) SupportedFeatures() []Feature {
	return []Feature{
		FeatureTLS,
		FeatureSTARTTLS,
		FeatureHTMLBody,
		FeatureTextBody,
		FeatureAttachments,
		FeatureConnectionPool,
	}
}

func (p *YahooProvider) GetConfig() *ProviderConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config
}

func (p *YahooProvider) checkRateLimit() error {
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
		return fmt.Errorf("%w: hourly limit %d/%d", ErrYahooRateLimitHit,
			p.rateLimiter.hourlySent, p.rateLimiter.hourlyLimit)
	}

	if p.rateLimiter.dailySent >= p.rateLimiter.dailyLimit {
		return fmt.Errorf("%w: daily limit %d/%d", ErrYahooRateLimitHit,
			p.rateLimiter.dailySent, p.rateLimiter.dailyLimit)
	}

	p.rateLimiter.hourlySent++
	p.rateLimiter.dailySent++

	return nil
}

func (p *YahooProvider) updateHealth(success bool, responseTime time.Duration) {
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

func (p *YahooProvider) incrementMessageCount() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messageCount++
	p.health.TotalSent++
}

func (p *YahooProvider) incrementErrorCount() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.errorCount++
	p.health.TotalFailed++
}

func (p *YahooProvider) GetHealth() *ProviderHealth {
	p.mu.RLock()
	defer p.mu.RUnlock()

	health := *p.health
	return &health
}

func (p *YahooProvider) GetStats() map[string]interface{} {
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
