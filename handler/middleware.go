package handler

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
)

var (
	cacheInfoCtxKey = ctxKeyType{}
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
			params := []string{r.URL.RequestURI()}
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

		cacheMiddleware(next).ServeHTTP(w, r)
	})
}

func staticMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), cacheInfoCtxKey, &cacheInfo{
			Prefix: cachePrefixStatic,
			Ttl:    cacheTtlStatic,
		})
		r = r.WithContext(ctx)
		trafficMiddleware(next).ServeHTTP(w, r)
	})
}

func userMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), cacheInfoCtxKey, &cacheInfo{
			Prefix: cachePrefixDynamic,
			Ttl:    cacheTtlUser,
		})
		r = r.WithContext(ctx)
		trafficMiddleware(next).ServeHTTP(w, r)
	})
}

func dynamicMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), cacheInfoCtxKey, &cacheInfo{
			Prefix: cachePrefixDynamic,
			Ttl:    cacheTtlDynamic,
		})
		r = r.WithContext(ctx)
		trafficMiddleware(next).ServeHTTP(w, r)
	})
}

func cacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var cacheKey string
		ctx := context.Background()
		info := r.Context().Value(cacheInfoCtxKey).(*cacheInfo)

		defer func() {
			if cacheKey == "" {
				next.ServeHTTP(w, r)
				return
			}
			var resp *http.Response
			b, err := redisClient.Get(ctx, cacheKey).Bytes()
			if err == nil {
				reader := bufio.NewReader(bytes.NewReader(b))
				resp, _ = http.ReadResponse(reader, r)
			}
			if resp == nil {
				nw := httptest.NewRecorder()
				next.ServeHTTP(nw, r)
				resp = nw.Result()
				defer func() {
					_, _ = w.Write(nw.Body.Bytes())

					if resp.StatusCode == http.StatusOK {
						b, err := httputil.DumpResponse(resp, true)
						if err != nil {
							return
						}
						redisClient.Set(ctx, cacheKey, b, info.Ttl)
					}
				}()
				w.Header().Set(headerCacheStatus, "MISS")
			} else {
				defer func() {
					_, _ = io.Copy(w, resp.Body)
				}()
				w.Header().Set(headerCacheStatus, "HIT")
			}

			for k, v := range resp.Header {
				w.Header()[k] = v
			}
			w.WriteHeader(resp.StatusCode)
		}()

		if redisClient == nil {
			return
		}
		if rangeInHeader := r.Header.Get(headerRange); rangeInHeader != "" {
			return
		}
		path := r.URL.EscapedPath()
		switch path {
		case "/:/eventsource/notifications",
			"/:/websockets/notifications":
			return
		}
		switch filepath.Ext(path) {
		case ".m3u8", ".mkv", ".mp4", ".ts":
			return
		}

		params := url.Values{}
		for name, values := range r.URL.Query() {
			switch {
			case name == headerPageSize, name == headerPageStart:
				params[name] = values
			case name == "url":
				for _, value := range values {
					u, err := url.Parse(value)
					if err == nil {
						switch u.Hostname() {
						case "", "127.0.0.1":
							params.Add(name, u.EscapedPath())
						default:
							params.Add(name, u.String())
						}
					} else {
						params.Add(name, value)
					}
				}
			case strings.HasPrefix(name, headerPlexPrefix):
				break
			default:
				params[name] = values
			}
		}
		for name, values := range r.Header {
			switch name {
			case headerPageSize, headerPageStart:
				params[name] = values
			case headerToken:
				if info.Prefix == cachePrefixDynamic {
					params[name] = values
				}
			}
		}
		request := r.Clone(ctx)
		request.URL.RawQuery = params.Encode()
		cacheKey = fmt.Sprintf("%s%s", info.Prefix, request.URL.RequestURI())
	})
}
