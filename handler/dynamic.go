package handler

import (
	"context"
	"net/http"
)

func DynamicMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dynamicCacheClient != nil {
			if token := getRequestParam(r, headerToken, true); token != "" {
				ctx := context.WithValue(context.Background(), cacheClientCtxKey, dynamicCacheClient)
				r = r.Clone(ctx)
				r.URL.Query().Set(headerToken, token)
			}
		}
		next.ServeHTTP(w, r)
	})
}
