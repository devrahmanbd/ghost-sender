package ratelimiter

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

type TokenBucketLimiter struct {
	buckets map[string]*bucket
	mu      sync.RWMutex
	config  *TokenBucketConfig
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

type TokenBucketConfig struct {
	DefaultRate       int
	DefaultPer        time.Duration
	DefaultBurst      int
	MaxWait           time.Duration
	CleanupInterval   time.Duration
	InactiveThreshold time.Duration
	EnableStats       bool
	EnableAutoCleanup bool
	PrecisionMode     bool
}

type bucket struct {
	key          string
	rate         float64
	capacity     int
	tokens       float64
	lastRefill   time.Time
	maxWait      time.Duration
	mu           sync.Mutex
	stats        *bucketStats
	lastAccessed time.Time
}

type bucketStats struct {
	totalRequests    int64
	allowedRequests  int64
	deniedRequests   int64
	waitedRequests   int64
	totalWaitTime    time.Duration
	lastRequestTime  time.Time
	createdAt        time.Time
	mu               sync.Mutex
}

func NewTokenBucketLimiter(config *TokenBucketConfig) *TokenBucketLimiter {
	if config == nil {
		config = DefaultTokenBucketConfig()
	}

	tbl := &TokenBucketLimiter{
		buckets: make(map[string]*bucket),
		config:  config,
		stopCh:  make(chan struct{}),
	}

	if config.EnableAutoCleanup {
		tbl.startCleanup()
	}

	return tbl
}

func DefaultTokenBucketConfig() *TokenBucketConfig {
	return &TokenBucketConfig{
		DefaultRate:       100,
		DefaultPer:        time.Minute,
		DefaultBurst:      10,
		MaxWait:           30 * time.Second,
		CleanupInterval:   5 * time.Minute,
		InactiveThreshold: 30 * time.Minute,
		EnableStats:       true,
		EnableAutoCleanup: true,
		PrecisionMode:     false,
	}
}

func (tbl *TokenBucketLimiter) Allow(key string) (bool, error) {
	return tbl.AllowN(key, 1)
}

func (tbl *TokenBucketLimiter) AllowN(key string, n int) (bool, error) {
	if n <= 0 {
		return false, fmt.Errorf("n must be positive")
	}

	b := tbl.getOrCreateBucket(key)
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	tbl.refillBucket(b, now)
	b.lastAccessed = now

	if b.tokens >= float64(n) {
		b.tokens -= float64(n)
		
		if tbl.config.EnableStats {
			b.stats.mu.Lock()
			b.stats.totalRequests++
			b.stats.allowedRequests++
			b.stats.lastRequestTime = now
			b.stats.mu.Unlock()
		}
		
		return true, nil
	}

	if tbl.config.EnableStats {
		b.stats.mu.Lock()
		b.stats.totalRequests++
		b.stats.deniedRequests++
		b.stats.mu.Unlock()
	}

	return false, nil
}

func (tbl *TokenBucketLimiter) Wait(ctx context.Context, key string) error {
	return tbl.WaitN(ctx, key, 1)
}

func (tbl *TokenBucketLimiter) WaitN(ctx context.Context, key string, n int) error {
	if n <= 0 {
		return fmt.Errorf("n must be positive")
	}

	startWait := time.Now()
	
	for {
		allowed, err := tbl.AllowN(key, n)
		if err != nil {
			return err
		}

		if allowed {
			if tbl.config.EnableStats {
				b := tbl.getOrCreateBucket(key)
				b.stats.mu.Lock()
				b.stats.waitedRequests++
				b.stats.totalWaitTime += time.Since(startWait)
				b.stats.mu.Unlock()
			}
			return nil
		}

		b := tbl.getOrCreateBucket(key)
		b.mu.Lock()
		waitTime := tbl.calculateWaitTime(b, n)
		b.mu.Unlock()

		if waitTime > b.maxWait {
			return fmt.Errorf("wait time %v exceeds max wait duration %v", waitTime, b.maxWait)
		}

		if waitTime <= 0 {
			continue
		}

		timer := time.NewTimer(waitTime)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			continue
		}
	}
}

func (tbl *TokenBucketLimiter) Reserve(key string) (*Reservation, error) {
	return tbl.ReserveN(key, 1)
}

func (tbl *TokenBucketLimiter) ReserveN(key string, n int) (*Reservation, error) {
	if n <= 0 {
		return nil, fmt.Errorf("n must be positive")
	}

	b := tbl.getOrCreateBucket(key)
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	tbl.refillBucket(b, now)
	b.lastAccessed = now

	reservation := &Reservation{
		key:    key,
		tokens: n,
	}

	if b.tokens >= float64(n) {
		b.tokens -= float64(n)
		reservation.OK = true
		reservation.TimeToAct = now
		reservation.delay = 0
	} else {
		deficit := float64(n) - b.tokens
		delay := time.Duration(float64(time.Second) * deficit / b.rate)

		if delay > b.maxWait {
			reservation.OK = false
			return reservation, nil
		}

		reservation.OK = true
		reservation.delay = delay
		reservation.TimeToAct = now.Add(delay)
		b.tokens = 0
	}

	return reservation, nil
}

