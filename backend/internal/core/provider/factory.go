package provider

import (
    "bytes"
    "context"
    "crypto/rand"
    "crypto/tls"
    "encoding/base64"
    "encoding/hex"
    "errors"
    "fmt"
    "mime/quotedprintable"
    "net"
    "net/smtp"
    "regexp"
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
    f.registry[ProviderTypeOffice365] = NewSMTPProviderImpl
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

    f.defaultConfigs[ProviderTypeOffice365] = &ProviderConfig{
        Type: ProviderTypeOffice365,
        Host: "smtp.office365.com",
        Port: 587,
        TLSConfig: &TLSConfig{
            Enabled:            true,
            InsecureSkipVerify: false,
            MinVersion:         tls.VersionTLS12,
            ServerName:         "smtp.office365.com",
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
        MaxMessageSize:   35 * 1024 * 1024,
        RateLimitPerDay:  10000,
        RateLimitPerHour: 1000,
        ConnectionPool: &ConnectionPoolConfig{
            MaxConnections: 5,
            MaxLifetime:    5 * time.Minute,
        },
    }

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
    proxyConfig *ProxyConfig
    proxyMu     sync.RWMutex
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


func (p *factorySMTPProvider) SetProxy(proxy *ProxyConfig) {
    p.proxyMu.Lock()
    defer p.proxyMu.Unlock()
    p.proxyConfig = proxy
}

func (p *factorySMTPProvider) getProxy() *ProxyConfig {
    p.proxyMu.RLock()
    defer p.proxyMu.RUnlock()
    return p.proxyConfig
}

func (p *factorySMTPProvider) sendMessage(ctx context.Context, message *models.Email, rawMessage []byte) error {
    pc := p.getProxy()
    if pc != nil && pc.Enabled {
        return p.sendMessageViaProxy(ctx, message, rawMessage, pc)
    }

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

func (p *factorySMTPProvider) sendMessageViaProxy(ctx context.Context, message *models.Email, rawMessage []byte, pc *ProxyConfig) error {
    address := fmt.Sprintf("%s:%d", p.config.Host, p.config.Port)
    proxyAddr := fmt.Sprintf("%s:%d", pc.Host, pc.Port)

    timeout := 30 * time.Second
    if p.config.TimeoutConfig != nil {
        timeout = p.config.TimeoutConfig.Connect
    }

    dialer := &net.Dialer{Timeout: timeout}
    rawConn, err := dialer.DialContext(ctx, "tcp", proxyAddr)
    if err != nil {
        return fmt.Errorf("proxy connect failed: %w", err)
    }

    switch strings.ToLower(pc.Type) {
    case "socks5":
        if err := socks5Connect(rawConn, address, pc.Username, pc.Password); err != nil {
            rawConn.Close()
            return fmt.Errorf("socks5 proxy handshake failed: %w", err)
        }
    default:
        connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", address, address)
        if pc.Username != "" {
            creds := pc.Username + ":" + pc.Password
            encoded := base64Encode(creds)
            connectReq += fmt.Sprintf("Proxy-Authorization: Basic %s\r\n", encoded)
        }
        connectReq += "\r\n"
        if _, err := rawConn.Write([]byte(connectReq)); err != nil {
            rawConn.Close()
            return fmt.Errorf("proxy CONNECT write failed: %w", err)
        }
        buf := make([]byte, 4096)
        n, err := rawConn.Read(buf)
        if err != nil {
            rawConn.Close()
            return fmt.Errorf("proxy CONNECT read failed: %w", err)
        }
        if !strings.Contains(string(buf[:n]), "200") {
            rawConn.Close()
            return fmt.Errorf("proxy CONNECT rejected: %s", strings.TrimSpace(string(buf[:n])))
        }
    }

    var smtpConn net.Conn = rawConn
    useDirectTLS := p.config.Port == 465

    if useDirectTLS {
        tlsCfg := &tls.Config{
            ServerName:         p.config.Host,
            InsecureSkipVerify: false,
            MinVersion:         tls.VersionTLS12,
        }
        if p.config.TLSConfig != nil {
            tlsCfg.InsecureSkipVerify = p.config.TLSConfig.InsecureSkipVerify
        }
        smtpConn = tls.Client(rawConn, tlsCfg)
    }

    client, err := smtp.NewClient(smtpConn, p.config.Host)
    if err != nil {
        smtpConn.Close()
        return fmt.Errorf("failed to create SMTP client via proxy: %w", err)
    }
    defer func() {
        client.Quit()
        client.Close()
    }()

    if !useDirectTLS {
        tlsCfg := &tls.Config{
            ServerName:         p.config.Host,
            InsecureSkipVerify: false,
            MinVersion:         tls.VersionTLS12,
        }
        if p.config.TLSConfig != nil {
            tlsCfg.InsecureSkipVerify = p.config.TLSConfig.InsecureSkipVerify
        }

        // Check STARTTLS capability before attempting
        if ok, _ := client.Extension("STARTTLS"); ok {
            if err := client.StartTLS(tlsCfg); err != nil {
                return fmt.Errorf("STARTTLS via proxy failed: %w", err)
            }
        } else if p.config.TLSConfig != nil && p.config.TLSConfig.Enabled {
            return fmt.Errorf("server %s does not support STARTTLS via proxy", p.config.Host)
        }
    }

    // Authenticate only after TLS — try PLAIN then LOGIN
    if p.config.Password != "" {
        if err := p.pool.authenticate(client); err != nil {
            return fmt.Errorf("auth via proxy failed: %w", err)
        }
    }

    if err := client.Mail(message.From.Address); err != nil {
        return fmt.Errorf("MAIL FROM via proxy failed: %w", err)
    }

    if err := client.Rcpt(message.To.Address); err != nil {
        return fmt.Errorf("RCPT TO via proxy failed: %w", err)
    }

    writer, err := client.Data()
    if err != nil {
        return fmt.Errorf("DATA via proxy failed: %w", err)
    }

    if _, err := writer.Write(rawMessage); err != nil {
        writer.Close()
        return fmt.Errorf("write via proxy failed: %w", err)
    }

    return writer.Close()
}

func socks5Connect(conn net.Conn, targetAddr, user, pass string) error {
    hasAuth := user != ""
    var methods []byte
    if hasAuth {
        methods = []byte{0x05, 0x02, 0x00, 0x02}
    } else {
        methods = []byte{0x05, 0x01, 0x00}
    }
    if _, err := conn.Write(methods); err != nil {
        return err
    }
    resp := make([]byte, 2)
    if _, err := conn.Read(resp); err != nil {
        return err
    }
    if resp[0] != 0x05 {
        return fmt.Errorf("invalid socks version: %d", resp[0])
    }
    if resp[1] == 0x02 && hasAuth {
        authReq := []byte{0x01, byte(len(user))}
        authReq = append(authReq, []byte(user)...)
        authReq = append(authReq, byte(len(pass)))
        authReq = append(authReq, []byte(pass)...)
        if _, err := conn.Write(authReq); err != nil {
            return err
        }
        authResp := make([]byte, 2)
        if _, err := conn.Read(authResp); err != nil {
            return err
        }
        if authResp[1] != 0x00 {
            return fmt.Errorf("socks5 auth rejected")
        }
    }
    host, portStr, err := net.SplitHostPort(targetAddr)
    if err != nil {
        return err
    }
    port := 0
    fmt.Sscanf(portStr, "%d", &port)
    req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}
    req = append(req, []byte(host)...)
    req = append(req, byte(port>>8), byte(port&0xff))
    if _, err := conn.Write(req); err != nil {
        return err
    }
    connResp := make([]byte, 256)
    if _, err := conn.Read(connResp); err != nil {
        return err
    }
    if connResp[1] != 0x00 {
        return fmt.Errorf("socks5 connect failed: status %d", connResp[1])
    }
    return nil
}

func base64Encode(s string) string {
    return base64.StdEncoding.EncodeToString([]byte(s))
}

func (p *factorySMTPProvider) recipientProvider(toAddress string) string {
    parts := strings.SplitN(toAddress, "@", 2)
    if len(parts) != 2 {
        return "generic"
    }
    domain := strings.ToLower(parts[1])
    switch domain {
    case "gmail.com", "googlemail.com":
        return "gmail"
    case "yahoo.com", "ymail.com", "yahoo.co.uk", "yahoo.co.in", "yahoo.ca":
        return "yahoo"
    case "hotmail.com", "outlook.com", "live.com", "msn.com", "hotmail.co.uk", "outlook.co.uk":
        return "outlook"
    case "icloud.com", "me.com", "mac.com":
        return "icloud"
    default:
        return "generic"
    }
}

func (p *factorySMTPProvider) buildRawMessage(message *models.Email) ([]byte, error) {
    var buf bytes.Buffer

    senderDomain := p.extractDomain(message.From.Address)
    recipientProv := p.recipientProvider(message.To.Address)

    randomBytes := make([]byte, 16)
    rand.Read(randomBytes)
    uniqueID := hex.EncodeToString(randomBytes)
    messageID := fmt.Sprintf("<%s.%d@%s>", uniqueID, time.Now().Unix(), senderDomain)

    if message.From.Name != "" {
        buf.WriteString(fmt.Sprintf("From: \"%s\" <%s>\r\n", message.From.Name, message.From.Address))
    } else {
        buf.WriteString(fmt.Sprintf("From: %s\r\n", message.From.Address))
    }

    if message.To.Name != "" && message.To.Name != message.To.Address {
        buf.WriteString(fmt.Sprintf("To: \"%s\" <%s>\r\n", message.To.Name, message.To.Address))
    } else {
        buf.WriteString(fmt.Sprintf("To: <%s>\r\n", message.To.Address))
    }

    if len(message.CC) > 0 {
        ccParts := make([]string, len(message.CC))
        for i, cc := range message.CC {
            if cc.Name != "" {
                ccParts[i] = fmt.Sprintf("\"%s\" <%s>", cc.Name, cc.Address)
            } else {
                ccParts[i] = cc.Address
            }
        }
        buf.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(ccParts, ", ")))
    }

    buf.WriteString(fmt.Sprintf("Subject: %s\r\n", message.Subject))
    buf.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
    buf.WriteString(fmt.Sprintf("Message-ID: %s\r\n", messageID))
    buf.WriteString("MIME-Version: 1.0\r\n")

    if message.ReplyTo != nil && message.ReplyTo.Address != "" {
        if message.ReplyTo.Name != "" {
            buf.WriteString(fmt.Sprintf("Reply-To: \"%s\" <%s>\r\n", message.ReplyTo.Name, message.ReplyTo.Address))
        } else {
            buf.WriteString(fmt.Sprintf("Reply-To: <%s>\r\n", message.ReplyTo.Address))
        }
    }

    unsubEmail := message.From.Address
    unsubURL := fmt.Sprintf("https://%s/unsubscribe?id=%s", senderDomain, uniqueID)
    buf.WriteString(fmt.Sprintf("List-Unsubscribe: <mailto:%s?subject=unsubscribe>, <%s>\r\n", unsubEmail, unsubURL))
    buf.WriteString("List-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n")

    buf.WriteString("Auto-Submitted: no\r\n")
    buf.WriteString("X-Auto-Response-Suppress: All\r\n")

    switch recipientProv {
    case "gmail":
        fbID := fmt.Sprintf("%s:%s:%s:%s", senderDomain, "campaign", uniqueID[:8], message.From.Address)
        buf.WriteString(fmt.Sprintf("Feedback-ID: %s\r\n", fbID))
        buf.WriteString(fmt.Sprintf("X-Entity-Ref-ID: %s\r\n", uniqueID))
    case "outlook":
        buf.WriteString(fmt.Sprintf("X-Entity-Ref-ID: %s\r\n", uniqueID))
    case "yahoo":
        buf.WriteString(fmt.Sprintf("X-Entity-Ref-ID: %s\r\n", uniqueID))
    case "icloud":
        buf.WriteString(fmt.Sprintf("X-Entity-Ref-ID: %s\r\n", uniqueID))
    }

    skippedHeaders := map[string]bool{
        "x-mailer": true, "user-agent": true, "x-priority": true,
        "importance": true, "list-unsubscribe": true, "reply-to": true,
        "list-unsubscribe-post": true, "auto-submitted": true,
        "x-auto-response-suppress": true, "feedback-id": true,
        "x-entity-ref-id": true, "return-path": true, "precedence": true,
        "x-bulk-mail": true, "x-mass-mail": true,
    }
    for key, value := range message.Headers {
        if skippedHeaders[strings.ToLower(key)] {
            continue
        }
        buf.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
    }

    plainText := message.PlainTextBody
    htmlBody := message.HTMLBody

    if htmlBody != "" && plainText == "" {
        plainText = stripHTML(htmlBody)
    }

    hasAttachments := len(message.Attachments) > 0
    fmt.Printf("[RAWMSG-DEBUG] buildRawMessage: attachments=%d, to=%s\n", len(message.Attachments), message.To.Address)

    if hasAttachments {
        mixedBoundary := fmt.Sprintf("----=_Mixed_%s_%d", uniqueID[:16], time.Now().UnixNano())
        buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed;\r\n boundary=\"%s\"\r\n", mixedBoundary))
        buf.WriteString("\r\n")

        buf.WriteString(fmt.Sprintf("--%s\r\n", mixedBoundary))
        p.writeBodyPart(&buf, uniqueID, plainText, htmlBody)

        for i, att := range message.Attachments {
            fmt.Printf("[RAWMSG-DEBUG] writing attachment[%d]: filename=%s, contentType=%s, contentLen=%d\n",
                i, att.Filename, att.ContentType, len(att.Content))
            buf.WriteString(fmt.Sprintf("--%s\r\n", mixedBoundary))
            buf.WriteString(fmt.Sprintf("Content-Type: %s; name=\"%s\"\r\n", att.ContentType, att.Filename))
            buf.WriteString("Content-Transfer-Encoding: base64\r\n")
            buf.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n", att.Filename))
            buf.WriteString("\r\n")
            encoded := base64.StdEncoding.EncodeToString(att.Content)
            for j := 0; j < len(encoded); j += 76 {
                end := j + 76
                if end > len(encoded) {
                    end = len(encoded)
                }
                buf.WriteString(encoded[j:end])
                buf.WriteString("\r\n")
            }
        }

        buf.WriteString(fmt.Sprintf("--%s--\r\n", mixedBoundary))
    } else {
        p.writeBodyPart(&buf, uniqueID, plainText, htmlBody)
    }

    return buf.Bytes(), nil
}

