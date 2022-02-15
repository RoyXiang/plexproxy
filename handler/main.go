package handler

import (
	"context"
	"net/http"

	"github.com/RoyXiang/plexproxy/common"
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

func (w *fakeCloseReadCloser) Close() error {
	return nil
}

func (w *fakeCloseReadCloser) RealClose() error {
	if w.ReadCloser == nil {
		return nil
	}
	return w.ReadCloser.Close()
}

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		r.Body = &fakeCloseReadCloser{r.Body}
		defer func() {
			_ = r.Body.(*fakeCloseReadCloser).RealClose()
		}()
	}
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
			plaxtProxy.ServeHTTP(common.NewCustomResponseWriter(), request)
		}()
	}

	proxy.ServeHTTP(w, r)
}
