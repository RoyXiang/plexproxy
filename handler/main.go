package handler

import (
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/RoyXiang/plexproxy/common"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
)

var (
	plexClient  *PlexClient
	redisClient *redis.Client

	emptyStruct = struct{}{}

	mu sync.RWMutex
	ml common.MultipleLock
)

func init() {
	plexClient = NewPlexClient(PlexConfig{
		BaseUrl:          os.Getenv("PLEX_BASEURL"),
		Token:            os.Getenv("PLEX_TOKEN"),
		PlaxtUrl:         os.Getenv("PLAXT_URL"),
		RedirectWebApp:   os.Getenv("REDIRECT_WEB_APP"),
		DisableTranscode: os.Getenv("DISABLE_TRANSCODE"),
	})
	if plexClient == nil {
		log.Fatalln("Please configure PLEX_BASEURL as a valid URL at first")
	}

	redisUrl := os.Getenv("REDIS_URL")
	if redisUrl != "" && plexClient.IsTokenSet() {
		options, err := redis.ParseURL(redisUrl)
		if err == nil {
			redisClient = redis.NewClient(options)
		}
	}

	ml = common.NewMultipleLock()
}

func NewRouter() http.Handler {
	r := mux.NewRouter()
	r.Use(normalizeMiddleware)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(trafficMiddleware)

	if redisClient != nil {
		// bypass cache
		r.PathPrefix("/:/").Handler(plexClient)
		r.PathPrefix("/library/parts/").Handler(plexClient)

		staticRouter := r.Methods(http.MethodGet).Subrouter()
		staticRouter.Use(staticMiddleware)
		staticRouter.Path("/library/media/{key}/chapterImages/{id}").Handler(plexClient)
		staticRouter.Path("/library/metadata/{key}/art/{id}").Handler(plexClient)
		staticRouter.Path("/library/metadata/{key}/thumb/{id}").Handler(plexClient)
		staticRouter.Path("/photo/:/transcode").Handler(plexClient)

		metadataRouter := r.Methods(http.MethodGet).PathPrefix("/library").Subrouter()
		metadataRouter.Use(metadataMiddleware)
		metadataRouter.PathPrefix("/collections/").Handler(plexClient)
		metadataRouter.PathPrefix("/metadata/").Handler(plexClient)
		metadataRouter.PathPrefix("/sections/").Handler(plexClient)

		dynamicRouter := r.Methods(http.MethodGet).Subrouter()
		dynamicRouter.Use(dynamicMiddleware)
		dynamicRouter.PathPrefix("/").Handler(plexClient)
	}

	r.PathPrefix("/").Handler(plexClient)
	return r
}
