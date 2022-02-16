package handler

import (
	"context"
	"io"
	"net/http/httptest"

	"github.com/go-redis/redis/v8"
	cache "github.com/victorspringer/http-cache"
)

type ctxKeyType struct{}

type fakeCloseReadCloser struct {
	io.ReadCloser
}

type mockHTTPRespWriter struct {
	*httptest.ResponseRecorder
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
