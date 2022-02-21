package handler

import (
	"time"
)

type ctxKeyType struct {
	name string
}

type cacheInfo struct {
	Prefix string
	Ttl    time.Duration
}
