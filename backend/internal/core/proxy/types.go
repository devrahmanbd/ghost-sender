package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ProxyProtocol string

const (
	ProtocolHTTP   ProxyProtocol = "http"
	ProtocolHTTPS  ProxyProtocol = "https"
	ProtocolSOCKS4 ProxyProtocol = "socks4"
	ProtocolSOCKS5 ProxyProtocol = "socks5"
)

type AuthType string

const (
	AuthTypeNone  AuthType = "none"
	AuthTypeBasic AuthType = "basic"
	AuthTypeNTLM  AuthType = "ntlm"
)

type ProxyStatus string

const (
	StatusActive   ProxyStatus = "active"
	StatusInactive ProxyStatus = "inactive"
	StatusTesting  ProxyStatus = "testing"
	StatusFailed   ProxyStatus = "failed"
	StatusBanned   ProxyStatus = "banned"
)

type ConnectionType string

const (
	ConnectionTypeDirect     ConnectionType = "direct"
	ConnectionTypeHTTPProxy  ConnectionType = "http_proxy"
	ConnectionTypeHTTPSProxy ConnectionType = "https_proxy"
	ConnectionTypeSOCKS5     ConnectionType = "socks5"
)

type ProxyFeature string

const (
	FeatureHTTP    ProxyFeature = "http"
	FeatureHTTPS   ProxyFeature = "https"
	FeatureSOCKS4  ProxyFeature = "socks4"
	FeatureSOCKS5  ProxyFeature = "socks5"
	FeatureUDP     ProxyFeature = "udp"
	FeatureCONNECT ProxyFeature = "connect"
)

