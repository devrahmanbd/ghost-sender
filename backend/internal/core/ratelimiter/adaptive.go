package ratelimiter

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

type AdaptiveLimiter struct {
	states    map[string]*adaptiveState
	mu        sync.RWMutex
	config    *AdaptiveConfig
	baseLimiter RateLimiter
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

type AdaptiveConfig struct {
	InitialRate           int
	MinRate               int
	MaxRate               int
	RatePer               time.Duration
	Burst                 int
	IncreaseStep          float64
	DecreaseStep          float64
	SuccessThreshold      float64
	ErrorThreshold        float64
	AdjustmentInterval    time.Duration
	CircuitBreakerEnabled bool
	CircuitBreakerTimeout time.Duration
	RecoveryRate          float64
	HealthWindow          time.Duration
	EnableAutoAdjust      bool
}

type adaptiveState struct {
	key                string
	currentRate        int
	successCount       int64
	errorCount         int64
	totalRequests      int64
	lastAdjustment     time.Time
	circuitState       CircuitState
	circuitOpenedAt    time.Time
	consecutiveSuccess int
	consecutiveErrors  int
	health             *HealthMetrics
	mu                 sync.Mutex
}

type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

type HealthMetrics struct {
	SuccessRate       float64
	ErrorRate         float64
	CurrentRate       int
	TargetRate        int
	AdjustmentCount   int
	LastAdjustmentAt  time.Time
	CircuitState      CircuitState
	CircuitBreakCount int
	RecoveryAttempts  int
	WindowStart       time.Time
	WindowEnd         time.Time
}

func NewAdaptiveLimiter(config *AdaptiveConfig, baseLimiter RateLimiter) *AdaptiveLimiter {
	if config == nil {
		config = DefaultAdaptiveConfig()
	}

	if baseLimiter == nil {
		baseLimiter = NewBaseLimiter(nil)
	}

	al := &AdaptiveLimiter{
		states:      make(map[string]*adaptiveState),
		config:      config,
		baseLimiter: baseLimiter,
		stopCh:      make(chan struct{}),
	}

	if config.EnableAutoAdjust {
		al.startAutoAdjustment()
	}

	return al
}

func DefaultAdaptiveConfig() *AdaptiveConfig {
	return &AdaptiveConfig{
		InitialRate:           100,
		MinRate:               10,
		MaxRate:               1000,
		RatePer:               time.Minute,
		Burst:                 10,
		IncreaseStep:          1.1,
		DecreaseStep:          0.5,
		SuccessThreshold:      0.95,
		ErrorThreshold:        0.1,
		AdjustmentInterval:    30 * time.Second,
		CircuitBreakerEnabled: true,
		CircuitBreakerTimeout: 60 * time.Second,
		RecoveryRate:          1.05,
		HealthWindow:          5 * time.Minute,
		EnableAutoAdjust:      true,
	}
}

func (al *AdaptiveLimiter) Allow(key string) (bool, error) {
	return al.AllowN(key, 1)
}

func (al *AdaptiveLimiter) AllowN(key string, n int) (bool, error) {
	state := al.getOrCreateState(key)
	state.mu.Lock()

	if al.config.CircuitBreakerEnabled {
		if state.circuitState == CircuitOpen {
			if time.Since(state.circuitOpenedAt) > al.config.CircuitBreakerTimeout {
				state.circuitState = CircuitHalfOpen
				state.consecutiveSuccess = 0
			} else {
				state.mu.Unlock()
				return false, fmt.Errorf("circuit breaker open")
			}
		}
	}

	state.mu.Unlock()

	allowed, err := al.baseLimiter.AllowN(key, n)
	
	state.mu.Lock()
	state.totalRequests++
	state.mu.Unlock()

	return allowed, err
}

func (al *AdaptiveLimiter) Wait(ctx context.Context, key string) error {
	return al.WaitN(ctx, key, 1)
}

func (al *AdaptiveLimiter) WaitN(ctx context.Context, key string, n int) error {
	state := al.getOrCreateState(key)
	state.mu.Lock()

	if al.config.CircuitBreakerEnabled && state.circuitState == CircuitOpen {
		if time.Since(state.circuitOpenedAt) < al.config.CircuitBreakerTimeout {
			state.mu.Unlock()
			return fmt.Errorf("circuit breaker open")
		}
		state.circuitState = CircuitHalfOpen
	}

	state.mu.Unlock()

	return al.baseLimiter.WaitN(ctx, key, n)
}

func (al *AdaptiveLimiter) Reserve(key string) (*Reservation, error) {
	return al.ReserveN(key, 1)
}

func (al *AdaptiveLimiter) ReserveN(key string, n int) (*Reservation, error) {
	state := al.getOrCreateState(key)
	state.mu.Lock()

	if al.config.CircuitBreakerEnabled && state.circuitState == CircuitOpen {
		state.mu.Unlock()
		return &Reservation{OK: false}, fmt.Errorf("circuit breaker open")
	}

	state.mu.Unlock()

	return al.baseLimiter.ReserveN(key, n)
}

func (al *AdaptiveLimiter) RecordSuccess(key string) {
	state := al.getOrCreateState(key)
	state.mu.Lock()
	defer state.mu.Unlock()

	state.successCount++
	state.consecutiveSuccess++
	state.consecutiveErrors = 0

	if al.config.CircuitBreakerEnabled && state.circuitState == CircuitHalfOpen {
		if state.consecutiveSuccess >= 5 {
			state.circuitState = CircuitClosed
			state.consecutiveSuccess = 0
		}
	}

	al.updateHealth(state)
}

func (al *AdaptiveLimiter) RecordFailure(key string) {
	state := al.getOrCreateState(key)
	state.mu.Lock()
	defer state.mu.Unlock()

	state.errorCount++
	state.consecutiveErrors++
	state.consecutiveSuccess = 0

	if al.config.CircuitBreakerEnabled {
		if state.circuitState == CircuitClosed && state.consecutiveErrors >= 5 {
			state.circuitState = CircuitOpen
			state.circuitOpenedAt = time.Now()
			state.health.CircuitBreakCount++
		} else if state.circuitState == CircuitHalfOpen {
			state.circuitState = CircuitOpen
			state.circuitOpenedAt = time.Now()
			state.health.CircuitBreakCount++
		}
	}

	al.updateHealth(state)
}

func (al *AdaptiveLimiter) AdjustRate(key string) error {
	state := al.getOrCreateState(key)
	state.mu.Lock()
	defer state.mu.Unlock()

	now := time.Now()
	if now.Sub(state.lastAdjustment) < al.config.AdjustmentInterval {
		return nil
	}

	if state.totalRequests == 0 {
		return nil
	}

	successRate := float64(state.successCount) / float64(state.totalRequests)
	errorRate := float64(state.errorCount) / float64(state.totalRequests)

	oldRate := state.currentRate

	if successRate >= al.config.SuccessThreshold && errorRate < al.config.ErrorThreshold {
		newRate := int(float64(state.currentRate) * al.config.IncreaseStep)
		if newRate > al.config.MaxRate {
			newRate = al.config.MaxRate
		}
		state.currentRate = newRate
	} else if errorRate >= al.config.ErrorThreshold {
		newRate := int(float64(state.currentRate) * al.config.DecreaseStep)
		if newRate < al.config.MinRate {
			newRate = al.config.MinRate
		}
		state.currentRate = newRate
	}

	if state.currentRate != oldRate {
		limit := RateLimit{
			Rate:  state.currentRate,
			Per:   al.config.RatePer,
			Burst: al.config.Burst,
		}
		al.baseLimiter.SetLimit(key, limit)

		state.health.AdjustmentCount++
		state.health.LastAdjustmentAt = now
	}

	state.lastAdjustment = now
	al.updateHealth(state)

	return nil
}

func (al *AdaptiveLimiter) GetHealth(key string) (*HealthMetrics, error) {
	al.mu.RLock()
	state, exists := al.states[key]
	al.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("key not found")
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	al.updateHealth(state)

	return &HealthMetrics{
		SuccessRate:       state.health.SuccessRate,
		ErrorRate:         state.health.ErrorRate,
		CurrentRate:       state.health.CurrentRate,
		TargetRate:        state.health.TargetRate,
		AdjustmentCount:   state.health.AdjustmentCount,
		LastAdjustmentAt:  state.health.LastAdjustmentAt,
		CircuitState:      state.health.CircuitState,
		CircuitBreakCount: state.health.CircuitBreakCount,
		RecoveryAttempts:  state.health.RecoveryAttempts,
		WindowStart:       state.health.WindowStart,
		WindowEnd:         state.health.WindowEnd,
	}, nil
}

func (al *AdaptiveLimiter) Reset(key string) error {
	al.mu.Lock()
	defer al.mu.Unlock()

	delete(al.states, key)
	return al.baseLimiter.Reset(key)
}

func (al *AdaptiveLimiter) GetLimit(key string) (*LimitInfo, error) {
	return al.baseLimiter.GetLimit(key)
}

func (al *AdaptiveLimiter) SetLimit(key string, limit RateLimit) error {
	state := al.getOrCreateState(key)
	state.mu.Lock()
	state.currentRate = limit.Rate
	state.mu.Unlock()

	return al.baseLimiter.SetLimit(key, limit)
}

func (al *AdaptiveLimiter) GetStats(key string) (*Stats, error) {
	return al.baseLimiter.GetStats(key)
}

func (al *AdaptiveLimiter) Close() error {
	close(al.stopCh)
	al.wg.Wait()
	return al.baseLimiter.Close()
}

func (al *AdaptiveLimiter) getOrCreateState(key string) *adaptiveState {
	al.mu.RLock()
	state, exists := al.states[key]
	al.mu.RUnlock()

	if exists {
		return state
	}

	al.mu.Lock()
	defer al.mu.Unlock()

	state, exists = al.states[key]
	if exists {
		return state
	}

	now := time.Now()
	state = &adaptiveState{
		key:            key,
		currentRate:    al.config.InitialRate,
		lastAdjustment: now,
		circuitState:   CircuitClosed,
		health: &HealthMetrics{
			CurrentRate: al.config.InitialRate,
			TargetRate:  al.config.InitialRate,
			WindowStart: now,
			CircuitState: CircuitClosed,
		},
	}

	al.states[key] = state

	limit := RateLimit{
		Rate:  al.config.InitialRate,
		Per:   al.config.RatePer,
		Burst: al.config.Burst,
	}
	al.baseLimiter.SetLimit(key, limit)

	return state
}

func (al *AdaptiveLimiter) updateHealth(state *adaptiveState) {
	now := time.Now()

	if state.totalRequests > 0 {
		state.health.SuccessRate = float64(state.successCount) / float64(state.totalRequests)
		state.health.ErrorRate = float64(state.errorCount) / float64(state.totalRequests)
	}

	state.health.CurrentRate = state.currentRate
	state.health.CircuitState = state.circuitState
	state.health.WindowEnd = now
}

func (al *AdaptiveLimiter) startAutoAdjustment() {
	al.wg.Add(1)
	go func() {
		defer al.wg.Done()
		ticker := time.NewTicker(al.config.AdjustmentInterval)
		defer ticker.Stop()

		for {
			select {
			case <-al.stopCh:
				return
			case <-ticker.C:
				al.adjustAllRates()
			}
		}
	}()
}

func (al *AdaptiveLimiter) adjustAllRates() {
	al.mu.RLock()
	keys := make([]string, 0, len(al.states))
	for key := range al.states {
		keys = append(keys, key)
	}
	al.mu.RUnlock()

	for _, key := range keys {
		al.AdjustRate(key)
	}
}

func (al *AdaptiveLimiter) GetAllStates() map[string]*HealthMetrics {
	al.mu.RLock()
	defer al.mu.RUnlock()

	result := make(map[string]*HealthMetrics)
	for key, state := range al.states {
		state.mu.Lock()
		al.updateHealth(state)
		result[key] = &HealthMetrics{
			SuccessRate:       state.health.SuccessRate,
			ErrorRate:         state.health.ErrorRate,
			CurrentRate:       state.health.CurrentRate,
			TargetRate:        state.health.TargetRate,
			AdjustmentCount:   state.health.AdjustmentCount,
			LastAdjustmentAt:  state.health.LastAdjustmentAt,
			CircuitState:      state.health.CircuitState,
			CircuitBreakCount: state.health.CircuitBreakCount,
			RecoveryAttempts:  state.health.RecoveryAttempts,
			WindowStart:       state.health.WindowStart,
			WindowEnd:         state.health.WindowEnd,
		}
		state.mu.Unlock()
	}

	return result
}

func (al *AdaptiveLimiter) ForceCircuitOpen(key string) error {
	state := al.getOrCreateState(key)
	state.mu.Lock()
	defer state.mu.Unlock()

	state.circuitState = CircuitOpen
	state.circuitOpenedAt = time.Now()
	state.health.CircuitBreakCount++

	return nil
}

func (al *AdaptiveLimiter) ForceCircuitClose(key string) error {
	state := al.getOrCreateState(key)
	state.mu.Lock()
	defer state.mu.Unlock()

	state.circuitState = CircuitClosed
	state.consecutiveErrors = 0
	state.consecutiveSuccess = 0

	return nil
}

func (al *AdaptiveLimiter) GetCircuitState(key string) (CircuitState, error) {
	al.mu.RLock()
	state, exists := al.states[key]
	al.mu.RUnlock()

	if !exists {
		return CircuitClosed, fmt.Errorf("key not found")
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	return state.circuitState, nil
}

func (al *AdaptiveLimiter) ResetStats(key string) error {
	state := al.getOrCreateState(key)
	state.mu.Lock()
	defer state.mu.Unlock()

	state.successCount = 0
	state.errorCount = 0
	state.totalRequests = 0
	state.consecutiveSuccess = 0
	state.consecutiveErrors = 0

	al.updateHealth(state)

	return nil
}

func (al *AdaptiveLimiter) SetTargetRate(key string, targetRate int) error {
	if targetRate < al.config.MinRate {
		targetRate = al.config.MinRate
	}
	if targetRate > al.config.MaxRate {
		targetRate = al.config.MaxRate
	}

	state := al.getOrCreateState(key)
	state.mu.Lock()
	defer state.mu.Unlock()

	state.health.TargetRate = targetRate

	return nil
}

func (al *AdaptiveLimiter) GradualRecovery(key string) error {
	state := al.getOrCreateState(key)
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.circuitState != CircuitClosed {
		return fmt.Errorf("circuit must be closed for recovery")
	}

	newRate := int(math.Min(
		float64(state.currentRate)*al.config.RecoveryRate,
		float64(al.config.MaxRate),
	))

	if newRate > state.currentRate {
		state.currentRate = newRate
		state.health.RecoveryAttempts++

		limit := RateLimit{
			Rate:  newRate,
			Per:   al.config.RatePer,
			Burst: al.config.Burst,
		}
		return al.baseLimiter.SetLimit(key, limit)
	}

	return nil
}

func (al *AdaptiveLimiter) IsHealthy(key string) bool {
	health, err := al.GetHealth(key)
	if err != nil {
		return false
	}

	return health.SuccessRate >= al.config.SuccessThreshold &&
		health.ErrorRate < al.config.ErrorThreshold &&
		health.CircuitState == CircuitClosed
}

func (al *AdaptiveLimiter) GetConfig() *AdaptiveConfig {
	return al.config
}

func (al *AdaptiveLimiter) UpdateConfig(config *AdaptiveConfig) {
	al.config = config
}

