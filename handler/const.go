package handler

const (
	headerPlexPrefix     = "X-Plex-"
	headerCacheStatus    = "X-Plex-Cache-Status"
	headerClientIdentity = "X-Plex-Client-Identifier"
	headerExtraProfile   = "X-Plex-Client-Profile-Extra"
	headerPageSize       = "X-Plex-Container-Size"
	headerPageStart      = "X-Plex-Container-Start"
	headerToken          = "X-Plex-Token"
	headerUserId         = "X-Plex-User-Id"

	headerAccept         = "Accept"
	headerAcceptLanguage = "Accept-Language"
	headerCacheControl   = "Cache-Control"
	headerContentType    = "Content-Type"
	headerRange          = "Range"
	headerVary           = "Vary"
	headerUpgrade        = "Upgrade"

	headerForwardedFor    = "X-Forwarded-For"
	headerRealIP          = "X-Real-IP"
	headerForwardedProto  = "X-Forwarded-Proto"
	headerForwardedScheme = "X-Forwarded-Scheme"

	cachePrefixDynamic = "dynamic"
	cachePrefixStatic  = "static"
	cachePrefixPlex    = "plex"

	contentTypeAny = "*/*"
	contentTypeXml = "xml"

	lockKeySections = "plex:library:sections"
	lockKeySessions = "plex:playback:sessions"
	lockKeyToken    = "plex:token"
	lockKeyUsers    = "plex:users"

	watchedThreshold = 90

	webhookEventPlay     = "media.play"
	webhookEventResume   = "media.resume"
	webhookEventPause    = "media.pause"
	webhookEventStop     = "media.stop"
	webhookEventScrobble = "media.scrobble"
)

const (
	sessionUnplayed sessionStatus = iota
	sessionPlaying
	sessionPaused
	sessionStopped
	sessionWatched
)
