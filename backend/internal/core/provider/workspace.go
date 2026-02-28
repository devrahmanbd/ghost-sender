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
	WorkspaceSMTPHost         = "smtp.gmail.com"
	WorkspaceSMTPPort         = 587
	WorkspaceMaxMessageSize   = 35 * 1024 * 1024
	WorkspaceDefaultDailyLim  = 2000
	WorkspaceDefaultHourlyLim = 500
)

var (
	ErrWorkspaceAuthFailed       = errors.New("workspace authentication failed")
	ErrWorkspaceRateLimitHit     = errors.New("workspace rate limit exceeded")
	ErrWorkspaceMessageRejected  = errors.New("workspace message rejected")
	ErrWorkspaceTemporaryFailure = errors.New("workspace temporary failure")
	ErrWorkspaceInvalidAppPass   = errors.New("workspace app password invalid or not configured")
	ErrWorkspaceAccountSuspended = errors.New("workspace account suspended")
)

type WorkspaceProvider struct {
	config       *ProviderConfig
	log          logger.Logger
	smtpProvider Provider
	mu           sync.RWMutex
	health       *ProviderHealth
	rateLimiter  *WorkspaceRateLimiter
	lastSend     time.Time
	messageCount int64
	errorCount   int64
	retryConfig  *WorkspaceRetryConfig
	domain       string
}

type WorkspaceRateLimiter struct {
	hourlyLimit   int
	dailyLimit    int
	hourlySent    int
	dailySent     int
	lastHourReset time.Time
	lastDayReset  time.Time
	mu            sync.Mutex
}

type WorkspaceRetryConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	BackoffFactor  float64
}

func NewWorkspaceProvider(config *ProviderConfig, log logger.Logger) (Provider, error) {
	if config == nil {
		return nil, ErrInvalidConfig
	}

	workspaceConfig := buildWorkspaceConfig(config)

	smtpProvider, err := NewSMTPProviderImpl(workspaceConfig, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace smtp provider: %w", err)
	}

	domain := extractDomain(config.Username)

	provider := &WorkspaceProvider{
		config:       workspaceConfig,
		log:          log,
		smtpProvider: smtpProvider,
		health: &ProviderHealth{
			Status:    HealthStatusUnknown,
			LastCheck: time.Now(),
		},
		rateLimiter: &WorkspaceRateLimiter{
			hourlyLimit:   workspaceConfig.RateLimitPerHour,
			dailyLimit:    workspaceConfig.RateLimitPerDay,
			lastHourReset: time.Now(),
			lastDayReset:  time.Now(),
		},
		lastSend: time.Now(),
		domain:   domain,
		retryConfig: &WorkspaceRetryConfig{
			MaxRetries:     3,
			InitialBackoff: 2 * time.Second,
			MaxBackoff:     30 * time.Second,
			BackoffFactor:  2.0,
		},
	}

	return provider, nil
}

func buildWorkspaceConfig(config *ProviderConfig) *ProviderConfig {
	workspaceConfig := &ProviderConfig{
		Type:     ProviderTypeGoogleWorkspace,
		Host:     WorkspaceSMTPHost,
		Port:     WorkspaceSMTPPort,
		Username: config.Username,
		Password: config.Password,
		TLSConfig: &TLSConfig{
			Enabled:            true,
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
			ServerName:         WorkspaceSMTPHost,
		},
		TimeoutConfig: &TimeoutConfig{
			Connect: 30 * time.Second,
			Send:    90 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
		},
		MaxMessageSize:   WorkspaceMaxMessageSize,
		RateLimitPerDay:  WorkspaceDefaultDailyLim,
		RateLimitPerHour: WorkspaceDefaultHourlyLim,
	}

	if config.RateLimitPerDay > 0 {
		workspaceConfig.RateLimitPerDay = config.RateLimitPerDay
	}

	if config.RateLimitPerHour > 0 {
		workspaceConfig.RateLimitPerHour = config.RateLimitPerHour
	}

	if config.ConnectionPool != nil {
		workspaceConfig.ConnectionPool = config.ConnectionPool
	}

	return workspaceConfig
}

func extractDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

func (p *WorkspaceProvider) Send(ctx context.Context, message *models.Email) (string, error) {
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
				(p.log).Debug(fmt.Sprintf("retrying workspace send: attempt=%d, backoff=%v, domain=%s", attempt, backoff, p.domain))
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
				(p.log).Debug(fmt.Sprintf("email sent via workspace: to=%s, domain=%s, duration=%v, attempt=%d", message.To.Address, p.domain, time.Since(startTime), attempt+1))
			}

			return messageID, nil
		}

		lastErr = err
		p.incrementErrorCount()
		p.updateHealth(false, time.Since(startTime))

		categorizedErr := p.categorizeError(err)

		if categorizedErr == ErrWorkspaceAccountSuspended || categorizedErr == ErrWorkspaceInvalidAppPass {
			if p.log != nil {
				(p.log).Error(fmt.Sprintf("non-recoverable workspace error: error=%v, username=%s, domain=%s", categorizedErr, p.config.Username, p.domain))
			}
			return "", categorizedErr
		}

		if !p.isRetryableError(err) {
			if p.log != nil {
				(p.log).Error(fmt.Sprintf("non-retryable workspace error: error=%v, to=%s, domain=%s", err, message.To.Address, p.domain))
			}
			return "", categorizedErr
		}

		if p.log != nil {
			(p.log).Warn(fmt.Sprintf("retryable workspace error: error=%v, attempt=%d, max_retries=%d, domain=%s", err, attempt+1, p.retryConfig.MaxRetries, p.domain))
		}
	}

	return "", fmt.Errorf("workspace send failed after %d attempts: %w", p.retryConfig.MaxRetries+1, lastErr)
}

