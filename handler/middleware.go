package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
)

func refreshMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if redisClient != nil && w.(middleware.WrapResponseWriter).Status() == http.StatusOK {
			go func() {
				mu.Lock()
				defer mu.Unlock()

				ctx := context.Background()
				keys := redisClient.Keys(ctx, fmt.Sprintf("%s*", cachePrefixDynamic)).Val()
				if len(keys) > 0 {
					redisClient.Del(ctx, keys...).Val()
				}
			}()
		}
	})
}

func trafficMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var lockKey string
		if hash := r.Header.Get(headerHash); hash != "" {
			lockKey = hash
		} else {
			lockKey = fmt.Sprintf("%s:%s", r.URL.RequestURI(), r.Header.Get(headerToken))
		}
		ml.Lock(lockKey)
		defer ml.Unlock(lockKey)

		ctxValue := r.Context().Value(cacheClientCtxKey)
		if ctxValue != nil {
			shouldCache := true
			cachePrefix := ctxValue.(*cacheClient).GetPrefix()
			if cachePrefix == cachePrefixDynamic {
				if token := r.Header.Get(headerToken); token != "" {
					r.URL.Query().Set(headerToken, token)
					if size := getRequestParam(r, headerPageSize, true); size != "" {
						r.URL.Query().Set(headerPageSize, size)
					}
					if start := getRequestParam(r, headerPageStart, true); start != "" {
						r.URL.Query().Set(headerPageStart, start)
					}
				} else {
					shouldCache = false
				}
			} else {
				for name := range r.URL.Query() {
					switch {
					case strings.HasPrefix(name, headerPlexPrefix):
						value := getRequestParam(r, name, true)
						r.Header.Set(name, value)
					case name == "url":
						value := getRequestParam(r, name, true)
						u, _ := url.Parse(value)
						switch u.Hostname() {
						case "", "127.0.0.1":
							r.URL.Query().Set(name, u.EscapedPath())
						default:
							r.URL.Query().Set(name, u.String())
						}
					}
				}
			}
			if shouldCache {
				next = ctxValue.(*cacheClient).Wrap(next)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func staticMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if staticCacheClient != nil {
			ctx := context.WithValue(r.Context(), cacheClientCtxKey, staticCacheClient)
			r = r.WithContext(ctx)
		}
		trafficMiddleware(next).ServeHTTP(w, r)
	})
}

func userMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userCacheClient != nil {
			ctx := context.WithValue(r.Context(), cacheClientCtxKey, userCacheClient)
			r = r.WithContext(ctx)
		}
		trafficMiddleware(next).ServeHTTP(w, r)
	})
}

func dynamicMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dynamicCacheClient != nil {
			ctx := context.WithValue(r.Context(), cacheClientCtxKey, dynamicCacheClient)
			r = r.WithContext(ctx)
		}
		trafficMiddleware(next).ServeHTTP(w, r)
	})
}
