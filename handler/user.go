package handler

import (
	"context"
	"net/http"
)

func UserMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(headerToken)
		if userCacheClient != nil && token != "" {
			ctx := context.WithValue(context.Background(), cacheClientCtxKey, userCacheClient)
			r = r.Clone(ctx)
			r.URL.Query().Set(headerToken, token)
		}
		next.ServeHTTP(w, r)
	})
}
