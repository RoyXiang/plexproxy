# Plex Proxy

`plexproxy` is a middleware runs before a [Plex Media Server](https://www.plex.tv/media-server-downloads/) which could increase
the performance of a low-end server.

## Features

1. Traffic control by devices
2. Response caching
3. Disable transcoding by forcing direct play/stream
4. Redirect web app to [official one](https://app.plex.tv/desktop)

## Prerequisites

1. Plex Media Server
2. Redis (Optional)

## Install

Download from [Releases](https://github.com/RoyXiang/plexproxy/releases), or build by yourself:

```sh
env CGO_ENABLED=0 go install -trimpath -ldflags="-s -w" github.com/RoyXiang/plexproxy@latest
```

## Usage

1. Configure environment variables in your preferred way
   - `PLEX_BASEURL` (Required, e.g. `http://127.0.0.1:32400`)
   - `REDIS_URL` (Optional, e.g. `redis://127.0.0.1:6379`)
     * If you need a cache layer, set a value for it
   - `PLEX_TOKEN` (Optional, if you need it, see [here](https://support.plex.tv/articles/204059436-finding-an-authentication-token-x-plex-token/))
     * It is used to receive notifications from Plex Media Server
     * Currently, notifications are used to flush the cache of metadata
   - `PLAXT_BASEURL` (Optional)
     * Set it only if you run an instance of [plaxt](https://github.com/RoyXiang/goplaxt/releases)
     * Or, you can set it to [my hosted one](https://plaxt.royxiang.me), e.g. `https://plaxt.royxiang.me`
     * Currently, webhooks from Plex Media Server do not contain the playback progress of a media item, setting this would
       sync it
2. Run the program

## TODO

- [ ] Native [Trakt](https://trakt.tv/) integration
