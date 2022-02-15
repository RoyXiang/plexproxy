package handler

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
)

var (
	proxy *httputil.ReverseProxy
)

func init() {
	baseUrl := os.Getenv("PLEX_BASEURL")
	if baseUrl == "" {
		log.Fatalln("Please configure PLEX_BASEURL at first")
	}
	u, err := url.Parse(baseUrl)
	if err != nil {
		log.Fatalln("Please ensure PLEX_BASEURL is a valid URL")
	}
	proxy = httputil.NewSingleHostReverseProxy(u)
}

func Handler(w http.ResponseWriter, r *http.Request) {
	proxy.ServeHTTP(w, r)
}
