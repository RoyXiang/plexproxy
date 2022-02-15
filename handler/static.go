package handler

import (
	"context"
	"net/http"
)

func StaticMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if staticCacheClient != nil {
			ctx := context.WithValue(context.Background(), cacheClientCtxKey, staticCacheClient)
			r = r.Clone(ctx)
			CacheMiddleware(next).ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
