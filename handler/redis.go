package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/victorspringer/http-cache"
)

type CacheAdapter struct {
	store *redis.Client
	ctx   context.Context
}

func getCacheKey(ctx context.Context, key uint64) string {
	return fmt.Sprintf("%s%s", ctx.Value(cachePrefixCtxKey).(string), cache.KeyAsString(key))
}

func (a *CacheAdapter) Get(key uint64) ([]byte, bool) {
	if c, err := a.store.Get(a.ctx, getCacheKey(a.ctx, key)).Bytes(); err == nil {
		return c, true
	}
	return nil, false
}

func (a *CacheAdapter) Set(key uint64, response []byte, expiration time.Time) {
	a.store.Set(a.ctx, getCacheKey(a.ctx, key), response, expiration.Sub(time.Now()))
}

func (a *CacheAdapter) Release(key uint64) {
	a.store.Del(a.ctx, getCacheKey(a.ctx, key))
}

func NewCacheAdapter(client *redis.Client, ctx context.Context) cache.Adapter {
	return &CacheAdapter{
		store: client,
		ctx:   ctx,
	}
}
