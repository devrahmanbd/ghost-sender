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
	ICloudSMTPHost         = "smtp.mail.me.com"
	ICloudSMTPPort         = 587
	ICloudMaxMessageSize   = 20 * 1024 * 1024
	ICloudDefaultDailyLim  = 1000
	ICloudDefaultHourlyLim = 200
)

var (
	ErrICloudAuthFailed       = errors.New("icloud authentication failed")
	ErrICloudRateLimitHit     = errors.New("icloud rate limit exceeded")
	ErrICloudMessageRejected  = errors.New("icloud message rejected")
	ErrICloudTemporaryFailure = errors.New("icloud temporary failure")
	ErrICloudInvalidPassword  = errors.New("icloud app-specific password required")
)

type ICloudProvider struct {
	config       *ProviderConfig
	log          logger.Logger
	smtpProvider Provider
	mu           sync.RWMutex
	health       *ProviderHealth
	rateLimiter  *ICloudRateLimiter
	lastSend     time.Time
	messageCount int64
	errorCount   int64
	retryConfig  *ICloudRetryConfig
}

type ICloudRateLimiter struct {
	hourlyLimit   int
	dailyLimit    int
	hourlySent    int
	dailySent     int
	lastHourReset time.Time
	lastDayReset  time.Time
	mu            sync.Mutex
}

type ICloudRetryConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	BackoffFactor  float64
}

func NewICloudProvider(config *ProviderConfig, log logger.Logger) (Provider, error) {
	if config == nil {
		return nil, ErrInvalidConfig
	}

	icloudConfig := buildICloudConfig(config)

	smtpProvider, err := NewSMTPProviderImpl(icloudConfig, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create icloud smtp provider: %w", err)
	}

	provider := &ICloudProvider{
		config:       icloudConfig,
		log:          log,
		smtpProvider: smtpProvider,
		health: &ProviderHealth{
			Status:    HealthStatusUnknown,
			LastCheck: time.Now(),
		},
		rateLimiter: &ICloudRateLimiter{
			hourlyLimit:   icloudConfig.RateLimitPerHour,
			dailyLimit:    icloudConfig.RateLimitPerDay,
			lastHourReset: time.Now(),
			lastDayReset:  time.Now(),
		},
		lastSend: time.Now(),
		retryConfig: &ICloudRetryConfig{
			MaxRetries:     3,
			InitialBackoff: 2 * time.Second,
			MaxBackoff:     30 * time.Second,
			BackoffFactor:  2.0,
		},
	}

	return provider, nil
}

func buildICloudConfig(config *ProviderConfig) *ProviderConfig {
	icloudConfig := &ProviderConfig{
		Type:     ProviderTypeICloud,
		Host:     ICloudSMTPHost,
		Port:     ICloudSMTPPort,
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
		MaxMessageSize:   ICloudMaxMessageSize,
		RateLimitPerDay:  ICloudDefaultDailyLim,
		RateLimitPerHour: ICloudDefaultHourlyLim,
	}

	if config.RateLimitPerDay > 0 {
		icloudConfig.RateLimitPerDay = config.RateLimitPerDay
	}

	if config.RateLimitPerHour > 0 {
		icloudConfig.RateLimitPerHour = config.RateLimitPerHour
	}

	if config.ConnectionPool != nil {
		icloudConfig.ConnectionPool = config.ConnectionPool
	}

	return icloudConfig
}

func (p *ICloudProvider) Send(ctx context.Context, message *models.Email) (string, error) {
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
				(p.log).Debug(fmt.Sprintf("retrying icloud send: attempt=%d, backoff=%v", attempt, backoff))
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
				(p.log).Debug(fmt.Sprintf("email sent via icloud: to=%s, duration=%v, attempt=%d", message.To.Address, time.Since(startTime), attempt+1))
			}

			return messageID, nil
		}

		lastErr = err
		p.incrementErrorCount()
		p.updateHealth(false, time.Since(startTime))

		categorizedErr := p.categorizeError(err)

		if categorizedErr == ErrICloudInvalidPassword {
			if p.log != nil {
				(p.log).Error(fmt.Sprintf("icloud app-specific password invalid: error=%v, username=%s", err, p.config.Username))
			}
			return "", categorizedErr
		}

		if !p.isRetryableError(err) {
			if p.log != nil {
				(p.log).Error(fmt.Sprintf("non-retryable icloud error: error=%v, to=%s", err, message.To.Address))
			}
			return "", categorizedErr
		}

		if p.log != nil {
			(p.log).Warn(fmt.Sprintf("retryable icloud error: error=%v, attempt=%d, max_retries=%d", err, attempt+1, p.retryConfig.MaxRetries))
		}
	}

	return "", fmt.Errorf("icloud send failed after %d attempts: %w", p.retryConfig.MaxRetries+1, lastErr)
}

