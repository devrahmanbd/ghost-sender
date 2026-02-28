package ratelimiter

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

type DistributedLimiter struct {
	client       *redis.Client
	config       *DistributedConfig
	scripts      map[string]*redis.Script
	fallback     RateLimiter
	mu           sync.RWMutex
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

type DistributedConfig struct {
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
	KeyPrefix         string
	DefaultRate       int
	DefaultPer        time.Duration
	DefaultBurst      int
	MaxWait           time.Duration
	EnableFallback    bool
	FallbackOnError   bool
	SyncInterval      time.Duration
	KeyExpiration     time.Duration
	PoolSize          int
	MinIdleConns      int
	MaxRetries        int
	DialTimeout       time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
}

type distributedState struct {
	tokens     float64
	lastUpdate int64
	rate       float64
	capacity   int
}

const (
	luaAllowScript = `
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])
local ttl = tonumber(ARGV[5])

local state = redis.call('HMGET', key, 'tokens', 'last_update')
local tokens = tonumber(state[1])
local last_update = tonumber(state[2])

if tokens == nil then
    tokens = capacity
    last_update = now
end

local elapsed = math.max(0, now - last_update)
tokens = math.min(capacity, tokens + (elapsed * rate))

local allowed = 0
if tokens >= requested then
    tokens = tokens - requested
    allowed = 1
end

redis.call('HMSET', key, 'tokens', tokens, 'last_update', now)
redis.call('EXPIRE', key, ttl)

return {allowed, tokens, now}
`

	luaReserveScript = `
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])
local max_wait = tonumber(ARGV[5])
local ttl = tonumber(ARGV[6])

local state = redis.call('HMGET', key, 'tokens', 'last_update')
local tokens = tonumber(state[1])
local last_update = tonumber(state[2])

if tokens == nil then
    tokens = capacity
    last_update = now
end

local elapsed = math.max(0, now - last_update)
tokens = math.min(capacity, tokens + (elapsed * rate))

local wait_time = 0
local ok = 1

if tokens >= requested then
    tokens = tokens - requested
else
    local deficit = requested - tokens
    wait_time = deficit / rate
    if wait_time > max_wait then
        ok = 0
    else
        tokens = 0
    end
end

if ok == 1 then
    redis.call('HMSET', key, 'tokens', tokens, 'last_update', now)
    redis.call('EXPIRE', key, ttl)
end

return {ok, wait_time, tokens}
`

	luaResetScript = `
local key = KEYS[1]
return redis.call('DEL', key)
`

	luaGetInfoScript = `
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

local state = redis.call('HMGET', key, 'tokens', 'last_update')
local tokens = tonumber(state[1])
local last_update = tonumber(state[2])

if tokens == nil then
    tokens = capacity
    last_update = now
end

local elapsed = math.max(0, now - last_update)
tokens = math.min(capacity, tokens + (elapsed * rate))

return {tokens, last_update}
`
)

func NewDistributedLimiter(config *DistributedConfig) (*DistributedLimiter, error) {
	if config == nil {
		config = DefaultDistributedConfig()
	}

	client := redis.NewClient(&redis.Options{
		Addr:         config.RedisAddr,
		Password:     config.RedisPassword,
		DB:           config.RedisDB,
		PoolSize:     config.PoolSize,
		MinIdleConns: config.MinIdleConns,
		MaxRetries:   config.MaxRetries,
		DialTimeout:  config.DialTimeout,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		if !config.EnableFallback {
			return nil, fmt.Errorf("failed to connect to Redis: %w", err)
		}
	}

	dl := &DistributedLimiter{
		client:  client,
		config:  config,
		scripts: make(map[string]*redis.Script),
		stopCh:  make(chan struct{}),
	}

	dl.scripts["allow"] = redis.NewScript(luaAllowScript)
	dl.scripts["reserve"] = redis.NewScript(luaReserveScript)
	dl.scripts["reset"] = redis.NewScript(luaResetScript)
	dl.scripts["info"] = redis.NewScript(luaGetInfoScript)

	if config.EnableFallback {
		dl.fallback = NewBaseLimiter(nil)
	}

	return dl, nil
}

func DefaultDistributedConfig() *DistributedConfig {
	return &DistributedConfig{
		RedisAddr:       "localhost:6379",
		RedisPassword:   "",
		RedisDB:         0,
		KeyPrefix:       "ratelimit:",
		DefaultRate:     100,
		DefaultPer:      time.Minute,
		DefaultBurst:    10,
		MaxWait:         30 * time.Second,
		EnableFallback:  true,
		FallbackOnError: true,
		SyncInterval:    5 * time.Second,
		KeyExpiration:   24 * time.Hour,
		PoolSize:        10,
		MinIdleConns:    2,
		MaxRetries:      3,
		DialTimeout:     5 * time.Second,
		ReadTimeout:     3 * time.Second,
		WriteTimeout:    3 * time.Second,
	}
}

func (dl *DistributedLimiter) Allow(key string) (bool, error) {
	return dl.AllowN(key, 1)
}

func (dl *DistributedLimiter) AllowN(key string, n int) (bool, error) {
	if n <= 0 {
		return false, fmt.Errorf("n must be positive")
	}

	redisKey := dl.getRedisKey(key)
	rate := float64(dl.config.DefaultRate) / dl.config.DefaultPer.Seconds()
	now := time.Now().Unix()
	ttl := int(dl.config.KeyExpiration.Seconds())

	ctx, cancel := context.WithTimeout(context.Background(), dl.config.WriteTimeout)
	defer cancel()

	result, err := dl.scripts["allow"].Run(ctx, dl.client, []string{redisKey},
		rate, dl.config.DefaultBurst, now, n, ttl).Result()

	if err != nil {
		if dl.config.FallbackOnError && dl.fallback != nil {
			return dl.fallback.AllowN(key, n)
		}
		return false, fmt.Errorf("redis error: %w", err)
	}

	resultSlice, ok := result.([]interface{})
	if !ok || len(resultSlice) < 1 {
		return false, fmt.Errorf("unexpected result format")
	}

	allowed, _ := resultSlice[0].(int64)
	return allowed == 1, nil
}

func (dl *DistributedLimiter) Wait(ctx context.Context, key string) error {
	return dl.WaitN(ctx, key, 1)
}

func (dl *DistributedLimiter) WaitN(ctx context.Context, key string, n int) error {
	if n <= 0 {
		return fmt.Errorf("n must be positive")
	}

	for {
		allowed, err := dl.AllowN(key, n)
		if err != nil {
			return err
		}

		if allowed {
			return nil
		}

		info, err := dl.GetLimit(key)
		if err != nil {
			return err
		}

		if info.RetryAfter > dl.config.MaxWait {
			return fmt.Errorf("wait time exceeds max wait duration")
		}

		timer := time.NewTimer(info.RetryAfter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			continue
		}
	}
}

func (dl *DistributedLimiter) Reserve(key string) (*Reservation, error) {
	return dl.ReserveN(key, 1)
}

func (dl *DistributedLimiter) ReserveN(key string, n int) (*Reservation, error) {
	if n <= 0 {
		return nil, fmt.Errorf("n must be positive")
	}

	redisKey := dl.getRedisKey(key)
	rate := float64(dl.config.DefaultRate) / dl.config.DefaultPer.Seconds()
	now := time.Now().Unix()
	maxWait := dl.config.MaxWait.Seconds()
	ttl := int(dl.config.KeyExpiration.Seconds())

	ctx, cancel := context.WithTimeout(context.Background(), dl.config.WriteTimeout)
	defer cancel()

	result, err := dl.scripts["reserve"].Run(ctx, dl.client, []string{redisKey},
		rate, dl.config.DefaultBurst, now, n, maxWait, ttl).Result()

	if err != nil {
		if dl.config.FallbackOnError && dl.fallback != nil {
			return dl.fallback.ReserveN(key, n)
		}
		return nil, fmt.Errorf("redis error: %w", err)
	}

	resultSlice, ok := result.([]interface{})
	if !ok || len(resultSlice) < 3 {
		return nil, fmt.Errorf("unexpected result format")
	}

	okVal, _ := resultSlice[0].(int64)
	waitTimeFloat, _ := resultSlice[1].(string)
	waitTime, _ := strconv.ParseFloat(waitTimeFloat, 64)

	delayDuration := time.Duration(waitTime * float64(time.Second))

	reservation := &Reservation{
		OK:     okVal == 1,
		tokens: n,
		key:    key,
	}

	if reservation.OK {
		reservation.TimeToAct = time.Now().Add(delayDuration)
	}

	return reservation, nil
}


func (dl *DistributedLimiter) Reset(key string) error {
	redisKey := dl.getRedisKey(key)

	ctx, cancel := context.WithTimeout(context.Background(), dl.config.WriteTimeout)
	defer cancel()

	_, err := dl.scripts["reset"].Run(ctx, dl.client, []string{redisKey}).Result()
	if err != nil {
		if dl.config.FallbackOnError && dl.fallback != nil {
			return dl.fallback.Reset(key)
		}
		return fmt.Errorf("redis error: %w", err)
	}

	return nil
}

func (dl *DistributedLimiter) GetLimit(key string) (*LimitInfo, error) {
	redisKey := dl.getRedisKey(key)
	rate := float64(dl.config.DefaultRate) / dl.config.DefaultPer.Seconds()
	now := time.Now().Unix()

	ctx, cancel := context.WithTimeout(context.Background(), dl.config.ReadTimeout)
	defer cancel()

	result, err := dl.scripts["info"].Run(ctx, dl.client, []string{redisKey},
		rate, dl.config.DefaultBurst, now).Result()

	if err != nil {
		if dl.config.FallbackOnError && dl.fallback != nil {
			return dl.fallback.GetLimit(key)
		}
		return nil, fmt.Errorf("redis error: %w", err)
	}

	resultSlice, ok := result.([]interface{})
	if !ok || len(resultSlice) < 2 {
		return nil, fmt.Errorf("unexpected result format")
	}

	tokensStr, _ := resultSlice[0].(string)
	tokens, _ := strconv.ParseFloat(tokensStr, 64)

	info := &LimitInfo{
		Key:       key,
		Rate:      dl.config.DefaultRate,
		Per:       dl.config.DefaultPer,
		Burst:     dl.config.DefaultBurst,
		Available: int(tokens),
		ResetAt:   time.Now().Add(dl.config.DefaultPer),
	}

	if tokens < 1 {
		deficit := 1.0 - tokens
		info.RetryAfter = time.Duration(deficit / rate * float64(time.Second))
	}

	return info, nil
}

func (dl *DistributedLimiter) SetLimit(key string, limit RateLimit) error {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	if err := limit.Validate(); err != nil {
		return err
	}

	dl.config.DefaultRate = limit.Rate
	dl.config.DefaultPer = limit.Per
	dl.config.DefaultBurst = limit.Burst

	if dl.fallback != nil {
		return dl.fallback.SetLimit(key, limit)
	}

	return nil
}

func (dl *DistributedLimiter) GetStats(key string) (*Stats, error) {
	if dl.fallback != nil {
		return dl.fallback.GetStats(key)
	}
	return nil, fmt.Errorf("stats not available in distributed mode without fallback")
}

func (dl *DistributedLimiter) Close() error {
	close(dl.stopCh)
	dl.wg.Wait()

	if dl.client != nil {
		if err := dl.client.Close(); err != nil {
			return err
		}
	}

	if dl.fallback != nil {
		return dl.fallback.Close()
	}

	return nil
}

func (dl *DistributedLimiter) getRedisKey(key string) string {
	return dl.config.KeyPrefix + key
}

func (dl *DistributedLimiter) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return dl.client.Ping(ctx).Err()
}

func (dl *DistributedLimiter) GetClient() *redis.Client {
	return dl.client
}

func (dl *DistributedLimiter) FlushAll() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pattern := dl.config.KeyPrefix + "*"
	iter := dl.client.Scan(ctx, 0, pattern, 0).Iterator()

	pipe := dl.client.Pipeline()
	count := 0

	for iter.Next(ctx) {
		pipe.Del(ctx, iter.Val())
		count++

		if count >= 100 {
			if _, err := pipe.Exec(ctx); err != nil {
				return err
			}
			pipe = dl.client.Pipeline()
			count = 0
		}
	}

	if count > 0 {
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
	}

	return iter.Err()
}

