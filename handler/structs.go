package handler

import (
	"time"
)

type ctxKeyType struct{}

type cacheInfo struct {
	Prefix string
	Ttl    time.Duration
}
