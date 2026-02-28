package cache

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client     redis.UniversalClient
	config     *CacheConfig
	keyPrefix  string
	stats      *redisCacheStats
	mu         sync.RWMutex
	closed     bool
	serializer Serializer
}

type redisCacheStats struct {
	hits      atomic.Int64
	misses    atomic.Int64
	evictions atomic.Int64
}

func NewRedisCache(config *CacheConfig) (*RedisCache, error) {
	if config == nil {
		return nil, fmt.Errorf("cache config is required")
	}

	opts := &redis.UniversalOptions{
		Addrs:        []string{fmt.Sprintf("%s:%d", config.Host, config.Port)},
		Password:     config.Password,
		DB:           config.Database,
		PoolSize:     config.PoolSize,
		MinIdleConns: config.MinIdleConns,
		DialTimeout:  config.ConnTimeout,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,	
		PoolTimeout:  config.PoolTimeout,
		ConnMaxIdleTime:  config.IdleTimeout,
		MaxRetries:   config.MaxRetries,
	}

	client := redis.NewUniversalClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, &CacheError{
			Op:  "connect",
			Err: fmt.Errorf("failed to connect to redis: %w", err),
		}
	}

	return &RedisCache{
		client:     client,
		config:     config,
		keyPrefix:  config.KeyPrefix,
		stats:      &redisCacheStats{},
		serializer: NewJSONSerializer(),
	}, nil
}

func (r *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	if r.isClosed() {
		return nil, &CacheError{Op: "get", Key: key, Err: fmt.Errorf("cache is closed")}
	}

	fullKey := r.buildKey(key)
	result, err := r.client.Get(ctx, fullKey).Bytes()

	if err == redis.Nil {
		r.stats.misses.Add(1)
		return nil, &CacheError{Op: "get", Key: key, Err: errCacheMiss}
	}

	if err != nil {
		return nil, &CacheError{Op: "get", Key: key, Err: err}
	}

	r.stats.hits.Add(1)
	return result, nil
}

func (r *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if r.isClosed() {
		return &CacheError{Op: "set", Key: key, Err: fmt.Errorf("cache is closed")}
	}

	if value == nil {
		return &CacheError{Op: "set", Key: key, Err: errInvalidValue}
	}

	fullKey := r.buildKey(key)
	err := r.client.Set(ctx, fullKey, value, ttl).Err()

	if err != nil {
		return &CacheError{Op: "set", Key: key, Err: err}
	}

	return nil
}

func (r *RedisCache) Delete(ctx context.Context, key string) error {
	if r.isClosed() {
		return &CacheError{Op: "delete", Key: key, Err: fmt.Errorf("cache is closed")}
	}

	fullKey := r.buildKey(key)
	err := r.client.Del(ctx, fullKey).Err()

	if err != nil {
		return &CacheError{Op: "delete", Key: key, Err: err}
	}

	return nil
}

func (r *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	if r.isClosed() {
		return false, &CacheError{Op: "exists", Key: key, Err: fmt.Errorf("cache is closed")}
	}

	fullKey := r.buildKey(key)
	result, err := r.client.Exists(ctx, fullKey).Result()

	if err != nil {
		return false, &CacheError{Op: "exists", Key: key, Err: err}
	}

	return result > 0, nil
}

func (r *RedisCache) Clear(ctx context.Context) error {
	if r.isClosed() {
		return &CacheError{Op: "clear", Err: fmt.Errorf("cache is closed")}
	}

	pattern := r.buildKey("*")
	iter := r.client.Scan(ctx, 0, pattern, 100).Iterator()

	pipe := r.client.Pipeline()
	count := 0

	for iter.Next(ctx) {
		pipe.Del(ctx, iter.Val())
		count++

		if count%1000 == 0 {
			if _, err := pipe.Exec(ctx); err != nil {
				return &CacheError{Op: "clear", Err: err}
			}
			pipe = r.client.Pipeline()
		}
	}

	if count%1000 != 0 {
		if _, err := pipe.Exec(ctx); err != nil {
			return &CacheError{Op: "clear", Err: err}
		}
	}

	if err := iter.Err(); err != nil {
		return &CacheError{Op: "clear", Err: err}
	}

	return nil
}

