package handler

import (
	"log"
	"net/http/httputil"
	"os"
	"sync"

	"github.com/RoyXiang/plexproxy/common"
	"github.com/go-redis/redis/v8"
)

var (
	plexBaseUrl string
	plexToken   string

	plexProxy   *httputil.ReverseProxy
	plaxtProxy  *httputil.ReverseProxy
	redisClient *redis.Client

	mu sync.RWMutex
	ml common.MultipleLock
)

func init() {
	plexBaseUrl = os.Getenv("PLEX_BASEURL")
	plexToken = os.Getenv("PLEX_TOKEN")

	plexProxy = newReverseProxy(plexBaseUrl)
	if plexProxy == nil {
		log.Fatalln("Please configure PLEX_BASEURL as a valid URL at first")
	}
	plaxtProxy = newReverseProxy(os.Getenv("PLAXT_BASEURL"))

	redisUrl := os.Getenv("REDIS_URL")
	if redisUrl != "" {
		options, err := redis.ParseURL(redisUrl)
		if err == nil {
			redisClient = redis.NewClient(options)
		}
	}

	ml = common.NewMultipleLock()
}
