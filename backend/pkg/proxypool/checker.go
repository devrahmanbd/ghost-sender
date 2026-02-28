package proxypool

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

var (
	ErrCheckTimeout     = errors.New("proxy check timeout")
	ErrConnectionFailed = errors.New("proxy connection failed")
	ErrInvalidResponse  = errors.New("invalid response from proxy")
	ErrAuthFailed       = errors.New("proxy authentication failed")
)

const (
	DefaultCheckTimeout = 10 * time.Second
	DefaultTestURL      = "https://www.google.com"
	DefaultTestTarget   = "8.8.8.8:53"
)

type CheckResult struct {
	ProxyID     string
	Success     bool
	Latency     time.Duration
	Error       error
	StatusCode  int
	TestURL     string
	Timestamp   time.Time
	Anonymous   bool
	Country     string
	City        string
	ISP         string
	Details     map[string]interface{}
}

type CheckConfig struct {
	Timeout         time.Duration
	TestURL         string
	TestTarget      string
	VerifyAnonymity bool
	CheckLocation   bool
	MaxConcurrent   int
	RetryCount      int
	RetryDelay      time.Duration
}

type Checker struct {
	config *CheckConfig
	client *http.Client
	mu     sync.RWMutex
}

func NewChecker(config *CheckConfig) *Checker {
	if config == nil {
		config = DefaultCheckConfig()
	}

	return &Checker{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func DefaultCheckConfig() *CheckConfig {
	return &CheckConfig{
		Timeout:         DefaultCheckTimeout,
		TestURL:         DefaultTestURL,
		TestTarget:      DefaultTestTarget,
		VerifyAnonymity: false,
		CheckLocation:   false,
		MaxConcurrent:   10,
		RetryCount:      2,
		RetryDelay:      1 * time.Second,
	}
}

func (c *Checker) Check(ctx context.Context, p *Proxy) *CheckResult {
	result := &CheckResult{
		ProxyID:   p.ID,
		TestURL:   c.config.TestURL,
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	start := time.Now()

	var err error
	switch p.Type {
	case ProxyTypeHTTP, ProxyTypeHTTPS:
		err = c.checkHTTP(ctx, p, result)
	case ProxyTypeSOCKS5:
		err = c.checkSOCKS5(ctx, p, result)
	default:
		err = errors.New("unsupported proxy type")
	}

	result.Latency = time.Since(start)
	result.Success = err == nil
	result.Error = err

	if result.Success && c.config.VerifyAnonymity {
		result.Anonymous = c.verifyAnonymity(ctx, p)
	}

	if result.Success && c.config.CheckLocation {
		c.getLocation(ctx, p, result)
	}

	return result
}

func (c *Checker) CheckBatch(ctx context.Context, proxies []*Proxy) []*CheckResult {
	results := make([]*CheckResult, len(proxies))
	
	semaphore := make(chan struct{}, c.config.MaxConcurrent)
	var wg sync.WaitGroup

	for i, p := range proxies {
		wg.Add(1)
		go func(index int, proxy *Proxy) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			results[index] = c.Check(ctx, proxy)
		}(i, p)
	}

	wg.Wait()
	return results
}

func (c *Checker) CheckWithRetry(ctx context.Context, p *Proxy) *CheckResult {
	var result *CheckResult

	for i := 0; i <= c.config.RetryCount; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return &CheckResult{
					ProxyID:   p.ID,
					Success:   false,
					Error:     ctx.Err(),
					Timestamp: time.Now(),
				}
			case <-time.After(c.config.RetryDelay):
			}
		}

		result = c.Check(ctx, p)
		if result.Success {
			return result
		}
	}

	return result
}

func (c *Checker) checkHTTP(ctx context.Context, p *Proxy, result *CheckResult) error {
	proxyURL, err := url.Parse(p.URL)
	if err != nil {
		return err
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{
				Timeout:   c.config.Timeout,
				KeepAlive: 30 * time.Second,
			}
			return dialer.DialContext(ctx, network, addr)
		},
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   c.config.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.config.TestURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode

	if resp.StatusCode == http.StatusProxyAuthRequired {
		return ErrAuthFailed
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return ErrInvalidResponse
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return err
	}

	if len(body) == 0 {
		return ErrInvalidResponse
	}

	return nil
}

func (c *Checker) checkSOCKS5(ctx context.Context, p *Proxy, result *CheckResult) error {
	var auth *proxy.Auth
	if p.Username != "" {
		auth = &proxy.Auth{
			User:     p.Username,
			Password: p.Password,
		}
	}

	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("%s:%d", p.Host, p.Port), auth, proxy.Direct)
	if err != nil {
		return err
	}

	conn, err := dialer.Dial("tcp", c.config.TestTarget)
	if err != nil {
		return ErrConnectionFailed
	}
	defer conn.Close()

	deadline := time.Now().Add(c.config.Timeout)
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}

	testData := []byte("test")
	_, err = conn.Write(testData)
	if err != nil {
		return err
	}

	return nil
}

