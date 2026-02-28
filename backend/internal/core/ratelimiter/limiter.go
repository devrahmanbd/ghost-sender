package ratelimiter

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type RateLimiter interface {
	Allow(key string) (bool, error)
	AllowN(key string, n int) (bool, error)
	Wait(ctx context.Context, key string) error
	WaitN(ctx context.Context, key string, n int) error
	Reserve(key string) (*Reservation, error)
	ReserveN(key string, n int) (*Reservation, error)
	Reset(key string) error
	GetLimit(key string) (*LimitInfo, error)
	SetLimit(key string, limit RateLimit) error
	GetStats(key string) (*Stats, error)
	Close() error
}

type Limiter interface {
	Allow() (bool, error)
	AllowN(n int) (bool, error)
	Wait(ctx context.Context) error
	WaitN(ctx context.Context, n int) error
	Reserve() (*Reservation, error)
	ReserveN(n int) (*Reservation, error)
	Reset() error
	GetLimit() (*LimitInfo, error)
	SetLimit(limit RateLimit) error
	GetStats() (*Stats, error)
}

type RateLimiterType string

const (
	TokenBucket   RateLimiterType = "token_bucket"
	FixedWindow   RateLimiterType = "fixed_window"
	SlidingWindow RateLimiterType = "sliding_window"
	LeakyBucket   RateLimiterType = "leaky_bucket"
	Adaptive      RateLimiterType = "adaptive"
	Distributed   RateLimiterType = "distributed"
)

type RateLimit struct {
	Rate    int
	Per     time.Duration
	Burst   int
	MaxWait time.Duration
}

type LimitInfo struct {
	Key        string
	Rate       int
	Per        time.Duration
	Burst      int
	Available  int
	ResetAt    time.Time
	RetryAfter time.Duration
}

type Stats struct {
	Key             string
	TotalRequests   int64
	AllowedRequests int64
	DeniedRequests  int64
	CurrentRate     float64
	AverageRate     float64
	PeakRate        float64
	LastRequestAt   time.Time
	WindowStart     time.Time
	WindowEnd       time.Time
}

type Reservation struct {
	OK        bool
	delay     time.Duration
	TimeToAct time.Time
	Limit     RateLimit
	limiter   *BaseLimiter
	key       string
	tokens    int
}

