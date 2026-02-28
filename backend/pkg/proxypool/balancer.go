package proxypool

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash/fnv"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrNoBackends      = errors.New("no backend proxies available")
	ErrInvalidAlgorithm = errors.New("invalid balancing algorithm")
)

type BalanceAlgorithm string

const (
	AlgorithmRoundRobin       BalanceAlgorithm = "round_robin"
	AlgorithmLeastConnections BalanceAlgorithm = "least_connections"
	AlgorithmWeightedRandom   BalanceAlgorithm = "weighted_random"
	AlgorithmIPHash           BalanceAlgorithm = "ip_hash"
	AlgorithmResponseTime     BalanceAlgorithm = "response_time"
	AlgorithmRandom           BalanceAlgorithm = "random"
	AlgorithmPowerOfTwo       BalanceAlgorithm = "power_of_two"
)

type Backend struct {
	Proxy           *Proxy
	Connections     int64
	TotalConnections int64
	Weight          int
	EffectiveWeight int
	CurrentWeight   int
	Failures        int64
	LastFailure     time.Time
	LastSuccess     time.Time
	ResponseTimes   []time.Duration
	mu              sync.RWMutex
}

type BalancerConfig struct {
	Algorithm           BalanceAlgorithm
	StickySession       bool
	SessionTimeout      time.Duration
	HealthCheckInterval time.Duration
	MaxFailures         int
	FailureTimeout      time.Duration
	WeightAdjustment    bool
}

type Balancer struct {
	backends        []*Backend
	algorithm       BalanceAlgorithm
	config          *BalancerConfig
	currentIndex    uint64
	sessions        map[string]*Backend
	sessionMu       sync.RWMutex
	mu              sync.RWMutex
	stats           *BalancerStats
	rand            *rand.Rand
}

type BalancerStats struct {
	TotalRequests    int64
	SuccessRequests  int64
	FailedRequests   int64
	TotalBackends    int
	ActiveBackends   int
	TotalConnections int64
	LastBalanced     time.Time
}

