package handler

import (
	"crypto/sha256"
	"net/http"
	"sort"
	"strings"
)

func TrafficMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for name, values := range r.URL.Query() {
			if strings.HasPrefix(name, headerPlexPrefix) &&
				name != headerContainerSize &&
				name != headerContainerStart {
				r.Header.Del(name)
				for _, value := range values {
					r.Header.Add(name, value)
				}
			}
		}
		if size := r.Header.Get(headerContainerSize); size != "" {
			r.URL.Query().Set(headerContainerSize, size)
			r.Header.Del(headerContainerSize)
		}
		if start := r.Header.Get(headerContainerStart); start != "" {
			r.URL.Query().Set(headerContainerStart, start)
			r.Header.Del(headerContainerStart)
		}

		params := r.URL.Query()
		for _, param := range params {
			sort.Slice(param, func(i, j int) bool {
				return param[i] < param[j]
			})
		}
		r.URL.RawQuery = params.Encode()

		if token := r.Header.Get(headerToken); token != "" {
			params.Set(headerToken, token)
		}
		if rg := r.Header.Get(headerRange); rg != "" {
			params.Set(headerRange, rg)
		}
		lockKey := sha256.Sum256([]byte(params.Encode()))
		ml.Lock(lockKey)
		defer ml.Unlock(lockKey)

		next.ServeHTTP(w, r)
	})
}
