module github.com/RoyXiang/plexproxy

go 1.17

require (
	github.com/go-chi/chi/v5 v5.0.7
	github.com/go-redis/redis/v8 v8.11.4
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.5.0
	github.com/jrudio/go-plex-client v0.0.0-20220106065909-9e1d590b99aa
	github.com/tidwall/gjson v1.14.0
	github.com/xanderstrike/plexhooks v0.0.0-20200926011736-c63bcd35fe3e
)

require (
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
)

replace github.com/jrudio/go-plex-client v0.0.0-20220106065909-9e1d590b99aa => github.com/RoyXiang/go-plex-client v0.0.0-20220223140842-7433de7e9b77
