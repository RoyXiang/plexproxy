package handler

import (
	"time"
)

const (
	headerPlexPrefix  = "X-Plex-"
	headerCacheStatus = "X-Plex-Cache-Status"
	headerExtra       = "X-Plex-Client-Profile-Extra"
	headerHash        = "X-Plex-Hash"
	headerPageSize    = "X-Plex-Container-Size"
	headerPageStart   = "X-Plex-Container-Start"
	headerToken       = "X-Plex-Token"
	headerAccept      = "Accept"
	headerLanguage    = "Accept-Language"
	headerRange       = "Range"

	cachePrefixDynamic = "dynamic"
	cachePrefixStatic  = "static"
	cachePrefixPlex    = "plex"

	cacheTtlStatic  = time.Hour
	cacheTtlUser    = time.Minute * 30
	cacheTtlDynamic = time.Second * 5

	contentTypeAny = "*/*"
	contentTypeXml = "xml"
)
