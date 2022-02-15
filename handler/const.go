package handler

const (
	headerPlexPrefix     = "X-Plex-"
	headerContainerSize  = "X-Plex-Container-Size"
	headerContainerStart = "X-Plex-Container-Start"
	headerToken          = "X-Plex-Token"
	headerRange          = "Range"

	cachePrefixDynamic = "dynamic:"
	cachePrefixStatic  = "static:"

	redisScriptRemoveAllWithPrefix = "return redis.call('del', unpack(redis.call('keys', ARGV[1])))"
)
