package provider

import (
    "context"
    "crypto/tls"
    "errors"
    "fmt"
    "net"
    "net/smtp"
    "strings"
    "sync"
    "sync/atomic"
    "time"

    "email-campaign-system/internal/models"
    "email-campaign-system/pkg/logger"
)
type Message struct {
	From     string
	To       []string
	CC       []string
	BCC      []string
	Subject  string
	Body     string
	HTMLBody string
	Headers  map[string]string
}

var (
	ErrProviderClosed     = errors.New("provider is closed")
	ErrRateLimitExceeded  = errors.New("rate limit exceeded")
    ErrUnsupportedProvider  = errors.New("unsupported provider type")
    ErrFactoryInvalidConfig = errors.New("invalid provider configuration")
    ErrProviderNotFound     = errors.New("provider not found in registry")
)

type ProviderFactory struct {
    log            logger.Logger
    registry       map[ProviderType]ProviderConstructor
    defaultConfigs map[ProviderType]*ProviderConfig
    mu             sync.RWMutex
}

type ProviderConstructor func(config *ProviderConfig, log logger.Logger) (Provider, error)

func NewProviderFactory(log logger.Logger) *ProviderFactory {
    factory := &ProviderFactory{
        log:            log,
        registry:       make(map[ProviderType]ProviderConstructor),
        defaultConfigs: make(map[ProviderType]*ProviderConfig),
    }

    factory.registerDefaultProviders()
    factory.loadDefaultConfigs()

    return factory
}

func (f *ProviderFactory) registerDefaultProviders() {
    f.registry[ProviderTypeGmail] = NewSMTPProviderImpl
    f.registry[ProviderTypeSMTP] = NewSMTPProviderImpl
    f.registry[ProviderTypeYahoo] = NewSMTPProviderImpl
    f.registry[ProviderTypeOutlook] = NewSMTPProviderImpl
    f.registry[ProviderTypeHotmail] = NewSMTPProviderImpl
    f.registry[ProviderTypeICloud] = NewSMTPProviderImpl
    f.registry[ProviderTypeGoogleWorkspace] = NewSMTPProviderImpl
}

