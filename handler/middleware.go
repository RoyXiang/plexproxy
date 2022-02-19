package handler

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	cacheInfoCtxKey = &ctxKeyType{"cacheInfo"}
)

func normalizeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := http.Header{}
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
			case strings.HasPrefix(name, headerPlexPrefix),
				name == headerAcceptLanguage,
				name == headerToken:
				headers[name] = values
			default:
				params[name] = values
			}
		}
		for name, values := range r.Header {
			switch name {
			case headerPageSize, headerPageStart:
				params[name] = values
			default:
				headers[name] = values
			}
		}

		nr := cloneRequest(r, headers, params)
		if fwd := getIP(r); fwd != "" {
			nr.RemoteAddr = fwd
		}
		next.ServeHTTP(w, nr)
	})
}

func trafficMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := r.URL.Query()
		if token := r.Header.Get(headerToken); token != "" {
			params.Set(headerToken, token)
		}
		if rg := r.Header.Get(headerRange); rg != "" {
			params.Set(headerRange, rg)
		}
		lockKey := fmt.Sprintf("%s?%s", r.URL.EscapedPath(), params.Encode())
		if !ml.TryLock(lockKey, time.Second) {
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		}
		defer ml.Unlock(lockKey)
		next.ServeHTTP(w, r)
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

func metadataMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), cacheInfoCtxKey, &cacheInfo{
			Prefix: cachePrefixMetadata,
			Ttl:    cacheTtlMetadata,
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
		var cacheKey string
		ctx := context.Background()
		info := r.Context().Value(cacheInfoCtxKey).(*cacheInfo)
		cacheTtl := info.Ttl
		if info.Prefix == cachePrefixMetadata {
			mu.RLock()
			defer mu.RUnlock()
		}

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
					w.Header().Set(headerCacheStatus, "MISS")
					w.WriteHeader(resp.StatusCode)
					_, _ = w.Write(nw.Body.Bytes())
					if resp.StatusCode == http.StatusOK {
						writeToCache(cacheKey, resp, cacheTtl)
					}
				}()
			} else {
				defer func() {
					w.Header().Set(headerCacheStatus, "HIT")
					w.WriteHeader(resp.StatusCode)
					_, _ = io.Copy(w, resp.Body)
					if info.Prefix == cachePrefixStatic {
						writeToCache(cacheKey, resp, cacheTtl)
					}
				}()
			}

			for k, v := range resp.Header {
				w.Header()[k] = v
			}
			if info.Prefix == cachePrefixStatic {
				w.Header().Set(headerCacheControl, "public, max-age=86400, s-maxage=259200")
			} else {
				w.Header().Set(headerCacheControl, "no-cache")
			}
		}()
		if redisClient == nil || isStreamRequest(r) {
			return
		}
		params := r.URL.Query()
		switch info.Prefix {
		case cachePrefixStatic:
			break
		case cachePrefixDynamic, cachePrefixMetadata:
			token := r.Header.Get(headerToken)
			if token == "" {
				return
			}
			userId := getPlexUserId(token)
			if userId > 0 {
				params.Set("X-Plex-User-Id", strconv.Itoa(userId))
				params.Set(headerAccept, getAcceptContentType(r))
				cacheTtl = cacheTtl * 2
			} else {
				params.Set(headerToken, token)
			}
		default:
			// invalid prefix
			return
		}
		params.Del("skipRefresh")
		if len(params) > 0 {
			cacheKey = fmt.Sprintf("%s:%s?%s", info.Prefix, r.URL.EscapedPath(), params.Encode())
		} else {
			cacheKey = fmt.Sprintf("%s:%s", info.Prefix, r.URL.EscapedPath())
		}
	})
}
