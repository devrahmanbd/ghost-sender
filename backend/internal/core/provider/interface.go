package provider

import (
	"context"
	"time"

	"email-campaign-system/internal/models"
)

type Provider interface {
	Send(ctx context.Context, message *models.Email) (string, error)
	TestConnection(ctx context.Context) error
	Validate() error
	Close() error
	Name() string
	Type() ProviderType
	SupportedFeatures() []Feature
	GetConfig() *ProviderConfig
	GetHealth() *ProviderHealth
}

type ProviderType string

const (
	ProviderTypeGmail           ProviderType = "gmail"
	ProviderTypeSMTP            ProviderType = "smtp"
	ProviderTypeYahoo           ProviderType = "yahoo"
	ProviderTypeOutlook         ProviderType = "outlook"
	ProviderTypeHotmail         ProviderType = "hotmail"
	ProviderTypeICloud          ProviderType = "icloud"
	ProviderTypeGoogleWorkspace ProviderType = "google_workspace"
)

type Feature string

const (
	FeatureSMTP           Feature = "smtp"
	FeatureAppPassword    Feature = "app_password"
	FeatureTLS            Feature = "tls"
	FeatureSSL            Feature = "ssl"
	FeatureSTARTTLS       Feature = "starttls"
	FeatureAttachments    Feature = "attachments"
	FeatureHTMLBody       Feature = "html_body"
	FeatureTextBody       Feature = "text_body"
	FeatureEmbedded       Feature = "embedded"
	FeatureProxy          Feature = "proxy"
	FeatureConnectionPool Feature = "connection_pool"
)

type ProviderConfig struct {
	Type             ProviderType
	Host             string
	Port             int
	Username         string
	Password         string
	TLSConfig        *TLSConfig
	ProxyConfig      *ProxyConfig
	TimeoutConfig    *TimeoutConfig
	ConnectionPool   *ConnectionPoolConfig
	RetryConfig      *RetryConfig
	EnableKeepAlive  bool
	MaxMessageSize   int64
	RateLimitPerHour int
	RateLimitPerDay  int
}

type TLSConfig struct {
	Enabled            bool
	InsecureSkipVerify bool
	MinVersion         uint16
	MaxVersion         uint16
	ServerName         string
	CertFile           string
	KeyFile            string
	CAFile             string
}

type ProxyConfig struct {
	Enabled  bool
	Type     string
	Host     string
	Port     int
	Username string
	Password string
	Timeout  time.Duration
}

type TimeoutConfig struct {
	Connect   time.Duration
	Send      time.Duration
	Read      time.Duration
	Write     time.Duration
	Idle      time.Duration
	KeepAlive time.Duration
}

type ConnectionPoolConfig struct {
	Enabled        bool
	MaxConnections int
	MaxIdleTime    time.Duration
	MaxLifetime    time.Duration
	MinConnections int
	AcquireTimeout time.Duration
}

type RetryConfig struct {
	Enabled      bool
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

type ProviderHealth struct {
	Status           HealthStatus
	LastCheck        time.Time
	LastSuccess      time.Time
	LastFailure      time.Time
	ConsecutiveFails int
	TotalSent        int64
	TotalFailed      int64
	ErrorRate        float64
	AvgResponseTime  time.Duration
	IsConnected      bool
	Message          string
}

type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusUnknown   HealthStatus = "unknown"
)

var (
	ErrInvalidConfig        = NewProviderError("invalid provider configuration")
	ErrConnectionFailed     = NewProviderError("failed to connect to provider")
	ErrAuthenticationFailed = NewProviderError("authentication failed")
	ErrSendFailed           = NewProviderError("failed to send message")
	ErrInvalidProvider      = NewProviderError("invalid provider type")
	ErrProviderNotSupported = NewProviderError("provider not supported")
)

type ProviderError struct {
	Message string
	Code    string
	Err     error
}

func NewProviderError(message string) error {
	return &ProviderError{
		Message: message,
	}
}

