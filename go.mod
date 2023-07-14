module github.com/RoyXiang/plexproxy

go 1.17

require (
	github.com/go-chi/chi/v5 v5.0.10
	github.com/go-redis/redis/v8 v8.11.5
	github.com/gorilla/handlers v1.5.1
	github.com/gorilla/mux v1.8.0
	github.com/jrudio/go-plex-client v0.0.0-20220106065909-9e1d590b99aa
	github.com/xanderstrike/plexhooks v0.0.0-20200926011736-c63bcd35fe3e
)

require (
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/felixge/httpsnoop v1.0.1 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
)

replace github.com/jrudio/go-plex-client v0.0.0-20220106065909-9e1d590b99aa => github.com/RoyXiang/go-plex-client v0.0.0-20220313053419-e24ff7ada173
