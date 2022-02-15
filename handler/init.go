package handler

import (
	"log"
	"net/http"
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
	redisClient *redis.Client
	cacheClient *cache.Client

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

	redisUrl := os.Getenv("REDIS_URL")
	if redisUrl != "" {
		options, err := redis.ParseURL(redisUrl)
		if err == nil {
			redisClient = redis.NewClient(options)
			adapter := NewCacheAdapter(redisClient)
			cacheClient, _ = cache.NewClient(
				cache.ClientWithAdapter(adapter),
				cache.ClientWithTTL(time.Hour*24),
			)
		}
	}

	ml = common.NewMultipleLock()
}

func Handler(w http.ResponseWriter, r *http.Request) {
	proxy.ServeHTTP(w, r)
}
