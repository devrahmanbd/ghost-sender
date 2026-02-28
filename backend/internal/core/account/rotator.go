package account

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"email-campaign-system/internal/core/provider"
	"email-campaign-system/pkg/logger"
)

type RotationStrategy string

const (
	StrategyRoundRobin  RotationStrategy = "round_robin"
	StrategyWeighted    RotationStrategy = "weighted"
	StrategyHealthBased RotationStrategy = "health_based"
	StrategyRandom      RotationStrategy = "random"
	StrategyLeastUsed   RotationStrategy = "least_used"
	StrategyAdaptive    RotationStrategy = "adaptive"
)

var (
	ErrNoAvailableAccount = errors.New("no available account for rotation")
	ErrRotationDisabled   = errors.New("rotation is disabled")
	ErrInvalidStrategy    = errors.New("invalid rotation strategy")
)

type AccountRotator struct {
	manager         *AccountManager
	log             logger.Logger // ← CHANGED: Remove pointer
	strategy        RotationStrategy
	currentIndex    int
	mu              sync.RWMutex
	rotationHistory []*RotationRecord
	historyMu       sync.RWMutex
	stats           *RotationStats
	statsMu         sync.RWMutex
	cooldownMap     map[string]time.Time
	cooldownMu      sync.RWMutex
	config          *RotatorConfig
	rand            *rand.Rand
}

type RotatorConfig struct {
	Strategy             RotationStrategy
	CooldownPeriod       time.Duration
	MaxRotationHistory   int
	HealthScoreThreshold float64
	UsageThreshold       float64
	EnableAdaptive       bool
	AdaptiveInterval     time.Duration
}

type RotationRecord struct {
	AccountID   string
	Email       string
	Timestamp   time.Time
	Strategy    RotationStrategy
	HealthScore float64
	UsageBefore int
	UsageAfter  int
	Success     bool
}

type RotationStats struct {
	TotalRotations      int64
	SuccessfulRotations int64
	FailedRotations     int64
	AverageHealth       float64
	LastRotation        time.Time
	StrategyBreakdown   map[RotationStrategy]int64
	AccountUsageMap     map[string]int64
}

type AccountScore struct {
	Account *ManagedAccount
	Score   float64
}

