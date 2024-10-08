# Plex Proxy

`plexproxy` is a middleware runs before a [Plex Media Server](https://www.plex.tv/media-server-downloads/) which could increase
the performance of a low-end server.

## Features

1. Traffic control by devices
2. Cross-device response caching by client type
3. Disable transcoding by forcing direct play/stream
4. Redirect web app to [official one](https://app.plex.tv/desktop)
5. [Plaxt](https://github.com/XanderStrike/goplaxt) integration

## Prerequisites

1. Plex Media Server
2. Redis (Optional)

## Install

Download from [Releases](https://github.com/RoyXiang/plexproxy/releases/latest), or build by yourself:

```sh
env CGO_ENABLED=0 go install -trimpath -ldflags="-s -w" github.com/RoyXiang/plexproxy@latest
```

## Usage

1. Configure environment variables in your preferred way
   - `PLEX_BASEURL` (Required, e.g. `http://127.0.0.1:32400`)
   - `PLAXT_URL` (Optional, e.g. `https://plaxt.astandke.com/api?id=generate-your-own-silly`)
     * `PLEX_TOKEN` is required
     * Set it if you run an instance of [Plaxt](https://github.com/XanderStrike/goplaxt)
     * Or, you can set it to [the official one](https://plaxt.astandke.com/)
   - `PLEX_TOKEN` (Optional, if you need it, see [here](https://support.plex.tv/articles/204059436-finding-an-authentication-token-x-plex-token/))
   - `STATIC_CACHE_SIZE` (Optional, the cache size of static files, e.g. CSS files, images, default: `1000`)
   - `STATIC_CACHE_TTL` (Optional, the cache TTL of static files, default: `72h`)
   - `REDIRECT_WEB_APP` (Optional, default: `true`)
   - `DISABLE_TRANSCODE` (Optional, default: `true`)
   - `NO_REQUEST_LOGS` (Optional, default: `false`)
2. Run the program