func NewBalancer(config *BalancerConfig) *Balancer {
	if config == nil {
		config = DefaultBalancerConfig()
	}

	return &Balancer{
		backends:  make([]*Backend, 0),
		algorithm: config.Algorithm,
		config:    config,
		sessions:  make(map[string]*Backend),
		stats:     &BalancerStats{},
		rand:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func DefaultBalancerConfig() *BalancerConfig {
	return &BalancerConfig{
		Algorithm:           AlgorithmRoundRobin,
		StickySession:       false,
		SessionTimeout:      30 * time.Minute,
		HealthCheckInterval: 5 * time.Minute,
		MaxFailures:         3,
		FailureTimeout:      1 * time.Minute,
		WeightAdjustment:    true,
	}
}

func (b *Balancer) AddBackend(proxy *Proxy, weight int) error {
	if proxy == nil {
		return ErrInvalidProxy
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if weight <= 0 {
		weight = 1
	}

	backend := &Backend{
		Proxy:           proxy,
		Weight:          weight,
		EffectiveWeight: weight,
		CurrentWeight:   0,
		ResponseTimes:   make([]time.Duration, 0, 10),
	}

	b.backends = append(b.backends, backend)
	b.updateStats()

	return nil
}

func (b *Balancer) RemoveBackend(proxyID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, backend := range b.backends {
		if backend.Proxy.ID == proxyID {
			b.backends = append(b.backends[:i], b.backends[i+1:]...)
			b.updateStats()
			return nil
		}
	}

	return ErrProxyNotFound
}

func (b *Balancer) Select(clientID string) (*Proxy, error) {
	if b.config.StickySession && clientID != "" {
		if backend := b.getSession(clientID); backend != nil {
			if b.isBackendHealthy(backend) {
				b.incrementConnections(backend)
				return backend.Proxy, nil
			}
		}
	}

	backend, err := b.selectBackend(clientID)
	if err != nil {
		return nil, err
	}

	if b.config.StickySession && clientID != "" {
		b.setSession(clientID, backend)
	}

	b.incrementConnections(backend)
	atomic.AddInt64(&b.stats.TotalRequests, 1)
	b.stats.LastBalanced = time.Now()

	return backend.Proxy, nil
}

func (b *Balancer) selectBackend(clientID string) (*Backend, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	healthyBackends := b.getHealthyBackends()
	if len(healthyBackends) == 0 {
		return nil, ErrNoBackends
	}

	switch b.algorithm {
	case AlgorithmRoundRobin:
		return b.roundRobin(healthyBackends), nil
	case AlgorithmLeastConnections:
		return b.leastConnections(healthyBackends), nil
	case AlgorithmWeightedRandom:
		return b.weightedRandom(healthyBackends), nil
	case AlgorithmIPHash:
		return b.ipHash(clientID, healthyBackends), nil
	case AlgorithmResponseTime:
		return b.responseTime(healthyBackends), nil
	case AlgorithmRandom:
		return b.random(healthyBackends), nil
	case AlgorithmPowerOfTwo:
		return b.powerOfTwo(healthyBackends), nil
	default:
		return b.roundRobin(healthyBackends), nil
	}
}

func (b *Balancer) roundRobin(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}

	index := atomic.AddUint64(&b.currentIndex, 1) - 1
	return backends[int(index)%len(backends)]
}

func (b *Balancer) leastConnections(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}

	var selected *Backend
	minConnections := int64(^uint64(0) >> 1)

	for _, backend := range backends {
		connections := atomic.LoadInt64(&backend.Connections)
		if connections < minConnections {
			minConnections = connections
			selected = backend
		}
	}

	return selected
}

func (b *Balancer) weightedRandom(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}

	totalWeight := 0
	for _, backend := range backends {
		totalWeight += backend.EffectiveWeight
	}

	if totalWeight == 0 {
		return backends[0]
	}

	r := b.rand.Intn(totalWeight)
	cumulative := 0

	for _, backend := range backends {
		cumulative += backend.EffectiveWeight
		if r < cumulative {
			return backend
		}
	}

	return backends[len(backends)-1]
}

func (b *Balancer) ipHash(clientID string, backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}

	if clientID == "" {
		return backends[0]
	}

	hash := fnv.New32a()
	hash.Write([]byte(clientID))
	index := int(hash.Sum32()) % len(backends)

	return backends[index]
}

func (b *Balancer) responseTime(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}

	var selected *Backend
	minResponseTime := time.Duration(^uint64(0) >> 1)

	for _, backend := range backends {
		backend.mu.RLock()
		avgTime := b.calculateAverageResponseTime(backend)
		backend.mu.RUnlock()

		if avgTime < minResponseTime {
			minResponseTime = avgTime
			selected = backend
		}
	}

	if selected == nil {
		return backends[0]
	}

	return selected
}

func (b *Balancer) random(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}

	return backends[b.rand.Intn(len(backends))]
}

func (b *Balancer) powerOfTwo(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}

	if len(backends) == 1 {
		return backends[0]
	}

	i := b.rand.Intn(len(backends))
	j := b.rand.Intn(len(backends))

	for i == j && len(backends) > 1 {
		j = b.rand.Intn(len(backends))
	}

	backend1 := backends[i]
	backend2 := backends[j]

	conn1 := atomic.LoadInt64(&backend1.Connections)
	conn2 := atomic.LoadInt64(&backend2.Connections)

	if conn1 <= conn2 {
		return backend1
	}
	return backend2
}

func (b *Balancer) Release(proxyID string) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, backend := range b.backends {
		if backend.Proxy.ID == proxyID {
			b.decrementConnections(backend)
			return nil
		}
	}

	return ErrProxyNotFound
}

