package proxy

import (
	"crypto/rand"
	"errors"
	"math/big"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"email-campaign-system/internal/models"
)

var (
	ErrNoStrategy = errors.New("no rotation strategy set")
)

type RotationStrategy string

const (
	StrategyRoundRobin  RotationStrategy = "round_robin"
	StrategyRandom      RotationStrategy = "random"
	StrategyWeighted    RotationStrategy = "weighted"
	StrategyHealthBased RotationStrategy = "health_based"
	StrategyLeastUsed   RotationStrategy = "least_used"
	StrategySticky      RotationStrategy = "sticky"
)

type ProxyRotator struct {
	mu        sync.RWMutex
	manager   *ProxyManager
	strategy  RotationStrategy
	index     int32
	weights   map[string]int
	stats     *RotationStats
	stickyMap map[string]string
	stickyTTL time.Duration
}

type RotationStats struct {
	mu               sync.RWMutex
	TotalRotations   uint64
	ProxyUsage       map[string]uint64
	LastRotation     time.Time
	StrategyChanges  uint64
	FailedRotations  uint64
	SuccessRotations uint64
}

type ProxyScore struct {
	Proxy       *models.Proxy
	Score       float64
	HealthScore float64
	UsageScore  float64
	WeightScore float64
}

func NewProxyRotator(manager *ProxyManager) *ProxyRotator {
	return &ProxyRotator{
		manager:   manager,
		strategy:  StrategyRoundRobin,
		index:     -1,
		weights:   make(map[string]int),
		stickyMap: make(map[string]string),
		stickyTTL: 5 * time.Minute,
		stats: &RotationStats{
			ProxyUsage: make(map[string]uint64),
		},
	}
}

func (pr *ProxyRotator) Next() (*models.Proxy, error) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	var proxy *models.Proxy
	var err error

	switch pr.strategy {
	case StrategyRoundRobin:
		proxy, err = pr.nextRoundRobin()
	case StrategyRandom:
		proxy, err = pr.nextRandom()
	case StrategyWeighted:
		proxy, err = pr.nextWeighted()
	case StrategyHealthBased:
		proxy, err = pr.nextHealthBased()
	case StrategyLeastUsed:
		proxy, err = pr.nextLeastUsed()
	case StrategySticky:
		proxy, err = pr.nextSticky()
	default:
		err = ErrNoStrategy
	}

	if err != nil {
		pr.recordFailedRotation()
		return nil, err
	}

	if proxy == nil {
		pr.recordFailedRotation()
		return nil, ErrNoProxiesAvailable
	}

	pr.recordRotation(proxy.ID)
	return proxy, nil
}

func (pr *ProxyRotator) nextRoundRobin() (*models.Proxy, error) {
	proxies := pr.getAvailableProxies()
	if len(proxies) == 0 {
		return nil, ErrNoProxiesAvailable
	}

	idx := atomic.AddInt32(&pr.index, 1)
	selectedProxy := proxies[int(idx)%len(proxies)]

	return selectedProxy, nil
}

func (pr *ProxyRotator) nextRandom() (*models.Proxy, error) {
	proxies := pr.getAvailableProxies()
	if len(proxies) == 0 {
		return nil, ErrNoProxiesAvailable
	}

	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(proxies))))
	if err != nil {
		return nil, err
	}

	return proxies[n.Int64()], nil
}

func (pr *ProxyRotator) nextWeighted() (*models.Proxy, error) {
	proxies := pr.getAvailableProxies()
	if len(proxies) == 0 {
		return nil, ErrNoProxiesAvailable
	}

	totalWeight := 0
	for _, proxy := range proxies {
		weight := 1
		if w, exists := pr.weights[proxy.ID]; exists {
			weight = w
		}
		totalWeight += weight
	}

	if totalWeight == 0 {
		return pr.nextRoundRobin()
	}

	n, err := rand.Int(rand.Reader, big.NewInt(int64(totalWeight)))
	if err != nil {
		return nil, err
	}

	target := int(n.Int64())
	current := 0

	for _, proxy := range proxies {
		weight := 1
		if w, exists := pr.weights[proxy.ID]; exists {
			weight = w
		}

		current += weight
		if current > target {
			return proxy, nil
		}
	}

	return proxies[0], nil
}