func NewAccountRotator(manager *AccountManager, log logger.Logger) *AccountRotator {
	var logValue logger.Logger
	if log != nil {
		logValue = log
	}

	return &AccountRotator{
		manager:         manager,
		log:             logValue, // ← CHANGED
		strategy:        StrategyHealthBased,
		currentIndex:    0,
		rotationHistory: make([]*RotationRecord, 0),
		cooldownMap:     make(map[string]time.Time),
		stats: &RotationStats{
			StrategyBreakdown: make(map[RotationStrategy]int64),
			AccountUsageMap:   make(map[string]int64),
		},
		config: DefaultRotatorConfig(),
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func DefaultRotatorConfig() *RotatorConfig {
	return &RotatorConfig{
		Strategy:             StrategyHealthBased,
		CooldownPeriod:       30 * time.Second,
		MaxRotationHistory:   1000,
		HealthScoreThreshold: 30.0,
		UsageThreshold:       0.8,
		EnableAdaptive:       true,
		AdaptiveInterval:     5 * time.Minute,
	}
}

func (r *AccountRotator) SetStrategy(strategy RotationStrategy) error {
	if !r.isValidStrategy(strategy) {
		return ErrInvalidStrategy
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.strategy = strategy
	r.log.Info("rotation strategy changed", logger.String("strategy", string(strategy))) // ← FIXED

	return nil
}

func (r *AccountRotator) isValidStrategy(strategy RotationStrategy) bool {
	validStrategies := []RotationStrategy{
		StrategyRoundRobin,
		StrategyWeighted,
		StrategyHealthBased,
		StrategyRandom,
		StrategyLeastUsed,
		StrategyAdaptive,
	}

	for _, valid := range validStrategies {
		if strategy == valid {
			return true
		}
	}

	return false
}

func (r *AccountRotator) GetNextAccount(filter *AccountFilter) (*ManagedAccount, error) {
	r.mu.Lock()
	currentStrategy := r.strategy
	r.mu.Unlock()

	availableAccounts := r.getAvailableAccounts(filter)
	if len(availableAccounts) == 0 {
		return nil, ErrNoAvailableAccount
	}

	availableAccounts = r.filterCooldown(availableAccounts)
	if len(availableAccounts) == 0 {
		return nil, fmt.Errorf("all accounts are in cooldown period")
	}

	var selectedAccount *ManagedAccount
	var err error

	switch currentStrategy {
	case StrategyRoundRobin:
		selectedAccount = r.roundRobinSelect(availableAccounts)
	case StrategyWeighted:
		selectedAccount = r.weightedSelect(availableAccounts)
	case StrategyHealthBased:
		selectedAccount = r.healthBasedSelect(availableAccounts)
	case StrategyRandom:
		selectedAccount = r.randomSelect(availableAccounts)
	case StrategyLeastUsed:
		selectedAccount = r.leastUsedSelect(availableAccounts)
	case StrategyAdaptive:
		selectedAccount = r.adaptiveSelect(availableAccounts)
	default:
		selectedAccount = r.healthBasedSelect(availableAccounts)
	}

	if selectedAccount == nil {
		return nil, ErrNoAvailableAccount
	}

	r.recordRotation(selectedAccount, currentStrategy, true)
	r.setCooldown(selectedAccount.Account.ID)

	return selectedAccount, err
}

func (r *AccountRotator) getAvailableAccounts(filter *AccountFilter) []*ManagedAccount {
	var accounts []*ManagedAccount

	if filter != nil {
		accounts = r.manager.ListAccounts(filter)
	} else {
		accounts = r.manager.GetActiveAccounts()
	}

	var available []*ManagedAccount
	for _, acc := range accounts {
		if r.isAccountAvailable(acc) {
			available = append(available, acc)
		}
	}

	return available
}

func (r *AccountRotator) isAccountAvailable(acc *ManagedAccount) bool {
	if !acc.IsAvailable() {
		return false
	}

	if acc.Health.Score < r.config.HealthScoreThreshold {
		return false
	}

	if acc.Limits.DailyLimit > 0 {
		usagePercent := float64(acc.Limits.DailySent) / float64(acc.Limits.DailyLimit)
		if usagePercent >= r.config.UsageThreshold {
			return false
		}
	}

	return true
}

func (r *AccountRotator) filterCooldown(accounts []*ManagedAccount) []*ManagedAccount {
	r.cooldownMu.RLock()
	defer r.cooldownMu.RUnlock()

	now := time.Now()
	var available []*ManagedAccount

	for _, acc := range accounts {
		cooldownEnd, exists := r.cooldownMap[acc.Account.ID]
		if !exists || now.After(cooldownEnd) {
			available = append(available, acc)
		}
	}

	return available
}

func (r *AccountRotator) roundRobinSelect(accounts []*ManagedAccount) *ManagedAccount {
	if len(accounts) == 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.currentIndex >= len(accounts) {
		r.currentIndex = 0
	}

	selected := accounts[r.currentIndex]
	r.currentIndex = (r.currentIndex + 1) % len(accounts)

	return selected
}

func (r *AccountRotator) weightedSelect(accounts []*ManagedAccount) *ManagedAccount {
	if len(accounts) == 0 {
		return nil
	}

	scores := r.calculateWeightedScores(accounts)
	if len(scores) == 0 {
		return accounts[0]
	}

	totalScore := 0.0
	for _, score := range scores {
		totalScore += score.Score
	}

	if totalScore == 0 {
		return r.randomSelect(accounts)
	}

	randomValue := r.rand.Float64() * totalScore
	cumulative := 0.0

	for _, score := range scores {
		cumulative += score.Score
		if randomValue <= cumulative {
			return score.Account
		}
	}

	return scores[0].Account
}

func (r *AccountRotator) calculateWeightedScores(accounts []*ManagedAccount) []AccountScore {
	scores := make([]AccountScore, 0, len(accounts))

	for _, acc := range accounts {
		score := r.calculateAccountScore(acc)
		scores = append(scores, AccountScore{
			Account: acc,
			Score:   score,
		})
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	return scores
}

func (r *AccountRotator) calculateAccountScore(acc *ManagedAccount) float64 {
	score := 0.0

	// Health weight
	healthWeight := 0.4
	score += acc.Health.Score * healthWeight

	// Usage weight
	usageWeight := 0.3
	if acc.Limits.DailyLimit > 0 {
		usagePercent := float64(acc.Limits.DailySent) / float64(acc.Limits.DailyLimit)
		usageScore := (1.0 - usagePercent) * 100.0
		score += usageScore * usageWeight
	} else {
		score += 100.0 * usageWeight
	}

	// Idle time weight
	idleWeight := 0.2
	idleTime := time.Since(acc.LastUsed)
	idleScore := float64(idleTime.Minutes())
	if idleScore > 100.0 {
		idleScore = 100.0
	}
	score += idleScore * idleWeight

	// Reliability weight
	reliabilityWeight := 0.1
	if acc.Account.Stats.TotalSent > 0 { // ← FIXED
		successRate := float64(acc.Account.Stats.TotalSent-acc.Account.Stats.TotalFailed) / float64(acc.Account.Stats.TotalSent) * 100.0 // ← FIXED
		score += successRate * reliabilityWeight
	} else {
		score += 100.0 * reliabilityWeight
	}

	return score
}

func (r *AccountRotator) healthBasedSelect(accounts []*ManagedAccount) *ManagedAccount {
	if len(accounts) == 0 {
		return nil
	}

	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].Health.Score > accounts[j].Health.Score
	})

	return accounts[0]
}

func (r *AccountRotator) randomSelect(accounts []*ManagedAccount) *ManagedAccount {
	if len(accounts) == 0 {
		return nil
	}

	index := r.rand.Intn(len(accounts))
	return accounts[index]
}

func (r *AccountRotator) leastUsedSelect(accounts []*ManagedAccount) *ManagedAccount {
	if len(accounts) == 0 {
		return nil
	}

	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].Limits.DailySent < accounts[j].Limits.DailySent
	})

	return accounts[0]
}