func (f *ProviderFactory) loadDefaultConfigs() {
    f.defaultConfigs[ProviderTypeGmail] = &ProviderConfig{
        Type: ProviderTypeGmail,
        Host: "smtp.gmail.com",
        Port: 587,
        TLSConfig: &TLSConfig{
            Enabled:            true,
            InsecureSkipVerify: false,
            MinVersion:         tls.VersionTLS12,
            ServerName:         "smtp.gmail.com",
        },
        TimeoutConfig: &TimeoutConfig{
            Connect: 30 * time.Second,
            Send:    60 * time.Second,
            Read:    30 * time.Second,
            Write:   30 * time.Second,
        },
		RetryConfig: &RetryConfig{
			MaxRetries: 3,
				InitialDelay: 1 * time.Second, 
				MaxDelay:     30 * time.Second,
				Multiplier: 2.0,
			},
        MaxMessageSize:   25 * 1024 * 1024,
        RateLimitPerDay:  500,
        RateLimitPerHour: 100,
        ConnectionPool: &ConnectionPoolConfig{
            MaxConnections: 5,
            MaxLifetime:    5 * time.Minute,
        },
    }

    f.defaultConfigs[ProviderTypeYahoo] = &ProviderConfig{
        Type: ProviderTypeYahoo,
        Host: "smtp.mail.yahoo.com",
        Port: 587,
        TLSConfig: &TLSConfig{
            Enabled:            true,
            InsecureSkipVerify: false,
            MinVersion:         tls.VersionTLS12,
            ServerName:         "smtp.mail.yahoo.com",
        },
        TimeoutConfig: &TimeoutConfig{
            Connect: 30 * time.Second,
            Send:    60 * time.Second,
            Read:    30 * time.Second,
            Write:   30 * time.Second,
        },
        RetryConfig: &RetryConfig{
            MaxRetries:      3,
            InitialDelay: 1 * time.Second,
            MaxDelay:     30 * time.Second,
            Multiplier:      2.0,
        },
        MaxMessageSize:   25 * 1024 * 1024,
        RateLimitPerDay:  500,
        RateLimitPerHour: 100,
        ConnectionPool: &ConnectionPoolConfig{
            MaxConnections: 5,
            MaxLifetime:    5 * time.Minute,
        },
    }

    f.defaultConfigs[ProviderTypeOutlook] = &ProviderConfig{
        Type: ProviderTypeOutlook,
        Host: "smtp-mail.outlook.com",
        Port: 587,
        TLSConfig: &TLSConfig{
            Enabled:            true,
            InsecureSkipVerify: false,
            MinVersion:         tls.VersionTLS12,
            ServerName:         "smtp-mail.outlook.com",
        },
        TimeoutConfig: &TimeoutConfig{
            Connect: 30 * time.Second,
            Send:    60 * time.Second,
            Read:    30 * time.Second,
            Write:   30 * time.Second,
        },
        RetryConfig: &RetryConfig{
            MaxRetries:      3,
            InitialDelay: 1 * time.Second,
            MaxDelay:     30 * time.Second,
            Multiplier:      2.0,
        },
        MaxMessageSize:   35 * 1024 * 1024,
        RateLimitPerDay:  300,
        RateLimitPerHour: 50,
        ConnectionPool: &ConnectionPoolConfig{
            MaxConnections: 5,
            MaxLifetime:    5 * time.Minute,
        },
    }

    f.defaultConfigs[ProviderTypeHotmail] = f.defaultConfigs[ProviderTypeOutlook]

    f.defaultConfigs[ProviderTypeICloud] = &ProviderConfig{
        Type: ProviderTypeICloud,
        Host: "smtp.mail.me.com",
        Port: 587,
        TLSConfig: &TLSConfig{
            Enabled:            true,
            InsecureSkipVerify: false,
            MinVersion:         tls.VersionTLS12,
            ServerName:         "smtp.mail.me.com",
        },
        TimeoutConfig: &TimeoutConfig{
            Connect: 30 * time.Second,
            Send:    60 * time.Second,
            Read:    30 * time.Second,
            Write:   30 * time.Second,
        },
        RetryConfig: &RetryConfig{
            MaxRetries:      3,
            InitialDelay: 1 * time.Second,
            MaxDelay:     30 * time.Second,
            Multiplier:      2.0,
        },
        MaxMessageSize:   20 * 1024 * 1024,
        RateLimitPerDay:  1000,
        RateLimitPerHour: 200,
        ConnectionPool: &ConnectionPoolConfig{
            MaxConnections: 5,
            MaxLifetime:    5 * time.Minute,
        },
    }

    f.defaultConfigs[ProviderTypeGoogleWorkspace] = &ProviderConfig{
        Type: ProviderTypeGoogleWorkspace,
        Host: "smtp.gmail.com",
        Port: 587,
        TLSConfig: &TLSConfig{
            Enabled:            true,
            InsecureSkipVerify: false,
            MinVersion:         tls.VersionTLS12,
            ServerName:         "smtp.gmail.com",
        },
        TimeoutConfig: &TimeoutConfig{
            Connect: 30 * time.Second,
            Send:    60 * time.Second,
            Read:    30 * time.Second,
            Write:   30 * time.Second,
        },
        RetryConfig: &RetryConfig{
            MaxRetries:      3,
            InitialDelay: 1 * time.Second,
            MaxDelay:     30 * time.Second,
            Multiplier:      2.0,
        },
        MaxMessageSize:   25 * 1024 * 1024,
        RateLimitPerDay:  2000,
        RateLimitPerHour: 500,
        ConnectionPool: &ConnectionPoolConfig{
            MaxConnections: 10,
            MaxLifetime:    5 * time.Minute,
        },
    }

    f.defaultConfigs[ProviderTypeSMTP] = &ProviderConfig{
        Type: ProviderTypeSMTP,
        Port: 587,
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
        RetryConfig: &RetryConfig{
            MaxRetries:      3,
            InitialDelay: 1 * time.Second,
            MaxDelay:     30 * time.Second,
            Multiplier:      2.0,
        },
        MaxMessageSize:   25 * 1024 * 1024,
        RateLimitPerDay:  500,
        RateLimitPerHour: 100,
        ConnectionPool: &ConnectionPoolConfig{
            MaxConnections: 5,
            MaxLifetime:    5 * time.Minute,
        },
    }
}

func (f *ProviderFactory) CreateProvider(ctx context.Context, account *models.Account) (Provider, error) {
    if account == nil {
        return nil, errors.New("account cannot be nil")
    }

    config := f.buildConfig(account)

    if err := f.validateConfig(config); err != nil {
        return nil, fmt.Errorf("invalid configuration: %w", err)
    }

    f.mu.RLock()
    constructor, exists := f.registry[config.Type]
    f.mu.RUnlock()

    if !exists {
        return nil, fmt.Errorf("%w: %s", ErrUnsupportedProvider, config.Type)
    }

    provider, err := constructor(config, f.log)
    if err != nil {
        return nil, fmt.Errorf("failed to create provider: %w", err)
    }

    f.log.Info(fmt.Sprintf("provider created: type=%s, username=%s, host=%s:%d",
        config.Type, config.Username, config.Host, config.Port))

    return provider, nil
}

