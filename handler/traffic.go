package handler

import (
	"net/http"
	"strings"
)

func TrafficMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var lockKey string
		if hash := r.Header.Get(headerHash); hash != "" {
			lockKey = hash
		} else {
			params := make([]string, 4)
			params = append(params, r.Method, r.URL.RequestURI())
			if token := getRequestParam(r, headerToken, false); token != "" {
				params = append(params, token)
			}
			if rg := getRequestParam(r, headerToken, false); rg != "" {
				params = append(params, rg)
			}
			lockKey = strings.Join(params, ":")
		}
		ml.Lock(lockKey)
		defer ml.Unlock(lockKey)

		next.ServeHTTP(w, r)
	})
}
