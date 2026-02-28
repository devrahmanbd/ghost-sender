package cache

import (
	"context"
	"time"
)

type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Clear(ctx context.Context) error
	Close() error
}

type CacheWithMulti interface {
	Cache
	GetMulti(ctx context.Context, keys []string) (map[string][]byte, error)
	SetMulti(ctx context.Context, items map[string][]byte, ttl time.Duration) error
	DeleteMulti(ctx context.Context, keys []string) error
}

type CacheWithCounter interface {
	Cache
	Increment(ctx context.Context, key string, delta int64) (int64, error)
	Decrement(ctx context.Context, key string, delta int64) (int64, error)
	IncrementWithTTL(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, error)
}

type CacheWithPattern interface {
	Cache
	DeletePattern(ctx context.Context, pattern string) error
	Keys(ctx context.Context, pattern string) ([]string, error)
}

type CacheWithExpiry interface {
	Cache
	TTL(ctx context.Context, key string) (time.Duration, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	Persist(ctx context.Context, key string) error
}

type CacheWithStats interface {
	Cache
	Stats(ctx context.Context) (*CacheStats, error)
	Size(ctx context.Context) (int64, error)
}

type AdvancedCache interface {
	CacheWithMulti
	CacheWithCounter
	CacheWithPattern
	CacheWithExpiry
	CacheWithStats
}

type CacheStats struct {
	Hits          int64
	Misses        int64
	Keys          int64
	MemoryUsed    int64
	MemoryLimit   int64
	Evictions     int64
	Connections   int
	Uptime        time.Duration
	LastSaveTime  time.Time
}

type CacheConfig struct {
	Type           string
	Host           string
	Port           int
	Password       string
	Database       int
	MaxRetries     int
	PoolSize       int
	MinIdleConns   int
	ConnTimeout    time.Duration
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	PoolTimeout    time.Duration
	IdleTimeout    time.Duration
	EnableMetrics  bool
	KeyPrefix      string
}

type CacheError struct {
	Op  string
	Key string
	Err error
}

func (e *CacheError) Error() string {
	if e.Key != "" {
		return "cache " + e.Op + " error for key '" + e.Key + "': " + e.Err.Error()
	}
	return "cache " + e.Op + " error: " + e.Err.Error()
}

func (e *CacheError) Unwrap() error {
	return e.Err
}

var (
	ErrCacheMiss       = &CacheError{Op: "get", Err: errCacheMiss}
	ErrKeyNotFound     = &CacheError{Op: "get", Err: errKeyNotFound}
	ErrInvalidValue    = &CacheError{Op: "set", Err: errInvalidValue}
	ErrConnectionFailed = &CacheError{Op: "connect", Err: errConnectionFailed}
	ErrTimeout         = &CacheError{Op: "operation", Err: errTimeout}
)

var (
	errCacheMiss        = newError("cache miss")
	errKeyNotFound      = newError("key not found")
	errInvalidValue     = newError("invalid value")
	errConnectionFailed = newError("connection failed")
	errTimeout          = newError("operation timeout")
)

type cacheErr struct {
	msg string
}

func (e *cacheErr) Error() string {
	return e.msg
}

func newError(msg string) error {
	return &cacheErr{msg: msg}
}