func (r *AccountRotator) adaptiveSelect(accounts []*ManagedAccount) *ManagedAccount {
	if len(accounts) == 0 {
		return nil
	}

	scores := r.calculateWeightedScores(accounts)

	r.statsMu.RLock()
	recentFailureRate := r.calculateRecentFailureRate()
	r.statsMu.RUnlock()

	if recentFailureRate > 0.3 {
		return r.healthBasedSelect(accounts)
	} else if recentFailureRate > 0.1 {
		return r.weightedSelect(accounts)
	}

	if len(scores) > 0 {
		return scores[0].Account
	}

	return accounts[0]
}

func (r *AccountRotator) calculateRecentFailureRate() float64 {
	r.historyMu.RLock()
	defer r.historyMu.RUnlock()

	if len(r.rotationHistory) == 0 {
		return 0.0
	}

	recentCount := 10
	if len(r.rotationHistory) < recentCount {
		recentCount = len(r.rotationHistory)
	}

	recentHistory := r.rotationHistory[len(r.rotationHistory)-recentCount:]
	failures := 0

	for _, record := range recentHistory {
		if !record.Success {
			failures++
		}
	}

	return float64(failures) / float64(recentCount)
}

func (r *AccountRotator) recordRotation(acc *ManagedAccount, strategy RotationStrategy, success bool) {
	record := &RotationRecord{
		AccountID:   acc.Account.ID,
		Email:       acc.Account.Email,
		Timestamp:   time.Now(),
		Strategy:    strategy,
		HealthScore: acc.Health.Score,
		UsageBefore: acc.Limits.DailySent,
		Success:     success,
	}

	r.historyMu.Lock()
	r.rotationHistory = append(r.rotationHistory, record)
	if len(r.rotationHistory) > r.config.MaxRotationHistory {
		r.rotationHistory = r.rotationHistory[1:]
	}
	r.historyMu.Unlock()

	r.statsMu.Lock()
	r.stats.TotalRotations++
	if success {
		r.stats.SuccessfulRotations++
	} else {
		r.stats.FailedRotations++
	}
	r.stats.LastRotation = time.Now()
	r.stats.StrategyBreakdown[strategy]++
	r.stats.AccountUsageMap[acc.Account.ID]++
	r.statsMu.Unlock()

	r.log.Debug("account rotation recorded", // ← FIXED
		logger.String("account_id", acc.Account.ID),
		logger.String("email", acc.Account.Email),
		logger.String("strategy", string(strategy)),
		logger.Float64("health_score", acc.Health.Score),
		logger.Bool("success", success),
	)
}

func (r *AccountRotator) setCooldown(accountID string) {
	r.cooldownMu.Lock()
	defer r.cooldownMu.Unlock()

	r.cooldownMap[accountID] = time.Now().Add(r.config.CooldownPeriod)
}

func (r *AccountRotator) GetCooldownRemaining(accountID string) time.Duration {
	r.cooldownMu.RLock()
	defer r.cooldownMu.RUnlock()

	cooldownEnd, exists := r.cooldownMap[accountID]
	if !exists {
		return 0
	}

	remaining := time.Until(cooldownEnd)
	if remaining < 0 {
		return 0
	}

	return remaining
}

func (r *AccountRotator) ClearCooldown(accountID string) {
	r.cooldownMu.Lock()
	defer r.cooldownMu.Unlock()

	delete(r.cooldownMap, accountID)
}

func (r *AccountRotator) GetRotationHistory(limit int) []*RotationRecord {
	r.historyMu.RLock()
	defer r.historyMu.RUnlock()

	if limit <= 0 || limit > len(r.rotationHistory) {
		limit = len(r.rotationHistory)
	}

	start := len(r.rotationHistory) - limit
	if start < 0 {
		start = 0
	}

	history := make([]*RotationRecord, limit)
	copy(history, r.rotationHistory[start:])

	return history
}

