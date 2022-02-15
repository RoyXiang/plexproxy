package handler

import (
	"net/http"

	cache "github.com/victorspringer/http-cache"
)

func CacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cacheClient := r.Context().Value(cacheClientCtxKey).(*cache.Client)
		rangeInHeader := r.Header.Get(headerRange)
		if cacheClient != nil && rangeInHeader == "" {
			mu.RLock()
			defer mu.RUnlock()

			cacheClient.Middleware(next).ServeHTTP(w, r)
		} else {
			next.ServeHTTP(w, r)
		}
	})
}
