package handler

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/victorspringer/http-cache"
)

type CacheAdapter struct {
	store *redis.Client
	ctx   context.Context
}

func (a *CacheAdapter) Get(key uint64) ([]byte, bool) {
	if c, err := a.store.Get(a.ctx, cache.KeyAsString(key)).Bytes(); err == nil {
		return c, true
	}
	return nil, false
}

func (a *CacheAdapter) Set(key uint64, response []byte, expiration time.Time) {
	a.store.Set(a.ctx, cache.KeyAsString(key), response, expiration.Sub(time.Now()))
}

func (a *CacheAdapter) Release(key uint64) {
	a.store.Del(a.ctx, cache.KeyAsString(key))
}

func NewCacheAdapter(client *redis.Client) cache.Adapter {
	return &CacheAdapter{
		store: client,
		ctx:   context.Background(),
	}
}
