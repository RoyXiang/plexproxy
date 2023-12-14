package handler

import (
	"context"
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