func (r *AccountRotator) GetStats() RotationStats {
	r.statsMu.RLock()
	defer r.statsMu.RUnlock()

	stats := RotationStats{
		TotalRotations:      r.stats.TotalRotations,
		SuccessfulRotations: r.stats.SuccessfulRotations,
		FailedRotations:     r.stats.FailedRotations,
		AverageHealth:       r.stats.AverageHealth,
		LastRotation:        r.stats.LastRotation,
		StrategyBreakdown:   make(map[RotationStrategy]int64),
		AccountUsageMap:     make(map[string]int64),
	}

	for k, v := range r.stats.StrategyBreakdown {
		stats.StrategyBreakdown[k] = v
	}

	for k, v := range r.stats.AccountUsageMap {
		stats.AccountUsageMap[k] = v
	}

	return stats
}

func (r *AccountRotator) GetMostUsedAccount() (string, int64) {
	r.statsMu.RLock()
	defer r.statsMu.RUnlock()

	var maxAccount string
	var maxUsage int64

	for accountID, usage := range r.stats.AccountUsageMap {
		if usage > maxUsage {
			maxUsage = usage
			maxAccount = accountID
		}
	}

	return maxAccount, maxUsage
}

func (r *AccountRotator) GetLeastUsedAccount() (string, int64) {
	r.statsMu.RLock()
	defer r.statsMu.RUnlock()

	var minAccount string
	var minUsage int64 = -1

	for accountID, usage := range r.stats.AccountUsageMap {
		if minUsage == -1 || usage < minUsage {
			minUsage = usage
			minAccount = accountID
		}
	}

	return minAccount, minUsage
}

func (r *AccountRotator) ResetStats() {
	r.statsMu.Lock()
	defer r.statsMu.Unlock()

	r.stats = &RotationStats{
		StrategyBreakdown: make(map[RotationStrategy]int64),
		AccountUsageMap:   make(map[string]int64),
	}

	r.historyMu.Lock()
	r.rotationHistory = make([]*RotationRecord, 0)
	r.historyMu.Unlock()

	r.log.Info("rotation stats reset") // ← FIXED
}

func (r *AccountRotator) GetCurrentStrategy() RotationStrategy {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.strategy
}

func (r *AccountRotator) GetConfig() *RotatorConfig {
	return r.config
}

func (r *AccountRotator) UpdateConfig(config *RotatorConfig) {
	if config == nil {
		return
	}

	r.config = config

	if config.Strategy != "" {
		r.SetStrategy(config.Strategy)
	}

	r.log.Info("rotator config updated", // ← FIXED
		logger.String("strategy", string(config.Strategy)),
		logger.Duration("cooldown", config.CooldownPeriod),
	)
}

func (r *AccountRotator) GetAccountRotationCount(accountID string) int64 {
	r.statsMu.RLock()
	defer r.statsMu.RUnlock()

	return r.stats.AccountUsageMap[accountID]
}

func (r *AccountRotator) GetSuccessRate() float64 {
	r.statsMu.RLock()
	defer r.statsMu.RUnlock()

	if r.stats.TotalRotations == 0 {
		return 0.0
	}

	return float64(r.stats.SuccessfulRotations) / float64(r.stats.TotalRotations) * 100.0
}

func (r *AccountRotator) OptimizeStrategy() RotationStrategy {
	r.statsMu.RLock()
	successRate := r.GetSuccessRate()
	totalRotations := r.stats.TotalRotations
	r.statsMu.RUnlock()

	if totalRotations < 10 {
		return StrategyHealthBased
	}

	if successRate >= 95.0 {
		return StrategyLeastUsed
	} else if successRate >= 80.0 {
		return StrategyWeighted
	} else if successRate >= 60.0 {
		return StrategyHealthBased
	}

	return StrategyAdaptive
}

func (r *AccountRotator) GetAccountDistribution() map[string]float64 {
	r.statsMu.RLock()
	defer r.statsMu.RUnlock()

	distribution := make(map[string]float64)
	total := float64(r.stats.TotalRotations)

	if total == 0 {
		return distribution
	}

	for accountID, usage := range r.stats.AccountUsageMap {
		distribution[accountID] = float64(usage) / total * 100.0
	}

	return distribution
}

func (r *AccountRotator) RotateWithProvider(providerType provider.ProviderType) (*ManagedAccount, error) {
	filter := &AccountFilter{
		ProviderType: providerType,
	}

	return r.GetNextAccount(filter)
}

func (r *AccountRotator) ForceRotate(excludeIDs []string) (*ManagedAccount, error) {
	filter := &AccountFilter{
		ExcludeIDs: excludeIDs,
	}

	return r.GetNextAccount(filter)
}
