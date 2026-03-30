package sender

import (
	"sync"
	"sync/atomic"
	"time"
)

var sendJobPool = sync.Pool{
	New: func() interface{} {
		return &SendJob{
			Metadata: make(map[string]interface{}, 4),
		}
	},
}

func AcquireSendJob() *SendJob {
	job := sendJobPool.Get().(*SendJob)
	job.ID = ""
	job.CampaignID = ""
	job.Recipient = nil
	job.Template = nil
	job.Account = nil
	job.Attachments = nil
	job.Priority = 0
	job.Retries = 0
	job.MaxRetries = 0
	job.CreatedAt = time.Time{}
	job.ScheduledAt = time.Time{}
	for k := range job.Metadata {
		delete(job.Metadata, k)
	}
	return job
}

func ReleaseSendJob(job *SendJob) {
	if job == nil {
		return
	}
	job.Recipient = nil
	job.Template = nil
	job.Account = nil
	job.Attachments = nil
	sendJobPool.Put(job)
}

var sendResultPool = sync.Pool{
	New: func() interface{} {
		return &SendResult{}
	},
}

func AcquireSendResult() *SendResult {
	r := sendResultPool.Get().(*SendResult)
	r.Job = nil
	r.Success = false
	r.Error = nil
	r.MessageID = ""
	r.SentAt = time.Time{}
	r.Duration = 0
	r.BytesSent = 0
	r.AccountUsed = ""
	r.ProviderUsed = ""
	r.RetryAttempt = 0
	return r
}

func ReleaseSendResult(r *SendResult) {
	if r == nil {
		return
	}
	r.Job = nil
	r.Error = nil
	sendResultPool.Put(r)
}

type RingBuffer struct {
	items []string
	index atomic.Uint64
	size  int
}

func NewRingBuffer(items []string) *RingBuffer {
	if len(items) == 0 {
		return &RingBuffer{size: 0}
	}
	cp := make([]string, len(items))
	copy(cp, items)
	return &RingBuffer{
		items: cp,
		size:  len(cp),
	}
}

func (rb *RingBuffer) Next() string {
	if rb.size == 0 {
		return ""
	}
	idx := rb.index.Add(1) - 1
	return rb.items[int(idx)%rb.size]
}

func (rb *RingBuffer) Len() int {
	return rb.size
}

type LRUCache struct {
	mu       sync.RWMutex
	capacity int
	items    map[string]*lruItem
	order    []string
}

type lruItem struct {
	value     string
	createdAt time.Time
	ttl       time.Duration
}

func NewLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		items:    make(map[string]*lruItem, capacity),
		order:    make([]string, 0, capacity),
	}
}

func (c *LRUCache) Get(key string) (string, bool) {
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}
	if item.ttl > 0 && time.Since(item.createdAt) > item.ttl {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return "", false
	}
	return item.value, true
}

func (c *LRUCache) Set(key, value string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.items[key]; exists {
		c.items[key] = &lruItem{value: value, createdAt: time.Now(), ttl: ttl}
		return
	}

	if len(c.items) >= c.capacity && len(c.order) > 0 {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.items, oldest)
	}

	c.items[key] = &lruItem{value: value, createdAt: time.Now(), ttl: ttl}
	c.order = append(c.order, key)
}

type BloomFilter struct {
	bits    []uint64
	size    uint64
	hashes  int
}

func NewBloomFilter(expectedItems int, falsePositiveRate float64) *BloomFilter {
	size := uint64(expectedItems * 10)
	if size < 64 {
		size = 64
	}
	words := (size + 63) / 64
	return &BloomFilter{
		bits:   make([]uint64, words),
		size:   size,
		hashes: 3,
	}
}

func (bf *BloomFilter) Add(item string) {
	for i := 0; i < bf.hashes; i++ {
		h := bf.hash(item, i)
		word := h / 64
		bit := h % 64
		bf.bits[word] |= 1 << bit
	}
}

func (bf *BloomFilter) Contains(item string) bool {
	for i := 0; i < bf.hashes; i++ {
		h := bf.hash(item, i)
		word := h / 64
		bit := h % 64
		if bf.bits[word]&(1<<bit) == 0 {
			return false
		}
	}
	return true
}

func (bf *BloomFilter) hash(item string, seed int) uint64 {
	h := uint64(seed * 0x9e3779b9)
	for _, c := range item {
		h = h*31 + uint64(c)
	}
	return h % bf.size
}
