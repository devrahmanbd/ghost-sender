package proxypool

import (
        "context"
        "errors"
        "fmt"
        "net/url"
        "sync"
        "time"
)

var (
        ErrEmptyPool        = errors.New("proxy pool is empty")
        ErrProxyNotFound    = errors.New("proxy not found in pool")
        ErrInvalidProxy     = errors.New("invalid proxy configuration")
        ErrAllProxiesFailed = errors.New("all proxies are unavailable")
        ErrPoolClosed       = errors.New("proxy pool is closed")
)

type ProxyType string

const (
        ProxyTypeHTTP   ProxyType = "http"
        ProxyTypeHTTPS  ProxyType = "https"
        ProxyTypeSOCKS5 ProxyType = "socks5"
)

type ProxyStatus string

const (
        ProxyStatusHealthy     ProxyStatus = "healthy"
        ProxyStatusUnhealthy   ProxyStatus = "unhealthy"
        ProxyStatusUntested    ProxyStatus = "untested"
        ProxyStatusSuspended   ProxyStatus = "suspended"
)

type RotationStrategy string

const (
        RotationRoundRobin  RotationStrategy = "round_robin"
        RotationRandom      RotationStrategy = "random"
        RotationWeighted    RotationStrategy = "weighted"
        RotationLeastUsed   RotationStrategy = "least_used"
        RotationHealthBased RotationStrategy = "health_based"
)

type Proxy struct {
        ID            string
        Type          ProxyType
        Host          string
        Port          int
        Username      string
        Password      string
        URL           string
        Status        ProxyStatus
        HealthScore   float64
        LastChecked   time.Time
        LastUsed      time.Time
        TotalRequests int64
        FailedCount   int64
        SuccessCount  int64
        AvgLatency    time.Duration
        Weight        int
        Enabled       bool
        CreatedAt     time.Time
        UpdatedAt     time.Time
        Metadata      map[string]interface{}
}

type PoolConfig struct {
        Strategy           RotationStrategy
        HealthCheckEnabled bool
        HealthCheckInterval time.Duration
        HealthCheckTimeout  time.Duration
        MaxRetries          int
        RetryDelay          time.Duration
        SuspendThreshold    int
        SuspendDuration     time.Duration
        MinHealthScore      float64
        EnableAutoRemove    bool
        MaxProxies          int
}

type PoolStats struct {
        TotalProxies     int
        HealthyProxies   int
        UnhealthyProxies int
        SuspendedProxies int
        TotalRequests    int64
        SuccessRequests  int64
        FailedRequests   int64
        AvgLatency       time.Duration
        LastRotation     time.Time
}

type Pool struct {
        proxies          []*Proxy
        config           *PoolConfig
        mu               sync.RWMutex
        currentIndex     int
        stats            *PoolStats
        closed           bool
        healthCheckStop  chan struct{}
        healthCheckDone  chan struct{}
        ctx              context.Context
        cancel           context.CancelFunc
}

func NewPool(config *PoolConfig) *Pool {
        if config == nil {
                config = DefaultPoolConfig()
        }

        ctx, cancel := context.WithCancel(context.Background())

        pool := &Pool{
                proxies:         make([]*Proxy, 0),
                config:          config,
                currentIndex:    0,
                stats:           &PoolStats{},
                closed:          false,
                healthCheckStop: make(chan struct{}),
                healthCheckDone: make(chan struct{}),
                ctx:             ctx,
                cancel:          cancel,
        }

        if config.HealthCheckEnabled {
                go pool.startHealthCheck()
        }

        return pool
}

func DefaultPoolConfig() *PoolConfig {
        return &PoolConfig{
                Strategy:            RotationRoundRobin,
                HealthCheckEnabled:  true,
                HealthCheckInterval: 5 * time.Minute,
                HealthCheckTimeout:  10 * time.Second,
                MaxRetries:          3,
                RetryDelay:          1 * time.Second,
                SuspendThreshold:    5,
                SuspendDuration:     30 * time.Minute,
                MinHealthScore:      0.5,
                EnableAutoRemove:    false,
                MaxProxies:          100,
        }
}