func (tbl *TokenBucketLimiter) Reset(key string) error {
	tbl.mu.Lock()
	defer tbl.mu.Unlock()

	delete(tbl.buckets, key)
	return nil
}

func (tbl *TokenBucketLimiter) GetLimit(key string) (*LimitInfo, error) {
	b := tbl.getOrCreateBucket(key)
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	tbl.refillBucket(b, now)

	rate := int(b.rate * float64(tbl.config.DefaultPer) / float64(time.Second))
	
	info := &LimitInfo{
		Key:       key,
		Rate:      rate,
		Per:       tbl.config.DefaultPer,
		Burst:     b.capacity,
		Available: int(b.tokens),
		ResetAt:   b.lastRefill.Add(tbl.config.DefaultPer),
	}

	if b.tokens < 1 {
		deficit := 1.0 - b.tokens
		info.RetryAfter = time.Duration(float64(time.Second) * deficit / b.rate)
	}

	return info, nil
}

func (tbl *TokenBucketLimiter) SetLimit(key string, limit RateLimit) error {
	if err := limit.Validate(); err != nil {
		return err
	}

	b := tbl.getOrCreateBucket(key)
	b.mu.Lock()
	defer b.mu.Unlock()

	b.rate = float64(limit.Rate) / limit.Per.Seconds()
	b.capacity = limit.Burst
	b.maxWait = limit.MaxWait

	if b.tokens > float64(limit.Burst) {
		b.tokens = float64(limit.Burst)
	}

	return nil
}

func (tbl *TokenBucketLimiter) GetStats(key string) (*Stats, error) {
	if !tbl.config.EnableStats {
		return nil, fmt.Errorf("stats not enabled")
	}

	tbl.mu.RLock()
	b, exists := tbl.buckets[key]
	tbl.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("key not found")
	}

	b.stats.mu.Lock()
	defer b.stats.mu.Unlock()

	var avgRate float64
	if b.stats.totalRequests > 0 {
		duration := time.Since(b.stats.createdAt)
		if duration > 0 {
			avgRate = float64(b.stats.totalRequests) / duration.Seconds()
		}
	}

	stats := &Stats{
		Key:             key,
		TotalRequests:   b.stats.totalRequests,
		AllowedRequests: b.stats.allowedRequests,
		DeniedRequests:  b.stats.deniedRequests,
		AverageRate:     avgRate,
		LastRequestAt:   b.stats.lastRequestTime,
		WindowStart:     b.stats.createdAt,
	}

	return stats, nil
}

func (tbl *TokenBucketLimiter) Close() error {
	close(tbl.stopCh)
	tbl.wg.Wait()
	return nil
}

func (tbl *TokenBucketLimiter) getOrCreateBucket(key string) *bucket {
	tbl.mu.RLock()
	b, exists := tbl.buckets[key]
	tbl.mu.RUnlock()

	if exists {
		return b
	}

	tbl.mu.Lock()
	defer tbl.mu.Unlock()

	b, exists = tbl.buckets[key]
	if exists {
		return b
	}

	now := time.Now()
	rate := float64(tbl.config.DefaultRate) / tbl.config.DefaultPer.Seconds()

	b = &bucket{
		key:          key,
		rate:         rate,
		capacity:     tbl.config.DefaultBurst,
		tokens:       float64(tbl.config.DefaultBurst),
		lastRefill:   now,
		maxWait:      tbl.config.MaxWait,
		lastAccessed: now,
		stats: &bucketStats{
			createdAt: now,
		},
	}

	tbl.buckets[key] = b
	return b
}

func (tbl *TokenBucketLimiter) refillBucket(b *bucket, now time.Time) {
	elapsed := now.Sub(b.lastRefill)
	if elapsed <= 0 {
		return
	}

	tokensToAdd := b.rate * elapsed.Seconds()
	
	if tbl.config.PrecisionMode {
		tokensToAdd = math.Floor(tokensToAdd*1000000) / 1000000
	}

	b.tokens += tokensToAdd
	if b.tokens > float64(b.capacity) {
		b.tokens = float64(b.capacity)
	}

	b.lastRefill = now
}

func (tbl *TokenBucketLimiter) calculateWaitTime(b *bucket, n int) time.Duration {
	now := time.Now()
	tbl.refillBucket(b, now)

	if b.tokens >= float64(n) {
		return 0
	}

	deficit := float64(n) - b.tokens
	return time.Duration(float64(time.Second) * deficit / b.rate)
}