func (b *Balancer) MarkSuccess(proxyID string, responseTime time.Duration) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, backend := range b.backends {
		if backend.Proxy.ID == proxyID {
			atomic.StoreInt64(&backend.Failures, 0)
			backend.LastSuccess = time.Now()

			backend.mu.Lock()
			if len(backend.ResponseTimes) >= 10 {
				backend.ResponseTimes = backend.ResponseTimes[1:]
			}
			backend.ResponseTimes = append(backend.ResponseTimes, responseTime)
			backend.mu.Unlock()

			if b.config.WeightAdjustment {
				b.adjustWeight(backend, true)
			}

			atomic.AddInt64(&b.stats.SuccessRequests, 1)
			return nil
		}
	}

	return ErrProxyNotFound
}

func (b *Balancer) MarkFailure(proxyID string) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, backend := range b.backends {
		if backend.Proxy.ID == proxyID {
			atomic.AddInt64(&backend.Failures, 1)
			backend.LastFailure = time.Now()

			if b.config.WeightAdjustment {
				b.adjustWeight(backend, false)
			}

			atomic.AddInt64(&b.stats.FailedRequests, 1)
			return nil
		}
	}

	return ErrProxyNotFound
}

func (b *Balancer) GetStats() *BalancerStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	stats := *b.stats
	stats.TotalBackends = len(b.backends)
	stats.ActiveBackends = len(b.getHealthyBackends())

	totalConns := int64(0)
	for _, backend := range b.backends {
		totalConns += atomic.LoadInt64(&backend.Connections)
	}
	stats.TotalConnections = totalConns

	return &stats
}

func (b *Balancer) GetBackends() []*Backend {
	b.mu.RLock()
	defer b.mu.RUnlock()

	backends := make([]*Backend, len(b.backends))
	copy(backends, b.backends)
	return backends
}

func (b *Balancer) SetAlgorithm(algorithm BalanceAlgorithm) error {
	validAlgorithms := map[BalanceAlgorithm]bool{
		AlgorithmRoundRobin:       true,
		AlgorithmLeastConnections: true,
		AlgorithmWeightedRandom:   true,
		AlgorithmIPHash:           true,
		AlgorithmResponseTime:     true,
		AlgorithmRandom:           true,
		AlgorithmPowerOfTwo:       true,
	}

	if !validAlgorithms[algorithm] {
		return ErrInvalidAlgorithm
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.algorithm = algorithm
	b.config.Algorithm = algorithm

	return nil
}

func (b *Balancer) getHealthyBackends() []*Backend {
	healthy := make([]*Backend, 0)

	for _, backend := range b.backends {
		if b.isBackendHealthy(backend) {
			healthy = append(healthy, backend)
		}
	}

	return healthy
}

func (b *Balancer) isBackendHealthy(backend *Backend) bool {
	if !backend.Proxy.Enabled {
		return false
	}

	if backend.Proxy.Status == ProxyStatusSuspended {
		return false
	}

	failures := atomic.LoadInt64(&backend.Failures)
	if failures >= int64(b.config.MaxFailures) {
		if time.Since(backend.LastFailure) < b.config.FailureTimeout {
			return false
		}
		atomic.StoreInt64(&backend.Failures, 0)
	}

	return true
}

func (b *Balancer) incrementConnections(backend *Backend) {
	atomic.AddInt64(&backend.Connections, 1)
	atomic.AddInt64(&backend.TotalConnections, 1)
}

func (b *Balancer) decrementConnections(backend *Backend) {
	atomic.AddInt64(&backend.Connections, -1)
}

func (b *Balancer) getSession(clientID string) *Backend {
	b.sessionMu.RLock()
	defer b.sessionMu.RUnlock()

	return b.sessions[clientID]
}

func (b *Balancer) setSession(clientID string, backend *Backend) {
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()

	b.sessions[clientID] = backend

	go func() {
		time.Sleep(b.config.SessionTimeout)
		b.sessionMu.Lock()
		delete(b.sessions, clientID)
		b.sessionMu.Unlock()
	}()
}

func (b *Balancer) ClearSessions() {
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()

	b.sessions = make(map[string]*Backend)
}

func (b *Balancer) calculateAverageResponseTime(backend *Backend) time.Duration {
	if len(backend.ResponseTimes) == 0 {
		return time.Duration(0)
	}

	var total time.Duration
	for _, rt := range backend.ResponseTimes {
		total += rt
	}

	return total / time.Duration(len(backend.ResponseTimes))
}

func (b *Balancer) adjustWeight(backend *Backend, success bool) {
	backend.mu.Lock()
	defer backend.mu.Unlock()

	if success {
		if backend.EffectiveWeight < backend.Weight {
			backend.EffectiveWeight++
		}
	} else {
		backend.EffectiveWeight--
		if backend.EffectiveWeight < 0 {
			backend.EffectiveWeight = 0
		}
	}
}

func (b *Balancer) updateStats() {
	b.stats.TotalBackends = len(b.backends)
	b.stats.ActiveBackends = len(b.getHealthyBackends())
}

func (b *Balancer) ResetStats() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.stats = &BalancerStats{
		TotalBackends:  len(b.backends),
		ActiveBackends: len(b.getHealthyBackends()),
	}

	for _, backend := range b.backends {
		atomic.StoreInt64(&backend.Connections, 0)
		atomic.StoreInt64(&backend.TotalConnections, 0)
		atomic.StoreInt64(&backend.Failures, 0)
		backend.mu.Lock()
		backend.ResponseTimes = make([]time.Duration, 0, 10)
		backend.mu.Unlock()
	}
}