func (r *RedisCache) GetMulti(ctx context.Context, keys []string) (map[string][]byte, error) {
	if r.isClosed() {
		return nil, &CacheError{Op: "get_multi", Err: fmt.Errorf("cache is closed")}
	}

	if len(keys) == 0 {
		return make(map[string][]byte), nil
	}

	fullKeys := make([]string, len(keys))
	for i, key := range keys {
		fullKeys[i] = r.buildKey(key)
	}

	results, err := r.client.MGet(ctx, fullKeys...).Result()
	if err != nil {
		return nil, &CacheError{Op: "get_multi", Err: err}
	}

	resultMap := make(map[string][]byte)
	for i, result := range results {
		if result != nil {
			if str, ok := result.(string); ok {
				resultMap[keys[i]] = []byte(str)
				r.stats.hits.Add(1)
			}
		} else {
			r.stats.misses.Add(1)
		}
	}

	return resultMap, nil
}

func (r *RedisCache) SetMulti(ctx context.Context, items map[string][]byte, ttl time.Duration) error {
	if r.isClosed() {
		return &CacheError{Op: "set_multi", Err: fmt.Errorf("cache is closed")}
	}

	if len(items) == 0 {
		return nil
	}

	pipe := r.client.Pipeline()

	for key, value := range items {
		fullKey := r.buildKey(key)
		pipe.Set(ctx, fullKey, value, ttl)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return &CacheError{Op: "set_multi", Err: err}
	}

	return nil
}

func (r *RedisCache) DeleteMulti(ctx context.Context, keys []string) error {
	if r.isClosed() {
		return &CacheError{Op: "delete_multi", Err: fmt.Errorf("cache is closed")}
	}

	if len(keys) == 0 {
		return nil
	}

	fullKeys := make([]string, len(keys))
	for i, key := range keys {
		fullKeys[i] = r.buildKey(key)
	}

	err := r.client.Del(ctx, fullKeys...).Err()
	if err != nil {
		return &CacheError{Op: "delete_multi", Err: err}
	}

	return nil
}

func (r *RedisCache) Increment(ctx context.Context, key string, delta int64) (int64, error) {
	if r.isClosed() {
		return 0, &CacheError{Op: "increment", Key: key, Err: fmt.Errorf("cache is closed")}
	}

	fullKey := r.buildKey(key)
	result, err := r.client.IncrBy(ctx, fullKey, delta).Result()

	if err != nil {
		return 0, &CacheError{Op: "increment", Key: key, Err: err}
	}

	return result, nil
}

func (r *RedisCache) Decrement(ctx context.Context, key string, delta int64) (int64, error) {
	if r.isClosed() {
		return 0, &CacheError{Op: "decrement", Key: key, Err: fmt.Errorf("cache is closed")}
	}

	fullKey := r.buildKey(key)
	result, err := r.client.DecrBy(ctx, fullKey, delta).Result()

	if err != nil {
		return 0, &CacheError{Op: "decrement", Key: key, Err: err}
	}

	return result, nil
}

func (r *RedisCache) IncrementWithTTL(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, error) {
	if r.isClosed() {
		return 0, &CacheError{Op: "increment_ttl", Key: key, Err: fmt.Errorf("cache is closed")}
	}

	fullKey := r.buildKey(key)

	pipe := r.client.Pipeline()
	incrCmd := pipe.IncrBy(ctx, fullKey, delta)
	pipe.Expire(ctx, fullKey, ttl)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, &CacheError{Op: "increment_ttl", Key: key, Err: err}
	}

	return incrCmd.Val(), nil
}

func (r *RedisCache) DeletePattern(ctx context.Context, pattern string) error {
	if r.isClosed() {
		return &CacheError{Op: "delete_pattern", Err: fmt.Errorf("cache is closed")}
	}

	fullPattern := r.buildKey(pattern)
	iter := r.client.Scan(ctx, 0, fullPattern, 100).Iterator()

	pipe := r.client.Pipeline()
	count := 0

	for iter.Next(ctx) {
		pipe.Del(ctx, iter.Val())
		count++

		if count%1000 == 0 {
			if _, err := pipe.Exec(ctx); err != nil {
				return &CacheError{Op: "delete_pattern", Err: err}
			}
			pipe = r.client.Pipeline()
		}
	}

	if count%1000 != 0 {
		if _, err := pipe.Exec(ctx); err != nil {
			return &CacheError{Op: "delete_pattern", Err: err}
		}
	}

	if err := iter.Err(); err != nil {
		return &CacheError{Op: "delete_pattern", Err: err}
	}

	return nil
}

