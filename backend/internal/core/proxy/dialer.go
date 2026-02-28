package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
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
	ErrDialFailed       = errors.New("dial failed")
	ErrConnectionClosed = errors.New("connection closed")
	ErrDialTimeout      = errors.New("dial timeout")
)

type ProxyDialer struct {
	mu              sync.RWMutex
	proxy           *models.Proxy
	timeout         time.Duration
	keepAlive       time.Duration
	fallbackDirect  bool
	tlsConfig       *tls.Config
	baseDialer      *net.Dialer
	connectionPool  *ConnectionPool
	metrics         *DialerMetrics
	authCache       map[string]string
}

type DialerConfig struct {
	Timeout            time.Duration
	KeepAlive          time.Duration
	FallbackDirect     bool
	TLSConfig          *tls.Config
	MaxIdleConns       int
	MaxConnsPerHost    int
	IdleConnTimeout    time.Duration
	EnableCompression  bool
	DisableKeepAlives  bool
	ForceAttemptHTTP2  bool
	DialContext        func(ctx context.Context, network, addr string) (net.Conn, error)
}

type DialerMetrics struct {
	mu                sync.RWMutex
	TotalDials        int64
	SuccessfulDials   int64
	FailedDials       int64
	TotalDialTime     time.Duration
	AverageDialTime   time.Duration
	ActiveConnections int64
}

type ConnectionPool struct {
	mu          sync.RWMutex
	connections map[string][]net.Conn
	maxIdle     int
	maxPerHost  int
	idleTimeout time.Duration
}

type ProxyConn struct {
	net.Conn
	proxy      *models.Proxy
	remoteAddr string
	localAddr  string
	createdAt  time.Time
	lastUsed   time.Time
	bytesSent  int64
	bytesRecv  int64
}

func NewProxyDialer(p *models.Proxy, config *DialerConfig) *ProxyDialer {
	if config == nil {
		config = DefaultDialerConfig()
	}

	baseDialer := &net.Dialer{
		Timeout:   config.Timeout,
		KeepAlive: config.KeepAlive,
	}

	if config.DialContext != nil {
		baseDialer.ControlContext = nil
	}

	return &ProxyDialer{
		proxy:          p,
		timeout:        config.Timeout,
		keepAlive:      config.KeepAlive,
		fallbackDirect: config.FallbackDirect,
		tlsConfig:      config.TLSConfig,
		baseDialer:     baseDialer,
		connectionPool: NewConnectionPool(config.MaxIdleConns, config.MaxConnsPerHost, config.IdleConnTimeout),
		metrics:        &DialerMetrics{},
		authCache:      make(map[string]string),
	}
}

func DefaultDialerConfig() *DialerConfig {
	return &DialerConfig{
		Timeout:           30 * time.Second,
		KeepAlive:         30 * time.Second,
		FallbackDirect:    false,
		MaxIdleConns:      100,
		MaxConnsPerHost:   10,
		IdleConnTimeout:   90 * time.Second,
		EnableCompression: true,
		DisableKeepAlives: false,
		ForceAttemptHTTP2: true,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}
}

func (pd *ProxyDialer) Dial(network, address string) (net.Conn, error) {
	return pd.DialContext(context.Background(), network, address)
}

func (pd *ProxyDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	startTime := time.Now()
	pd.recordDialAttempt()

	if ctx == nil {
		ctx = context.Background()
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, pd.timeout)
	defer cancel()

	var conn net.Conn
	var err error

	switch pd.proxy.Type {
	case models.ProxyTypeHTTP, models.ProxyTypeHTTPS:
		conn, err = pd.dialHTTP(timeoutCtx, network, address)
	case models.ProxyTypeSOCKS5:
		conn, err = pd.dialSOCKS5(timeoutCtx, network, address)
	default:
		if pd.fallbackDirect {
			conn, err = pd.dialDirect(timeoutCtx, network, address)
		} else {
			err = fmt.Errorf("unsupported proxy type: %s", pd.proxy.Type)
		}
	}

	if err != nil {
		pd.recordDialFailure()
		return nil, err
	}

	pd.recordDialSuccess(time.Since(startTime))

	proxyConn := &ProxyConn{
		Conn:       conn,
		proxy:      pd.proxy,
		remoteAddr: address,
		localAddr:  conn.LocalAddr().String(),
		createdAt:  time.Now(),
		lastUsed:   time.Now(),
	}

	return proxyConn, nil
}

