package cache

import (
	"context"
	"fmt"
	"time"
)

type TieredCache struct {
	primary   Cache
	fallback  Cache
	usePrimary bool
}

func NewTieredCache(cfg *CacheConfig) *TieredCache {
	tc := &TieredCache{}

	if cfg != nil && cfg.Host != "" {
		redisCache, err := NewRedisCache(cfg)
		if err == nil {
			tc.primary = redisCache
			tc.usePrimary = true
		}
	}

	tc.fallback = NewMemoryCache(&MemoryCacheConfig{
		MaxSize:          50000,
		CleanupInterval:  2 * time.Minute,
		DefaultTTL:       30 * time.Minute,
		EvictionPolicy:   "lru",
		MaxMemoryPercent: 70.0,
	})

	return tc
}

func (t *TieredCache) active() Cache {
	if t.usePrimary && t.primary != nil {
		return t.primary
	}
	return t.fallback
}

func (t *TieredCache) Get(ctx context.Context, key string) ([]byte, error) {
	return t.active().Get(ctx, key)
}

func (t *TieredCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return t.active().Set(ctx, key, value, ttl)
}

func (t *TieredCache) Delete(ctx context.Context, key string) error {
	return t.active().Delete(ctx, key)
}

func (t *TieredCache) Exists(ctx context.Context, key string) (bool, error) {
	return t.active().Exists(ctx, key)
}

func (t *TieredCache) Clear(ctx context.Context) error {
	return t.active().Clear(ctx)
}

func (t *TieredCache) Close() error {
	var errs []error
	if t.primary != nil {
		if err := t.primary.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if t.fallback != nil {
		if err := t.fallback.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

func (t *TieredCache) IsRedisAvailable() bool {
	return t.usePrimary
}

func (t *TieredCache) GetOrSet(ctx context.Context, key string, ttl time.Duration, loader func() ([]byte, error)) ([]byte, error) {
	data, err := t.Get(ctx, key)
	if err == nil {
		return data, nil
	}

	data, err = loader()
	if err != nil {
		return nil, err
	}

	_ = t.Set(ctx, key, data, ttl)
	return data, nil
}
