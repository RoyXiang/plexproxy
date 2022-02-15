package handler

const (
	headerToken = "X-Plex-Token"
	headerRange = "Range"

	cachePrefixDynamic = "dynamic:"
	cachePrefixStatic  = "static:"

	redisScriptRemoveAllWithPrefix = "return redis.call('del', unpack(redis.call('keys', ARGV[1])))"
)