func (p *factorySMTPProvider) writeBodyPart(buf *bytes.Buffer, uniqueID string, plainText string, htmlBody string) {
    if htmlBody != "" && plainText != "" {
        boundary := fmt.Sprintf("----=_Part_%s_%d", uniqueID[:16], time.Now().Unix())
        buf.WriteString(fmt.Sprintf("Content-Type: multipart/alternative;\r\n boundary=\"%s\"\r\n", boundary))
        buf.WriteString("\r\n")

        buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
        buf.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
        buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
        buf.WriteString("\r\n")
        var qpPlain bytes.Buffer
        qpPlainWriter := quotedprintable.NewWriter(&qpPlain)
        qpPlainWriter.Write([]byte(plainText))
        qpPlainWriter.Close()
        buf.Write(qpPlain.Bytes())
        buf.WriteString("\r\n")

        buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
        buf.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
        buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
        buf.WriteString("\r\n")
        var qpBuf bytes.Buffer
        qpWriter := quotedprintable.NewWriter(&qpBuf)
        qpWriter.Write([]byte(htmlBody))
        qpWriter.Close()
        buf.Write(qpBuf.Bytes())
        buf.WriteString("\r\n")

        buf.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
    } else if htmlBody != "" {
        buf.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
        buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
        buf.WriteString("\r\n")
        var qpBuf bytes.Buffer
        qpWriter := quotedprintable.NewWriter(&qpBuf)
        qpWriter.Write([]byte(htmlBody))
        qpWriter.Close()
        buf.Write(qpBuf.Bytes())
    } else {
        buf.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
        buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
        buf.WriteString("\r\n")
        var qpPlain bytes.Buffer
        qpPlainWriter := quotedprintable.NewWriter(&qpPlain)
        qpPlainWriter.Write([]byte(plainText))
        qpPlainWriter.Close()
        buf.Write(qpPlain.Bytes())
    }
}