type BaseLimiter struct {
	limiters map[string]*limiterState
	mu       sync.RWMutex
	config   *Config
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

type limiterState struct {
	key        string
	limit      RateLimit
	tokens     float64
	lastUpdate time.Time
	requests   []time.Time
	stats      *Stats
	mu         sync.Mutex
}

type Config struct {
	Type              RateLimiterType
	DefaultRate       int
	DefaultPer        time.Duration
	DefaultBurst      int
	MaxWait           time.Duration
	CleanupInterval   time.Duration
	StatsRetention    time.Duration
	EnableStats       bool
	EnableAutoCleanup bool
}

type SingleKeyLimiter struct {
	baseLimiter *BaseLimiter
	key         string
}

func NewBaseLimiter(config *Config) *BaseLimiter {
	if config == nil {
		config = DefaultConfig()
	}

	limiter := &BaseLimiter{
		limiters: make(map[string]*limiterState),
		config:   config,
		stopCh:   make(chan struct{}),
	}

	if config.EnableAutoCleanup {
		limiter.startCleanup()
	}

	return limiter
}

func NewLimiter(key string, limit RateLimit) Limiter {
	config := DefaultConfig()
	config.DefaultRate = limit.Rate
	config.DefaultPer = limit.Per
	config.DefaultBurst = limit.Burst
	if limit.MaxWait > 0 {
		config.MaxWait = limit.MaxWait
	}

	baseLimiter := NewBaseLimiter(config)
	baseLimiter.SetLimit(key, limit)

	return &SingleKeyLimiter{
		baseLimiter: baseLimiter,
		key:         key,
	}
}

func NewLimiterWithConfig(key string, config *Config) Limiter {
	baseLimiter := NewBaseLimiter(config)

	return &SingleKeyLimiter{
		baseLimiter: baseLimiter,
		key:         key,
	}
}

func DefaultConfig() *Config {
	return &Config{
		Type:              TokenBucket,
		DefaultRate:       100,
		DefaultPer:        time.Minute,
		DefaultBurst:      10,
		MaxWait:           30 * time.Second,
		CleanupInterval:   5 * time.Minute,
		StatsRetention:    24 * time.Hour,
		EnableStats:       true,
		EnableAutoCleanup: true,
	}
}

func (skl *SingleKeyLimiter) Allow() (bool, error) {
	return skl.baseLimiter.Allow(skl.key)
}

func (skl *SingleKeyLimiter) AllowN(n int) (bool, error) {
	return skl.baseLimiter.AllowN(skl.key, n)
}

func (skl *SingleKeyLimiter) Wait(ctx context.Context) error {
	return skl.baseLimiter.Wait(ctx, skl.key)
}

func (skl *SingleKeyLimiter) WaitN(ctx context.Context, n int) error {
	return skl.baseLimiter.WaitN(ctx, skl.key, n)
}

func (skl *SingleKeyLimiter) Reserve() (*Reservation, error) {
	return skl.baseLimiter.Reserve(skl.key)
}

func (skl *SingleKeyLimiter) ReserveN(n int) (*Reservation, error) {
	return skl.baseLimiter.ReserveN(skl.key, n)
}

func (skl *SingleKeyLimiter) Reset() error {
	return skl.baseLimiter.Reset(skl.key)
}

func (skl *SingleKeyLimiter) GetLimit() (*LimitInfo, error) {
	return skl.baseLimiter.GetLimit(skl.key)
}

func (skl *SingleKeyLimiter) SetLimit(limit RateLimit) error {
	return skl.baseLimiter.SetLimit(skl.key, limit)
}

func (skl *SingleKeyLimiter) GetStats() (*Stats, error) {
	return skl.baseLimiter.GetStats(skl.key)
}

func (bl *BaseLimiter) Allow(key string) (bool, error) {
	return bl.AllowN(key, 1)
}

func (bl *BaseLimiter) AllowN(key string, n int) (bool, error) {
	if n <= 0 {
		return false, fmt.Errorf("n must be positive")
	}

	state := bl.getOrCreateState(key)
	state.mu.Lock()
	defer state.mu.Unlock()

	now := time.Now()
	bl.refillTokens(state, now)

	if state.tokens >= float64(n) {
		state.tokens -= float64(n)
		state.lastUpdate = now

		if bl.config.EnableStats {
			state.stats.TotalRequests++
			state.stats.AllowedRequests++
			state.stats.LastRequestAt = now
			bl.updateRate(state, now)
		}

		return true, nil
	}

	if bl.config.EnableStats {
		state.stats.TotalRequests++
		state.stats.DeniedRequests++
	}

	return false, nil
}

func (bl *BaseLimiter) Wait(ctx context.Context, key string) error {
	return bl.WaitN(ctx, key, 1)
}

func (bl *BaseLimiter) WaitN(ctx context.Context, key string, n int) error {
	if n <= 0 {
		return fmt.Errorf("n must be positive")
	}

	for {
		allowed, err := bl.AllowN(key, n)
		if err != nil {
			return err
		}

		if allowed {
			return nil
		}

		state := bl.getOrCreateState(key)
		state.mu.Lock()
		waitTime := bl.calculateWaitTime(state, n)
		state.mu.Unlock()

		if waitTime > bl.config.MaxWait {
			return fmt.Errorf("wait time exceeds max wait duration")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			continue
		}
	}
}

func (bl *BaseLimiter) Reserve(key string) (*Reservation, error) {
	return bl.ReserveN(key, 1)
}

func (bl *BaseLimiter) ReserveN(key string, n int) (*Reservation, error) {
	if n <= 0 {
		return nil, fmt.Errorf("n must be positive")
	}

	state := bl.getOrCreateState(key)
	state.mu.Lock()
	defer state.mu.Unlock()

	now := time.Now()
	bl.refillTokens(state, now)

	reservation := &Reservation{
		Limit:   state.limit,
		limiter: bl,
		key:     key,
		tokens:  n,
	}

	if state.tokens >= float64(n) {
		state.tokens -= float64(n)
		reservation.OK = true
		reservation.TimeToAct = now
		reservation.delay = 0
	} else {
		deficit := float64(n) - state.tokens
		rate := float64(state.limit.Rate) / float64(state.limit.Per)
		delayDuration := time.Duration(deficit / rate)

		if delayDuration > bl.config.MaxWait {
			reservation.OK = false
			return reservation, nil
		}

		reservation.OK = true
		reservation.delay = delayDuration
		reservation.TimeToAct = now.Add(delayDuration)
		state.tokens = 0
	}

	state.lastUpdate = now
	return reservation, nil
}

func (bl *BaseLimiter) Reset(key string) error {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	delete(bl.limiters, key)
	return nil
}

func (bl *BaseLimiter) GetLimit(key string) (*LimitInfo, error) {
	state := bl.getOrCreateState(key)
	state.mu.Lock()
	defer state.mu.Unlock()

	now := time.Now()
	bl.refillTokens(state, now)

	info := &LimitInfo{
		Key:       key,
		Rate:      state.limit.Rate,
		Per:       state.limit.Per,
		Burst:     state.limit.Burst,
		Available: int(state.tokens),
		ResetAt:   state.lastUpdate.Add(state.limit.Per),
	}

	if state.tokens < 1 {
		deficit := 1.0 - state.tokens
		rate := float64(state.limit.Rate) / float64(state.limit.Per)
		info.RetryAfter = time.Duration(deficit / rate)
	}

	return info, nil
}

func (bl *BaseLimiter) SetLimit(key string, limit RateLimit) error {
	if limit.Rate <= 0 || limit.Per <= 0 {
		return fmt.Errorf("invalid rate limit")
	}

	state := bl.getOrCreateState(key)
	state.mu.Lock()
	defer state.mu.Unlock()

	state.limit = limit
	if state.tokens > float64(limit.Burst) {
		state.tokens = float64(limit.Burst)
	}

	return nil
}

func (bl *BaseLimiter) GetStats(key string) (*Stats, error) {
	if !bl.config.EnableStats {
		return nil, fmt.Errorf("stats not enabled")
	}

	bl.mu.RLock()
	state, exists := bl.limiters[key]
	bl.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("key not found")
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	stats := &Stats{
		Key:             key,
		TotalRequests:   state.stats.TotalRequests,
		AllowedRequests: state.stats.AllowedRequests,
		DeniedRequests:  state.stats.DeniedRequests,
		CurrentRate:     state.stats.CurrentRate,
		AverageRate:     state.stats.AverageRate,
		PeakRate:        state.stats.PeakRate,
		LastRequestAt:   state.stats.LastRequestAt,
		WindowStart:     state.stats.WindowStart,
		WindowEnd:       state.stats.WindowEnd,
	}

	return stats, nil
}

func (bl *BaseLimiter) Close() error {
	close(bl.stopCh)
	bl.wg.Wait()
	return nil
}

func (bl *BaseLimiter) getOrCreateState(key string) *limiterState {
	bl.mu.RLock()
	state, exists := bl.limiters[key]
	bl.mu.RUnlock()

	if exists {
		return state
	}

	bl.mu.Lock()
	defer bl.mu.Unlock()

	state, exists = bl.limiters[key]
	if exists {
		return state
	}

	limit := RateLimit{
		Rate:    bl.config.DefaultRate,
		Per:     bl.config.DefaultPer,
		Burst:   bl.config.DefaultBurst,
		MaxWait: bl.config.MaxWait,
	}

	state = &limiterState{
		key:        key,
		limit:      limit,
		tokens:     float64(limit.Burst),
		lastUpdate: time.Now(),
		requests:   make([]time.Time, 0),
		stats: &Stats{
			Key:         key,
			WindowStart: time.Now(),
		},
	}

	bl.limiters[key] = state
	return state
}

func (bl *BaseLimiter) refillTokens(state *limiterState, now time.Time) {
	elapsed := now.Sub(state.lastUpdate)
	if elapsed <= 0 {
		return
	}

	rate := float64(state.limit.Rate) / float64(state.limit.Per)
	tokensToAdd := rate * float64(elapsed)

	state.tokens += tokensToAdd
	if state.tokens > float64(state.limit.Burst) {
		state.tokens = float64(state.limit.Burst)
	}
}

func (bl *BaseLimiter) calculateWaitTime(state *limiterState, n int) time.Duration {
	now := time.Now()
	bl.refillTokens(state, now)

	if state.tokens >= float64(n) {
		return 0
	}

	deficit := float64(n) - state.tokens
	rate := float64(state.limit.Rate) / float64(state.limit.Per)
	return time.Duration(deficit / rate)
}

func (bl *BaseLimiter) updateRate(state *limiterState, now time.Time) {
	cutoff := now.Add(-state.limit.Per)

	validRequests := make([]time.Time, 0)
	for _, reqTime := range state.requests {
		if reqTime.After(cutoff) {
			validRequests = append(validRequests, reqTime)
		}
	}

	validRequests = append(validRequests, now)
	state.requests = validRequests

	if len(validRequests) > 1 {
		duration := now.Sub(validRequests[0])
		if duration > 0 {
			state.stats.CurrentRate = float64(len(validRequests)) / duration.Seconds()
		}
	}

	if state.stats.TotalRequests > 0 {
		totalDuration := now.Sub(state.stats.WindowStart)
		if totalDuration > 0 {
			state.stats.AverageRate = float64(state.stats.TotalRequests) / totalDuration.Seconds()
		}
	}

	if state.stats.CurrentRate > state.stats.PeakRate {
		state.stats.PeakRate = state.stats.CurrentRate
	}
}

func (bl *BaseLimiter) startCleanup() {
	bl.wg.Add(1)
	go func() {
		defer bl.wg.Done()
		ticker := time.NewTicker(bl.config.CleanupInterval)
		defer ticker.Stop()

		for {
			select {
			case <-bl.stopCh:
				return
			case <-ticker.C:
				bl.cleanup()
			}
		}
	}()
}

func (bl *BaseLimiter) cleanup() {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-bl.config.StatsRetention)

	for key, state := range bl.limiters {
		state.mu.Lock()
		if state.stats.LastRequestAt.Before(cutoff) {
			delete(bl.limiters, key)
		}
		state.mu.Unlock()
	}
}

