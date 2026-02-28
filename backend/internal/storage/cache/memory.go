package cache

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type cacheEntry struct {
	value     []byte
	expiresAt time.Time
	lastAccess time.Time
}

type MemoryCache struct {
	data       map[string]*cacheEntry
	mu         sync.RWMutex
	stats      *memoryCacheStats
	config     *MemoryCacheConfig
	stopChan   chan struct{}
	closed     bool
	serializer Serializer
}

type memoryCacheStats struct {
	hits      atomic.Int64
	misses    atomic.Int64
	evictions atomic.Int64
	keys      atomic.Int64
}

type MemoryCacheConfig struct {
	MaxSize          int64
	CleanupInterval  time.Duration
	DefaultTTL       time.Duration
	EvictionPolicy   string
	MaxMemoryPercent float64
}

func NewMemoryCache(config *MemoryCacheConfig) *MemoryCache {
	if config == nil {
		config = DefaultMemoryCacheConfig()
	}

	mc := &MemoryCache{
		data:       make(map[string]*cacheEntry),
		stats:      &memoryCacheStats{},
		config:     config,
		stopChan:   make(chan struct{}),
		serializer: NewJSONSerializer(),
	}

	go mc.cleanupLoop()

	return mc
}

func DefaultMemoryCacheConfig() *MemoryCacheConfig {
	return &MemoryCacheConfig{
		MaxSize:          10000,
		CleanupInterval:  5 * time.Minute,
		DefaultTTL:       1 * time.Hour,
		EvictionPolicy:   "lru",
		MaxMemoryPercent: 80.0,
	}
}

func (m *MemoryCache) Get(ctx context.Context, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, &CacheError{Op: "get", Key: key, Err: fmt.Errorf("cache is closed")}
	}

	entry, exists := m.data[key]
	if !exists {
		m.stats.misses.Add(1)
		return nil, &CacheError{Op: "get", Key: key, Err: errCacheMiss}
	}

	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		m.stats.misses.Add(1)
		return nil, &CacheError{Op: "get", Key: key, Err: errCacheMiss}
	}

	entry.lastAccess = time.Now()
	m.stats.hits.Add(1)

	valueCopy := make([]byte, len(entry.value))
	copy(valueCopy, entry.value)
	return valueCopy, nil
}

func (m *MemoryCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return &CacheError{Op: "set", Key: key, Err: fmt.Errorf("cache is closed")}
	}

	if value == nil {
		return &CacheError{Op: "set", Key: key, Err: errInvalidValue}
	}

	if m.shouldEvict() {
		m.evictEntries()
	}

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)

	if _, exists := m.data[key]; !exists {
		m.stats.keys.Add(1)
	}

	m.data[key] = &cacheEntry{
		value:      valueCopy,
		expiresAt:  expiresAt,
		lastAccess: time.Now(),
	}

	return nil
}

func (m *MemoryCache) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return &CacheError{Op: "delete", Key: key, Err: fmt.Errorf("cache is closed")}
	}

	if _, exists := m.data[key]; exists {
		delete(m.data, key)
		m.stats.keys.Add(-1)
	}

	return nil
}

func (m *MemoryCache) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return false, &CacheError{Op: "exists", Key: key, Err: fmt.Errorf("cache is closed")}
	}

	entry, exists := m.data[key]
	if !exists {
		return false, nil
	}

	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		return false, nil
	}

	return true, nil
}

func (m *MemoryCache) Clear(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return &CacheError{Op: "clear", Err: fmt.Errorf("cache is closed")}
	}

	m.data = make(map[string]*cacheEntry)
	m.stats.keys.Store(0)

	return nil
}

func (m *MemoryCache) GetMulti(ctx context.Context, keys []string) (map[string][]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, &CacheError{Op: "get_multi", Err: fmt.Errorf("cache is closed")}
	}

	result := make(map[string][]byte)
	now := time.Now()

	for _, key := range keys {
		entry, exists := m.data[key]
		if !exists {
			m.stats.misses.Add(1)
			continue
		}

		if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
			m.stats.misses.Add(1)
			continue
		}

		valueCopy := make([]byte, len(entry.value))
		copy(valueCopy, entry.value)
		result[key] = valueCopy
		entry.lastAccess = now
		m.stats.hits.Add(1)
	}

	return result, nil
}

func (m *MemoryCache) SetMulti(ctx context.Context, items map[string][]byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return &CacheError{Op: "set_multi", Err: fmt.Errorf("cache is closed")}
	}

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	for key, value := range items {
		if value == nil {
			continue
		}

		valueCopy := make([]byte, len(value))
		copy(valueCopy, value)

		if _, exists := m.data[key]; !exists {
			m.stats.keys.Add(1)
		}

		m.data[key] = &cacheEntry{
			value:      valueCopy,
			expiresAt:  expiresAt,
			lastAccess: time.Now(),
		}
	}

	return nil
}

func (m *MemoryCache) DeleteMulti(ctx context.Context, keys []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return &CacheError{Op: "delete_multi", Err: fmt.Errorf("cache is closed")}
	}

	for _, key := range keys {
		if _, exists := m.data[key]; exists {
			delete(m.data, key)
			m.stats.keys.Add(-1)
		}
	}

	return nil
}

func (m *MemoryCache) Increment(ctx context.Context, key string, delta int64) (int64, error) {
	return m.IncrementWithTTL(ctx, key, delta, m.config.DefaultTTL)
}

func (m *MemoryCache) Decrement(ctx context.Context, key string, delta int64) (int64, error) {
	return m.IncrementWithTTL(ctx, key, -delta, m.config.DefaultTTL)
}