func (p *ICloudProvider) validateMessage(message *models.Email) error {
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
		return fmt.Errorf("message size %d exceeds icloud limit %d", totalSize, p.config.MaxMessageSize)
	}

	return nil
}

func (p *ICloudProvider) isRetryableError(err error) bool {
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
		"invalid password",
		"bad credentials",
		"authentication failed",
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

func (p *ICloudProvider) categorizeError(err error) error {
	if err == nil {
		return nil
	}

	errStr := strings.ToLower(err.Error())

	if strings.Contains(errStr, "invalid password") || strings.Contains(errStr, "bad credentials") {
		return ErrICloudInvalidPassword
	}

	if strings.Contains(errStr, "authentication") || strings.Contains(errStr, "auth") ||
		strings.Contains(errStr, "535") {
		return ErrICloudAuthFailed
	}

	if strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "too many") ||
		strings.Contains(errStr, "quota") {
		return ErrICloudRateLimitHit
	}

	if strings.Contains(errStr, "rejected") || strings.Contains(errStr, "blocked") ||
		strings.Contains(errStr, "spam") || strings.Contains(errStr, "550") ||
		strings.Contains(errStr, "551") {
		return ErrICloudMessageRejected
	}

	if strings.Contains(errStr, "temporary") || strings.Contains(errStr, "421") ||
		strings.Contains(errStr, "450") || strings.Contains(errStr, "451") ||
		strings.Contains(errStr, "452") {
		return ErrICloudTemporaryFailure
	}

	return err
}

func (p *ICloudProvider) TestConnection(ctx context.Context) error {
	if p.log != nil {
		(p.log).Debug("testing icloud connection")
	}

	if p.smtpProvider == nil {
		return errors.New("icloud smtp provider not initialized")
	}

	startTime := time.Now()
	err := p.smtpProvider.TestConnection(ctx)

	if err != nil {
		p.updateHealth(false, time.Since(startTime))
		return fmt.Errorf("icloud connection test failed: %w", err)
	}

	p.updateHealth(true, time.Since(startTime))
	if p.log != nil {
		(p.log).Info("icloud connection test successful")
	}

	return nil
}

func (p *ICloudProvider) Validate() error {
	if p.config == nil {
		return ErrInvalidConfig
	}

	if p.config.Username == "" {
		return fmt.Errorf("icloud username is required")
	}

	if p.config.Password == "" {
		return fmt.Errorf("icloud app-specific password is required")
	}

	emailLower := strings.ToLower(p.config.Username)
	validDomains := []string{"@icloud.com", "@me.com", "@mac.com"}

	isValid := false
	for _, domain := range validDomains {
		if strings.Contains(emailLower, domain) {
			isValid = true
			break
		}
	}

	if !isValid {
		return fmt.Errorf("invalid icloud email address")
	}

	return nil
}

func (p *ICloudProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.smtpProvider != nil {
		if err := p.smtpProvider.Close(); err != nil {
			if p.log != nil {
				(p.log).Error(fmt.Sprintf("error closing icloud smtp provider: %v", err))
			}
		}
	}

	p.health.IsConnected = false
	if p.log != nil {
		(p.log).Info("icloud provider closed")
	}

	return nil
}

func (p *ICloudProvider) Name() string {
	return fmt.Sprintf("iCloud (%s)", p.config.Username)
}

func (p *ICloudProvider) Type() ProviderType {
	return ProviderTypeICloud
}

func (p *ICloudProvider) SupportedFeatures() []Feature {
	return []Feature{
		FeatureTLS,
		FeatureSTARTTLS,
		FeatureHTMLBody,
		FeatureTextBody,
		FeatureAttachments,
		FeatureConnectionPool,
	}
}

func (p *ICloudProvider) GetConfig() *ProviderConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config
}

func (p *ICloudProvider) checkRateLimit() error {
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
		return fmt.Errorf("%w: hourly limit %d/%d", ErrICloudRateLimitHit,
			p.rateLimiter.hourlySent, p.rateLimiter.hourlyLimit)
	}

	if p.rateLimiter.dailySent >= p.rateLimiter.dailyLimit {
		return fmt.Errorf("%w: daily limit %d/%d", ErrICloudRateLimitHit,
			p.rateLimiter.dailySent, p.rateLimiter.dailyLimit)
	}

	p.rateLimiter.hourlySent++
	p.rateLimiter.dailySent++

	return nil
}

func (p *ICloudProvider) updateHealth(success bool, responseTime time.Duration) {
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

func (p *ICloudProvider) incrementMessageCount() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messageCount++
	p.health.TotalSent++
}

func (p *ICloudProvider) incrementErrorCount() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.errorCount++
	p.health.TotalFailed++
}

func (p *ICloudProvider) GetHealth() *ProviderHealth {
	p.mu.RLock()
	defer p.mu.RUnlock()

	health := *p.health
	return &health
}

func (p *ICloudProvider) GetStats() map[string]interface{} {
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
