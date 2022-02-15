package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func Handler(w http.ResponseWriter, r *http.Request) {
	proxy.ServeHTTP(w, r)
}

func WebHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "https://app.plex.tv/desktop", http.StatusMovedPermanently)
}

func TimelineHandler(w http.ResponseWriter, r *http.Request) {
	if plaxtProxy != nil {
		request := r.Clone(context.Background())
		go func() {
			getRequestParam(request, headerToken, true)
			plaxtProxy.ServeHTTP(newMockHTTPRespWriter(), request)
		}()
	}

	proxy.ServeHTTP(w, r)
}
