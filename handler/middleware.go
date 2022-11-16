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
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/RoyXiang/plexproxy/common"
)

var (
	cacheInfoCtxKey = &ctxKeyType{"cacheInfo"}
	tokenCtxKey     = &ctxKeyType{"token"}
	userCtxKey      = &ctxKeyType{"user"}
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
							query := u.Query()
							for k := range query {
								if strings.HasPrefix(k, headerPlexPrefix) {
									query.Del(k)
								}
							}
							u.RawQuery = query.Encode()
							params.Add(name, u.RequestURI())
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

		next.ServeHTTP(w, cloneRequest(r, headers, params))
	})
}

func wrapMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token := r.Header.Get(headerToken); token != "" {
			ctx := context.WithValue(r.Context(), tokenCtxKey, token)
			if user := plexClient.GetUser(token); user != nil {
				ctx = context.WithValue(ctx, userCtxKey, user)
			} else {
				common.GetLogger().Printf("Cannot get user info: %s", token)
			}
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(wrapResponseWriter(w, r.ProtoMajor), r)
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
		if !plexClient.MulLock.TryLock(lockKey, time.Second) {
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		}
		defer plexClient.MulLock.Unlock(lockKey)
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

func dynamicMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ctx context.Context
		switch filepath.Ext(r.URL.EscapedPath()) {
		case ".css", ".ico", ".jpeg", ".jpg", ".webp":
			ctx = context.WithValue(r.Context(), cacheInfoCtxKey, &cacheInfo{
				Prefix: cachePrefixStatic,
				Ttl:    cacheTtlStatic,
			})
		case ".m3u8", ".ts":
			ctx = r.Context()
		default:
			if rh := r.Header.Get(headerRange); rh != "" {
				ctx = r.Context()
				break
			}
			ctx = context.WithValue(r.Context(), cacheInfoCtxKey, &cacheInfo{
				Prefix: cachePrefixDynamic,
				Ttl:    cacheTtlDynamic,
			})
		}
		cacheMiddleware(next).ServeHTTP(w, r.WithContext(ctx))
	})
}

func cacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxValue := r.Context().Value(cacheInfoCtxKey)
		if ctxValue == nil {
			next.ServeHTTP(w, r)
			return
		}

		var cacheKey string
		ctx := context.Background()
		info := ctxValue.(*cacheInfo)

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
				nw := wrapResponseWriter(httptest.NewRecorder(), r.ProtoMajor)
				next.ServeHTTP(nw, r)
				resp = nw.Unwrap().(*httptest.ResponseRecorder).Result()
				defer func() {
					w.Header().Set(headerCacheStatus, "MISS")
					w.WriteHeader(resp.StatusCode)
					_, _ = w.Write(nw.Unwrap().(*httptest.ResponseRecorder).Body.Bytes())
					if resp.StatusCode == http.StatusOK {
						writeToCache(cacheKey, resp, info.Ttl)
					}
				}()
			} else {
				defer func() {
					w.Header().Set(headerCacheStatus, "HIT")
					w.WriteHeader(resp.StatusCode)
					_, _ = io.Copy(w, resp.Body)
					if info.Prefix == cachePrefixStatic {
						writeToCache(cacheKey, resp, info.Ttl)
					}
				}()
			}
			for k, v := range resp.Header {
				w.Header()[k] = v
			}
		}()
		params := r.URL.Query()
		switch info.Prefix {
		case cachePrefixStatic:
			break
		case cachePrefixDynamic:
			if user := r.Context().Value(userCtxKey); user != nil {
				params.Set(headerUserId, strconv.Itoa(user.(*plexUser).Id))
				params.Set(headerAccept, getAcceptContentType(r))
			} else if token := r.Context().Value(tokenCtxKey); token != nil {
				params.Set(headerToken, token.(string))
			} else {
				return
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