func (f *ProviderFactory) buildConfig(account *models.Account) *ProviderConfig {
    providerType := ProviderType(account.Provider)
    config := f.getDefaultConfig(providerType)

    config.Username = account.Email

    if account.Credentials != nil {
        if account.Credentials.AppPassword != "" {
            config.Password = account.Credentials.AppPassword
        } else if account.Credentials.Password != "" {
            config.Password = account.Credentials.Password
        } else if account.AuthType == models.AuthTypeOAuth2 && account.Credentials.AccessToken != "" {
            config.Password = account.Credentials.AccessToken
        }
    }

    if account.SMTPConfig != nil {
        if account.SMTPConfig.Host != "" {
            config.Host = account.SMTPConfig.Host
            if config.TLSConfig != nil {
                config.TLSConfig.ServerName = account.SMTPConfig.Host
            }
        }
        if account.SMTPConfig.Port > 0 {
            config.Port = account.SMTPConfig.Port
        }
        if account.SMTPConfig.UseTLS {
            if config.TLSConfig == nil {
                config.TLSConfig = &TLSConfig{}
            }
            config.TLSConfig.Enabled = true
        }
        if account.SMTPConfig.UseSSL {
            config.Port = 465
            if config.TLSConfig == nil {
                config.TLSConfig = &TLSConfig{}
            }
            config.TLSConfig.Enabled = true
        }
    }

    if account.Limits.DailyLimit > 0 {
        config.RateLimitPerDay = account.Limits.DailyLimit
    }

    return config
}

func (f *ProviderFactory) getDefaultConfig(providerType ProviderType) *ProviderConfig {
    f.mu.RLock()
    defer f.mu.RUnlock()

    if defaultConfig, exists := f.defaultConfigs[providerType]; exists {
        config := *defaultConfig

        if defaultConfig.TLSConfig != nil {
            tlsCopy := *defaultConfig.TLSConfig
            config.TLSConfig = &tlsCopy
        }
        if defaultConfig.TimeoutConfig != nil {
            timeoutCopy := *defaultConfig.TimeoutConfig
            config.TimeoutConfig = &timeoutCopy
        }
        if defaultConfig.RetryConfig != nil {
            retryCopy := *defaultConfig.RetryConfig
            config.RetryConfig = &retryCopy
        }

        return &config
    }

    return &ProviderConfig{
        Type: providerType,
        Port: 587,
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
        RetryConfig: &RetryConfig{
            MaxRetries:      3,
            InitialDelay: 1 * time.Second,
            MaxDelay:     30 * time.Second,
            Multiplier:      2.0,
        },
        MaxMessageSize:   25 * 1024 * 1024,
        RateLimitPerDay:  500,
        RateLimitPerHour: 100,
        ConnectionPool: &ConnectionPoolConfig{
            MaxConnections: 5,
            MaxLifetime:    5 * time.Minute,
        },
    }
}

func (f *ProviderFactory) validateConfig(config *ProviderConfig) error {
    if config == nil {
        return ErrFactoryInvalidConfig
    }

    if config.Type == "" {
        return fmt.Errorf("%w: provider type is required", ErrFactoryInvalidConfig)
    }

    if config.Host == "" {
        return fmt.Errorf("%w: host is required", ErrFactoryInvalidConfig)
    }

    if config.Port <= 0 || config.Port > 65535 {
        return fmt.Errorf("%w: invalid port number %d", ErrFactoryInvalidConfig, config.Port)
    }

    if config.Username == "" {
        return fmt.Errorf("%w: username is required", ErrFactoryInvalidConfig)
    }

    if config.Password == "" {
        return fmt.Errorf("%w: password is required", ErrFactoryInvalidConfig)
    }

    return nil
}

func (f *ProviderFactory) RegisterProvider(providerType ProviderType, constructor ProviderConstructor) error {
    if constructor == nil {
        return errors.New("constructor cannot be nil")
    }

    f.mu.Lock()
    defer f.mu.Unlock()

    f.registry[providerType] = constructor

    f.log.Info(fmt.Sprintf("provider registered: type=%s", providerType))

    return nil
}

func (f *ProviderFactory) GetSupportedProviders() []ProviderType {
    f.mu.RLock()
    defer f.mu.RUnlock()

    providers := make([]ProviderType, 0, len(f.registry))
    for providerType := range f.registry {
        providers = append(providers, providerType)
    }

    return providers
}

func (f *ProviderFactory) GetDefaultConfig(providerType ProviderType) *ProviderConfig {
    return f.getDefaultConfig(providerType)
}

