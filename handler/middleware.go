package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"

	"github.com/urfave/negroni"
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
		logger.SetFormat("{{.StartTime}} | {{.Status}} | \t {{.Duration}} | {{.Method}} {{.Path}}")

		n := negroni.New(recovery, logger)
		n.UseHandler(next)
		n.ServeHTTP(w, r)
	})
}

func refreshMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if redisClient != nil {
			result := reflect.ValueOf(w).MethodByName("Status").Call([]reflect.Value{})
			statusCode := result[0].Interface().(int)
			if statusCode == http.StatusOK {
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
		ctx := context.WithValue(r.Context(), cacheClientCtxKey, staticCacheClient)
		r = r.Clone(ctx)

		trafficMiddleware(next).ServeHTTP(w, r)
	})
}

func userMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), cacheClientCtxKey, userCacheClient)
		r = r.Clone(ctx)

		trafficMiddleware(next).ServeHTTP(w, r)
	})
}

func dynamicMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), cacheClientCtxKey, dynamicCacheClient)
		r = r.Clone(ctx)

		trafficMiddleware(next).ServeHTTP(w, r)
	})
}
