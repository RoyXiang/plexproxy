package handler

import (
	"net/http"

	cache "github.com/victorspringer/http-cache"
)

func CacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		defer mu.RUnlock()

		cacheClient := r.Context().Value(cacheClientCtxKey).(*cache.Client)
		cacheClient.Middleware(next).ServeHTTP(w, r)
	})
}