type factorySMTPProvider struct {
    config      *ProviderConfig
    log         logger.Logger
    pool        *factoryConnectionPool
    rateLimiter *factoryRateLimiter
    stats       *factoryProviderStats
    statsMu     sync.RWMutex
    closed      atomic.Bool
}

type factoryProviderStats struct {
    TotalSent     int64
    TotalFailed   int64
    LastSentAt    time.Time
    LastError     error
    LastErrorAt   time.Time
    BytesSent     int64
    TotalSendTime int64
}

func NewSMTPProviderImpl(config *ProviderConfig, log logger.Logger) (Provider, error) {
    if config == nil {
        return nil, errors.New("config cannot be nil")
    }

    provider := &factorySMTPProvider{
        config: config,
        log:    log,
        stats: &factoryProviderStats{
            TotalSent:   0,
            TotalFailed: 0,
            LastSentAt:  time.Time{},
        },
    }
    provider.closed.Store(false)

    pool, err := newFactoryConnectionPool(config, log)
    if err != nil {
        return nil, fmt.Errorf("failed to create connection pool: %w", err)
    }
    provider.pool = pool

    if config.RateLimitPerHour > 0 {
        provider.rateLimiter = newFactoryRateLimiter(config.RateLimitPerHour, time.Hour)
    } else if config.RateLimitPerDay > 0 {
        provider.rateLimiter = newFactoryRateLimiter(config.RateLimitPerDay, 24*time.Hour)
    }

    log.Info(fmt.Sprintf("SMTP provider initialized: host=%s, port=%d, type=%s",
        config.Host, config.Port, config.Type))

    return provider, nil
}

func (p *factorySMTPProvider) GetConfig() *ProviderConfig {
    return p.config
}
func (p *factorySMTPProvider) GetHealth() *ProviderHealth {  // Add * pointer
    p.statsMu.RLock()
    defer p.statsMu.RUnlock()
    
    health := ProviderHealth{
        LastCheck:    time.Now(),
        TotalSent:    p.stats.TotalSent,
        TotalFailed:  p.stats.TotalFailed,
        LastSuccess:  p.stats.LastSentAt,
        LastFailure:  p.stats.LastErrorAt,
        Message:      "operational",
        IsConnected:  !p.closed.Load(),
    }
    
    total := health.TotalSent + health.TotalFailed
    if total > 0 {
        health.ErrorRate = float64(health.TotalFailed) / float64(total) * 100
    }
    
    if p.closed.Load() {
        health.Status = HealthStatusUnhealthy
        health.Message = "provider closed"
    } else if health.ErrorRate > 50 {
        health.Status = HealthStatusUnhealthy
        health.Message = "high error rate"
    } else if health.ErrorRate > 20 {
        health.Status = HealthStatusDegraded
        health.Message = "elevated error rate"
    } else {
        health.Status = HealthStatusHealthy
    }
    
    if p.stats.TotalSent > 0 {
        health.AvgResponseTime = time.Duration(p.stats.TotalSendTime / p.stats.TotalSent)
    }
    
    return &health  // Return pointer
}

// Add after GetHealth() method (around line 540)

func (p *factorySMTPProvider) Name() string {
    return fmt.Sprintf("SMTP (%s)", p.config.Username)
}

func (p *factorySMTPProvider) Type() ProviderType {
    return p.config.Type
}

func (p *factorySMTPProvider) SupportedFeatures() []Feature {
    return []Feature{
        FeatureSMTP,
        FeatureTLS,
        FeatureSTARTTLS,
        FeatureAttachments,
        FeatureHTMLBody,
        FeatureTextBody,
        FeatureConnectionPool,
    }
}

func (p *factorySMTPProvider) Validate() error {
    if p.config == nil {
        return ErrFactoryInvalidConfig
    }

    if p.config.Username == "" {
        return fmt.Errorf("username is required")
    }

    if p.config.Password == "" {
        return fmt.Errorf("password is required")
    }

    if p.config.Host == "" {
        return fmt.Errorf("host is required")
    }

    if p.config.Port <= 0 || p.config.Port > 65535 {
        return fmt.Errorf("invalid port number %d", p.config.Port)
    }

    return nil
}

