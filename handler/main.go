package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/mux"
)

func getRequestParam(r *http.Request, key string, delete bool) string {
	var result string
	if value := r.URL.Query().Get(key); value != "" {
		if delete {
			r.URL.Query().Del(key)
		}
		result = value
	}
	if value := r.Header.Get(key); value != "" {
		if delete {
			r.Header.Del(key)
		}
		result = value
	}
	return result
}

func NewRouter() http.Handler {
	r := mux.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	defaultRouter := r.MatcherFunc(bypassStreamMatcher).Subrouter()
	defaultRouter.Methods(http.MethodGet).PathPrefix("/web/").HandlerFunc(webHandler)
	defaultRouter.Path("/:/timeline").HandlerFunc(timelineHandler)
	defaultRouter.Path("/video/:/transcode/universal/decision").HandlerFunc(decisionHandler)

	refreshRouter := defaultRouter.PathPrefix("/library/sections").Subrouter()
	refreshRouter.Use(refreshMiddleware)
	refreshRouter.Path("/{id}/refresh").HandlerFunc(handler)

	staticRouter := defaultRouter.Methods(http.MethodGet).Subrouter()
	staticRouter.Use(staticMiddleware)
	staticRouter.Path("/library/metadata/{key}/art/{id}").HandlerFunc(handler)
	staticRouter.Path("/library/metadata/{key}/thumb/{id}").HandlerFunc(handler)
	staticRouter.Path("/photo/:/transcode").HandlerFunc(handler)

	userRouter := defaultRouter.Methods(http.MethodGet).PathPrefix("/library").Subrouter()
	userRouter.Use(userMiddleware)
	userRouter.PathPrefix("/collections/").HandlerFunc(handler)
	userRouter.PathPrefix("/metadata/").HandlerFunc(handler)
	userRouter.PathPrefix("/sections/").HandlerFunc(handler)

	dynamicRouter := defaultRouter.Methods(http.MethodGet).Subrouter()
	dynamicRouter.Use(dynamicMiddleware)
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
			getRequestParam(request, headerToken, true)
			plaxtProxy.ServeHTTP(httptest.NewRecorder(), request)
		}()
	}

	proxy.ServeHTTP(w, r)
}

func decisionHandler(w http.ResponseWriter, r *http.Request) {
	r.URL.Query().Set("autoAdjustQuality", "0")
	r.URL.Query().Set("copyts", "0")
	r.URL.Query().Set("directPlay", "1")
	r.URL.Query().Set("directStream", "1")
	r.URL.Query().Set("directStreamAudio", "1")
	r.URL.Query().Set("hasMDE", "0")
	r.URL.Query().Set("videoQuality", "100")
	r.URL.Query().Set("videoResolution", "4096x2160")
	r.URL.Query().Del("maxVideoBitrate")
	r.URL.Query().Del("videoBitrate")

	extraProfile := getRequestParam(r, headerExtra, true)
	if extraProfile != "" {
		params := strings.Split(extraProfile, "+")
		i := 0
		for _, value := range params {
			if !strings.HasPrefix(value, "add-limitation") {
				params[i] = value
				i++
			}
		}
		r.Header.Set(headerExtra, strings.Join(params[:i], "+"))
	}

	proxy.ServeHTTP(w, r)
}
