package attachment

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrCacheNotFound = errors.New("cache: entry not found")
	ErrCacheExpired  = errors.New("cache: entry expired")
	ErrCacheFull     = errors.New("cache: storage full")
)

type CacheConfig struct {
	MaxMemorySize int64
	MaxDiskSize   int64
	MaxEntries    int
	TTL           time.Duration
	DiskDir       string
	EnableDisk    bool
	EnableMemory  bool
	EvictionPolicy EvictionPolicy
}

type EvictionPolicy string

const (
	EvictionLRU      EvictionPolicy = "lru"
	EvictionLFU      EvictionPolicy = "lfu"
	EvictionTTL      EvictionPolicy = "ttl"
	EvictionFIFO     EvictionPolicy = "fifo"
)

type HybridCache struct {
	mu       sync.RWMutex
	config   *CacheConfig
	memory   *MemoryCache
	disk     *DiskCache
	stats    *CacheStats
	closed   bool
}

type CacheStats struct {
	mu            sync.RWMutex
	hits          uint64
	misses        uint64
	sets          uint64
	evictions     uint64
	memorySize    int64
	diskSize      int64
	entryCount    int
}

func NewHybridCache(cfg *CacheConfig) (*HybridCache, error) {
	if cfg == nil {
		cfg = &CacheConfig{
			MaxMemorySize:  100 * 1024 * 1024,
			MaxDiskSize:    500 * 1024 * 1024,
			MaxEntries:     1000,
			TTL:            24 * time.Hour,
			EnableMemory:   true,
			EnableDisk:     true,
			EvictionPolicy: EvictionLRU,
		}
	}

	hc := &HybridCache{
		config: cfg,
		stats:  &CacheStats{},
	}

	if cfg.EnableMemory {
		hc.memory = NewMemoryCache(&MemoryCacheConfig{
			MaxSize:        cfg.MaxMemorySize,
			MaxEntries:     cfg.MaxEntries,
			TTL:            cfg.TTL,
			EvictionPolicy: cfg.EvictionPolicy,
		})
	}

	if cfg.EnableDisk {
		diskCache, err := NewDiskCache(&DiskCacheConfig{
			Directory:  cfg.DiskDir,
			MaxSize:    cfg.MaxDiskSize,
			TTL:        cfg.TTL,
		})
		if err != nil {
			return nil, fmt.Errorf("create disk cache: %w", err)
		}
		hc.disk = diskCache
	}

	return hc, nil
}

func (hc *HybridCache) Get(ctx context.Context, key string) (*CacheEntry, error) {
	hc.mu.RLock()
	closed := hc.closed
	hc.mu.RUnlock()

	if closed {
		return nil, errors.New("cache: closed")
	}

	if hc.memory != nil {
		if entry, err := hc.memory.Get(key); err == nil {
			atomic.AddUint64(&hc.stats.hits, 1)
			return entry, nil
		}
	}

	if hc.disk != nil {
		if entry, err := hc.disk.Get(key); err == nil {
			atomic.AddUint64(&hc.stats.hits, 1)
			
			if hc.memory != nil {
				_ = hc.memory.Set(key, entry)
			}
			
			return entry, nil
		}
	}

	atomic.AddUint64(&hc.stats.misses, 1)
	return nil, ErrCacheNotFound
}

func (hc *HybridCache) Set(ctx context.Context, key string, entry *CacheEntry) error {
	hc.mu.RLock()
	closed := hc.closed
	hc.mu.RUnlock()

	if closed {
		return errors.New("cache: closed")
	}

	if entry == nil {
		return errors.New("cache: nil entry")
	}

	entry.LastAccess = time.Now()
	atomic.AddUint64(&entry.AccessCount, 1)

	atomic.AddUint64(&hc.stats.sets, 1)

	if hc.memory != nil {
		if err := hc.memory.Set(key, entry); err == nil {
			return nil
		}
	}

	if hc.disk != nil {
		return hc.disk.Set(key, entry)
	}

	return errors.New("cache: no storage available")
}

func (hc *HybridCache) Invalidate(ctx context.Context, templateID string) error {
	hc.mu.RLock()
	closed := hc.closed
	hc.mu.RUnlock()

	if closed {
		return errors.New("cache: closed")
	}

	var errs []error

	if hc.memory != nil {
		if err := hc.memory.InvalidateTemplate(templateID); err != nil {
			errs = append(errs, err)
		}
	}

	if hc.disk != nil {
		if err := hc.disk.InvalidateTemplate(templateID); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("cache: invalidate errors: %v", errs)
	}

	return nil
}

func (hc *HybridCache) Clear(ctx context.Context) error {
	hc.mu.RLock()
	closed := hc.closed
	hc.mu.RUnlock()

	if closed {
		return errors.New("cache: closed")
	}

	var errs []error

	if hc.memory != nil {
		if err := hc.memory.Clear(); err != nil {
			errs = append(errs, err)
		}
	}

	if hc.disk != nil {
		if err := hc.disk.Clear(); err != nil {
			errs = append(errs, err)
		}
	}

	hc.stats = &CacheStats{}

	if len(errs) > 0 {
		return fmt.Errorf("cache: clear errors: %v", errs)
	}

	return nil
}