func (p *factorySMTPProvider) Send(ctx context.Context, message *models.Email) (string, error) {
    if p.closed.Load() {
        return "", ErrConnectionClosed
    }

    startTime := time.Now()

    if p.rateLimiter != nil {
        if !p.rateLimiter.Allow() {
            return "", ErrRateLimitExceeded
        }
    }

    if err := p.validateMessage(message); err != nil {
        p.recordFailure(err)
        return "", fmt.Errorf("message validation failed: %w", err)
    }

    rawMessage, err := p.buildRawMessage(message)
    if err != nil {
        p.recordFailure(err)
        return "", fmt.Errorf("failed to build message: %w", err)
    }

    messageSize := int64(len(rawMessage))
    if p.config.MaxMessageSize > 0 && messageSize > p.config.MaxMessageSize {
        err := fmt.Errorf("message size %d exceeds limit %d", messageSize, p.config.MaxMessageSize)
        p.recordFailure(err)
        return "", err
    }

    var lastErr error
    maxRetries := 1
    if p.config.RetryConfig != nil {
        maxRetries = p.config.RetryConfig.MaxRetries
        if maxRetries < 1 {
            maxRetries = 1
        }
    }

    for attempt := 0; attempt < maxRetries; attempt++ {
        if attempt > 0 {
            waitTime := p.calculateBackoff(attempt)
            p.log.Warn(fmt.Sprintf("retry attempt %d after %v", attempt+1, waitTime))

            select {
            case <-ctx.Done():
                return "", ctx.Err()
            case <-time.After(waitTime):
            }
        }

        err := p.sendMessage(ctx, message, rawMessage)
        if err == nil {
            duration := time.Since(startTime)
            p.recordSuccess(int(messageSize), duration)
            
            // Generate a message ID
            messageID := fmt.Sprintf("<%d.%s@%s>", 
                time.Now().UnixNano(), 
                p.config.Username, 
                p.config.Host)
            
            return messageID, nil
        }

        lastErr = err

        if !p.isRetryableError(err) {
            break
        }
    }

    p.recordFailure(lastErr)
    return "", fmt.Errorf("failed to send message after %d attempts: %w", maxRetries, lastErr)
}


func (p *factorySMTPProvider) sendMessage(ctx context.Context, message *models.Email, rawMessage []byte) error {
    conn, err := p.pool.Get(ctx)
    if err != nil {
        return fmt.Errorf("failed to get connection: %w", err)
    }
    defer p.pool.Put(conn)

    client := conn.Client
    if client == nil {
        return errors.New("client is nil")
    }

    if err := client.Reset(); err != nil {
        return fmt.Errorf("RSET command failed: %w", err)
    }

    // Use .Address field from models.EmailAddress
    if err := client.Mail(message.From.Address); err != nil {
        return fmt.Errorf("MAIL FROM failed: %w", err)
    }

    // To is models.EmailAddress with .Address field
    if err := client.Rcpt(message.To.Address); err != nil {
        return fmt.Errorf("RCPT TO failed for %s: %w", message.To.Address, err)
    }

    // CC is []models.EmailAddress
    for _, cc := range message.CC {
        if err := client.Rcpt(cc.Address); err != nil {
            return fmt.Errorf("RCPT TO failed for %s (CC): %w", cc.Address, err)
        }
    }

    // BCC is []models.EmailAddress
    for _, bcc := range message.BCC {
        if err := client.Rcpt(bcc.Address); err != nil {
            return fmt.Errorf("RCPT TO failed for %s (BCC): %w", bcc.Address, err)
        }
    }

    writer, err := client.Data()
    if err != nil {
        return fmt.Errorf("DATA command failed: %w", err)
    }

    _, err = writer.Write(rawMessage)
    if err != nil {
        writer.Close()
        return fmt.Errorf("failed to write message data: %w", err)
    }

    if err := writer.Close(); err != nil {
        return fmt.Errorf("failed to close data writer: %w", err)
    }

    return nil
}


func (p *factorySMTPProvider) buildRawMessage(message *models.Email) ([]byte, error) {
    var buffer []byte

    // From is models.EmailAddress
    buffer = append(buffer, []byte(fmt.Sprintf("From: %s\r\n", message.From.Address))...)
    
    // To is single models.EmailAddress (not a slice)
    buffer = append(buffer, []byte(fmt.Sprintf("To: %s\r\n", message.To.Address))...)

    // CC is []models.EmailAddress - need to convert to []string
    if len(message.CC) > 0 {
        ccAddrs := make([]string, len(message.CC))
        for i, cc := range message.CC {
            ccAddrs[i] = cc.Address
        }
        buffer = append(buffer, []byte(fmt.Sprintf("Cc: %s\r\n", strings.Join(ccAddrs, ", ")))...)
    }

    buffer = append(buffer, []byte(fmt.Sprintf("Subject: %s\r\n", message.Subject))...)
    buffer = append(buffer, []byte(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))...)
    buffer = append(buffer, []byte("MIME-Version: 1.0\r\n")...)

    for key, value := range message.Headers {
        buffer = append(buffer, []byte(fmt.Sprintf("%s: %s\r\n", key, value))...)
    }

    if message.HTMLBody != "" {
        buffer = append(buffer, []byte("Content-Type: text/html; charset=UTF-8\r\n")...)
    } else {
        buffer = append(buffer, []byte("Content-Type: text/plain; charset=UTF-8\r\n")...)
    }

    buffer = append(buffer, []byte("\r\n")...)

    if message.HTMLBody != "" {
        buffer = append(buffer, []byte(message.HTMLBody)...)
    } else {
        buffer = append(buffer, []byte(message.PlainTextBody)...)  // Use PlainTextBody
    }

    return buffer, nil
}

