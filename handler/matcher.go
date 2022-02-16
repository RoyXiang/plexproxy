package handler

import (
	"net/http"
	"path/filepath"

	"github.com/gorilla/mux"
)

func bypassStreamMatcher(r *http.Request, _ *mux.RouteMatch) bool {
	if rangeInHeader := r.Header.Get(headerRange); rangeInHeader != "" {
		return false
	}
	path := r.URL.EscapedPath()
	switch path {
	case "/:/eventsource/notifications",
		"/:/websockets/notifications":
		return false
	}
	switch filepath.Ext(path) {
	case ".m3u8", ".mkv", ".mp4", ".ts":
		return false
	}
	return true
}