type ProxyLocation struct {
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	City        string  `json:"city"`
	Region      string  `json:"region"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Timezone    string  `json:"timezone"`
	ISP         string  `json:"isp"`
}

type ProxyMetrics struct {
	TotalRequests    int64         `json:"total_requests"`
	SuccessRequests  int64         `json:"success_requests"`
	FailedRequests   int64         `json:"failed_requests"`
	AverageLatency   time.Duration `json:"average_latency"`
	MinLatency       time.Duration `json:"min_latency"`
	MaxLatency       time.Duration `json:"max_latency"`
	BytesSent        int64         `json:"bytes_sent"`
	BytesReceived    int64         `json:"bytes_received"`
	LastUsedAt       time.Time     `json:"last_used_at"`
	LastSuccessAt    time.Time     `json:"last_success_at"`
	LastFailureAt    time.Time     `json:"last_failure_at"`
	ConsecutiveFails int           `json:"consecutive_fails"`
	Uptime           time.Duration `json:"uptime"`
	SuccessRate      float64       `json:"success_rate"`
}

type ProxyCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Domain   string `json:"domain,omitempty"`
}

type ProxySettings struct {
	Timeout            time.Duration `json:"timeout"`
	KeepAlive          time.Duration `json:"keep_alive"`
	MaxIdleConns       int           `json:"max_idle_conns"`
	MaxConnsPerHost    int           `json:"max_conns_per_host"`
	IdleConnTimeout    time.Duration `json:"idle_conn_timeout"`
	TLSHandshake       time.Duration `json:"tls_handshake"`
	ResponseHeader     time.Duration `json:"response_header"`
	ExpectContinue     time.Duration `json:"expect_continue"`
	DialTimeout        time.Duration `json:"dial_timeout"`
	DialKeepAlive      time.Duration `json:"dial_keep_alive"`
	InsecureSkipVerify bool          `json:"insecure_skip_verify"`
}

type ProxyFilter struct {
	Protocol       ProxyProtocol  `json:"protocol,omitempty"`
	Status         ProxyStatus    `json:"status,omitempty"`
	Country        string         `json:"country,omitempty"`
	MinLatency     time.Duration  `json:"min_latency,omitempty"`
	MaxLatency     time.Duration  `json:"max_latency,omitempty"`
	MinSuccessRate float64        `json:"min_success_rate,omitempty"`
	Features       []ProxyFeature `json:"features,omitempty"`
	Tags           []string       `json:"tags,omitempty"`
	Anonymous      bool           `json:"anonymous,omitempty"`
}

type ProxyTestResult struct {
	ProxyID       string        `json:"proxy_id"`
	Success       bool          `json:"success"`
	Latency       time.Duration `json:"latency"`
	Error         string        `json:"error,omitempty"`
	StatusCode    int           `json:"status_code"`
	ResponseTime  time.Duration `json:"response_time"`
	DNSTime       time.Duration `json:"dns_time"`
	ConnectTime   time.Duration `json:"connect_time"`
	TLSTime       time.Duration `json:"tls_time"`
	Anonymous     bool          `json:"anonymous"`
	RealIP        string        `json:"real_ip,omitempty"`
	ProxyIP       string        `json:"proxy_ip,omitempty"`
	TestedAt      time.Time     `json:"tested_at"`
}

type ProxyConnection struct {
	ID           string         `json:"id"`
	ProxyID      string         `json:"proxy_id"`
	LocalAddr    string         `json:"local_addr"`
	RemoteAddr   string         `json:"remote_addr"`
	State        string         `json:"state"`
	CreatedAt    time.Time      `json:"created_at"`
	LastActivity time.Time      `json:"last_activity"`
	BytesSent    int64          `json:"bytes_sent"`
	BytesRecv    int64          `json:"bytes_recv"`
	Duration     time.Duration  `json:"duration"`
}

type ProxyChain struct {
	Name    string   `json:"name"`
	Proxies []string `json:"proxies"`
	Enabled bool     `json:"enabled"`
}

type ProxyPool struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	ProxyIDs    []string      `json:"proxy_ids"`
	Strategy    string        `json:"strategy"`
	Enabled     bool          `json:"enabled"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

func (pp ProxyProtocol) String() string {
	return string(pp)
}

func (pp ProxyProtocol) IsValid() bool {
	switch pp {
	case ProtocolHTTP, ProtocolHTTPS, ProtocolSOCKS4, ProtocolSOCKS5:
		return true
	default:
		return false
	}
}

func ParseProxyProtocol(s string) (ProxyProtocol, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	
	switch s {
	case "http":
		return ProtocolHTTP, nil
	case "https":
		return ProtocolHTTPS, nil
	case "socks4":
		return ProtocolSOCKS4, nil
	case "socks5", "socks":
		return ProtocolSOCKS5, nil
	default:
		return "", fmt.Errorf("invalid proxy protocol: %s", s)
	}
}

func (at AuthType) String() string {
	return string(at)
}

func (at AuthType) IsValid() bool {
	switch at {
	case AuthTypeNone, AuthTypeBasic, AuthTypeNTLM:
		return true
	default:
		return false
	}
}

func ParseAuthType(s string) (AuthType, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	
	switch s {
	case "none", "":
		return AuthTypeNone, nil
	case "basic":
		return AuthTypeBasic, nil
	case "ntlm":
		return AuthTypeNTLM, nil
	default:
		return "", fmt.Errorf("invalid auth type: %s", s)
	}
}

func (ps ProxyStatus) String() string {
	return string(ps)
}

func (ps ProxyStatus) IsActive() bool {
	return ps == StatusActive
}

func ParseProxyStatus(s string) (ProxyStatus, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	
	switch s {
	case "active":
		return StatusActive, nil
	case "inactive":
		return StatusInactive, nil
	case "testing":
		return StatusTesting, nil
	case "failed":
		return StatusFailed, nil
	case "banned":
		return StatusBanned, nil
	default:
		return "", fmt.Errorf("invalid proxy status: %s", s)
	}
}

func (ct ConnectionType) String() string {
	return string(ct)
}

func (pm *ProxyMetrics) CalculateSuccessRate() float64 {
	if pm.TotalRequests == 0 {
		return 0.0
	}
	return float64(pm.SuccessRequests) / float64(pm.TotalRequests) * 100.0
}

func (pm *ProxyMetrics) RecordSuccess(latency time.Duration) {
	pm.TotalRequests++
	pm.SuccessRequests++
	pm.LastSuccessAt = time.Now()
	pm.LastUsedAt = time.Now()
	pm.ConsecutiveFails = 0
	
	if pm.MinLatency == 0 || latency < pm.MinLatency {
		pm.MinLatency = latency
	}
	
	if latency > pm.MaxLatency {
		pm.MaxLatency = latency
	}
	
	pm.AverageLatency = time.Duration((int64(pm.AverageLatency)*pm.SuccessRequests + int64(latency)) / pm.SuccessRequests)
	pm.SuccessRate = pm.CalculateSuccessRate()
}

func (pm *ProxyMetrics) RecordFailure() {
	pm.TotalRequests++
	pm.FailedRequests++
	pm.LastFailureAt = time.Now()
	pm.LastUsedAt = time.Now()
	pm.ConsecutiveFails++
	pm.SuccessRate = pm.CalculateSuccessRate()
}

func (pm *ProxyMetrics) Reset() {
	pm.TotalRequests = 0
	pm.SuccessRequests = 0
	pm.FailedRequests = 0
	pm.AverageLatency = 0
	pm.MinLatency = 0
	pm.MaxLatency = 0
	pm.BytesSent = 0
	pm.BytesReceived = 0
	pm.ConsecutiveFails = 0
	pm.SuccessRate = 0
}

func (pc *ProxyCredentials) IsEmpty() bool {
	return pc.Username == "" && pc.Password == ""
}

func (pc *ProxyCredentials) Validate() error {
	if pc.Username == "" && pc.Password != "" {
		return errors.New("username is required when password is provided")
	}
	
	if pc.Username != "" && pc.Password == "" {
		return errors.New("password is required when username is provided")
	}
	
	return nil
}

func (pc *ProxyCredentials) String() string {
	if pc.IsEmpty() {
		return ""
	}
	return fmt.Sprintf("%s:***", pc.Username)
}

func DefaultProxySettings() *ProxySettings {
	return &ProxySettings{
		Timeout:            30 * time.Second,
		KeepAlive:          30 * time.Second,
		MaxIdleConns:       100,
		MaxConnsPerHost:    10,
		IdleConnTimeout:    90 * time.Second,
		TLSHandshake:       10 * time.Second,
		ResponseHeader:     10 * time.Second,
		ExpectContinue:     1 * time.Second,
		DialTimeout:        10 * time.Second,
		DialKeepAlive:      30 * time.Second,
		InsecureSkipVerify: false,
	}
}

func (ps *ProxySettings) Validate() error {
	if ps.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	
	if ps.MaxIdleConns < 0 {
		return errors.New("max_idle_conns cannot be negative")
	}
	
	if ps.MaxConnsPerHost < 0 {
		return errors.New("max_conns_per_host cannot be negative")
	}
	
	return nil
}

func (pf *ProxyFilter) Matches(protocol ProxyProtocol, status ProxyStatus, country string, latency time.Duration, successRate float64) bool {
	if pf.Protocol != "" && pf.Protocol != protocol {
		return false
	}
	
	if pf.Status != "" && pf.Status != status {
		return false
	}
	
	if pf.Country != "" && pf.Country != country {
		return false
	}
	
	if pf.MinLatency > 0 && latency < pf.MinLatency {
		return false
	}
	
	if pf.MaxLatency > 0 && latency > pf.MaxLatency {
		return false
	}
	
	if pf.MinSuccessRate > 0 && successRate < pf.MinSuccessRate {
		return false
	}
	
	return true
}

func (ptr *ProxyTestResult) IsSuccess() bool {
	return ptr.Success && ptr.Error == ""
}

func (ptr *ProxyTestResult) MarshalJSON() ([]byte, error) {
	type Alias ProxyTestResult
	return json.Marshal(&struct {
		Latency      string `json:"latency"`
		ResponseTime string `json:"response_time"`
		DNSTime      string `json:"dns_time"`
		ConnectTime  string `json:"connect_time"`
		TLSTime      string `json:"tls_time"`
		*Alias
	}{
		Latency:      ptr.Latency.String(),
		ResponseTime: ptr.ResponseTime.String(),
		DNSTime:      ptr.DNSTime.String(),
		ConnectTime:  ptr.ConnectTime.String(),
		TLSTime:      ptr.TLSTime.String(),
		Alias:        (*Alias)(ptr),
	})
}

func (pl *ProxyLocation) String() string {
	if pl.City != "" && pl.Country != "" {
		return fmt.Sprintf("%s, %s", pl.City, pl.Country)
	}
	
	if pl.Country != "" {
		return pl.Country
	}
	
	return "Unknown"
}

func (pl *ProxyLocation) IsEmpty() bool {
	return pl.Country == "" && pl.City == "" && pl.CountryCode == ""
}

func NewProxyMetrics() *ProxyMetrics {
	return &ProxyMetrics{
		LastUsedAt:    time.Now(),
		LastSuccessAt: time.Time{},
		LastFailureAt: time.Time{},
	}
}

func NewProxyCredentials(username, password string) *ProxyCredentials {
	return &ProxyCredentials{
		Username: username,
		Password: password,
	}
}

func IsValidPort(port int) bool {
	return port > 0 && port <= 65535
}

func IsCommonProxyPort(port int) bool {
	commonPorts := []int{80, 443, 1080, 3128, 8080, 8888, 9050}
	for _, p := range commonPorts {
		if port == p {
			return true
		}
	}
	return false
}

func GetDefaultPortForProtocol(protocol ProxyProtocol) int {
	switch protocol {
	case ProtocolHTTP:
		return 8080
	case ProtocolHTTPS:
		return 443
	case ProtocolSOCKS4, ProtocolSOCKS5:
		return 1080
	default:
		return 8080
	}
}
