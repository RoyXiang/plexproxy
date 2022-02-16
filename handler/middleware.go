package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/negroni"
	cache "github.com/victorspringer/http-cache"
)

func (w *fakeCloseReadCloser) Close() error {
	return nil
}

func (w *fakeCloseReadCloser) RealClose() error {
	if w.ReadCloser == nil {
		return nil
	}
	return w.ReadCloser.Close()
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = &fakeCloseReadCloser{r.Body}
			defer func() {
				_ = r.Body.(*fakeCloseReadCloser).RealClose()
			}()
		}

		recovery := negroni.NewRecovery()
		logger := &negroni.Logger{ALogger: log.New(os.Stdout, "", 0)}
		logger.SetDateFormat(negroni.LoggerDefaultDateFormat)
		logger.SetFormat(negroni.LoggerDefaultFormat)

		n := negroni.New(recovery, logger)
		n.UseHandler(next)
		n.ServeHTTP(w, r)
	})
}

func refreshMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lrw := negroni.NewResponseWriter(w)
		next.ServeHTTP(lrw, r)

		if redisClient != nil && lrw.Status() < http.StatusMultipleChoices {
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
			params := make([]string, 4)
			params = append(params, r.Method, r.URL.RequestURI())
			if token := r.Header.Get(headerToken); token != "" {
				params = append(params, token)
			}
			if rg := r.Header.Get(headerRange); rg != "" {
				params = append(params, rg)
			}
			lockKey = strings.Join(params, ":")
		}
		ml.Lock(lockKey)
		defer ml.Unlock(lockKey)

		next.ServeHTTP(w, r)
	})
}

func staticMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler := trafficMiddleware(next)
		if staticCacheClient != nil {
			ctx := context.WithValue(context.Background(), cacheClientCtxKey, staticCacheClient)
			r = r.Clone(ctx)
			handler = cacheMiddleware(handler)
		}
		handler.ServeHTTP(w, r)
	})
}

func userMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler := trafficMiddleware(next)
		if userCacheClient != nil {
			if token := r.Header.Get(headerToken); token != "" {
				ctx := context.WithValue(context.Background(), cacheClientCtxKey, userCacheClient)
				r = r.Clone(ctx)
				r.URL.Query().Set(headerToken, token)
				handler = cacheMiddleware(handler)
			}
		}
		handler.ServeHTTP(w, r)
	})
}

func dynamicMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler := trafficMiddleware(next)
	cache:
		for dynamicCacheClient != nil {
			if rangeInHeader := r.Header.Get(headerRange); rangeInHeader != "" {
				break
			}
			switch filepath.Ext(r.URL.EscapedPath()) {
			case ".m3u8", ".mkv", ".mp4", ".ts":
				break cache
			}
			if token := r.Header.Get(headerToken); token != "" {
				ctx := context.WithValue(context.Background(), cacheClientCtxKey, dynamicCacheClient)
				r = r.Clone(ctx)
				r.URL.Query().Set(headerToken, token)
				handler = cacheMiddleware(handler)
			}
			break
		}
		handler.ServeHTTP(w, r)
	})
}

func cacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		defer mu.RUnlock()

		cacheClient := r.Context().Value(cacheClientCtxKey).(*cache.Client)
		cacheClient.Middleware(next).ServeHTTP(w, r)
	})
}
