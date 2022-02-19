package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/mux"
)

func NewRouter() http.Handler {
	r := mux.NewRouter()
	r.Use(globalMiddleware)

	r.Methods(http.MethodGet).PathPrefix("/web/").HandlerFunc(webHandler)
	r.Path("/:/eventsource/notifications").HandlerFunc(handler)
	r.Path("/:/timeline").HandlerFunc(timelineHandler)
	r.Path("/:/websockets/notifications").HandlerFunc(handler)
	r.Path("/library/sections/{id}/refresh").HandlerFunc(refreshHandler)
	r.PathPrefix("/library/parts/").HandlerFunc(handler)

	getRouter := r.Methods(http.MethodGet).Subrouter()
	getRouter.Use(trafficMiddleware)

	staticRouter := getRouter.NewRoute().Subrouter()
	staticRouter.Use(staticMiddleware)
	staticRouter.Path("/library/media/{key}/chapterImages/{id}").HandlerFunc(handler)
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
		go func() {
			headers := r.Header
			headers.Del(headerToken)

			nr, cancel := cloneRequest(r, context.Background(), headers, nil)
			defer cancel()

			plaxtProxy.ServeHTTP(httptest.NewRecorder(), nr)
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
	query.Set("directPlay", "1")
	query.Set("directStream", "1")
	query.Set("directStreamAudio", "1")
	query.Set("videoQuality", "100")
	query.Set("videoResolution", "4096x2160")

	protocol := query.Get("protocol")
	switch protocol {
	case "http":
		query.Set("copyts", "0")
		query.Set("hasMDE", "0")
	}

	headers := r.Header
	if extraProfile := headers.Get(headerExtraProfile); extraProfile != "" {
		params := strings.Split(extraProfile, "+")
		i := 0
		for _, value := range params {
			if !strings.HasPrefix(value, "add-limitation") {
				params[i] = value
				i++
			}
		}
		headers.Set(headerExtraProfile, strings.Join(params[:i], "+"))
	}

	nr, cancel := cloneRequest(r, r.Context(), headers, query)
	defer cancel()

	proxy.ServeHTTP(w, nr)
}

func proxyErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	logEntry := middleware.GetLogEntry(r)
	if logEntry != nil {
		logEntry.Panic(err, debug.Stack())
	} else {
		middleware.PrintPrettyStack(err)
	}
	w.WriteHeader(http.StatusBadGateway)
}
