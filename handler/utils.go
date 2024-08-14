package handler

import (
	"context"
	"mime"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
)

func wrapResponseWriter(w http.ResponseWriter, protoMajor int) middleware.WrapResponseWriter {
	if nw, ok := w.(middleware.WrapResponseWriter); ok {
		return nw
	}
	return middleware.NewWrapResponseWriter(w, protoMajor)
}

func modifyResponse(resp *http.Response) error {
	var mediaType string
	if contentType := resp.Header.Get(headerContentType); contentType != "" {
		mediaType, _, _ = mime.ParseMediaType(contentType)
	}
	switch {
	case mediaType == "text/css",
		mediaType == "text/javascript",
		strings.HasPrefix(mediaType, "image/"),
		strings.HasPrefix(mediaType, "font/"):
		resp.Header.Set(headerCacheControl, "public, max-age=86400, s-maxage=259200")
	default:
		resp.Header.Set(headerCacheControl, "no-cache, no-store, no-transform, must-revalidate, private, max-age=0, s-maxage=0")
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
	if fwd := getIP(headers); fwd != "" {
		nr.RemoteAddr = fwd
	}
	if scheme := getScheme(headers); scheme != "" {
		nr.URL.Scheme = scheme
	}
	return nr
}

func getIP(headers http.Header) (addr string) {
	if fwd := headers.Get(headerForwardedFor); fwd != "" {
		s := strings.Index(fwd, ", ")
		if s == -1 {
			s = len(fwd)
		}
		addr = fwd[:s]
	} else if fwd = headers.Get(headerRealIP); fwd != "" {
		addr = fwd
	}
	return
}

func getScheme(headers http.Header) (scheme string) {
	if proto := headers.Get(headerForwardedProto); proto != "" {
		scheme = strings.ToLower(proto)
	} else if proto = headers.Get(headerForwardedScheme); proto != "" {
		scheme = strings.ToLower(proto)
	}
	return
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
