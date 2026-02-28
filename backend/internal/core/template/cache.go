package template

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"email-campaign-system/pkg/logger"
)

var (
	ErrCacheMiss    = errors.New("cache miss")
	ErrCacheFull    = errors.New("cache is full")
	ErrInvalidKey   = errors.New("invalid cache key")
	ErrEntryExpired = errors.New("cache entry expired")
)

type Cache struct {
	log           logger.Logger
	config        *CacheConfig
	entries       map[string]*CacheEntry
	accessOrder   []string
	mu            sync.RWMutex
	stats         *CacheStats
	stopCleanup   chan struct{}
	cleanupTicker *time.Ticker
}

type CacheConfig struct {
	MaxEntries        int
	MaxSizeBytes      int64
	TTL               time.Duration
	CleanupInterval   time.Duration
	EnableLRU         bool
	EnableCompression bool
	EnableStats       bool
}

type CacheEntry struct {
	Key          string
	Content      string
	Metadata     map[string]interface{}
	Size         int64
	CreatedAt    time.Time
	LastAccessAt time.Time
	AccessCount  int64
	ExpiresAt    time.Time
	Compressed   bool
}

type CacheStats struct {
	Hits              int64
	Misses            int64
	Sets              int64
	Deletes           int64
	Evictions         int64
	Expirations       int64
	TotalSize         int64
	EntryCount        int64
	HitRate           float64
	AverageAccessTime time.Duration
	mu                sync.RWMutex
}

type CacheKey struct {
	TemplateID   string
	TemplatePath string
	Variables    map[string]string
	Checksum     string
}

func NewCache(log logger.Logger, config *CacheConfig) *Cache {
	if config == nil {
		config = DefaultCacheConfig()
	}

	cache := &Cache{
		log:         log,
		config:      config,
		entries:     make(map[string]*CacheEntry),
		accessOrder: make([]string, 0),
		stats:       &CacheStats{},
		stopCleanup: make(chan struct{}),
	}

	if config.CleanupInterval > 0 {
		cache.startCleanupWorker()
	}

	return cache
}

func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		MaxEntries:        1000,
		MaxSizeBytes:      100 * 1024 * 1024,
		TTL:               1 * time.Hour,
		CleanupInterval:   5 * time.Minute,
		EnableLRU:         true,
		EnableCompression: false,
		EnableStats:       true,
	}
}

func (c *Cache) Get(key string) (string, error) {
	if key == "" {
		return "", ErrInvalidKey
	}

	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		c.recordMiss()
		return "", ErrCacheMiss
	}

	if time.Now().After(entry.ExpiresAt) {
		c.recordExpiration()
		c.Delete(key)
		return "", ErrEntryExpired
	}

	c.updateAccessOrder(key)
	c.recordHit()

	c.mu.Lock()
	entry.LastAccessAt = time.Now()
	entry.AccessCount++
	c.mu.Unlock()

	return entry.Content, nil
}

func (c *Cache) Set(key, content string, metadata map[string]interface{}) error {
	if key == "" {
		return ErrInvalidKey
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	size := int64(len(content))

	if c.config.MaxSizeBytes > 0 {
		currentSize := c.calculateTotalSize()
		if currentSize+size > c.config.MaxSizeBytes {
			if c.config.EnableLRU {
				c.evictLRU(size)
			} else {
				return ErrCacheFull
			}
		}
	}

	if c.config.MaxEntries > 0 && len(c.entries) >= c.config.MaxEntries {
		if c.config.EnableLRU {
			c.evictLRU(0)
		} else {
			return ErrCacheFull
		}
	}

	now := time.Now()
	entry := &CacheEntry{
		Key:          key,
		Content:      content,
		Metadata:     metadata,
		Size:         size,
		CreatedAt:    now,
		LastAccessAt: now,
		AccessCount:  0,
		ExpiresAt:    now.Add(c.config.TTL),
		Compressed:   false,
	}

	c.entries[key] = entry
	c.accessOrder = append(c.accessOrder, key)

	c.recordSet()
	c.stats.TotalSize += size
	c.stats.EntryCount++

	return nil
}

func (c *Cache) GetOrSet(key string, generator func() (string, error), metadata map[string]interface{}) (string, error) {
	content, err := c.Get(key)
	if err == nil {
		return content, nil
	}

	if err != ErrCacheMiss && err != ErrEntryExpired {
		return "", err
	}

	content, err = generator()
	if err != nil {
		return "", err
	}

	if err := c.Set(key, content, metadata); err != nil {
		c.log.Warn(fmt.Sprintf("failed to cache content: %v", err))
	}

	return content, nil
}

func (c *Cache) Has(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return false
	}

	return time.Now().Before(entry.ExpiresAt)
}

