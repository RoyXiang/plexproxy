package handler

import (
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
)

var (
	plexClient  *PlexClient
	redisClient *redis.Client

	emptyStruct = struct{}{}
)

func init() {
	plexClient = NewPlexClient(PlexConfig{
		BaseUrl:          os.Getenv("PLEX_BASEURL"),
		Token:            os.Getenv("PLEX_TOKEN"),
		PlaxtUrl:         os.Getenv("PLAXT_URL"),
		RedirectWebApp:   os.Getenv("REDIRECT_WEB_APP"),
		DisableTranscode: os.Getenv("DISABLE_TRANSCODE"),
		NoRequestLogs:    os.Getenv("NO_REQUEST_LOGS"),
	})
	if plexClient == nil {
		log.Fatalln("Please configure PLEX_BASEURL as a valid URL at first")
	}

	redisUrl := os.Getenv("REDIS_URL")
	if redisUrl != "" {
		options, err := redis.ParseURL(redisUrl)
		if err == nil {
			redisClient = redis.NewClient(options)
		}
	}
}

func NewRouter() http.Handler {
	r := mux.NewRouter()
	r.Use(normalizeMiddleware)
	if !plexClient.NoRequestLogs {
		r.Use(middleware.Logger)
	}
	r.Use(wrapMiddleware, middleware.Recoverer, trafficMiddleware)

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
		staticRouter.PathPrefix("/web/js/").Handler(plexClient)
		staticRouter.PathPrefix("/web/static/").Handler(plexClient)

		dynamicRouter := r.Methods(http.MethodGet).Subrouter()
		dynamicRouter.Use(dynamicMiddleware)
		dynamicRouter.PathPrefix("/").Handler(plexClient)
	}

	r.PathPrefix("/").Handler(plexClient)
	return r
}
