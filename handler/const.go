package handler

import (
	"time"
)

const (
	headerPlexPrefix     = "X-Plex-"
	headerCacheStatus    = "X-Plex-Cache-Status"
	headerExtra          = "X-Plex-Client-Profile-Extra"
	headerPageSize       = "X-Plex-Container-Size"
	headerPageStart      = "X-Plex-Container-Start"
	headerToken          = "X-Plex-Token"
	headerAccept         = "Accept"
	headerAcceptLanguage = "Accept-Language"
	headerCacheControl   = "Cache-Control"
	headerForwardedFor   = "X-Forwarded-For"
	headerRange          = "Range"
	headerRealIP         = "X-Real-IP"

	cachePrefixDynamic  = "dynamic"
	cachePrefixMetadata = "metadata"
	cachePrefixStatic   = "static"
	cachePrefixPlex     = "plex"

	cacheTtlDynamic  = time.Second * 5
	cacheTtlMetadata = time.Minute * 30
	cacheTtlStatic   = time.Hour

	contentTypeAny = "*/*"
	contentTypeXml = "xml"
)