func (c *Checker) verifyAnonymity(ctx context.Context, p *Proxy) bool {
	proxyURL, err := url.Parse(p.URL)
	if err != nil {
		return false
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   c.config.Timeout,
	}

	checkURL := "https://api.ipify.org?format=json"
	req, err := http.NewRequestWithContext(ctx, "GET", checkURL, nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	return true
}

func (c *Checker) getLocation(ctx context.Context, p *Proxy, result *CheckResult) {
	proxyURL, err := url.Parse(p.URL)
	if err != nil {
		return
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   c.config.Timeout,
	}

	locationURL := "https://ipapi.co/json/"
	req, err := http.NewRequestWithContext(ctx, "GET", locationURL, nil)
	if err != nil {
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		result.Country = "Unknown"
		result.City = "Unknown"
	}
}

func (c *Checker) TestConnection(ctx context.Context, p *Proxy) error {
	result := c.Check(ctx, p)
	return result.Error
}

func (c *Checker) MeasureLatency(ctx context.Context, p *Proxy, iterations int) (time.Duration, error) {
	if iterations <= 0 {
		iterations = 3
	}

	var totalLatency time.Duration
	successCount := 0

	for i := 0; i < iterations; i++ {
		result := c.Check(ctx, p)
		if result.Success {
			totalLatency += result.Latency
			successCount++
		}

		if i < iterations-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	if successCount == 0 {
		return 0, errors.New("all latency measurements failed")
	}

	return totalLatency / time.Duration(successCount), nil
}

func (c *Checker) CheckSpeed(ctx context.Context, p *Proxy, downloadSize int64) (float64, error) {
	if downloadSize <= 0 {
		downloadSize = 1024 * 1024
	}

	proxyURL, err := url.Parse(p.URL)
	if err != nil {
		return 0, err
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	testURL := fmt.Sprintf("https://speed.cloudflare.com/__down?bytes=%d", downloadSize)
	req, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
	if err != nil {
		return 0, err
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	written, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		return 0, err
	}

	duration := time.Since(start).Seconds()
	if duration == 0 {
		return 0, errors.New("invalid duration")
	}

	speedMbps := (float64(written) * 8) / (duration * 1024 * 1024)
	return speedMbps, nil
}

func (c *Checker) ValidateProxy(p *Proxy) error {
	if p == nil {
		return ErrInvalidProxy
	}

	if p.Host == "" {
		return errors.New("proxy host is empty")
	}

	if p.Port <= 0 || p.Port > 65535 {
		return errors.New("invalid proxy port")
	}

	if p.Type == "" {
		return errors.New("proxy type is not specified")
	}

	return nil
}

func (c *Checker) QuickCheck(ctx context.Context, p *Proxy) bool {
	timeout := 5 * time.Second
	
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var conn net.Conn
	var err error

	dialer := &net.Dialer{
		Timeout: timeout,
	}

	conn, err = dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", p.Host, p.Port))
	if err != nil {
		return false
	}
	defer conn.Close()

	return true
}

func (c *Checker) CheckReachability(host string, port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (c *Checker) SetTimeout(timeout time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.config.Timeout = timeout
	c.client.Timeout = timeout
}

func (c *Checker) SetTestURL(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.config.TestURL = url
}

func (c *Checker) SetMaxConcurrent(max int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if max > 0 {
		c.config.MaxConcurrent = max
	}
}

func testProxyConnection(ctx context.Context, p *Proxy) error {
	checker := NewChecker(nil)
	result := checker.Check(ctx, p)
	return result.Error
}

func (r *CheckResult) String() string {
	if r.Success {
		return fmt.Sprintf("Proxy %s: OK (latency: %v, status: %d)", 
			r.ProxyID, r.Latency, r.StatusCode)
	}
	return fmt.Sprintf("Proxy %s: FAILED (%v)", r.ProxyID, r.Error)
}

func (r *CheckResult) IsHealthy() bool {
	return r.Success && r.Latency < 5*time.Second
}

func (r *CheckResult) GetScore() float64 {
	if !r.Success {
		return 0.0
	}

	baseScore := 1.0

	latencySeconds := r.Latency.Seconds()
	if latencySeconds < 1 {
		baseScore = 1.0
	} else if latencySeconds < 3 {
		baseScore = 0.8
	} else if latencySeconds < 5 {
		baseScore = 0.6
	} else if latencySeconds < 10 {
		baseScore = 0.4
	} else {
		baseScore = 0.2
	}

	if r.Anonymous {
		baseScore *= 1.1
	}

	if baseScore > 1.0 {
		baseScore = 1.0
	}

	return baseScore
}

func CheckProxies(ctx context.Context, proxies []*Proxy, config *CheckConfig) map[string]*CheckResult {
	checker := NewChecker(config)
	results := checker.CheckBatch(ctx, proxies)

	resultMap := make(map[string]*CheckResult)
	for _, result := range results {
		resultMap[result.ProxyID] = result
	}

	return resultMap
}

func FilterHealthyProxies(results []*CheckResult) []*CheckResult {
	healthy := make([]*CheckResult, 0)
	for _, result := range results {
		if result.IsHealthy() {
			healthy = append(healthy, result)
		}
	}
	return healthy
}

func GetFastestProxy(results []*CheckResult) *CheckResult {
	if len(results) == 0 {
		return nil
	}

	fastest := results[0]
	for _, result := range results {
		if result.Success && result.Latency < fastest.Latency {
			fastest = result
		}
	}

	if !fastest.Success {
		return nil
	}

	return fastest
}

func GetAverageLatency(results []*CheckResult) time.Duration {
	if len(results) == 0 {
		return 0
	}

	var total time.Duration
	count := 0

	for _, result := range results {
		if result.Success {
			total += result.Latency
			count++
		}
	}

	if count == 0 {
		return 0
	}

	return total / time.Duration(count)
}

func GetSuccessRate(results []*CheckResult) float64 {
	if len(results) == 0 {
		return 0
	}

	successCount := 0
	for _, result := range results {
		if result.Success {
			successCount++
		}
	}

	return float64(successCount) / float64(len(results))
}
