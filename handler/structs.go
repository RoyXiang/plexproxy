package handler

import (
	"time"

	"github.com/jrudio/go-plex-client"
	"github.com/xanderstrike/plexhooks"
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
	guids    []plexhooks.ExternalGuid
	status   sessionStatus
}
