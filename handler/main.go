package handler

import (
	"context"
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
	r.Use(trafficMiddleware)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Methods(http.MethodGet).PathPrefix("/web/").HandlerFunc(webHandler)
	r.Path("/:/timeline").HandlerFunc(timelineHandler)

	refreshRouter := r.PathPrefix("/library/sections").Subrouter()
	refreshRouter.Use(refreshMiddleware)
	refreshRouter.Path("/{id}/refresh").HandlerFunc(handler)

	staticRouter := r.Methods(http.MethodGet).Subrouter()
	staticRouter.Use(staticMiddleware)
	staticRouter.Path("/library/metadata/{key}/art/{id}").HandlerFunc(handler)
	staticRouter.Path("/library/metadata/{key}/thumb/{id}").HandlerFunc(handler)
	staticRouter.Path("/photo/:/transcode").HandlerFunc(handler)

	userRouter := r.Methods(http.MethodGet).PathPrefix("/library").Subrouter()
	userRouter.Use(userMiddleware)
	userRouter.PathPrefix("/collections/").HandlerFunc(handler)
	userRouter.PathPrefix("/metadata/").HandlerFunc(handler)
	userRouter.PathPrefix("/sections/").HandlerFunc(handler)

	dynamicRouter := r.Methods(http.MethodGet).Subrouter()
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
