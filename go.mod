module github.com/RoyXiang/plexproxy

go 1.17

require (
	github.com/bluele/gcache v0.0.2
	github.com/go-chi/chi/v5 v5.0.11
	github.com/gorilla/mux v1.8.1
	github.com/jrudio/go-plex-client v0.0.0-20220106065909-9e1d590b99aa
	github.com/xanderstrike/plexhooks v0.0.0-20200926011736-c63bcd35fe3e
)

require (
	github.com/google/uuid v1.3.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/onsi/gomega v1.18.1 // indirect
)

replace github.com/jrudio/go-plex-client v0.0.0-20220106065909-9e1d590b99aa => github.com/RoyXiang/go-plex-client v0.0.0-20220313053419-e24ff7ada173