func (e *ProviderError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

func (e *ProviderError) WithError(err error) *ProviderError {
	e.Err = err
	return e
}

func (e *ProviderError) WithCode(code string) *ProviderError {
	e.Code = code
	return e
}

func DefaultProviderConfig() *ProviderConfig {
	return &ProviderConfig{
		TLSConfig: &TLSConfig{
			Enabled:            true,
			InsecureSkipVerify: false,
			MinVersion:         0x0303,
		},
		TimeoutConfig: &TimeoutConfig{
			Connect:   30 * time.Second,
			Send:      60 * time.Second,
			Read:      30 * time.Second,
			Write:     30 * time.Second,
			Idle:      90 * time.Second,
			KeepAlive: 30 * time.Second,
		},
		ConnectionPool: &ConnectionPoolConfig{
			Enabled:        true,
			MaxConnections: 10,
			MaxIdleTime:    5 * time.Minute,
			MaxLifetime:    30 * time.Minute,
			MinConnections: 2,
			AcquireTimeout: 10 * time.Second,
		},
		RetryConfig: &RetryConfig{
			Enabled:      true,
			MaxRetries:   3,
			InitialDelay: 1 * time.Second,
			MaxDelay:     30 * time.Second,
			Multiplier:   2.0,
		},
		EnableKeepAlive:  true,
		MaxMessageSize:   25 * 1024 * 1024,
		RateLimitPerHour: 100,
		RateLimitPerDay:  500,
	}
}

func GetProviderDefaults(providerType ProviderType) *ProviderConfig {
	config := DefaultProviderConfig()
	config.Type = providerType

	switch providerType {
	case ProviderTypeGmail:
		config.Host = "smtp.gmail.com"
		config.Port = 587
		config.RateLimitPerDay = 500
		config.RateLimitPerHour = 100

	case ProviderTypeGoogleWorkspace:
		config.Host = "smtp.gmail.com"
		config.Port = 587
		config.RateLimitPerDay = 2000
		config.RateLimitPerHour = 500

	case ProviderTypeYahoo:
		config.Host = "smtp.mail.yahoo.com"
		config.Port = 587
		config.RateLimitPerDay = 500
		config.RateLimitPerHour = 100

	case ProviderTypeOutlook, ProviderTypeHotmail:
		config.Host = "smtp-mail.outlook.com"
		config.Port = 587
		config.RateLimitPerDay = 300
		config.RateLimitPerHour = 100

	case ProviderTypeICloud:
		config.Host = "smtp.mail.me.com"
		config.Port = 587
		config.RateLimitPerDay = 1000
		config.RateLimitPerHour = 200

	default:
		config.Port = 587
	}

	return config
}

func (pc *ProviderConfig) Validate() error {
	if pc.Type == "" {
		return ErrInvalidConfig
	}

	if pc.Username == "" {
		return &ProviderError{Message: "username is required"}
	}

	if pc.Password == "" {
		return &ProviderError{Message: "password is required"}
	}

	if pc.Host == "" {
		return &ProviderError{Message: "host is required"}
	}

	if pc.Port == 0 {
		return &ProviderError{Message: "port is required"}
	}

	return nil
}

func (pc *ProviderConfig) Clone() *ProviderConfig {
	clone := *pc

	if pc.TLSConfig != nil {
		tlsClone := *pc.TLSConfig
		clone.TLSConfig = &tlsClone
	}

	if pc.ProxyConfig != nil {
		proxyClone := *pc.ProxyConfig
		clone.ProxyConfig = &proxyClone
	}

	if pc.TimeoutConfig != nil {
		timeoutClone := *pc.TimeoutConfig
		clone.TimeoutConfig = &timeoutClone
	}

	if pc.ConnectionPool != nil {
		poolClone := *pc.ConnectionPool
		clone.ConnectionPool = &poolClone
	}

	if pc.RetryConfig != nil {
		retryClone := *pc.RetryConfig
		clone.RetryConfig = &retryClone
	}

	return &clone
}
