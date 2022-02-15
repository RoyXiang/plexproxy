package handler

import (
	"context"
	"net/http"
)

func UserMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler := TrafficMiddleware(next)
		if userCacheClient != nil {
			if token := r.Header.Get(headerToken); token != "" {
				ctx := context.WithValue(context.Background(), cacheClientCtxKey, userCacheClient)
				r = r.Clone(ctx)
				r.URL.Query().Set(headerToken, token)
				handler = CacheMiddleware(handler)
			}
		}
		handler.ServeHTTP(w, r)
	})
}
