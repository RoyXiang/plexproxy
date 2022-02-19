package handler

import (
	"log"
	"net/http/httputil"
	"os"
	"sync"

	"github.com/DirtyCajunRice/go-plex"
	"github.com/RoyXiang/plexproxy/common"
	"github.com/go-redis/redis/v8"
)

var (
	proxy       *httputil.ReverseProxy
	plaxtProxy  *httputil.ReverseProxy
	redisClient *redis.Client
	plexApp     *plex.App

	mu sync.RWMutex
	ml common.MultipleLock
)

func init() {
	proxy = newReverseProxy(os.Getenv("PLEX_BASEURL"))
	if proxy == nil {
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

	plexApp = plex.New("plex-proxy")

	ml = common.NewMultipleLock()
}
