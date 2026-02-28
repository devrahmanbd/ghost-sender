package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"email-campaign-system/internal/models"

	"golang.org/x/net/proxy"
)

var (
	ErrInvalidProxyURL     = errors.New("invalid proxy URL")
	ErrProxyUnreachable    = errors.New("proxy is unreachable")
	ErrProxyTimeout        = errors.New("proxy connection timeout")
	ErrProxyAuthFailed     = errors.New("proxy authentication failed")
	ErrInvalidProxyFormat  = errors.New("invalid proxy format")
)

type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnknown   HealthStatus = "unknown"
)

type ProxyValidator struct {
	config        *ValidatorConfig
	testEndpoints []string
	httpClient    *http.Client
}

type ValidatorConfig struct {
	Timeout            time.Duration
	MaxLatency         time.Duration
	RetryAttempts      int
	RetryDelay         time.Duration
	CheckAnonymity     bool
	VerifySSL          bool
	TestEndpoints      []string
	ConcurrentChecks   int
}

type ProxyHealth struct {
	Status         HealthStatus
	Latency        time.Duration
	LastChecked    time.Time
	CheckedAt      time.Time
	Message        string
	ErrorCount     int
	SuccessCount   int
	AnonymityLevel string
	Location       string
	ResponseCode   int
}

type ValidationResult struct {
	Valid   bool
	Errors  []string
	Health  *ProxyHealth
	ProxyID string
}

func NewProxyValidator(config *ValidatorConfig) *ProxyValidator {
	if config == nil {
		config = DefaultValidatorConfig()
	}

	return &ProxyValidator{
		config: config,
		testEndpoints: []string{
			"https://www.google.com",
			"https://api.ipify.org",
			"https://ifconfig.me/ip",
		},
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

func DefaultValidatorConfig() *ValidatorConfig {
	return &ValidatorConfig{
		Timeout:          10 * time.Second,
		MaxLatency:       5 * time.Second,
		RetryAttempts:    3,
		RetryDelay:       2 * time.Second,
		CheckAnonymity:   false,
		VerifySSL:        true,
		ConcurrentChecks: 5,
		TestEndpoints: []string{
			"https://www.google.com",
		},
	}
}

func (pv *ProxyValidator) Validate(p *models.Proxy) error {
	if p == nil {
		return errors.New("proxy is nil")
	}

	if p.Host == "" {
		return errors.New("proxy host is required")
	}

	if p.Port <= 0 || p.Port > 65535 {
		return errors.New("proxy port must be between 1 and 65535")
	}

	if err := pv.validateProxyType(p.Type); err != nil {
		return err
	}

	if p.Username != "" && p.Password == "" {
		return errors.New("password is required when username is provided")
	}

	proxyURL := pv.buildProxyURL(p)
	if _, err := url.Parse(proxyURL); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidProxyURL, err)
	}

	return nil
}

func (pv *ProxyValidator) validateProxyType(proxyType models.ProxyType) error {
	switch proxyType {
	case models.ProxyTypeHTTP, models.ProxyTypeHTTPS, models.ProxyTypeSOCKS5:
		return nil
	default:
		return ErrInvalidProxyType
	}
}

func (pv *ProxyValidator) CheckHealth(ctx context.Context, p *models.Proxy) (*ProxyHealth, error) {
	if err := pv.Validate(p); err != nil {
		return &ProxyHealth{
			Status:      HealthStatusUnhealthy,
			Message:     err.Error(),
			CheckedAt:   time.Now(),
			LastChecked: time.Now(),
		}, err
	}

	health := &ProxyHealth{
		CheckedAt:   time.Now(),
		LastChecked: time.Now(),
	}

	startTime := time.Now()

	if err := pv.testConnection(ctx, p); err != nil {
		health.Status = HealthStatusUnhealthy
		health.Message = err.Error()
		health.ErrorCount++
		return health, err
	}

	health.Latency = time.Since(startTime)
	health.SuccessCount++

	if health.Latency > pv.config.MaxLatency {
		health.Status = HealthStatusDegraded
		health.Message = fmt.Sprintf("high latency: %v", health.Latency)
	} else {
		health.Status = HealthStatusHealthy
		health.Message = "proxy is working"
	}

	if pv.config.CheckAnonymity {
		level, err := pv.checkAnonymityLevel(ctx, p)
		if err == nil {
			health.AnonymityLevel = level
		}
	}

	return health, nil
}

func (pv *ProxyValidator) testConnection(ctx context.Context, p *models.Proxy) error {
	proxyURL := pv.buildProxyURL(p)

	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidProxyURL, err)
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(parsedURL),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !pv.config.VerifySSL,
		},
		DialContext: (&net.Dialer{
			Timeout:   pv.config.Timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   pv.config.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	testURL := pv.config.TestEndpoints[0]
	if len(pv.testEndpoints) > 0 {
		testURL = pv.testEndpoints[0]
	}

	req, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return ErrProxyTimeout
		}
		return fmt.Errorf("%w: %v", ErrProxyUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 407 {
		return ErrProxyAuthFailed
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("proxy returned status code: %d", resp.StatusCode)
	}

	_, err = io.Copy(io.Discard, resp.Body)
	return err
}

func (pv *ProxyValidator) TestConnectionWithEndpoint(ctx context.Context, p *models.Proxy, endpoint string) error {
	proxyURL := pv.buildProxyURL(p)

	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidProxyURL, err)
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(parsedURL),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !pv.config.VerifySSL,
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   pv.config.Timeout,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("endpoint returned status: %d", resp.StatusCode)
	}

	return nil
}

func (pv *ProxyValidator) MeasureLatency(ctx context.Context, p *models.Proxy) (time.Duration, error) {
	startTime := time.Now()

	if err := pv.testConnection(ctx, p); err != nil {
		return 0, err
	}

	return time.Since(startTime), nil
}

