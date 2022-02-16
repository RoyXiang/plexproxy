package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/RoyXiang/plexproxy/common"
	"github.com/gorilla/handlers"
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

func newMockHTTPRespWriter() *mockHTTPRespWriter {
	return &mockHTTPRespWriter{
		httptest.NewRecorder(),
	}
}

func (w *mockHTTPRespWriter) WriteResponse() error {
	return nil
}

func (w *mockHTTPRespWriter) WriteRespHeaders(status int, header http.Header) error {
	w.WriteHeader(status)
	for header, val := range header {
		w.Header()[header] = val
	}
	return nil
}

func (w *mockHTTPRespWriter) Read(_ []byte) (int, error) {
	return 0, fmt.Errorf("mockHTTPRespWriter doesn't implement io.Reader")
}

func NewRouter() http.Handler {
	r := mux.NewRouter()
	r.Use(handlers.ProxyHeaders)
	r.Use(loggingMiddleware)

	defaultRouter := r.MatcherFunc(bypassStreamMatcher).Subrouter()
	defaultRouter.Use(handlers.RecoveryHandler(
		handlers.RecoveryLogger(common.GetLogger()),
		handlers.PrintRecoveryStack(true),
	))
	defaultRouter.Methods(http.MethodGet).PathPrefix("/web/").HandlerFunc(webHandler)
	defaultRouter.Path("/:/timeline").HandlerFunc(timelineHandler)

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
			plaxtProxy.ServeHTTP(newMockHTTPRespWriter(), request)
		}()
	}

	proxy.ServeHTTP(w, r)
}