func (pd *ProxyDialer) dialHTTP(ctx context.Context, network, address string) (net.Conn, error) {
	proxyAddr := fmt.Sprintf("%s:%d", pd.proxy.Host, pd.proxy.Port)

	conn, err := pd.baseDialer.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to proxy: %w", err)
	}

	connectReq := &http.Request{
		Method: "CONNECT",
		URL:    &url.URL{Opaque: address},
		Host:   address,
		Header: make(http.Header),
	}

	if pd.proxy.Username != "" && pd.proxy.Password != "" {
		auth := pd.proxy.Username + ":" + pd.proxy.Password
		basicAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
		connectReq.Header.Set("Proxy-Authorization", basicAuth)
	}

	connectReq.Header.Set("User-Agent", "Mozilla/5.0")

	if err := connectReq.Write(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to send CONNECT request: %w", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, connectReq)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to read CONNECT response: %w", err)
	}

	if resp.StatusCode != 200 {
		conn.Close()
		return nil, fmt.Errorf("proxy returned non-200 status: %d", resp.StatusCode)
	}

	return conn, nil
}

func (pd *ProxyDialer) dialSOCKS5(ctx context.Context, network, address string) (net.Conn, error) {
	proxyAddr := fmt.Sprintf("%s:%d", pd.proxy.Host, pd.proxy.Port)

	var auth *proxy.Auth
	if pd.proxy.Username != "" && pd.proxy.Password != "" {
		auth = &proxy.Auth{
			User:     pd.proxy.Username,
			Password: pd.proxy.Password,
		}
	}

	dialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, pd.baseDialer)
	if err != nil {
		return nil, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
	}

	contextDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil, errors.New("SOCKS5 dialer does not support context")
	}

	conn, err := contextDialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, fmt.Errorf("SOCKS5 dial failed: %w", err)
	}

	return conn, nil
}

func (pd *ProxyDialer) dialDirect(ctx context.Context, network, address string) (net.Conn, error) {
	return pd.baseDialer.DialContext(ctx, network, address)
}

func (pd *ProxyDialer) DialTLS(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := pd.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}

	host, _, err := net.SplitHostPort(address)
	if err != nil {
		conn.Close()
		return nil, err
	}

	tlsConfig := pd.tlsConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{}
	}

	if tlsConfig.ServerName == "" {
		tlsConfig = tlsConfig.Clone()
		tlsConfig.ServerName = host
	}

	tlsConn := tls.Client(conn, tlsConfig)

	if err := tlsConn.HandshakeContext(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("TLS handshake failed: %w", err)
	}

	return tlsConn, nil
}

func (pd *ProxyDialer) recordDialAttempt() {
	pd.metrics.mu.Lock()
	defer pd.metrics.mu.Unlock()
	pd.metrics.TotalDials++
}

func (pd *ProxyDialer) recordDialSuccess(duration time.Duration) {
	pd.metrics.mu.Lock()
	defer pd.metrics.mu.Unlock()
	pd.metrics.SuccessfulDials++
	pd.metrics.TotalDialTime += duration
	pd.metrics.AverageDialTime = time.Duration(int64(pd.metrics.TotalDialTime) / pd.metrics.SuccessfulDials)
	pd.metrics.ActiveConnections++
}

func (pd *ProxyDialer) recordDialFailure() {
	pd.metrics.mu.Lock()
	defer pd.metrics.mu.Unlock()
	pd.metrics.FailedDials++
}

func (pd *ProxyDialer) GetMetrics() *DialerMetrics {
	pd.metrics.mu.RLock()
	defer pd.metrics.mu.RUnlock()

	return &DialerMetrics{
		TotalDials:        pd.metrics.TotalDials,
		SuccessfulDials:   pd.metrics.SuccessfulDials,
		FailedDials:       pd.metrics.FailedDials,
		TotalDialTime:     pd.metrics.TotalDialTime,
		AverageDialTime:   pd.metrics.AverageDialTime,
		ActiveConnections: pd.metrics.ActiveConnections,
	}
}

func (pd *ProxyDialer) ResetMetrics() {
	pd.metrics.mu.Lock()
	defer pd.metrics.mu.Unlock()

	pd.metrics.TotalDials = 0
	pd.metrics.SuccessfulDials = 0
	pd.metrics.FailedDials = 0
	pd.metrics.TotalDialTime = 0
	pd.metrics.AverageDialTime = 0
}

