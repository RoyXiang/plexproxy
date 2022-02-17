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
)

var (
	cacheInfoCtxKey = ctxKeyType{}
)

func trafficMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := http.Header{}
		params := url.Values{}
		for name, values := range r.URL.Query() {
			switch {
			case name == headerPageSize, name == headerPageStart:
				params[name] = values
			case name == headerToken:
				headers[name] = values
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
			case strings.HasPrefix(name, headerPlexPrefix), name == headerLanguage:
				headers[name] = values
			default:
				params[name] = values
			}
		}
		for name, values := range r.Header {
			switch name {
			case headerPageSize, headerPageStart:
				params[name] = values
			case headerToken:
				headers[name] = values
				params[name] = values
			default:
				headers[name] = values
			}
		}

		nr := r.Clone(r.Context())
		nr.Header = headers
		nr.URL.RawQuery = params.Encode()

		var lockKey string
		if hash := headers.Get(headerHash); hash != "" {
			lockKey = hash
		} else {
			if rg := headers.Get(headerRange); rg != "" {
				params.Set(headerRange, rg)
			}
			lockKey = fmt.Sprintf("%s?%s", r.URL.EscapedPath(), params.Encode())
		}
		ml.Lock(lockKey)
		defer ml.Unlock(lockKey)

		next.ServeHTTP(w, nr)
	})
}

func staticMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), cacheInfoCtxKey, &cacheInfo{
			Prefix: cachePrefixStatic,
			Ttl:    cacheTtlStatic,
		})
		r = r.WithContext(ctx)
		cacheMiddleware(next).ServeHTTP(w, r)
	})
}

func userMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), cacheInfoCtxKey, &cacheInfo{
			Prefix: cachePrefixDynamic,
			Ttl:    cacheTtlUser,
		})
		r = r.WithContext(ctx)
		cacheMiddleware(next).ServeHTTP(w, r)
	})
}

func dynamicMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), cacheInfoCtxKey, &cacheInfo{
			Prefix: cachePrefixDynamic,
			Ttl:    cacheTtlDynamic,
		})
		r = r.WithContext(ctx)
		cacheMiddleware(next).ServeHTTP(w, r)
	})
}

func cacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		defer mu.RUnlock()

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

		params := r.URL.Query()
		if info.Prefix == cachePrefixStatic {
			params.Del(headerToken)
		}
		if len(params) > 0 {
			cacheKey = fmt.Sprintf("%s:%s?%s", info.Prefix, r.URL.EscapedPath(), params.Encode())
		} else {
			cacheKey = fmt.Sprintf("%s:%s", info.Prefix, r.URL.EscapedPath())
		}
	})
}
