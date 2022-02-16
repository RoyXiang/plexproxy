package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/victorspringer/http-cache"
)

func (a *cacheAdapter) getCacheKey(key uint64) string {
	return fmt.Sprintf("%s%s", a.prefix, cache.KeyAsString(key))
}

func (a *cacheAdapter) Get(key uint64) ([]byte, bool) {
	if c, err := a.store.Get(a.ctx, a.getCacheKey(key)).Bytes(); err == nil {
		return c, true
	}
	return nil, false
}

func (a *cacheAdapter) Set(key uint64, response []byte, expiration time.Time) {
	a.store.Set(a.ctx, a.getCacheKey(key), response, expiration.Sub(time.Now()))
}

func (a *cacheAdapter) Release(key uint64) {
	a.store.Del(a.ctx, a.getCacheKey(key))
}

func (c *cacheClient) GetPrefix() string {
	return c.adapter.prefix
}

func (c *cacheClient) Wrap(next http.Handler) http.Handler {
	return c.client.Middleware(next)
}

func NewCacheClient(client *redis.Client, prefix string, ttl time.Duration) *cacheClient {
	adapter := &cacheAdapter{
		store:  client,
		ctx:    context.Background(),
		prefix: prefix,
	}
	cc, _ := cache.NewClient(
		cache.ClientWithAdapter(adapter),
		cache.ClientWithTTL(ttl),
	)
	return &cacheClient{
		client:  cc,
		adapter: adapter,
	}
}
