package handler

import (
	"fmt"
	"net/http"
)

func TrafficMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lockKey := fmt.Sprintf("%s:%s", r.Method, r.URL.RequestURI())
		ml.Lock(lockKey)
		defer ml.Unlock(lockKey)

		next.ServeHTTP(w, r)
	})
}