func (r *Reservation) Cancel() {
	if !r.OK {
		return
	}

	state := r.limiter.getOrCreateState(r.key)
	state.mu.Lock()
	defer state.mu.Unlock()

	state.tokens += float64(r.tokens)
	if state.tokens > float64(state.limit.Burst) {
		state.tokens = float64(state.limit.Burst)
	}
}

func (r *Reservation) Delay() time.Duration {
	return r.delay
}

func (r *Reservation) DelayFrom(now time.Time) time.Duration {
	if r.TimeToAct.Before(now) {
		return 0
	}
	return r.TimeToAct.Sub(now)
}

func CreateRateLimit(rate int, per time.Duration, burst int) RateLimit {
	return RateLimit{
		Rate:  rate,
		Per:   per,
		Burst: burst,
	}
}

func RateLimitPerSecond(rate int, burst int) RateLimit {
	return CreateRateLimit(rate, time.Second, burst)
}

func RateLimitPerMinute(rate int, burst int) RateLimit {
	return CreateRateLimit(rate, time.Minute, burst)
}

func RateLimitPerHour(rate int, burst int) RateLimit {
	return CreateRateLimit(rate, time.Hour, burst)
}

func RateLimitPerDay(rate int, burst int) RateLimit {
	return CreateRateLimit(rate, 24*time.Hour, burst)
}