func (p *Pool) Add(proxy *Proxy) error {
        if proxy == nil {
                return ErrInvalidProxy
        }

        p.mu.Lock()
        defer p.mu.Unlock()

        if p.closed {
                return ErrPoolClosed
        }

        if p.config.MaxProxies > 0 && len(p.proxies) >= p.config.MaxProxies {
                return errors.New("proxy pool is full")
        }

        if proxy.ID == "" {
                proxy.ID = generateProxyID(proxy)
        }

        if proxy.Status == "" {
                proxy.Status = ProxyStatusUntested
        }

        if proxy.HealthScore == 0 {
                proxy.HealthScore = 1.0
        }

        if proxy.Weight == 0 {
                proxy.Weight = 1
        }

        proxy.Enabled = true
        proxy.CreatedAt = time.Now()
        proxy.UpdatedAt = time.Now()

        p.proxies = append(p.proxies, proxy)
        p.updateStats()

        return nil
}

func (p *Pool) AddBulk(proxies []*Proxy) []error {
        errors := make([]error, 0)

        for _, proxy := range proxies {
                if err := p.Add(proxy); err != nil {
                        errors = append(errors, err)
                }
        }

        return errors
}

func (p *Pool) Remove(proxyID string) error {
        p.mu.Lock()
        defer p.mu.Unlock()

        if p.closed {
                return ErrPoolClosed
        }

        for i, proxy := range p.proxies {
                if proxy.ID == proxyID {
                        p.proxies = append(p.proxies[:i], p.proxies[i+1:]...)
                        p.updateStats()
                        return nil
                }
        }

        return ErrProxyNotFound
}

func (p *Pool) Get() (*Proxy, error) {
        p.mu.Lock()
        defer p.mu.Unlock()

        if p.closed {
                return nil, ErrPoolClosed
        }

        if len(p.proxies) == 0 {
                return nil, ErrEmptyPool
        }

        healthyProxies := p.getHealthyProxies()
        if len(healthyProxies) == 0 {
                return nil, ErrAllProxiesFailed
        }

        var selected *Proxy

        switch p.config.Strategy {
        case RotationRoundRobin:
                selected = p.getRoundRobin(healthyProxies)
        case RotationRandom:
                selected = p.getRandom(healthyProxies)
        case RotationWeighted:
                selected = p.getWeighted(healthyProxies)
        case RotationLeastUsed:
                selected = p.getLeastUsed(healthyProxies)
        case RotationHealthBased:
                selected = p.getHealthBased(healthyProxies)
        default:
                selected = p.getRoundRobin(healthyProxies)
        }

        if selected != nil {
                selected.LastUsed = time.Now()
                selected.TotalRequests++
                p.stats.TotalRequests++
                p.stats.LastRotation = time.Now()
        }

        return selected, nil
}

func (p *Pool) GetByID(proxyID string) (*Proxy, error) {
        p.mu.RLock()
        defer p.mu.RUnlock()

        if p.closed {
                return nil, ErrPoolClosed
        }

        for _, proxy := range p.proxies {
                if proxy.ID == proxyID {
                        return proxy, nil
                }
        }

        return nil, ErrProxyNotFound
}

func (p *Pool) GetAll() []*Proxy {
        p.mu.RLock()
        defer p.mu.RUnlock()

        result := make([]*Proxy, len(p.proxies))
        copy(result, p.proxies)
        return result
}

func (p *Pool) GetHealthy() []*Proxy {
        p.mu.RLock()
        defer p.mu.RUnlock()

        return p.getHealthyProxies()
}

