package handler

import (
	"context"
	"io"

	"github.com/go-redis/redis/v8"
	"github.com/victorspringer/http-cache"
)

type ctxKeyType struct{}

type fakeCloseReadCloser struct {
	io.ReadCloser
}

type cacheAdapter struct {
	store  *redis.Client
	ctx    context.Context
	prefix string
}

type cacheClient struct {
	client  *cache.Client
	adapter *cacheAdapter
}