func (b *Balancer) GetBackendStats(proxyID string) map[string]interface{} {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, backend := range b.backends {
		if backend.Proxy.ID == proxyID {
			backend.mu.RLock()
			avgResponseTime := b.calculateAverageResponseTime(backend)
			backend.mu.RUnlock()

			return map[string]interface{}{
				"proxy_id":           backend.Proxy.ID,
				"connections":        atomic.LoadInt64(&backend.Connections),
				"total_connections":  atomic.LoadInt64(&backend.TotalConnections),
				"failures":           atomic.LoadInt64(&backend.Failures),
				"weight":             backend.Weight,
				"effective_weight":   backend.EffectiveWeight,
				"avg_response_time":  avgResponseTime,
				"last_success":       backend.LastSuccess,
				"last_failure":       backend.LastFailure,
			}
		}
	}

	return nil
}

func HashClientID(clientID string) string {
	hash := sha256.Sum256([]byte(clientID))
	return string(hash[:])
}

func (b *Backend) GetConnectionCount() int64 {
	return atomic.LoadInt64(&b.Connections)
}

func (b *Backend) GetTotalConnectionCount() int64 {
	return atomic.LoadInt64(&b.TotalConnections)
}

func (b *Backend) GetFailureCount() int64 {
	return atomic.LoadInt64(&b.Failures)
}

func (b *Backend) IsHealthy(maxFailures int, failureTimeout time.Duration) bool {
	failures := atomic.LoadInt64(&b.Failures)
	
	if failures >= int64(maxFailures) {
		if time.Since(b.LastFailure) < failureTimeout {
			return false
		}
	}

	return b.Proxy.Enabled && b.Proxy.Status != ProxyStatusSuspended
}

func hashString(s string) string {
	h := fnv.New32a()
	h.Write([]byte(s))
	return string(h.Sum(nil))
}

func randomInt(min, max int) int {
	return min + rand.Intn(max-min)
}

func randomFloat(min, max float64) float64 {
	return min + rand.Float64()*(max-min)
}

func parseInt(s string) int {
	var result int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		}
	}
	return result
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}

	var buf [20]byte
	i := len(buf) - 1
	
	for n > 0 {
		buf[i] = byte('0' + n%10)
		n /= 10
		i--
	}

	return string(buf[i+1:])
}

func CalculateHash(data []byte) string {
	hash := sha256.Sum256(data)
	result := make([]byte, len(hash)*2)
	
	const hexDigits = "0123456789abcdef"
	for i, b := range hash {
		result[i*2] = hexDigits[b>>4]
		result[i*2+1] = hexDigits[b&0x0f]
	}
	
	return string(result)
}

func uint64ToBytes(n uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, n)
	return buf
}

func bytesToUint64(b []byte) uint64 {
	if len(b) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}