func (hc *HybridCache) Stats() *CacheStats {
	hc.stats.mu.RLock()
	defer hc.stats.mu.RUnlock()

	snapshot := &CacheStats{
		hits:       atomic.LoadUint64(&hc.stats.hits),
		misses:     atomic.LoadUint64(&hc.stats.misses),
		sets:       atomic.LoadUint64(&hc.stats.sets),
		evictions:  atomic.LoadUint64(&hc.stats.evictions),
	}

	if hc.memory != nil {
		snapshot.memorySize = hc.memory.Size()
		snapshot.entryCount += hc.memory.Count()
	}

	if hc.disk != nil {
		snapshot.diskSize = hc.disk.Size()
	}

	return snapshot
}

func (hc *HybridCache) Close() error {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if hc.closed {
		return nil
	}
	hc.closed = true

	if hc.memory != nil {
		_ = hc.memory.Close()
	}

	if hc.disk != nil {
		_ = hc.disk.Close()
	}

	return nil
}

type MemoryCache struct {
	mu       sync.RWMutex
	config   *MemoryCacheConfig
	entries  map[string]*cacheNode
	lruList  *list.List
	size     int64
	count    int
}

type MemoryCacheConfig struct {
	MaxSize        int64
	MaxEntries     int
	TTL            time.Duration
	EvictionPolicy EvictionPolicy
}

type cacheNode struct {
	key       string
	entry     *CacheEntry
	element   *list.Element
	addedAt   time.Time
}

func NewMemoryCache(cfg *MemoryCacheConfig) *MemoryCache {
	return &MemoryCache{
		config:  cfg,
		entries: make(map[string]*cacheNode),
		lruList: list.New(),
	}
}

func (mc *MemoryCache) Get(key string) (*CacheEntry, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	node, exists := mc.entries[key]
	if !exists {
		return nil, ErrCacheNotFound
	}

	if mc.config.TTL > 0 && time.Since(node.addedAt) > mc.config.TTL {
		mc.removeNode(node)
		return nil, ErrCacheExpired
	}

	mc.lruList.MoveToFront(node.element)
	node.entry.LastAccess = time.Now()
	atomic.AddUint64(&node.entry.AccessCount, 1)

	return node.entry, nil
}

func (mc *MemoryCache) Set(key string, entry *CacheEntry) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if node, exists := mc.entries[key]; exists {
		mc.size -= node.entry.Size
		node.entry = entry
		node.addedAt = time.Now()
		mc.size += entry.Size
		mc.lruList.MoveToFront(node.element)
		return nil
	}

	for mc.size+entry.Size > mc.config.MaxSize || (mc.config.MaxEntries > 0 && mc.count >= mc.config.MaxEntries) {
		if !mc.evictOne() {
			return ErrCacheFull
		}
	}

	node := &cacheNode{
		key:     key,
		entry:   entry,
		addedAt: time.Now(),
	}
	node.element = mc.lruList.PushFront(node)
	mc.entries[key] = node
	mc.size += entry.Size
	mc.count++

	return nil
}

func (mc *MemoryCache) evictOne() bool {
	element := mc.lruList.Back()
	if element == nil {
		return false
	}

	node := element.Value.(*cacheNode)
	mc.removeNode(node)
	return true
}

func (mc *MemoryCache) removeNode(node *cacheNode) {
	delete(mc.entries, node.key)
	mc.lruList.Remove(node.element)
	mc.size -= node.entry.Size
	mc.count--
}

func (mc *MemoryCache) InvalidateTemplate(templateID string) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	toRemove := make([]*cacheNode, 0)
	for _, node := range mc.entries {
		if node.entry.TemplateID == templateID {
			toRemove = append(toRemove, node)
		}
	}

	for _, node := range toRemove {
		mc.removeNode(node)
	}

	return nil
}

func (mc *MemoryCache) Clear() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.entries = make(map[string]*cacheNode)
	mc.lruList = list.New()
	mc.size = 0
	mc.count = 0

	return nil
}

func (mc *MemoryCache) Size() int64 {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.size
}

func (mc *MemoryCache) Count() int {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.count
}

func (mc *MemoryCache) Close() error {
	return mc.Clear()
}

type DiskCache struct {
	mu      sync.RWMutex
	config  *DiskCacheConfig
	index   map[string]*diskEntry
	size    int64
}

type DiskCacheConfig struct {
	Directory string
	MaxSize   int64
	TTL       time.Duration
}

type diskEntry struct {
	Key        string
	Path       string
	Size       int64
	TemplateID string
	Format     Format
	CreatedAt  time.Time
	AccessedAt time.Time
}

func NewDiskCache(cfg *DiskCacheConfig) (*DiskCache, error) {
	if cfg.Directory == "" {
		cfg.Directory = filepath.Join(os.TempDir(), "attachment-cache")
	}

	if err := os.MkdirAll(cfg.Directory, 0755); err != nil {
		return nil, fmt.Errorf("create cache directory: %w", err)
	}

	dc := &DiskCache{
		config: cfg,
		index:  make(map[string]*diskEntry),
	}

	if err := dc.loadIndex(); err != nil {
		return nil, fmt.Errorf("load index: %w", err)
	}

	return dc, nil
}