func (p *factorySMTPProvider) extractDomain(email string) string {
    parts := strings.SplitN(email, "@", 2)
    if len(parts) == 2 && parts[1] != "" {
        return parts[1]
    }
    return p.config.Host
}

var htmlTagRegexp = regexp.MustCompile(`<[^>]*>`)
var htmlEntityRegexp = regexp.MustCompile(`&[a-zA-Z]+;|&#[0-9]+;`)
var multiNewlineRegexp = regexp.MustCompile(`\n{3,}`)
var multiSpaceRegexp = regexp.MustCompile(`[ \t]+`)

func stripHTML(html string) string {
    text := strings.ReplaceAll(html, "<br>", "\n")
    text = strings.ReplaceAll(text, "<br/>", "\n")
    text = strings.ReplaceAll(text, "<br />", "\n")
    text = strings.ReplaceAll(text, "</p>", "\n\n")
    text = strings.ReplaceAll(text, "</div>", "\n")
    text = strings.ReplaceAll(text, "</li>", "\n")
    text = strings.ReplaceAll(text, "</tr>", "\n")
    text = strings.ReplaceAll(text, "</h1>", "\n\n")
    text = strings.ReplaceAll(text, "</h2>", "\n\n")
    text = strings.ReplaceAll(text, "</h3>", "\n\n")
    text = strings.ReplaceAll(text, "</h4>", "\n\n")
    text = strings.ReplaceAll(text, "</h5>", "\n\n")
    text = strings.ReplaceAll(text, "</h6>", "\n\n")

    text = htmlTagRegexp.ReplaceAllString(text, "")

    text = strings.ReplaceAll(text, "&amp;", "&")
    text = strings.ReplaceAll(text, "&lt;", "<")
    text = strings.ReplaceAll(text, "&gt;", ">")
    text = strings.ReplaceAll(text, "&quot;", "\"")
    text = strings.ReplaceAll(text, "&apos;", "'")
    text = strings.ReplaceAll(text, "&nbsp;", " ")
    text = htmlEntityRegexp.ReplaceAllString(text, "")

    text = multiSpaceRegexp.ReplaceAllString(text, " ")
    text = multiNewlineRegexp.ReplaceAllString(text, "\n\n")
    text = strings.TrimSpace(text)

    return text
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

    var maxConns int32 = 5
    if p.config != nil && p.config.ConnectionPool != nil && p.config.ConnectionPool.MaxConnections > 0 {
        maxConns = int32(p.config.ConnectionPool.MaxConnections)
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

    if !useDirectTLS {
        tlsConfig := &tls.Config{
            ServerName:         p.config.Host,
            InsecureSkipVerify: false,
            MinVersion:         tls.VersionTLS12,
        }
        if p.config.TLSConfig != nil {
            tlsConfig.InsecureSkipVerify = p.config.TLSConfig.InsecureSkipVerify
            if p.config.TLSConfig.MinVersion != 0 {
                tlsConfig.MinVersion = p.config.TLSConfig.MinVersion
            }
            if p.config.TLSConfig.ServerName != "" {
                tlsConfig.ServerName = p.config.TLSConfig.ServerName
            }
        }

        // Always check STARTTLS capability before attempting it
        if ok, _ := client.Extension("STARTTLS"); ok {
            if err := client.StartTLS(tlsConfig); err != nil {
                client.Close()
                conn.Close()
                return nil, fmt.Errorf("STARTTLS failed: %w", err)
            }
        } else if p.config.TLSConfig != nil && p.config.TLSConfig.Enabled {
            // TLS is explicitly required but server doesn't advertise STARTTLS
            client.Close()
            conn.Close()
            return nil, fmt.Errorf("server %s does not support STARTTLS — cannot authenticate securely", p.config.Host)
        }
        // If TLS is not required and server doesn't advertise it, proceed without TLS
    }

    // Authenticate only AFTER TLS is established
    // Try PLAIN first, fall back to LOGIN (required by some Office 365 configs)
    if err := p.authenticate(client); err != nil {
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

    maxLifetime := 5 * time.Minute
    if p.config != nil && p.config.ConnectionPool != nil && p.config.ConnectionPool.MaxLifetime > 0 {
        maxLifetime = p.config.ConnectionPool.MaxLifetime
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
    return NewSMTPProviderImpl(config, log)
}

// authenticate tries AUTH PLAIN first, then falls back to AUTH LOGIN.
// This handles Office 365 and other servers that may prefer LOGIN over PLAIN.
func (p *factoryConnectionPool) authenticate(client *smtp.Client) error {
    username := p.config.Username
    password := p.config.Password

    if password == "" {
        return nil
    }

    // Check which auth mechanisms the server advertises
    supportsPlain := false
    supportsLogin := false
    if ext, params := client.Extension("AUTH"); ext {
        fields := strings.Fields(params)
        for _, f := range fields {
            switch strings.ToUpper(f) {
            case "PLAIN":
                supportsPlain = true
            case "LOGIN":
                supportsLogin = true
            }
        }
    }

    // Try PLAIN first (preferred, more efficient)
    if supportsPlain || (!supportsPlain && !supportsLogin) {
        auth := smtp.PlainAuth("", username, password, p.config.Host)
        if err := client.Auth(auth); err == nil {
            return nil
        } else if supportsPlain {
            // PLAIN was advertised but failed — try LOGIN before giving up
            if supportsLogin {
                loginAuth := &smtpLoginAuth{username: username, password: password}
                return client.Auth(loginAuth)
            }
            return err
        }
    }

    // Fall back to LOGIN auth (Office 365 onmicrosoft.com often requires this)
    loginAuth := &smtpLoginAuth{username: username, password: password}
    return client.Auth(loginAuth)
}

// smtpLoginAuth implements the AUTH LOGIN mechanism used by Office 365 / Exchange.
type smtpLoginAuth struct {
    username, password string
}

func (a *smtpLoginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
    return "LOGIN", nil, nil
}

func (a *smtpLoginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
    if more {
        prompt := strings.ToLower(strings.TrimRight(string(fromServer), ": "))
        switch prompt {
        case "username", "user name":
            return []byte(a.username), nil
        case "password":
            return []byte(a.password), nil
        default:
            // Some servers just send challenges in order: first username, then password
            // Respond with username first, then password
            if a.username != "" {
                user := a.username
                a.username = ""
                return []byte(user), nil
            }
            return []byte(a.password), nil
        }
    }
    return nil, nil
}

type plainAuthUnencrypted struct {
    identity, username, password, host string
}

func (a *plainAuthUnencrypted) Start(server *smtp.ServerInfo) (string, []byte, error) {
    resp := []byte(a.identity + "\x00" + a.username + "\x00" + a.password)
    return "PLAIN", resp, nil
}

func (a *plainAuthUnencrypted) Next(fromServer []byte, more bool) ([]byte, error) {
    if more {
        return nil, fmt.Errorf("unexpected server challenge")
    }
    return nil, nil
}