func (pv *ProxyValidator) checkAnonymityLevel(ctx context.Context, p *models.Proxy) (string, error) {
	proxyURL := pv.buildProxyURL(p)
	parsedURL, _ := url.Parse(proxyURL)

	transport := &http.Transport{
		Proxy: http.ProxyURL(parsedURL),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   pv.config.Timeout,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.ipify.org?format=json", nil)
	if err != nil {
		return "unknown", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "unknown", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "unknown", err
	}

	if strings.Contains(string(body), "\"ip\"") {
		return "transparent", nil
	}

	return "anonymous", nil
}

func (pv *ProxyValidator) ValidateAuth(ctx context.Context, p *models.Proxy) error {
	if p.Username == "" && p.Password == "" {
		return nil
	}

	return pv.testConnection(ctx, p)
}

func (pv *ProxyValidator) BatchValidate(ctx context.Context, proxies []*models.Proxy) []*ValidationResult {
	results := make([]*ValidationResult, len(proxies))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, pv.config.ConcurrentChecks)

	for i, p := range proxies {
		wg.Add(1)
		go func(idx int, proxy *models.Proxy) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result := &ValidationResult{
				ProxyID: proxy.ID,
				Errors:  []string{},
			}

			if err := pv.Validate(proxy); err != nil {
				result.Valid = false
				result.Errors = append(result.Errors, err.Error())
				results[idx] = result
				return
			}

			health, err := pv.CheckHealth(ctx, proxy)
			if err != nil {
				result.Valid = false
				result.Errors = append(result.Errors, err.Error())
				result.Health = health
			} else {
				result.Valid = true
				result.Health = health
			}

			results[idx] = result
		}(i, p)
	}

	wg.Wait()
	return results
}

func (pv *ProxyValidator) TestSOCKS5(ctx context.Context, p *models.Proxy) error {
	address := fmt.Sprintf("%s:%d", p.Host, p.Port)

	var auth *proxy.Auth
	if p.Username != "" {
		auth = &proxy.Auth{
			User:     p.Username,
			Password: p.Password,
		}
	}

	dialer, err := proxy.SOCKS5("tcp", address, auth, proxy.Direct)
	if err != nil {
		return fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
	}

	conn, err := dialer.Dial("tcp", "www.google.com:80")
	if err != nil {
		return fmt.Errorf("failed to dial through SOCKS5: %w", err)
	}
	defer conn.Close()

	return nil
}

func (pv *ProxyValidator) CheckPort(host string, port int, timeout time.Duration) error {
	address := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return fmt.Errorf("port check failed: %w", err)
	}
	defer conn.Close()
	return nil
}

func (pv *ProxyValidator) ResolveDNS(host string) ([]string, error) {
	ips, err := net.LookupHost(host)
	if err != nil {
		return nil, fmt.Errorf("DNS resolution failed: %w", err)
	}
	return ips, nil
}

func (pv *ProxyValidator) buildProxyURL(p *models.Proxy) string {
	scheme := strings.ToLower(string(p.Type))
	
	if p.Username != "" && p.Password != "" {
		return fmt.Sprintf("%s://%s:%s@%s:%d",
			scheme,
			url.QueryEscape(p.Username),
			url.QueryEscape(p.Password),
			p.Host,
			p.Port,
		)
	}

	return fmt.Sprintf("%s://%s:%d", scheme, p.Host, p.Port)
}

func (pv *ProxyValidator) ValidateFormat(proxyString string) error {
	parts := strings.Split(proxyString, ":")
	if len(parts) < 2 {
		return ErrInvalidProxyFormat
	}

	host := parts[0]
	if host == "" {
		return errors.New("proxy host is empty")
	}

	if len(parts) < 2 {
		return errors.New("proxy port is missing")
	}

	return nil
}

func (pv *ProxyValidator) ParseProxyString(proxyString string) (*models.Proxy, error) {
	proxyString = strings.TrimSpace(proxyString)

	var proxyURL *url.URL
	var err error

	if strings.Contains(proxyString, "://") {
		proxyURL, err = url.Parse(proxyString)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}
	} else {
		parts := strings.Split(proxyString, ":")
		if len(parts) < 2 {
			return nil, ErrInvalidProxyFormat
		}

		proxyURL = &url.URL{
			Scheme: "http",
			Host:   proxyString,
		}
	}

	proxy := &models.Proxy{
		Host: proxyURL.Hostname(),
	}

	port := proxyURL.Port()
	if port == "" {
		port = "80"
	}
	
	var portNum int
	fmt.Sscanf(port, "%d", &portNum)
	proxy.Port = portNum

	if proxyURL.User != nil {
		proxy.Username = proxyURL.User.Username()
		proxy.Password, _ = proxyURL.User.Password()
	}

	switch proxyURL.Scheme {
	case "http":
		proxy.Type = models.ProxyTypeHTTP
	case "https":
		proxy.Type = models.ProxyTypeHTTPS
	case "socks5":
		proxy.Type = models.ProxyTypeSOCKS5
	default:
		proxy.Type = models.ProxyTypeHTTP
	}

	return proxy, nil
}

func (pv *ProxyValidator) SetTestEndpoints(endpoints []string) {
	pv.testEndpoints = endpoints
}

func (pv *ProxyValidator) GetTestEndpoints() []string {
	return pv.testEndpoints
}

func (pv *ProxyValidator) SetTimeout(timeout time.Duration) {
	pv.config.Timeout = timeout
	pv.httpClient.Timeout = timeout
}

func (pv *ProxyValidator) GetConfig() *ValidatorConfig {
	return pv.config
}
