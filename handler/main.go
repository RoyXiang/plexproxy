package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

func NewRouter() http.Handler {
	r := mux.NewRouter()
	r.Use(handlers.ProxyHeaders)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Methods(http.MethodGet).PathPrefix("/web/").HandlerFunc(webHandler)
	r.Path("/:/timeline").HandlerFunc(timelineHandler)
	r.Path("/library/sections/{id}/refresh").HandlerFunc(refreshHandler)
	r.PathPrefix("/:/").HandlerFunc(handler)

	getRouter := r.Methods(http.MethodGet).Subrouter()
	getRouter.Use(trafficMiddleware)

	staticRouter := getRouter.NewRoute().Subrouter()
	staticRouter.Use(staticMiddleware)
	staticRouter.Path("/library/metadata/{key}/art/{id}").HandlerFunc(handler)
	staticRouter.Path("/library/metadata/{key}/thumb/{id}").HandlerFunc(handler)
	staticRouter.Path("/photo/:/transcode").HandlerFunc(handler)

	metadataRouter := getRouter.PathPrefix("/library").Subrouter()
	metadataRouter.Use(metadataMiddleware)
	metadataRouter.PathPrefix("/collections/").HandlerFunc(handler)
	metadataRouter.PathPrefix("/metadata/").HandlerFunc(handler)
	metadataRouter.PathPrefix("/sections/").HandlerFunc(handler)

	dynamicRouter := getRouter.NewRoute().Subrouter()
	dynamicRouter.Use(dynamicMiddleware)
	dynamicRouter.Path("/video/:/transcode/universal/decision").HandlerFunc(decisionHandler)
	dynamicRouter.PathPrefix("/").HandlerFunc(handler)

	r.PathPrefix("/").HandlerFunc(handler)
	return r
}

func handler(w http.ResponseWriter, r *http.Request) {
	proxy.ServeHTTP(w, r)
}

func webHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "https://app.plex.tv/desktop", http.StatusMovedPermanently)
}

func timelineHandler(w http.ResponseWriter, r *http.Request) {
	if plaxtProxy != nil {
		ctx := context.WithValue(context.Background(), http.ServerContextKey, r.Context().Value(http.ServerContextKey))
		request := r.Clone(ctx)
		go func() {
			request.Header.Del(headerToken)
			params := request.URL.Query()
			params.Del(headerToken)
			request.URL.RawQuery = params.Encode()
			plaxtProxy.ServeHTTP(httptest.NewRecorder(), request)
		}()
	}

	proxy.ServeHTTP(w, r)
}

func refreshHandler(w http.ResponseWriter, r *http.Request) {
	proxy.ServeHTTP(w, r)
	if redisClient != nil && w.(middleware.WrapResponseWriter).Status() == http.StatusOK {
		go func() {
			mu.Lock()
			defer mu.Unlock()

			ctx := context.Background()
			keys := redisClient.Keys(ctx, fmt.Sprintf("%s:*", cachePrefixMetadata)).Val()
			if len(keys) > 0 {
				redisClient.Del(ctx, keys...).Val()
			}
		}()
	}
}

func decisionHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	query.Del("maxVideoBitrate")
	query.Del("videoBitrate")
	query.Set("autoAdjustQuality", "0")
	query.Set("copyts", "0")
	query.Set("directPlay", "1")
	query.Set("directStream", "1")
	query.Set("directStreamAudio", "1")
	query.Set("hasMDE", "0")
	query.Set("videoQuality", "100")
	query.Set("videoResolution", "4096x2160")

	nr := r.Clone(r.Context())
	nr.URL.RawQuery = query.Encode()
	if extraProfile := r.Header.Get(headerExtra); extraProfile != "" {
		params := strings.Split(extraProfile, "+")
		i := 0
		for _, value := range params {
			if !strings.HasPrefix(value, "add-limitation") {
				params[i] = value
				i++
			}
		}
		nr.Header.Set(headerExtra, strings.Join(params[:i], "+"))
	}
	proxy.ServeHTTP(w, nr)
}