func (p *Pool) MarkSuccess(proxyID string, latency time.Duration) error {
        p.mu.Lock()
        defer p.mu.Unlock()

        proxy, err := p.findProxyByID(proxyID)
        if err != nil {
                return err
        }

        proxy.SuccessCount++
        proxy.FailedCount = 0
        proxy.Status = ProxyStatusHealthy
        proxy.LastUsed = time.Now()
        proxy.UpdatedAt = time.Now()

        if proxy.AvgLatency == 0 {
                proxy.AvgLatency = latency
        } else {
                proxy.AvgLatency = (proxy.AvgLatency + latency) / 2
        }

        proxy.HealthScore = calculateHealthScore(proxy)
        p.stats.SuccessRequests++
        p.updateStats()

        return nil
}

func (p *Pool) MarkFailure(proxyID string) error {
        p.mu.Lock()
        defer p.mu.Unlock()

        proxy, err := p.findProxyByID(proxyID)
        if err != nil {
                return err
        }

        proxy.FailedCount++
        proxy.UpdatedAt = time.Now()

        if proxy.FailedCount >= int64(p.config.SuspendThreshold) {
                proxy.Status = ProxyStatusSuspended
                proxy.Enabled = false
        } else {
                proxy.Status = ProxyStatusUnhealthy
        }

        proxy.HealthScore = calculateHealthScore(proxy)
        p.stats.FailedRequests++
        p.updateStats()

        return nil
}

func (p *Pool) Enable(proxyID string) error {
        p.mu.Lock()
        defer p.mu.Unlock()

        proxy, err := p.findProxyByID(proxyID)
        if err != nil {
                return err
        }

        proxy.Enabled = true
        proxy.Status = ProxyStatusUntested
        proxy.FailedCount = 0
        proxy.UpdatedAt = time.Now()
        p.updateStats()

        return nil
}

func (p *Pool) Disable(proxyID string) error {
        p.mu.Lock()
        defer p.mu.Unlock()

        proxy, err := p.findProxyByID(proxyID)
        if err != nil {
                return err
        }

        proxy.Enabled = false
        proxy.UpdatedAt = time.Now()
        p.updateStats()

        return nil
}

func (p *Pool) Suspend(proxyID string, duration time.Duration) error {
        p.mu.Lock()
        defer p.mu.Unlock()

        proxy, err := p.findProxyByID(proxyID)
        if err != nil {
                return err
        }

        proxy.Status = ProxyStatusSuspended
        proxy.Enabled = false
        proxy.UpdatedAt = time.Now()
        p.updateStats()

        if duration > 0 {
                go func() {
                        time.Sleep(duration)
                        p.Enable(proxyID)
                }()
        }

        return nil
}

func (p *Pool) ResetStats(proxyID string) error {
        p.mu.Lock()
        defer p.mu.Unlock()

        proxy, err := p.findProxyByID(proxyID)
        if err != nil {
                return err
        }

        proxy.TotalRequests = 0
        proxy.SuccessCount = 0
        proxy.FailedCount = 0
        proxy.AvgLatency = 0
        proxy.HealthScore = 1.0
        proxy.UpdatedAt = time.Now()

        return nil
}

func (p *Pool) Clear() error {
        p.mu.Lock()
        defer p.mu.Unlock()

        if p.closed {
                return ErrPoolClosed
        }

        p.proxies = make([]*Proxy, 0)
        p.currentIndex = 0
        p.updateStats()

        return nil
}

func (p *Pool) Size() int {
        p.mu.RLock()
        defer p.mu.RUnlock()

        return len(p.proxies)
}

func (p *Pool) GetStats() *PoolStats {
        p.mu.RLock()
        defer p.mu.RUnlock()

        statsCopy := *p.stats
        return &statsCopy
}

func (p *Pool) SetStrategy(strategy RotationStrategy) {
        p.mu.Lock()
        defer p.mu.Unlock()

        p.config.Strategy = strategy
}

func (p *Pool) Close() error {
        p.mu.Lock()
        defer p.mu.Unlock()

        if p.closed {
                return ErrPoolClosed
        }

        p.closed = true
        p.cancel()

        if p.config.HealthCheckEnabled {
                close(p.healthCheckStop)
                <-p.healthCheckDone
        }

        return nil
}