func (p *factorySMTPProvider) validateMessage(message *models.Email) error {
    if message == nil {
        return errors.New("message cannot be nil")
    }

    // From is models.EmailAddress with .Address field
    if message.From.Address == "" {
        return errors.New("from address is required")
    }

    // To is single models.EmailAddress (not a slice)
    if message.To.Address == "" {
        return errors.New("recipient address is required")
    }

    if message.Subject == "" {
        return errors.New("subject is required")
    }

    // Use PlainTextBody instead of Body
    if message.PlainTextBody == "" && message.HTMLBody == "" {
        return errors.New("message body is required")
    }

    return nil
}

func (p *factorySMTPProvider) calculateBackoff(attempt int) time.Duration {
    if p.config.RetryConfig == nil {
        return time.Second * time.Duration(attempt+1)
    }
    
    wait := float64(p.config.RetryConfig.InitialDelay) * p.config.RetryConfig.Multiplier * float64(attempt)
    if time.Duration(wait) > p.config.RetryConfig.MaxDelay {
        return p.config.RetryConfig.MaxDelay
    }
    return time.Duration(wait)
}

func (p *factorySMTPProvider) isRetryableError(err error) bool {
    if err == nil {
        return false
    }

    errStr := strings.ToLower(err.Error())
    retryableErrors := []string{
        "timeout",
        "connection refused",
        "connection reset",
        "temporary failure",
        "try again",
        "service not available",
        "450",
        "451",
        "452",
    }

    for _, retryable := range retryableErrors {
        if strings.Contains(errStr, retryable) {
            return true
        }
    }

    return false
}

func (p *factorySMTPProvider) recordSuccess(bytesSent int, duration time.Duration) {
    p.statsMu.Lock()
    defer p.statsMu.Unlock()

    p.stats.TotalSent++
    p.stats.LastSentAt = time.Now()
    p.stats.BytesSent += int64(bytesSent)
    p.stats.TotalSendTime += int64(duration)
}

func (p *factorySMTPProvider) recordFailure(err error) {
    p.statsMu.Lock()
    defer p.statsMu.Unlock()

    p.stats.TotalFailed++
    p.stats.LastError = err
    p.stats.LastErrorAt = time.Now()
}

func (p *factorySMTPProvider) TestConnection(ctx context.Context) error {
    if p.closed.Load() {
        return ErrConnectionClosed
    }

    conn, err := p.pool.Get(ctx)
    if err != nil {
        return fmt.Errorf("connection test failed: %w", err)
    }
    defer p.pool.Put(conn)

    if conn.Client == nil {
        return errors.New("client is nil")
    }

    if err := conn.Client.Noop(); err != nil {
        return fmt.Errorf("NOOP command failed: %w", err)
    }

    return nil
}

func (p *factorySMTPProvider) GetStats() interface{} {
    p.statsMu.RLock()
    defer p.statsMu.RUnlock()

    statsCopy := *p.stats
    var avgSendTime time.Duration
    if statsCopy.TotalSent > 0 {
        avgSendTime = time.Duration(statsCopy.TotalSendTime / statsCopy.TotalSent)
    }

    return &struct {
        TotalSent       int64
        TotalFailed     int64
        LastSentAt      time.Time
        LastError       error
        LastErrorAt     time.Time
        BytesSent       int64
        AverageSendTime time.Duration
    }{
        TotalSent:       statsCopy.TotalSent,
        TotalFailed:     statsCopy.TotalFailed,
        LastSentAt:      statsCopy.LastSentAt,
        LastError:       statsCopy.LastError,
        LastErrorAt:     statsCopy.LastErrorAt,
        BytesSent:       statsCopy.BytesSent,
        AverageSendTime: avgSendTime,
    }
}

func (p *factorySMTPProvider) Close() error {
    if p.closed.Swap(true) {
        return nil
    }

    if p.pool != nil {
        p.pool.Close()
    }

    p.log.Info(fmt.Sprintf("SMTP provider closed: type=%s", p.config.Type))

    return nil
}

type factoryConnectionPool struct {
    config        *ProviderConfig
    log           logger.Logger
    connections   chan *factorySMTPConnection
    activeCount   atomic.Int32
    mu            sync.Mutex
    closed        atomic.Bool
    cleanupTicker *time.Ticker
    cleanupDone   chan struct{}
}

