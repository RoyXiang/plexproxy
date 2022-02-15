package handler

import (
	"net/http"
)

func TrafficMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lockKey := r.Header.Get("X-Plex-Hash")
		if lockKey != "" {
			ml.Lock(lockKey)
			defer ml.Unlock(lockKey)
		}

		next.ServeHTTP(w, r)
	})
}
