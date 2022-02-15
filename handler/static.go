package handler

import (
	"context"
	"net/http"
)

func StaticMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler := TrafficMiddleware(next)
		if staticCacheClient != nil {
			ctx := context.WithValue(context.Background(), cacheClientCtxKey, staticCacheClient)
			r = r.Clone(ctx)
			handler = CacheMiddleware(handler)
		}
		handler.ServeHTTP(w, r)
	})
}
