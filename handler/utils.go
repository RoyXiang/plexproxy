package handler

import (
	"context"
	"net/http"
	"net/http/httputil"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/RoyXiang/plexproxy/common"
	"github.com/go-chi/chi/v5/middleware"
)

func newSingleHostReverseProxy(target *url.URL, hostname string) *httputil.ReverseProxy {
	targetQuery := target.RawQuery
	director := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		if hostname != "" {
			req.URL.Host = hostname
		} else {
			req.URL.Host = target.Host
		}
		req.URL.Path, req.URL.RawPath = joinURLPath(target, req.URL)
		if targetQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = targetQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
		}
		if _, ok := req.Header["User-Agent"]; !ok {
			// explicitly disable User-Agent so it's not set to default value
			req.Header.Set("User-Agent", "")
		}
	}
	proxy := &httputil.ReverseProxy{Director: director}

	proxy.FlushInterval = -1
	proxy.ErrorLog = common.GetLogger()
	proxy.ModifyResponse = modifyResponse
	proxy.ErrorHandler = proxyErrorHandler
	return proxy
}

func joinURLPath(a, b *url.URL) (path, rawpath string) {
	if a.RawPath == "" && b.RawPath == "" {
		return singleJoiningSlash(a.Path, b.Path), ""
	}
	// Same as singleJoiningSlash, but uses EscapedPath to determine
	// whether a slash should be added
	apath := a.EscapedPath()
	bpath := b.EscapedPath()

	aslash := strings.HasSuffix(apath, "/")
	bslash := strings.HasPrefix(bpath, "/")

	switch {
	case aslash && bslash:
		return a.Path + b.Path[1:], apath + bpath[1:]
	case !aslash && !bslash:
		return a.Path + "/" + b.Path, apath + "/" + bpath
	}
	return a.Path + b.Path, apath + bpath
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

func wrapResponseWriter(w http.ResponseWriter, protoMajor int) middleware.WrapResponseWriter {
	if nw, ok := w.(middleware.WrapResponseWriter); ok {
		return nw
	}
	return middleware.NewWrapResponseWriter(w, protoMajor)
}

func modifyResponse(resp *http.Response) error {
	contentType := resp.Header.Get(headerContentType)
	if contentType == "" {
		return nil
	}
	pieces := strings.Split(contentType, "/")
	if len(pieces) == 0 {
		return nil
	}
	switch pieces[0] {
	case "audio", "video":
		resp.Header.Set(headerCacheControl, "no-cache")
		resp.Header.Set(headerVary, "*")
	case "image":
		resp.Header.Set(headerCacheControl, "public, max-age=86400, s-maxage=259200")
	default:
		resp.Header.Set(headerCacheControl, "no-cache")
	}
	return nil
}

func proxyErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	ctxErr := r.Context().Err()
	switch ctxErr {
	case context.Canceled:
		w.WriteHeader(http.StatusBadRequest)
	case context.DeadlineExceeded:
		w.WriteHeader(http.StatusGatewayTimeout)
	default:
		logEntry := middleware.GetLogEntry(r)
		if logEntry != nil {
			logEntry.Panic(err, debug.Stack())
		} else {
			middleware.PrintPrettyStack(err)
		}
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func cloneRequest(r *http.Request, headers http.Header, query url.Values) *http.Request {
	nr := r.Clone(r.Context())
	if headers != nil {
		nr.Header = headers
	}
	if query != nil {
		nr.URL.RawQuery = query.Encode()
		nr.RequestURI = nr.URL.RequestURI()
	}
	return nr
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