type factorySMTPConnection struct {
    Client     *smtp.Client
    Conn       net.Conn
    CreatedAt  time.Time
    LastUsedAt time.Time
    UseCount   int
}

func newFactoryConnectionPool(config *ProviderConfig, log logger.Logger) (*factoryConnectionPool, error) {
    maxConns := 5
    if config.ConnectionPool != nil && config.ConnectionPool.MaxConnections > 0 {
        maxConns = config.ConnectionPool.MaxConnections
    }

    pool := &factoryConnectionPool{
        config:        config,
        log:           log,
        connections:   make(chan *factorySMTPConnection, maxConns),
        cleanupDone:   make(chan struct{}),
        cleanupTicker: time.NewTicker(1 * time.Minute),
    }
    pool.closed.Store(false)
    pool.activeCount.Store(0)

    go pool.cleanupRoutine()

    return pool, nil
}

func (p *factoryConnectionPool) cleanupRoutine() {
    for {
        select {
        case <-p.cleanupTicker.C:
            p.cleanupIdleConnections()
        case <-p.cleanupDone:
            p.cleanupTicker.Stop()
            return
        }
    }
}

func (p *factoryConnectionPool) cleanupIdleConnections() {
    if p.closed.Load() {
        return
    }

    for {
        select {
        case conn := <-p.connections:
            if !p.isConnectionValid(conn) {
                p.closeConnection(conn)
                p.activeCount.Add(-1)
            } else {
                select {
                case p.connections <- conn:
                default:
                    p.closeConnection(conn)
                    p.activeCount.Add(-1)
                }
            }
        default:
            return
        }
    }
}

func (p *factoryConnectionPool) Get(ctx context.Context) (*factorySMTPConnection, error) {
    if p.closed.Load() {
        return nil, errors.New("connection pool is closed")
    }

    select {
    case conn := <-p.connections:
        if p.isConnectionValid(conn) {
            conn.LastUsedAt = time.Now()
            conn.UseCount++
            return conn, nil
        }
        p.closeConnection(conn)
        p.activeCount.Add(-1)
    default:
    }

    maxConns := int32(p.config.ConnectionPool.MaxConnections)
    if maxConns <= 0 {
        maxConns = 5
    }

    currentCount := p.activeCount.Load()
    if currentCount >= maxConns {
        select {
        case conn := <-p.connections:
            if p.isConnectionValid(conn) {
                conn.LastUsedAt = time.Now()
                conn.UseCount++
                return conn, nil
            }
            p.closeConnection(conn)
            p.activeCount.Add(-1)
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-time.After(30 * time.Second):
            return nil, errors.New("connection pool exhausted")
        }
    }

    conn, err := p.createConnection(ctx)
    if err != nil {
        return nil, err
    }

    p.activeCount.Add(1)
    return conn, nil
}

func (p *factoryConnectionPool) Put(conn *factorySMTPConnection) {
    if conn == nil {
        return
    }

    if p.closed.Load() {
        p.closeConnection(conn)
        p.activeCount.Add(-1)
        return
    }

    if !p.isConnectionValid(conn) {
        p.closeConnection(conn)
        p.activeCount.Add(-1)
        return
    }

    select {
    case p.connections <- conn:
    default:
        p.closeConnection(conn)
        p.activeCount.Add(-1)
    }
}

func (p *factoryConnectionPool) createConnection(ctx context.Context) (*factorySMTPConnection, error) {
    address := fmt.Sprintf("%s:%d", p.config.Host, p.config.Port)

    timeout := 30 * time.Second
    if p.config.TimeoutConfig != nil {
        timeout = p.config.TimeoutConfig.Connect
    }

    dialer := &net.Dialer{
        Timeout: timeout,
    }

    useDirectTLS := p.config.Port == 465

    var conn net.Conn
    var err error

    if useDirectTLS {
        tlsConfig := &tls.Config{
            ServerName:         p.config.Host,
            InsecureSkipVerify: false,
            MinVersion:         tls.VersionTLS12,
        }
        if p.config.TLSConfig != nil {
            tlsConfig.InsecureSkipVerify = p.config.TLSConfig.InsecureSkipVerify
            tlsConfig.MinVersion = p.config.TLSConfig.MinVersion
        }

        conn, err = tls.DialWithDialer(dialer, "tcp", address, tlsConfig)
    } else {
        conn, err = dialer.DialContext(ctx, "tcp", address)
    }

    if err != nil {
        return nil, fmt.Errorf("connection failed: %w", err)
    }

    select {
    case <-ctx.Done():
        conn.Close()
        return nil, ctx.Err()
    default:
    }

    client, err := smtp.NewClient(conn, p.config.Host)
    if err != nil {
        conn.Close()
        return nil, fmt.Errorf("failed to create SMTP client: %w", err)
    }

    if !useDirectTLS && p.config.TLSConfig != nil && p.config.TLSConfig.Enabled {
        tlsConfig := &tls.Config{
            ServerName:         p.config.Host,
            InsecureSkipVerify: p.config.TLSConfig.InsecureSkipVerify,
            MinVersion:         p.config.TLSConfig.MinVersion,
        }

        if err := client.StartTLS(tlsConfig); err != nil {
            client.Close()
            conn.Close()
            return nil, fmt.Errorf("STARTTLS failed: %w", err)
        }
    }

    auth := smtp.PlainAuth("", p.config.Username, p.config.Password, p.config.Host)
    if err := client.Auth(auth); err != nil {
        client.Close()
        conn.Close()
        return nil, fmt.Errorf("authentication failed: %w", err)
    }

    smtpConn := &factorySMTPConnection{
        Client:     client,
        Conn:       conn,
        CreatedAt:  time.Now(),
        LastUsedAt: time.Now(),
        UseCount:   0,
    }

    return smtpConn, nil
}

