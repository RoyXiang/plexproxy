package handler

import (
	"time"

	"github.com/jrudio/go-plex-client"
)

type ctxKeyType struct {
	name string
}

type cacheInfo struct {
	Prefix string
	Ttl    time.Duration
}

type sessionStatus int64

type sessionData struct {
	metadata plex.Metadata
	status   sessionStatus
}