func (p *Pool) getHealthyProxies() []*Proxy {
        healthy := make([]*Proxy, 0)

        for _, proxy := range p.proxies {
                if proxy.Enabled && 
                   proxy.Status != ProxyStatusSuspended &&
                   proxy.HealthScore >= p.config.MinHealthScore {
                        healthy = append(healthy, proxy)
                }
        }

        return healthy
}

func (p *Pool) getRoundRobin(proxies []*Proxy) *Proxy {
        if len(proxies) == 0 {
                return nil
        }

        if p.currentIndex >= len(proxies) {
                p.currentIndex = 0
        }

        proxy := proxies[p.currentIndex]
        p.currentIndex++

        return proxy
}

func (p *Pool) getRandom(proxies []*Proxy) *Proxy {
        if len(proxies) == 0 {
                return nil
        }

        return proxies[randomInt(0, len(proxies))]
}

func (p *Pool) getWeighted(proxies []*Proxy) *Proxy {
        if len(proxies) == 0 {
                return nil
        }

        totalWeight := 0
        for _, proxy := range proxies {
                totalWeight += proxy.Weight
        }

        if totalWeight == 0 {
                return proxies[0]
        }

        r := randomInt(0, totalWeight)
        cumulative := 0

        for _, proxy := range proxies {
                cumulative += proxy.Weight
                if r < cumulative {
                        return proxy
                }
        }

        return proxies[len(proxies)-1]
}

func (p *Pool) getLeastUsed(proxies []*Proxy) *Proxy {
        if len(proxies) == 0 {
                return nil
        }

        leastUsed := proxies[0]
        for _, proxy := range proxies {
                if proxy.TotalRequests < leastUsed.TotalRequests {
                        leastUsed = proxy
                }
        }

        return leastUsed
}

func (p *Pool) getHealthBased(proxies []*Proxy) *Proxy {
        if len(proxies) == 0 {
                return nil
        }

        totalScore := 0.0
        for _, proxy := range proxies {
                totalScore += proxy.HealthScore
        }

        if totalScore == 0 {
                return proxies[0]
        }

        r := randomFloat(0, totalScore)
        cumulative := 0.0

        for _, proxy := range proxies {
                cumulative += proxy.HealthScore
                if r < cumulative {
                        return proxy
                }
        }

        return proxies[len(proxies)-1]
}

func (p *Pool) findProxyByID(proxyID string) (*Proxy, error) {
        for _, proxy := range p.proxies {
                if proxy.ID == proxyID {
                        return proxy, nil
                }
        }
        return nil, ErrProxyNotFound
}

func (p *Pool) updateStats() {
        p.stats.TotalProxies = len(p.proxies)
        p.stats.HealthyProxies = 0
        p.stats.UnhealthyProxies = 0
        p.stats.SuspendedProxies = 0

        var totalLatency time.Duration
        latencyCount := 0

        for _, proxy := range p.proxies {
                switch proxy.Status {
                case ProxyStatusHealthy:
                        p.stats.HealthyProxies++
                case ProxyStatusUnhealthy:
                        p.stats.UnhealthyProxies++
                case ProxyStatusSuspended:
                        p.stats.SuspendedProxies++
                }

                if proxy.AvgLatency > 0 {
                        totalLatency += proxy.AvgLatency
                        latencyCount++
                }
        }

        if latencyCount > 0 {
                p.stats.AvgLatency = totalLatency / time.Duration(latencyCount)
        }
}

func (p *Pool) startHealthCheck() {
        ticker := time.NewTicker(p.config.HealthCheckInterval)
        defer ticker.Stop()
        defer close(p.healthCheckDone)

        for {
                select {
                case <-ticker.C:
                        p.performHealthCheck()
                case <-p.healthCheckStop:
                        return
                case <-p.ctx.Done():
                        return
                }
        }
}

func (p *Pool) performHealthCheck() {
        proxies := p.GetAll()

        for _, proxy := range proxies {
                if !proxy.Enabled {
                        continue
                }

                if proxy.Status == ProxyStatusSuspended {
                        if time.Since(proxy.UpdatedAt) >= p.config.SuspendDuration {
                                p.Enable(proxy.ID)
                        }
                        continue
                }

                go p.checkProxyHealth(proxy)
        }
}