func (p *WorkspaceProvider) validateMessage(message *models.Email) error {
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
		return fmt.Errorf("message size %d exceeds workspace limit %d", totalSize, p.config.MaxMessageSize)
	}

	return nil
}

func (p *WorkspaceProvider) isRetryableError(err error) bool {
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
		"invalid credentials",
		"app password",
		"suspended",
		"disabled",
		"invalid username",
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

func (p *WorkspaceProvider) categorizeError(err error) error {
	if err == nil {
		return nil
	}

	errStr := strings.ToLower(err.Error())

	if strings.Contains(errStr, "app password") || strings.Contains(errStr, "application-specific") {
		return ErrWorkspaceInvalidAppPass
	}

	if strings.Contains(errStr, "suspended") || strings.Contains(errStr, "disabled") ||
		strings.Contains(errStr, "locked") {
		return ErrWorkspaceAccountSuspended
	}

	if strings.Contains(errStr, "authentication") || strings.Contains(errStr, "auth") ||
		strings.Contains(errStr, "invalid credentials") || strings.Contains(errStr, "535") {
		return ErrWorkspaceAuthFailed
	}

	if strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "too many") ||
		strings.Contains(errStr, "quota exceeded") || strings.Contains(errStr, "sending limit") {
		return ErrWorkspaceRateLimitHit
	}

	if strings.Contains(errStr, "rejected") || strings.Contains(errStr, "blocked") ||
		strings.Contains(errStr, "spam") || strings.Contains(errStr, "550") ||
		strings.Contains(errStr, "551") {
		return ErrWorkspaceMessageRejected
	}

	if strings.Contains(errStr, "temporary") || strings.Contains(errStr, "421") ||
		strings.Contains(errStr, "450") || strings.Contains(errStr, "451") ||
		strings.Contains(errStr, "452") {
		return ErrWorkspaceTemporaryFailure
	}

	return err
}

func (p *WorkspaceProvider) TestConnection(ctx context.Context) error {
	if p.log != nil {
		(p.log).Debug(fmt.Sprintf("testing workspace connection: domain=%s", p.domain))
	}

	if p.smtpProvider == nil {
		return errors.New("workspace smtp provider not initialized")
	}

	startTime := time.Now()
	err := p.smtpProvider.TestConnection(ctx)

	if err != nil {
		p.updateHealth(false, time.Since(startTime))
		return fmt.Errorf("workspace connection test failed: %w", err)
	}

	p.updateHealth(true, time.Since(startTime))
	if p.log != nil {
		(p.log).Info(fmt.Sprintf("workspace connection test successful: domain=%s", p.domain))
	}

	return nil
}

func (p *WorkspaceProvider) Validate() error {
	if p.config == nil {
		return ErrInvalidConfig
	}

	if p.config.Username == "" {
		return fmt.Errorf("workspace username is required")
	}

	if p.config.Password == "" {
		return fmt.Errorf("workspace app password is required")
	}

	if !strings.Contains(p.config.Username, "@") {
		return fmt.Errorf("invalid workspace email format")
	}

	emailLower := strings.ToLower(p.config.Username)
	if strings.Contains(emailLower, "@gmail.com") {
		return fmt.Errorf("use regular gmail provider for @gmail.com addresses")
	}

	return nil
}

func (p *WorkspaceProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.smtpProvider != nil {
		if err := p.smtpProvider.Close(); err != nil {
			if p.log != nil {
				(p.log).Error(fmt.Sprintf("error closing workspace smtp provider: %v", err))
			}
		}
	}

	p.health.IsConnected = false
	if p.log != nil {
		(p.log).Info(fmt.Sprintf("workspace provider closed: domain=%s", p.domain))
	}

	return nil
}

func (p *WorkspaceProvider) Name() string {
	return fmt.Sprintf("Google Workspace (%s)", p.config.Username)
}

func (p *WorkspaceProvider) Type() ProviderType {
	return ProviderTypeGoogleWorkspace
}

func (p *WorkspaceProvider) SupportedFeatures() []Feature {
	return []Feature{
		FeatureTLS,
		FeatureSTARTTLS,
		FeatureHTMLBody,
		FeatureTextBody,
		FeatureAttachments,
		FeatureConnectionPool,
	}
}

func (p *WorkspaceProvider) GetConfig() *ProviderConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config
}

func (p *WorkspaceProvider) checkRateLimit() error {
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
		return fmt.Errorf("%w: hourly limit %d/%d", ErrWorkspaceRateLimitHit,
			p.rateLimiter.hourlySent, p.rateLimiter.hourlyLimit)
	}

	if p.rateLimiter.dailySent >= p.rateLimiter.dailyLimit {
		return fmt.Errorf("%w: daily limit %d/%d", ErrWorkspaceRateLimitHit,
			p.rateLimiter.dailySent, p.rateLimiter.dailyLimit)
	}

	p.rateLimiter.hourlySent++
	p.rateLimiter.dailySent++

	return nil
}

func (p *WorkspaceProvider) updateHealth(success bool, responseTime time.Duration) {
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

func (p *WorkspaceProvider) incrementMessageCount() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messageCount++
	p.health.TotalSent++
}

func (p *WorkspaceProvider) incrementErrorCount() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.errorCount++
	p.health.TotalFailed++
}

func (p *WorkspaceProvider) GetHealth() *ProviderHealth {
	p.mu.RLock()
	defer p.mu.RUnlock()

	health := *p.health
	return &health
}

func (p *WorkspaceProvider) GetStats() map[string]interface{} {
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
		"domain":            p.domain,
	}
}

func (p *WorkspaceProvider) GetDomain() string {
	return p.domain
}