func (pd *ProxyDialer) SetTimeout(timeout time.Duration) {
	pd.mu.Lock()
	defer pd.mu.Unlock()
	pd.timeout = timeout
	pd.baseDialer.Timeout = timeout
}

func (pd *ProxyDialer) SetKeepAlive(keepAlive time.Duration) {
	pd.mu.Lock()
	defer pd.mu.Unlock()
	pd.keepAlive = keepAlive
	pd.baseDialer.KeepAlive = keepAlive
}

func (pd *ProxyDialer) SetTLSConfig(config *tls.Config) {
	pd.mu.Lock()
	defer pd.mu.Unlock()
	pd.tlsConfig = config
}

func NewConnectionPool(maxIdle, maxPerHost int, idleTimeout time.Duration) *ConnectionPool {
	return &ConnectionPool{
		connections: make(map[string][]net.Conn),
		maxIdle:     maxIdle,
		maxPerHost:  maxPerHost,
		idleTimeout: idleTimeout,
	}
}

func (cp *ConnectionPool) Get(key string) net.Conn {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	conns, exists := cp.connections[key]
	if !exists || len(conns) == 0 {
		return nil
	}

	conn := conns[len(conns)-1]
	cp.connections[key] = conns[:len(conns)-1]

	return conn
}

func (cp *ConnectionPool) Put(key string, conn net.Conn) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	conns := cp.connections[key]
	if len(conns) >= cp.maxPerHost {
		conn.Close()
		return
	}

	cp.connections[key] = append(conns, conn)
}

func (cp *ConnectionPool) CloseAll() {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	for key, conns := range cp.connections {
		for _, conn := range conns {
			conn.Close()
		}
		delete(cp.connections, key)
	}
}

func (cp *ConnectionPool) CloseIdleConnections() {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	for key, conns := range cp.connections {
		for i, conn := range conns {
			if proxyConn, ok := conn.(*ProxyConn); ok {
				if time.Since(proxyConn.lastUsed) > cp.idleTimeout {
					conn.Close()
					cp.connections[key] = append(conns[:i], conns[i+1:]...)
				}
			}
		}
	}
}

func (pc *ProxyConn) Read(b []byte) (n int, err error) {
	n, err = pc.Conn.Read(b)
	pc.bytesRecv += int64(n)
	pc.lastUsed = time.Now()
	return
}

func (pc *ProxyConn) Write(b []byte) (n int, err error) {
	n, err = pc.Conn.Write(b)
	pc.bytesSent += int64(n)
	pc.lastUsed = time.Now()
	return
}

func (pc *ProxyConn) Close() error {
	return pc.Conn.Close()
}

func (pc *ProxyConn) GetMetrics() (bytesSent, bytesRecv int64, duration time.Duration) {
	return pc.bytesSent, pc.bytesRecv, time.Since(pc.createdAt)
}

func CreateHTTPTransport(p *models.Proxy, config *DialerConfig) *http.Transport {
	dialer := NewProxyDialer(p, config)

	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     config.ForceAttemptHTTP2,
		MaxIdleConns:          config.MaxIdleConns,
		MaxIdleConnsPerHost:   config.MaxConnsPerHost,
		IdleConnTimeout:       config.IdleConnTimeout,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     config.DisableKeepAlives,
		DisableCompression:    !config.EnableCompression,
	}

	if config.TLSConfig != nil {
		transport.TLSClientConfig = config.TLSConfig
	}

	return transport
}

func TestProxyConnection(p *models.Proxy, timeout time.Duration) error {
	config := &DialerConfig{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	dialer := NewProxyDialer(p, config)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := dialer.DialContext(ctx, "tcp", "www.google.com:80")
	if err != nil {
		return err
	}
	defer conn.Close()

	return nil
}

func ParseProxyURL(proxyURL string) (*models.Proxy, error) {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}

	proxy := &models.Proxy{
		Host: u.Hostname(),
	}

	port := u.Port()
	if port == "" {
		port = "80"
	}
	fmt.Sscanf(port, "%d", &proxy.Port)

	if u.User != nil {
		proxy.Username = u.User.Username()
		proxy.Password, _ = u.User.Password()
	}

	switch strings.ToLower(u.Scheme) {
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

func (dm *DialerMetrics) GetSuccessRate() float64 {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	if dm.TotalDials == 0 {
		return 0
	}

	return float64(dm.SuccessfulDials) / float64(dm.TotalDials) * 100
}