func (p *Pool) checkProxyHealth(proxy *Proxy) {
        ctx, cancel := context.WithTimeout(p.ctx, p.config.HealthCheckTimeout)
        defer cancel()

        start := time.Now()
        err := testProxyConnection(ctx, proxy)
        latency := time.Since(start)

        if err != nil {
                p.MarkFailure(proxy.ID)
        } else {
                p.MarkSuccess(proxy.ID, latency)
        }

        p.mu.Lock()
        proxy.LastChecked = time.Now()
        p.mu.Unlock()
}

func calculateHealthScore(proxy *Proxy) float64 {
        if proxy.TotalRequests == 0 {
                return 1.0
        }

        successRate := float64(proxy.SuccessCount) / float64(proxy.TotalRequests)

        consecutiveFailurePenalty := 1.0
        if proxy.FailedCount > 0 {
                consecutiveFailurePenalty = 1.0 / (1.0 + float64(proxy.FailedCount)*0.2)
        }

        score := successRate * consecutiveFailurePenalty

        if score < 0 {
                score = 0
        }
        if score > 1 {
                score = 1
        }

        return score
}

func generateProxyID(proxy *Proxy) string {
        return hashString(fmt.Sprintf("%s:%d", proxy.Host, proxy.Port))
}

func NewProxy(proxyType ProxyType, host string, port int, username, password string) (*Proxy, error) {
        if host == "" || port <= 0 {
                return nil, ErrInvalidProxy
        }

        proxyURL := buildProxyURL(proxyType, host, port, username, password)

        proxy := &Proxy{
                Type:        proxyType,
                Host:        host,
                Port:        port,
                Username:    username,
                Password:    password,
                URL:         proxyURL,
                Status:      ProxyStatusUntested,
                HealthScore: 1.0,
                Weight:      1,
                Enabled:     true,
                CreatedAt:   time.Now(),
                UpdatedAt:   time.Now(),
                Metadata:    make(map[string]interface{}),
        }

        proxy.ID = generateProxyID(proxy)

        return proxy, nil
}

func ParseProxy(proxyURL string) (*Proxy, error) {
        u, err := url.Parse(proxyURL)
        if err != nil {
                return nil, err
        }

        var proxyType ProxyType
        switch u.Scheme {
        case "http":
                proxyType = ProxyTypeHTTP
        case "https":
                proxyType = ProxyTypeHTTPS
        case "socks5":
                proxyType = ProxyTypeSOCKS5
        default:
                return nil, errors.New("unsupported proxy type: " + u.Scheme)
        }

        username := ""
        password := ""
        if u.User != nil {
                username = u.User.Username()
                password, _ = u.User.Password()
        }

        port := 0
        if u.Port() != "" {
                port = parseInt(u.Port())
        }

        return NewProxy(proxyType, u.Hostname(), port, username, password)
}

func buildProxyURL(proxyType ProxyType, host string, port int, username, password string) string {
        var scheme string
        switch proxyType {
        case ProxyTypeHTTP:
                scheme = "http"
        case ProxyTypeHTTPS:
                scheme = "https"
        case ProxyTypeSOCKS5:
                scheme = "socks5"
        default:
                scheme = "http"
        }

        auth := ""
        if username != "" {
                if password != "" {
                        auth = username + ":" + password + "@"
                } else {
                        auth = username + "@"
                }
        }

        return scheme + "://" + auth + host + ":" + intToString(port)
}

func (p *Proxy) GetURL() string {
        return p.URL
}

func (p *Proxy) IsHealthy() bool {
        return p.Enabled && 
               p.Status == ProxyStatusHealthy && 
               p.HealthScore >= 0.5
}

func (p *Proxy) Clone() *Proxy {
        clone := *p
        clone.Metadata = make(map[string]interface{})
        for k, v := range p.Metadata {
                clone.Metadata[k] = v
        }
        return &clone
}