func (dl *DistributedLimiter) GetAllKeys() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pattern := dl.config.KeyPrefix + "*"
	var keys []string

	iter := dl.client.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		keys = append(keys, key[len(dl.config.KeyPrefix):])
	}

	return keys, iter.Err()
}

func (dl *DistributedLimiter) SetKeyExpiration(key string, expiration time.Duration) error {
	redisKey := dl.getRedisKey(key)

	ctx, cancel := context.WithTimeout(context.Background(), dl.config.WriteTimeout)
	defer cancel()

	return dl.client.Expire(ctx, redisKey, expiration).Err()
}

func (dl *DistributedLimiter) GetKeyTTL(key string) (time.Duration, error) {
	redisKey := dl.getRedisKey(key)

	ctx, cancel := context.WithTimeout(context.Background(), dl.config.ReadTimeout)
	defer cancel()

	return dl.client.TTL(ctx, redisKey).Result()
}

func (dl *DistributedLimiter) ExecuteLua(script string, keys []string, args ...interface{}) (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dl.config.WriteTimeout)
	defer cancel()

	hash := sha1.Sum([]byte(script))
	scriptHash := hex.EncodeToString(hash[:])

	result, err := dl.client.EvalSha(ctx, scriptHash, keys, args...).Result()
	if err != nil {
		if err.Error() == "NOSCRIPT No matching script. Please use EVAL." {
			result, err = dl.client.Eval(ctx, script, keys, args...).Result()
		}
	}

	return result, err
}

func (dl *DistributedLimiter) AcquireLock(key string, expiration time.Duration) (bool, error) {
	lockKey := dl.getRedisKey("lock:" + key)

	ctx, cancel := context.WithTimeout(context.Background(), dl.config.WriteTimeout)
	defer cancel()

	return dl.client.SetNX(ctx, lockKey, "1", expiration).Result()
}

func (dl *DistributedLimiter) ReleaseLock(key string) error {
	lockKey := dl.getRedisKey("lock:" + key)

	ctx, cancel := context.WithTimeout(context.Background(), dl.config.WriteTimeout)
	defer cancel()

	return dl.client.Del(ctx, lockKey).Err()
}

func (dl *DistributedLimiter) IsHealthy() bool {
	return dl.Ping() == nil
}

func (dl *DistributedLimiter) GetConfig() *DistributedConfig {
	dl.mu.RLock()
	defer dl.mu.RUnlock()
	return dl.config
}

func (dl *DistributedLimiter) UpdateConfig(config *DistributedConfig) error {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	dl.config = config
	return nil
}