func (m *MemoryCache) IncrementWithTTL(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return 0, &CacheError{Op: "increment", Key: key, Err: fmt.Errorf("cache is closed")}
	}

	entry, exists := m.data[key]
	var currentValue int64 = 0

	if exists {
		if len(entry.value) >= 8 {
			currentValue = int64(entry.value[0]) | int64(entry.value[1])<<8 |
				int64(entry.value[2])<<16 | int64(entry.value[3])<<24 |
				int64(entry.value[4])<<32 | int64(entry.value[5])<<40 |
				int64(entry.value[6])<<48 | int64(entry.value[7])<<56
		}
	} else {
		m.stats.keys.Add(1)
	}

	newValue := currentValue + delta
	valueBytes := make([]byte, 8)
	valueBytes[0] = byte(newValue)
	valueBytes[1] = byte(newValue >> 8)
	valueBytes[2] = byte(newValue >> 16)
	valueBytes[3] = byte(newValue >> 24)
	valueBytes[4] = byte(newValue >> 32)
	valueBytes[5] = byte(newValue >> 40)
	valueBytes[6] = byte(newValue >> 48)
	valueBytes[7] = byte(newValue >> 56)

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	m.data[key] = &cacheEntry{
		value:      valueBytes,
		expiresAt:  expiresAt,
		lastAccess: time.Now(),
	}

	return newValue, nil
}

func (m *MemoryCache) DeletePattern(ctx context.Context, pattern string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return &CacheError{Op: "delete_pattern", Err: fmt.Errorf("cache is closed")}
	}

	keysToDelete := []string{}
	for key := range m.data {
		matched, _ := filepath.Match(pattern, key)
		if matched {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(m.data, key)
		m.stats.keys.Add(-1)
	}

	return nil
}

func (m *MemoryCache) Keys(ctx context.Context, pattern string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, &CacheError{Op: "keys", Err: fmt.Errorf("cache is closed")}
	}

	var keys []string
	for key := range m.data {
		matched, _ := filepath.Match(pattern, key)
		if matched {
			keys = append(keys, key)
		}
	}

	return keys, nil
}

func (m *MemoryCache) TTL(ctx context.Context, key string) (time.Duration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return 0, &CacheError{Op: "ttl", Key: key, Err: fmt.Errorf("cache is closed")}
	}

	entry, exists := m.data[key]
	if !exists {
		return -2 * time.Second, nil
	}

	if entry.expiresAt.IsZero() {
		return -1 * time.Second, nil
	}

	ttl := time.Until(entry.expiresAt)
	if ttl < 0 {
		return -2 * time.Second, nil
	}

	return ttl, nil
}

func (m *MemoryCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return &CacheError{Op: "expire", Key: key, Err: fmt.Errorf("cache is closed")}
	}

	entry, exists := m.data[key]
	if !exists {
		return &CacheError{Op: "expire", Key: key, Err: errKeyNotFound}
	}

	entry.expiresAt = time.Now().Add(ttl)
	return nil
}

func (m *MemoryCache) Persist(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return &CacheError{Op: "persist", Key: key, Err: fmt.Errorf("cache is closed")}
	}

	entry, exists := m.data[key]
	if !exists {
		return &CacheError{Op: "persist", Key: key, Err: errKeyNotFound}
	}

	entry.expiresAt = time.Time{}
	return nil
}

func (m *MemoryCache) Stats(ctx context.Context) (*CacheStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, &CacheError{Op: "stats", Err: fmt.Errorf("cache is closed")}
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return &CacheStats{
		Hits:        m.stats.hits.Load(),
		Misses:      m.stats.misses.Load(),
		Keys:        m.stats.keys.Load(),
		Evictions:   m.stats.evictions.Load(),
		MemoryUsed:  int64(memStats.Alloc),
		MemoryLimit: int64(memStats.Sys),
	}, nil
}

func (m *MemoryCache) Size(ctx context.Context) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return 0, &CacheError{Op: "size", Err: fmt.Errorf("cache is closed")}
	}

	return m.stats.keys.Load(), nil
}

func (m *MemoryCache) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true
	close(m.stopChan)
	m.data = nil

	return nil
}

func (m *MemoryCache) cleanupLoop() {
	ticker := time.NewTicker(m.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanup()
		case <-m.stopChan:
			return
		}
	}
}

func (m *MemoryCache) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for key, entry := range m.data {
		if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
			delete(m.data, key)
			m.stats.keys.Add(-1)
		}
	}
}

func (m *MemoryCache) shouldEvict() bool {
	return int64(len(m.data)) >= m.config.MaxSize
}

func (m *MemoryCache) evictEntries() {
	if m.config.EvictionPolicy == "lru" {
		m.evictLRU()
	} else {
		m.evictOldest()
	}
}

func (m *MemoryCache) evictLRU() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range m.data {
		if oldestTime.IsZero() || entry.lastAccess.Before(oldestTime) {
			oldestTime = entry.lastAccess
			oldestKey = key
		}
	}

	if oldestKey != "" {
		delete(m.data, oldestKey)
		m.stats.keys.Add(-1)
		m.stats.evictions.Add(1)
	}
}

func (m *MemoryCache) evictOldest() {
	var oldestKey string
	var oldestExpiry time.Time

	for key, entry := range m.data {
		if !entry.expiresAt.IsZero() {
			if oldestExpiry.IsZero() || entry.expiresAt.Before(oldestExpiry) {
				oldestExpiry = entry.expiresAt
				oldestKey = key
			}
		}
	}

	if oldestKey != "" {
		delete(m.data, oldestKey)
		m.stats.keys.Add(-1)
		m.stats.evictions.Add(1)
	}
}
