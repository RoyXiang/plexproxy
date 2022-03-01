package handler

import (
	"time"
)

const (
	headerPlexPrefix     = "X-Plex-"
	headerCacheStatus    = "X-Plex-Cache-Status"
	headerExtraProfile   = "X-Plex-Client-Profile-Extra"
	headerPageSize       = "X-Plex-Container-Size"
	headerPageStart      = "X-Plex-Container-Start"
	headerToken          = "X-Plex-Token"
	headerUserId         = "X-Plex-User-Id"
	headerAccept         = "Accept"
	headerAcceptLanguage = "Accept-Language"
	headerCacheControl   = "Cache-Control"
	headerContentType    = "Content-Type"
	headerForwardedFor   = "X-Forwarded-For"
	headerRange          = "Range"
	headerRealIP         = "X-Real-IP"
	headerVary           = "Vary"

	cachePrefixDynamic  = "dynamic"
	cachePrefixMetadata = "metadata"
	cachePrefixStatic   = "static"
	cachePrefixPlex     = "plex"

	cacheTtlDynamic  = time.Second * 5
	cacheTtlMetadata = time.Hour * 24
	cacheTtlStatic   = time.Hour

	contentTypeAny = "*/*"
	contentTypeXml = "xml"

	watchedThreshold = 90
)

const (
	sessionUnplayed sessionStatus = iota
	sessionPlaying
	sessionWatched
)