func (pr *ProxyRotator) nextHealthBased() (*models.Proxy, error) {
	proxies := pr.getAvailableProxies()
	if len(proxies) == 0 {
		return nil, ErrNoProxiesAvailable
	}

	scores := make([]*ProxyScore, 0, len(proxies))

	for _, proxy := range proxies {
		score := pr.calculateProxyScore(proxy)
		scores = append(scores, score)
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	if len(scores) > 0 {
		return scores[0].Proxy, nil
	}

	return nil, ErrNoProxiesAvailable
}

func (pr *ProxyRotator) nextLeastUsed() (*models.Proxy, error) {
	proxies := pr.getAvailableProxies()
	if len(proxies) == 0 {
		return nil, ErrNoProxiesAvailable
	}

	pr.stats.mu.RLock()
	defer pr.stats.mu.RUnlock()

	var leastUsedProxy *models.Proxy
	minUsage := uint64(^uint64(0))

	for _, proxy := range proxies {
		usage := pr.stats.ProxyUsage[proxy.ID]
		if usage < minUsage {
			minUsage = usage
			leastUsedProxy = proxy
		}
	}

	if leastUsedProxy == nil {
		return proxies[0], nil
	}

	return leastUsedProxy, nil
}

func (pr *ProxyRotator) nextSticky() (*models.Proxy, error) {
	proxies := pr.getAvailableProxies()
	if len(proxies) == 0 {
		return nil, ErrNoProxiesAvailable
	}

	return proxies[0], nil
}

func (pr *ProxyRotator) calculateProxyScore(proxy *models.Proxy) *ProxyScore {
	score := &ProxyScore{
		Proxy: proxy,
	}

	entry, exists := pr.manager.proxies[proxy.ID]
	if !exists {
		return score
	}

	entry.mu.RLock()
	defer entry.mu.RUnlock()

	if entry.Health.Status == HealthStatusHealthy {
		score.HealthScore = 100.0
	} else if entry.Health.Status == HealthStatusDegraded {
		score.HealthScore = 50.0
	} else {
		score.HealthScore = 0.0
	}

	totalRequests := entry.SuccessCount + entry.FailureCount
	if totalRequests > 0 {
		successRate := float64(entry.SuccessCount) / float64(totalRequests)
		score.HealthScore *= successRate
	}

	pr.stats.mu.RLock()
	usage := pr.stats.ProxyUsage[proxy.ID]
	pr.stats.mu.RUnlock()

	maxUsage := uint64(1)
	for _, u := range pr.stats.ProxyUsage {
		if u > maxUsage {
			maxUsage = u
		}
	}

	if maxUsage > 0 {
		score.UsageScore = 100.0 * (1.0 - float64(usage)/float64(maxUsage))
	} else {
		score.UsageScore = 100.0
	}

	weight := 1
	if w, exists := pr.weights[proxy.ID]; exists {
		weight = w
	}
	score.WeightScore = float64(weight * 10)

	score.Score = (score.HealthScore * 0.5) + (score.UsageScore * 0.3) + (score.WeightScore * 0.2)

	return score
}

func (pr *ProxyRotator) getAvailableProxies() []*models.Proxy {
	pr.manager.mu.RLock()
	defer pr.manager.mu.RUnlock()

	proxies := make([]*models.Proxy, 0)

	for _, entry := range pr.manager.proxies {
		entry.mu.RLock()
		isActive := entry.Proxy.IsActive  // Changed from Enabled to IsActive
		healthStatus := entry.Health.Status
		entry.mu.RUnlock()

		if isActive && (healthStatus == HealthStatusHealthy || healthStatus == HealthStatusUnknown) {
			// Convert repository.Proxy to models.Proxy
			modelProxy := &models.Proxy{
				ID:       entry.Proxy.ID,
				Host:     entry.Proxy.Host,
				Port:     entry.Proxy.Port,
				Type:     models.ProxyType(entry.Proxy.Type),
				Username: entry.Proxy.Username,
				Password: entry.Proxy.Password,
			}
			proxies = append(proxies, modelProxy)
		}
	}

	return proxies
}

func (pr *ProxyRotator) SetStrategy(strategy RotationStrategy) error {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	switch strategy {
	case StrategyRoundRobin, StrategyRandom, StrategyWeighted,
		StrategyHealthBased, StrategyLeastUsed, StrategySticky:
		pr.strategy = strategy
		atomic.StoreInt32(&pr.index, -1)

		pr.stats.mu.Lock()
		pr.stats.StrategyChanges++
		pr.stats.mu.Unlock()

		return nil
	default:
		return errors.New("invalid rotation strategy")
	}
}

func (pr *ProxyRotator) GetStrategy() RotationStrategy {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	return pr.strategy
}

func (pr *ProxyRotator) SetWeights(weights map[string]int) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	pr.weights = make(map[string]int)
	for id, weight := range weights {
		if weight > 0 {
			pr.weights[id] = weight
		}
	}
}

func (pr *ProxyRotator) GetWeights() map[string]int {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	weights := make(map[string]int)
	for id, weight := range pr.weights {
		weights[id] = weight
	}

	return weights
}

func (pr *ProxyRotator) SetWeight(proxyID string, weight int) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if weight > 0 {
		pr.weights[proxyID] = weight
	} else {
		delete(pr.weights, proxyID)
	}
}

func (pr *ProxyRotator) recordRotation(proxyID string) {
	pr.stats.mu.Lock()
	defer pr.stats.mu.Unlock()

	atomic.AddUint64(&pr.stats.TotalRotations, 1)
	atomic.AddUint64(&pr.stats.SuccessRotations, 1)
	pr.stats.ProxyUsage[proxyID]++
	pr.stats.LastRotation = time.Now()
}

func (pr *ProxyRotator) recordFailedRotation() {
	pr.stats.mu.Lock()
	defer pr.stats.mu.Unlock()

	atomic.AddUint64(&pr.stats.FailedRotations, 1)
}

