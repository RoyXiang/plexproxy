package handler

import (
	"io"
	"time"
)

type ctxKeyType struct {
	name string
}

type fakeCloseReadCloser struct {
	io.ReadCloser
}

type cacheInfo struct {
	Prefix string
	Ttl    time.Duration
}