func (r *RedisCache) Keys(ctx context.Context, pattern string) ([]string, error) {
	if r.isClosed() {
		return nil, &CacheError{Op: "keys", Err: fmt.Errorf("cache is closed")}
	}

	fullPattern := r.buildKey(pattern)
	var keys []string

	iter := r.client.Scan(ctx, 0, fullPattern, 100).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		if r.keyPrefix != "" {
			key = strings.TrimPrefix(key, r.keyPrefix)
		}
		keys = append(keys, key)
	}

	if err := iter.Err(); err != nil {
		return nil, &CacheError{Op: "keys", Err: err}
	}

	return keys, nil
}

func (r *RedisCache) TTL(ctx context.Context, key string) (time.Duration, error) {
	if r.isClosed() {
		return 0, &CacheError{Op: "ttl", Key: key, Err: fmt.Errorf("cache is closed")}
	}

	fullKey := r.buildKey(key)
	duration, err := r.client.TTL(ctx, fullKey).Result()

	if err != nil {
		return 0, &CacheError{Op: "ttl", Key: key, Err: err}
	}

	return duration, nil
}

func (r *RedisCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if r.isClosed() {
		return &CacheError{Op: "expire", Key: key, Err: fmt.Errorf("cache is closed")}
	}

	fullKey := r.buildKey(key)
	err := r.client.Expire(ctx, fullKey, ttl).Err()

	if err != nil {
		return &CacheError{Op: "expire", Key: key, Err: err}
	}

	return nil
}

func (r *RedisCache) Persist(ctx context.Context, key string) error {
	if r.isClosed() {
		return &CacheError{Op: "persist", Key: key, Err: fmt.Errorf("cache is closed")}
	}

	fullKey := r.buildKey(key)
	err := r.client.Persist(ctx, fullKey).Err()

	if err != nil {
		return &CacheError{Op: "persist", Key: key, Err: err}
	}

	return nil
}

func (r *RedisCache) Stats(ctx context.Context) (*CacheStats, error) {
	if r.isClosed() {
		return nil, &CacheError{Op: "stats", Err: fmt.Errorf("cache is closed")}
	}

	info, err := r.client.Info(ctx, "stats", "memory", "server").Result()
	if err != nil {
		return nil, &CacheError{Op: "stats", Err: err}
	}

	stats := &CacheStats{
		Hits:   r.stats.hits.Load(),
		Misses: r.stats.misses.Load(),
	}

	lines := strings.Split(info, "\r\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "used_memory:") {
			fmt.Sscanf(line, "used_memory:%d", &stats.MemoryUsed)
		} else if strings.HasPrefix(line, "maxmemory:") {
			fmt.Sscanf(line, "maxmemory:%d", &stats.MemoryLimit)
		} else if strings.HasPrefix(line, "evicted_keys:") {
			fmt.Sscanf(line, "evicted_keys:%d", &stats.Evictions)
		} else if strings.HasPrefix(line, "uptime_in_seconds:") {
			var seconds int64
			fmt.Sscanf(line, "uptime_in_seconds:%d", &seconds)
			stats.Uptime = time.Duration(seconds) * time.Second
		}
	}

	dbSize, err := r.client.DBSize(ctx).Result()
	if err == nil {
		stats.Keys = dbSize
	}

	poolStats := r.client.PoolStats()
	stats.Connections = int(poolStats.TotalConns)

	return stats, nil
}

func (r *RedisCache) Size(ctx context.Context) (int64, error) {
	if r.isClosed() {
		return 0, &CacheError{Op: "size", Err: fmt.Errorf("cache is closed")}
	}

	size, err := r.client.DBSize(ctx).Result()
	if err != nil {
		return 0, &CacheError{Op: "size", Err: err}
	}

	return size, nil
}

func (r *RedisCache) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	r.closed = true
	return r.client.Close()
}

func (r *RedisCache) buildKey(key string) string {
	if r.keyPrefix == "" {
		return key
	}
	return r.keyPrefix + key
}

func (r *RedisCache) isClosed() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.closed
}

func (r *RedisCache) Pipeline() redis.Pipeliner {
	return r.client.Pipeline()
}

func (r *RedisCache) Client() redis.UniversalClient {
	return r.client
}