func (p *factoryConnectionPool) isConnectionValid(conn *factorySMTPConnection) bool {
    if conn == nil || conn.Client == nil {
        return false
    }

    maxLifetime := p.config.ConnectionPool.MaxLifetime
    if maxLifetime <= 0 {
        maxLifetime = 5 * time.Minute
    }

    if time.Since(conn.CreatedAt) > maxLifetime {
        return false
    }

    maxIdleTime := 2 * time.Minute
    if time.Since(conn.LastUsedAt) > maxIdleTime {
        return false
    }

    if err := conn.Client.Noop(); err != nil {
        return false
    }

    return true
}

func (p *factoryConnectionPool) closeConnection(conn *factorySMTPConnection) {
    if conn == nil {
        return
    }

    if conn.Client != nil {
        conn.Client.Quit()
        conn.Client.Close()
    }

    if conn.Conn != nil {
        conn.Conn.Close()
    }
}

func (p *factoryConnectionPool) Close() {
    if p.closed.Swap(true) {
        return
    }

    close(p.cleanupDone)
    close(p.connections)

    for conn := range p.connections {
        p.closeConnection(conn)
    }
}

type factoryRateLimiter struct {
    limit    int
    window   time.Duration
    requests []time.Time
    mu       sync.Mutex
}

func newFactoryRateLimiter(limit int, window time.Duration) *factoryRateLimiter {
    return &factoryRateLimiter{
        limit:    limit,
        window:   window,
        requests: make([]time.Time, 0, limit+10),
    }
}

func (r *factoryRateLimiter) Allow() bool {
    r.mu.Lock()
    defer r.mu.Unlock()

    now := time.Now()
    cutoff := now.Add(-r.window)

    validCount := 0
    firstValidIdx := -1
    for i, reqTime := range r.requests {
        if reqTime.After(cutoff) {
            if firstValidIdx == -1 {
                firstValidIdx = i
            }
            validCount++
        }
    }

    if firstValidIdx > 0 {
        r.requests = r.requests[firstValidIdx:]
    } else if firstValidIdx == -1 {
        r.requests = r.requests[:0]
    }

    if len(r.requests) >= r.limit {
        return false
    }

    r.requests = append(r.requests, now)
    return true
}

func NewGmailProviderImpl(config *ProviderConfig, log logger.Logger) (Provider, error) {
    return NewSMTPProviderImpl(config, log)
}

func NewYahooProviderImpl(config *ProviderConfig, log logger.Logger) (Provider, error) {
    return NewSMTPProviderImpl(config, log)
}

func NewOutlookProviderImpl(config *ProviderConfig, log logger.Logger) (Provider, error) {
    return NewSMTPProviderImpl(config, log)
}

func NewHotmailProviderImpl(config *ProviderConfig, log logger.Logger) (Provider, error) {
    return NewSMTPProviderImpl(config, log)
}

func NewICloudProviderImpl(config *ProviderConfig, log logger.Logger) (Provider, error) {
    return NewSMTPProviderImpl(config, log)
}

func NewGoogleWorkspaceProviderImpl(config *ProviderConfig, log logger.Logger) (Provider, error) {
    return NewSMTPProviderImpl(config, log)
}

func NewProvider(config *ProviderConfig, log logger.Logger) (Provider, error) {
    fmt.Printf("🟠 NewProvider called: type=%q (%T)\n", config.Type, config.Type)
    return NewSMTPProviderImpl(config, log)
}
