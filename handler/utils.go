package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"path/filepath"
	"strings"
	"time"
)

func getIP(r *http.Request) string {
	var addr string
	if fwd := r.Header.Get(headerForwardedFor); fwd != "" {
		s := strings.Index(fwd, ", ")
		if s == -1 {
			s = len(fwd)
		}
		addr = fwd[:s]
	} else if fwd := r.Header.Get(headerRealIP); fwd != "" {
		addr = fwd
	}
	return addr
}

func isStreamRequest(r *http.Request) bool {
	if rangeInHeader := r.Header.Get(headerRange); rangeInHeader != "" {
		return true
	}
	switch filepath.Ext(r.URL.EscapedPath()) {
	case ".m3u8", ".ts":
		return true
	}
	return false
}

func getPlexUserId(token string) int {
	ctx := context.Background()
	cacheKey := fmt.Sprintf("%s:token:%s", cachePrefixPlex, token)
	id, err := redisClient.Get(ctx, cacheKey).Int()
	if err == nil {
		return id
	}
	defer func() {
		redisClient.Set(ctx, cacheKey, id, 0)
	}()
	user, err := plexApp.User(token)
	if err == nil {
		id = user.ID
	}
	return id
}

func getAcceptContentType(r *http.Request) string {
	accept := r.Header.Get(headerAccept)
	if accept == "" {
		return contentTypeXml
	}
	fields := strings.FieldsFunc(accept, func(r rune) bool {
		return r == ',' || r == ' '
	})
	for _, field := range fields {
		if field == contentTypeAny {
			continue
		}
		parts := strings.Split(field, "/")
		if len(parts) == 2 {
			return parts[1]
		}
	}
	return contentTypeXml
}

func writeToCache(key string, resp *http.Response, ttl time.Duration) {
	b, err := httputil.DumpResponse(resp, true)
	if err != nil {
		return
	}
	redisClient.Set(context.Background(), key, b, ttl)
}