func (c *Cache) Delete(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[key]
	if !exists {
		return ErrCacheMiss
	}

	delete(c.entries, key)
	c.removeFromAccessOrder(key)

	c.recordDelete()
	c.stats.TotalSize -= entry.Size
	c.stats.EntryCount--

	return nil
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*CacheEntry)
	c.accessOrder = make([]string, 0)

	c.stats.TotalSize = 0
	c.stats.EntryCount = 0

	c.log.Info("cache cleared")
}

func (c *Cache) Size() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.calculateTotalSize()
}

func (c *Cache) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.entries)
}

func (c *Cache) GetStats() *CacheStats {
	c.stats.mu.RLock()
	defer c.stats.mu.RUnlock()

	total := c.stats.Hits + c.stats.Misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(c.stats.Hits) / float64(total) * 100
	}

	return &CacheStats{
		Hits:        c.stats.Hits,
		Misses:      c.stats.Misses,
		Sets:        c.stats.Sets,
		Deletes:     c.stats.Deletes,
		Evictions:   c.stats.Evictions,
		Expirations: c.stats.Expirations,
		TotalSize:   c.stats.TotalSize,
		EntryCount:  c.stats.EntryCount,
		HitRate:     hitRate,
	}
}

func (c *Cache) ResetStats() {
	c.stats.mu.Lock()
	defer c.stats.mu.Unlock()

	c.stats.Hits = 0
	c.stats.Misses = 0
	c.stats.Sets = 0
	c.stats.Deletes = 0
	c.stats.Evictions = 0
	c.stats.Expirations = 0
}

func (c *Cache) Cleanup() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	expiredKeys := make([]string, 0)

	for key, entry := range c.entries {
		if now.After(entry.ExpiresAt) {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		entry := c.entries[key]
		delete(c.entries, key)
		c.removeFromAccessOrder(key)

		c.stats.TotalSize -= entry.Size
		c.stats.EntryCount--
		c.recordExpiration()
	}

	return len(expiredKeys)
}

func (c *Cache) GetEntry(key string) (*CacheEntry, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return nil, ErrCacheMiss
	}

	if time.Now().After(entry.ExpiresAt) {
		return nil, ErrEntryExpired
	}

	entryCopy := *entry
	return &entryCopy, nil
}

func (c *Cache) SetTTL(ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.config.TTL = ttl
}

func (c *Cache) SetMaxSize(maxSize int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.config.MaxSizeBytes = maxSize
}

func (c *Cache) SetMaxEntries(maxEntries int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.config.MaxEntries = maxEntries
}

func (c *Cache) GetConfig() *CacheConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()

	configCopy := *c.config
	return &configCopy
}