func (pr *ProxyRotator) GetStats() RotationStats {
	pr.stats.mu.RLock()
	defer pr.stats.mu.RUnlock()

	usage := make(map[string]uint64)
	for id, count := range pr.stats.ProxyUsage {
		usage[id] = count
	}

	return RotationStats{
		TotalRotations:   atomic.LoadUint64(&pr.stats.TotalRotations),
		ProxyUsage:       usage,
		LastRotation:     pr.stats.LastRotation,
		StrategyChanges:  pr.stats.StrategyChanges,
		FailedRotations:  atomic.LoadUint64(&pr.stats.FailedRotations),
		SuccessRotations: atomic.LoadUint64(&pr.stats.SuccessRotations),
	}
}

func (pr *ProxyRotator) GetProxyUsage(proxyID string) uint64 {
	pr.stats.mu.RLock()
	defer pr.stats.mu.RUnlock()
	return pr.stats.ProxyUsage[proxyID]
}

func (pr *ProxyRotator) Reset() {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	atomic.StoreInt32(&pr.index, -1)

	pr.stats.mu.Lock()
	pr.stats.ProxyUsage = make(map[string]uint64)
	pr.stats.mu.Unlock()
}

func (pr *ProxyRotator) ResetProxyUsage(proxyID string) {
	pr.stats.mu.Lock()
	defer pr.stats.mu.Unlock()

	delete(pr.stats.ProxyUsage, proxyID)
}

func (pr *ProxyRotator) GetTopProxies(limit int) []*ProxyScore {
	proxies := pr.getAvailableProxies()

	scores := make([]*ProxyScore, 0, len(proxies))
	for _, proxy := range proxies {
		score := pr.calculateProxyScore(proxy)
		scores = append(scores, score)
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	if limit > 0 && limit < len(scores) {
		return scores[:limit]
	}

	return scores
}

func (pr *ProxyRotator) GetBottomProxies(limit int) []*ProxyScore {
	proxies := pr.getAvailableProxies()

	scores := make([]*ProxyScore, 0, len(proxies))
	for _, proxy := range proxies {
		score := pr.calculateProxyScore(proxy)
		scores = append(scores, score)
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score < scores[j].Score
	})

	if limit > 0 && limit < len(scores) {
		return scores[:limit]
	}

	return scores
}

func (pr *ProxyRotator) GetNextN(n int) ([]*models.Proxy, error) {
	if n <= 0 {
		return nil, errors.New("n must be positive")
	}

	proxies := make([]*models.Proxy, 0, n)

	for i := 0; i < n; i++ {
		proxy, err := pr.Next()
		if err != nil {
			if len(proxies) > 0 {
				return proxies, nil
			}
			return nil, err
		}
		proxies = append(proxies, proxy)
	}

	return proxies, nil
}

func (pr *ProxyRotator) HasAvailableProxies() bool {
	proxies := pr.getAvailableProxies()
	return len(proxies) > 0
}

func (pr *ProxyRotator) AvailableCount() int {
	proxies := pr.getAvailableProxies()
	return len(proxies)
}

func (pr *ProxyRotator) GetDistribution() map[string]float64 {
	pr.stats.mu.RLock()
	defer pr.stats.mu.RUnlock()

	total := uint64(0)
	for _, count := range pr.stats.ProxyUsage {
		total += count
	}

	if total == 0 {
		return make(map[string]float64)
	}

	distribution := make(map[string]float64)
	for id, count := range pr.stats.ProxyUsage {
		distribution[id] = float64(count) / float64(total) * 100.0
	}

	return distribution
}

func (pr *ProxyRotator) BalanceUsage() {
	pr.stats.mu.Lock()
	defer pr.stats.mu.Unlock()

	if len(pr.stats.ProxyUsage) == 0 {
		return
	}

	total := uint64(0)
	for _, count := range pr.stats.ProxyUsage {
		total += count
	}

	avg := total / uint64(len(pr.stats.ProxyUsage))

	for id := range pr.stats.ProxyUsage {
		pr.stats.ProxyUsage[id] = avg
	}
}

func (pr *ProxyRotator) GetSuccessRate() float64 {
	total := atomic.LoadUint64(&pr.stats.TotalRotations)
	if total == 0 {
		return 0.0
	}

	success := atomic.LoadUint64(&pr.stats.SuccessRotations)
	return float64(success) / float64(total) * 100.0
}

func (pr *ProxyRotator) SetStickyTTL(ttl time.Duration) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	pr.stickyTTL = ttl
}

func (pr *ProxyRotator) ClearStickyMap() {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	pr.stickyMap = make(map[string]string)
}

func ParseRotationStrategy(s string) (RotationStrategy, error) {
	switch s {
	case "round_robin", "roundrobin":
		return StrategyRoundRobin, nil
	case "random":
		return StrategyRandom, nil
	case "weighted":
		return StrategyWeighted, nil
	case "health_based", "healthbased", "health":
		return StrategyHealthBased, nil
	case "least_used", "leastused":
		return StrategyLeastUsed, nil
	case "sticky":
		return StrategySticky, nil
	default:
		return "", errors.New("invalid rotation strategy")
	}
}
