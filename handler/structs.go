package handler

import (
	"context"

	"github.com/go-redis/redis/v8"
	"github.com/victorspringer/http-cache"
)

type ctxKeyType struct{}

type cacheAdapter struct {
	store  *redis.Client
	ctx    context.Context
	prefix string
}

type cacheClient struct {
	client  *cache.Client
	adapter *cacheAdapter
}