func (dc *DiskCache) Get(key string) (*CacheEntry, error) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	entry, exists := dc.index[key]
	if !exists {
		return nil, ErrCacheNotFound
	}

	if dc.config.TTL > 0 && time.Since(entry.CreatedAt) > dc.config.TTL {
		dc.removeEntry(entry)
		return nil, ErrCacheExpired
	}

	data, err := os.ReadFile(entry.Path)
	if err != nil {
		dc.removeEntry(entry)
		return nil, fmt.Errorf("read cache file: %w", err)
	}

	entry.AccessedAt = time.Now()

	return &CacheEntry{
		Key:         entry.Key,
		Data:        data,
		Size:        entry.Size,
		Hash:        computeHash(data),
		TemplateID:  entry.TemplateID,
		Format:      entry.Format,
		GeneratedAt: entry.CreatedAt,
		LastAccess:  entry.AccessedAt,
	}, nil
}

func (dc *DiskCache) Set(key string, entry *CacheEntry) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	for dc.size+entry.Size > dc.config.MaxSize {
		if !dc.evictOldest() {
			return ErrCacheFull
		}
	}

	path := filepath.Join(dc.config.Directory, fmt.Sprintf("%s%s", key, GetFileExtension(entry.Format)))
	
	if err := os.WriteFile(path, entry.Data, 0644); err != nil {
		return fmt.Errorf("write cache file: %w", err)
	}

	diskEntry := &diskEntry{
		Key:        key,
		Path:       path,
		Size:       entry.Size,
		TemplateID: entry.TemplateID,
		Format:     entry.Format,
		CreatedAt:  entry.GeneratedAt,
		AccessedAt: time.Now(),
	}

	if old, exists := dc.index[key]; exists {
		dc.size -= old.Size
		os.Remove(old.Path)
	}

	dc.index[key] = diskEntry
	dc.size += entry.Size

	return dc.saveIndex()
}

func (dc *DiskCache) evictOldest() bool {
	var oldest *diskEntry
	var oldestTime time.Time

	for _, entry := range dc.index {
		if oldest == nil || entry.AccessedAt.Before(oldestTime) {
			oldest = entry
			oldestTime = entry.AccessedAt
		}
	}

	if oldest == nil {
		return false
	}

	dc.removeEntry(oldest)
	return true
}

func (dc *DiskCache) removeEntry(entry *diskEntry) {
	delete(dc.index, entry.Key)
	dc.size -= entry.Size
	os.Remove(entry.Path)
}

func (dc *DiskCache) InvalidateTemplate(templateID string) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	toRemove := make([]*diskEntry, 0)
	for _, entry := range dc.index {
		if entry.TemplateID == templateID {
			toRemove = append(toRemove, entry)
		}
	}

	for _, entry := range toRemove {
		dc.removeEntry(entry)
	}

	return dc.saveIndex()
}

func (dc *DiskCache) Clear() error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	for _, entry := range dc.index {
		os.Remove(entry.Path)
	}

	dc.index = make(map[string]*diskEntry)
	dc.size = 0

	return dc.saveIndex()
}

func (dc *DiskCache) Size() int64 {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.size
}

func (dc *DiskCache) Close() error {
	return dc.saveIndex()
}

func (dc *DiskCache) loadIndex() error {
	indexPath := filepath.Join(dc.config.Directory, "index.json")
	
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var entries []*diskEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}

	for _, entry := range entries {
		if _, err := os.Stat(entry.Path); err == nil {
			dc.index[entry.Key] = entry
			dc.size += entry.Size
		}
	}

	return nil
}

func (dc *DiskCache) saveIndex() error {
	indexPath := filepath.Join(dc.config.Directory, "index.json")
	
	entries := make([]*diskEntry, 0, len(dc.index))
	for _, entry := range dc.index {
		entries = append(entries, entry)
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(indexPath, data, 0644)
}

func GenerateCacheKey(templateID string, format Format, personData map[string]string) string {
	h := sha256.New()
	h.Write([]byte(templateID))
	h.Write([]byte(string(format)))

	for k, v := range personData {
		h.Write([]byte(k))
		h.Write([]byte(v))
	}

	return hex.EncodeToString(h.Sum(nil))
}

func (cs *CacheStats) HitRate() float64 {
	total := cs.hits + cs.misses
	if total == 0 {
		return 0
	}
	return float64(cs.hits) / float64(total) * 100
}

func (cs *CacheStats) String() string {
	return fmt.Sprintf("Hits: %d, Misses: %d, Sets: %d, Evictions: %d, HitRate: %.2f%%, MemSize: %d, DiskSize: %d, Entries: %d",
		cs.hits, cs.misses, cs.sets, cs.evictions, cs.HitRate(), cs.memorySize, cs.diskSize, cs.entryCount)
}