func (c *Cache) GenerateKey(templateID string, variables map[string]string) string {
	cacheKey := CacheKey{
		TemplateID: templateID,
		Variables:  variables,
	}

	data, _ := json.Marshal(cacheKey)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func (c *Cache) GenerateKeyFromPath(templatePath string, variables map[string]string) string {
	cacheKey := CacheKey{
		TemplatePath: templatePath,
		Variables:    variables,
	}

	data, _ := json.Marshal(cacheKey)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func (c *Cache) GenerateKeyWithChecksum(templateID, checksum string, variables map[string]string) string {
	cacheKey := CacheKey{
		TemplateID: templateID,
		Checksum:   checksum,
		Variables:  variables,
	}

	data, _ := json.Marshal(cacheKey)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func (c *Cache) calculateTotalSize() int64 {
	totalSize := int64(0)
	for _, entry := range c.entries {
		totalSize += entry.Size
	}
	return totalSize
}

func (c *Cache) evictLRU(requiredSpace int64) {
	if len(c.accessOrder) == 0 {
		return
	}

	freedSpace := int64(0)
	evictedCount := 0

	for len(c.accessOrder) > 0 {
		oldestKey := c.accessOrder[0]
		entry, exists := c.entries[oldestKey]

		if exists {
			freedSpace += entry.Size
			c.stats.TotalSize -= entry.Size
			c.stats.EntryCount--
			delete(c.entries, oldestKey)
			evictedCount++
		}

		c.accessOrder = c.accessOrder[1:]
		c.recordEviction()

		if requiredSpace > 0 && freedSpace >= requiredSpace {
			break
		}

		if requiredSpace == 0 && evictedCount >= 1 {
			break
		}
	}

	c.log.Info(fmt.Sprintf("evicted %d entries, freed %d bytes", evictedCount, freedSpace))
}

func (c *Cache) updateAccessOrder(key string) {
	if !c.config.EnableLRU {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.removeFromAccessOrder(key)
	c.accessOrder = append(c.accessOrder, key)
}

func (c *Cache) removeFromAccessOrder(key string) {
	for i, k := range c.accessOrder {
		if k == key {
			c.accessOrder = append(c.accessOrder[:i], c.accessOrder[i+1:]...)
			break
		}
	}
}

func (c *Cache) startCleanupWorker() {
	c.cleanupTicker = time.NewTicker(c.config.CleanupInterval)

	go func() {
		for {
			select {
			case <-c.cleanupTicker.C:
				removed := c.Cleanup()
				if removed > 0 {
					c.log.Info(fmt.Sprintf("cleanup removed %d expired entries", removed))
				}
			case <-c.stopCleanup:
				c.cleanupTicker.Stop()
				return
			}
		}
	}()
}

func (c *Cache) StopCleanup() {
	if c.cleanupTicker != nil {
		close(c.stopCleanup)
	}
}

func (c *Cache) recordHit() {
	if !c.config.EnableStats {
		return
	}

	c.stats.mu.Lock()
	c.stats.Hits++
	c.stats.mu.Unlock()
}

func (c *Cache) recordMiss() {
	if !c.config.EnableStats {
		return
	}

	c.stats.mu.Lock()
	c.stats.Misses++
	c.stats.mu.Unlock()
}

func (c *Cache) recordSet() {
	if !c.config.EnableStats {
		return
	}

	c.stats.mu.Lock()
	c.stats.Sets++
	c.stats.mu.Unlock()
}

func (c *Cache) recordDelete() {
	if !c.config.EnableStats {
		return
	}

	c.stats.mu.Lock()
	c.stats.Deletes++
	c.stats.mu.Unlock()
}

func (c *Cache) recordEviction() {
	if !c.config.EnableStats {
		return
	}

	c.stats.mu.Lock()
	c.stats.Evictions++
	c.stats.mu.Unlock()
}

func (c *Cache) recordExpiration() {
	if !c.config.EnableStats {
		return
	}

	c.stats.mu.Lock()
	c.stats.Expirations++
	c.stats.mu.Unlock()
}

func (c *Cache) GetAllKeys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, len(c.entries))
	for key := range c.entries {
		keys = append(keys, key)
	}
	return keys
}

func (c *Cache) GetExpiredKeys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	expiredKeys := make([]string, 0)

	for key, entry := range c.entries {
		if now.After(entry.ExpiresAt) {
			expiredKeys = append(expiredKeys, key)
		}
	}

	return expiredKeys
}

func (c *Cache) Extend(key string, duration time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[key]
	if !exists {
		return ErrCacheMiss
	}

	entry.ExpiresAt = entry.ExpiresAt.Add(duration)
	return nil
}

func (c *Cache) Touch(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[key]
	if !exists {
		return ErrCacheMiss
	}

	entry.LastAccessAt = time.Now()
	entry.ExpiresAt = time.Now().Add(c.config.TTL)
	return nil
}