func (tbl *TokenBucketLimiter) startCleanup() {
	tbl.wg.Add(1)
	go func() {
		defer tbl.wg.Done()
		ticker := time.NewTicker(tbl.config.CleanupInterval)
		defer ticker.Stop()

		for {
			select {
			case <-tbl.stopCh:
				return
			case <-ticker.C:
				tbl.cleanup()
			}
		}
	}()
}

func (tbl *TokenBucketLimiter) cleanup() {
	tbl.mu.Lock()
	defer tbl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-tbl.config.InactiveThreshold)

	for key, b := range tbl.buckets {
		b.mu.Lock()
		lastAccess := b.lastAccessed
		b.mu.Unlock()

		if lastAccess.Before(cutoff) {
			delete(tbl.buckets, key)
		}
	}
}

func (tbl *TokenBucketLimiter) GetBucketCount() int {
	tbl.mu.RLock()
	defer tbl.mu.RUnlock()
	return len(tbl.buckets)
}

func (tbl *TokenBucketLimiter) GetAllKeys() []string {
	tbl.mu.RLock()
	defer tbl.mu.RUnlock()

	keys := make([]string, 0, len(tbl.buckets))
	for key := range tbl.buckets {
		keys = append(keys, key)
	}
	return keys
}

func (tbl *TokenBucketLimiter) ResetAll() error {
	tbl.mu.Lock()
	defer tbl.mu.Unlock()

	tbl.buckets = make(map[string]*bucket)
	return nil
}

func (tbl *TokenBucketLimiter) GetBucketInfo(key string) (*BucketInfo, error) {
	tbl.mu.RLock()
	b, exists := tbl.buckets[key]
	tbl.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("bucket not found")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	tbl.refillBucket(b, now)

	info := &BucketInfo{
		Key:           key,
		Rate:          b.rate,
		Capacity:      b.capacity,
		CurrentTokens: b.tokens,
		LastRefill:    b.lastRefill,
		LastAccessed:  b.lastAccessed,
		MaxWait:       b.maxWait,
	}

	return info, nil
}

func (tbl *TokenBucketLimiter) SetBucketCapacity(key string, capacity int) error {
	if capacity <= 0 {
		return fmt.Errorf("capacity must be positive")
	}

	b := tbl.getOrCreateBucket(key)
	b.mu.Lock()
	defer b.mu.Unlock()

	b.capacity = capacity
	if b.tokens > float64(capacity) {
		b.tokens = float64(capacity)
	}

	return nil
}

func (tbl *TokenBucketLimiter) SetBucketRate(key string, rate float64) error {
	if rate <= 0 {
		return fmt.Errorf("rate must be positive")
	}

	b := tbl.getOrCreateBucket(key)
	b.mu.Lock()
	defer b.mu.Unlock()

	b.rate = rate
	return nil
}

func (tbl *TokenBucketLimiter) AddTokens(key string, tokens int) error {
	if tokens <= 0 {
		return fmt.Errorf("tokens must be positive")
	}

	b := tbl.getOrCreateBucket(key)
	b.mu.Lock()
	defer b.mu.Unlock()

	b.tokens += float64(tokens)
	if b.tokens > float64(b.capacity) {
		b.tokens = float64(b.capacity)
	}

	return nil
}

func (tbl *TokenBucketLimiter) RemoveTokens(key string, tokens int) error {
	if tokens <= 0 {
		return fmt.Errorf("tokens must be positive")
	}

	b := tbl.getOrCreateBucket(key)
	b.mu.Lock()
	defer b.mu.Unlock()

	b.tokens -= float64(tokens)
	if b.tokens < 0 {
		b.tokens = 0
	}

	return nil
}

func (tbl *TokenBucketLimiter) TryAllowN(key string, n int, timeout time.Duration) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- tbl.WaitN(ctx, key, n)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return false, err
		}
		return true, nil
	case <-time.After(timeout):
		return false, fmt.Errorf("timeout waiting for tokens")
	}
}

type BucketInfo struct {
	Key           string
	Rate          float64
	Capacity      int
	CurrentTokens float64
	LastRefill    time.Time
	LastAccessed  time.Time
	MaxWait       time.Duration
}

func (bi *BucketInfo) AvailableTokens() int {
	return int(bi.CurrentTokens)
}

func (bi *BucketInfo) IsEmpty() bool {
	return bi.CurrentTokens < 1
}

func (bi *BucketInfo) IsFull() bool {
	return int(bi.CurrentTokens) >= bi.Capacity
}

func (bi *BucketInfo) FillPercentage() float64 {
	return (bi.CurrentTokens / float64(bi.Capacity)) * 100
}

func (bs *bucketStats) GetAverageWaitTime() time.Duration {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.waitedRequests == 0 {
		return 0
	}
	return bs.totalWaitTime / time.Duration(bs.waitedRequests)
}

func (bs *bucketStats) GetSuccessRate() float64 {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.totalRequests == 0 {
		return 0
	}
	return (float64(bs.allowedRequests) / float64(bs.totalRequests)) * 100
}

