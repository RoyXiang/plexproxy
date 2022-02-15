package handler

import (
	"net/http"
)

func CacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(headerToken)
		if cacheClient != nil && token != "" {
			mu.RLock()
			defer mu.RUnlock()

			r.URL.Query().Set(headerToken, token)
			cacheClient.Middleware(next).ServeHTTP(w, r)
		} else {
			next.ServeHTTP(w, r)
		}
	})
}