func (rl *RateLimit) Validate() error {
	if rl.Rate <= 0 {
		return fmt.Errorf("rate must be positive")
	}
	if rl.Per <= 0 {
		return fmt.Errorf("duration must be positive")
	}
	if rl.Burst < 0 {
		return fmt.Errorf("burst cannot be negative")
	}
	if rl.Burst < rl.Rate {
		rl.Burst = rl.Rate
	}
	return nil
}

func (rl *RateLimit) RatePerSecond() float64 {
	return float64(rl.Rate) / rl.Per.Seconds()
}

func (rl *RateLimit) String() string {
	return fmt.Sprintf("%d per %s (burst: %d)", rl.Rate, rl.Per, rl.Burst)
}

func (s *Stats) SuccessRate() float64 {
	if s.TotalRequests == 0 {
		return 0
	}
	return float64(s.AllowedRequests) / float64(s.TotalRequests) * 100
}

func (s *Stats) DenialRate() float64 {
	if s.TotalRequests == 0 {
		return 0
	}
	return float64(s.DeniedRequests) / float64(s.TotalRequests) * 100
}

func (s *Stats) Reset() {
	s.TotalRequests = 0
	s.AllowedRequests = 0
	s.DeniedRequests = 0
	s.CurrentRate = 0
	s.AverageRate = 0
	s.WindowStart = time.Now()
}
