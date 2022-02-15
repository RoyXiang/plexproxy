package handler

import (
	"context"
	"net/http"
)

func DynamicMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler := TrafficMiddleware(next)
		for dynamicCacheClient != nil {
			if rangeInHeader := r.Header.Get(headerRange); rangeInHeader != "" {
				break
			}
			if token := r.Header.Get(headerToken); token != "" {
				ctx := context.WithValue(context.Background(), cacheClientCtxKey, dynamicCacheClient)
				r = r.Clone(ctx)
				r.URL.Query().Set(headerToken, token)
				handler = CacheMiddleware(handler)
			}
			break
		}
		handler.ServeHTTP(w, r)
	})
}
