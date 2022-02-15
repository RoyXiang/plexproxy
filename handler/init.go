package handler

import (
	"context"
	"log"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/RoyXiang/plexproxy/common"
	"github.com/go-redis/redis/v8"
	"github.com/victorspringer/http-cache"
)

var (
	proxy       *httputil.ReverseProxy
	plaxtProxy  *httputil.ReverseProxy
	redisClient *redis.Client

	cacheClientCtxKey  = ctxKeyType{}
	cachePrefixCtxKey  = ctxKeyType{}
	userCacheClient    *cache.Client
	dynamicCacheClient *cache.Client
	staticCacheClient  *cache.Client

	mu sync.RWMutex
	ml common.MultipleLock
)

func init() {
	baseUrl := os.Getenv("PLEX_BASEURL")
	if baseUrl == "" {
		log.Fatalln("Please configure PLEX_BASEURL at first")
	}
	u, err := url.Parse(baseUrl)
	if err != nil {
		log.Fatalln("Please ensure PLEX_BASEURL is a valid URL")
	}
	proxy = httputil.NewSingleHostReverseProxy(u)
	proxy.FlushInterval = -1

	plaxtBaseUrl := os.Getenv("PLAXT_BASEURL")
	if plaxtBaseUrl != "" {
		if u, err := url.Parse(plaxtBaseUrl); err == nil {
			plaxtProxy = httputil.NewSingleHostReverseProxy(u)
			plaxtProxy.FlushInterval = -1
		}
	}

	redisUrl := os.Getenv("REDIS_URL")
	if redisUrl != "" {
		options, err := redis.ParseURL(redisUrl)
		if err == nil {
			redisClient = redis.NewClient(options)

			dynamicCtx := context.WithValue(context.Background(), cachePrefixCtxKey, cachePrefixDynamic)
			dynamicAdapter := NewCacheAdapter(redisClient, dynamicCtx)
			userCacheClient, _ = cache.NewClient(
				cache.ClientWithAdapter(dynamicAdapter),
				cache.ClientWithTTL(time.Hour*24),
			)
			dynamicCacheClient, _ = cache.NewClient(
				cache.ClientWithAdapter(dynamicAdapter),
				cache.ClientWithTTL(time.Second*5),
			)

			staticCtx := context.WithValue(context.Background(), cachePrefixCtxKey, cachePrefixStatic)
			staticAdapter := NewCacheAdapter(redisClient, staticCtx)
			staticCacheClient, _ = cache.NewClient(
				cache.ClientWithAdapter(staticAdapter),
				cache.ClientWithTTL(time.Hour*24*7),
			)
		}
	}

	ml = common.NewMultipleLock()
}
