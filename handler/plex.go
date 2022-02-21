package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/RoyXiang/plexproxy/common"
	"github.com/jrudio/go-plex-client"
)

type PlexConfig struct {
	BaseUrl      string
	Token        string
	PlaxtBaseUrl string
}

type PlexClient struct {
	proxy  *httputil.ReverseProxy
	client *plex.Plex

	plaxtBaseUrl string

	mu sync.RWMutex
}

func NewPlexClient(config PlexConfig) *PlexClient {
	if config.BaseUrl == "" {
		return nil
	}
	u, err := url.Parse(config.BaseUrl)
	if err != nil {
		return nil
	}

	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.FlushInterval = -1
	proxy.ErrorLog = common.GetLogger()
	proxy.ModifyResponse = modifyResponse
	proxy.ErrorHandler = proxyErrorHandler

	client, _ := plex.New(config.BaseUrl, config.Token)

	var plaxtBaseUrl string
	u, err = url.Parse(config.PlaxtBaseUrl)
	if err == nil {
		plaxtBaseUrl = config.PlaxtBaseUrl
	}

	return &PlexClient{
		proxy:        proxy,
		client:       client,
		plaxtBaseUrl: plaxtBaseUrl,
	}
}

func (c *PlexClient) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.EscapedPath()
	switch {
	case path == "/:/timeline":
		defer c.syncTimelineWithPlaxt(r)
	case path == "/video/:/transcode/universal/decision":
		r = c.disableTranscoding(r)
	case strings.HasPrefix(path, "/web/"):
		if r.Method == http.MethodGet {
			http.Redirect(w, r, "https://app.plex.tv/desktop", http.StatusMovedPermanently)
			return
		}
	}

	c.proxy.ServeHTTP(w, r)
}

func (c *PlexClient) IsTokenSet() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.client.Token != ""
}

func (c *PlexClient) TestReachability() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result, _ := c.client.Test()
	return result
}

func (c *PlexClient) SubscribeToNotifications(events *plex.NotificationEvents, interrupt <-chan os.Signal, fn func(error)) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	c.client.SubscribeToNotifications(events, interrupt, fn)
}

func (c *PlexClient) GetUserId(token string) (id int) {
	var err error
	ctx := context.Background()
	cacheKey := fmt.Sprintf("%s:token:%s", cachePrefixPlex, token)
	id, err = redisClient.Get(ctx, cacheKey).Int()
	if err == nil {
		return id
	}

	response := c.GetSharedServers()
	for _, friend := range response.Friends {
		key := fmt.Sprintf("%s:token:%s", cachePrefixPlex, friend.AccessToken)
		redisClient.Set(ctx, key, friend.UserId, 0)
		if friend.AccessToken == token {
			id = friend.UserId
		}
	}
	if id > 0 {
		return
	}

	user := c.GetAccountInfo(token)
	if user.ID > 0 {
		redisClient.Set(ctx, cacheKey, user.ID, 0)
		id = user.ID
	}
	return
}

func (c *PlexClient) GetSharedServers() (response plex.SharedServersResponse) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	identity, err := c.client.GetServerIdentity()
	if err != nil {
		return
	}
	response, _ = c.client.GetSharedServers(identity.MediaContainer.MachineIdentifier)
	return
}

func (c *PlexClient) GetAccountInfo(token string) (user plex.UserPlexTV) {
	c.mu.Lock()
	originalToken := token
	defer func() {
		c.client.Token = originalToken
		c.mu.Unlock()
	}()

	c.client.Token = token
	user, _ = c.client.MyAccount()
	return
}

func (c *PlexClient) syncTimelineWithPlaxt(r *http.Request) {
	if c.plaxtBaseUrl == "" {
		return
	}
	var client string
	if client = r.Header.Get(headerClientIdentity); client == "" {
		return
	}
	plaxtUrl := fmt.Sprintf("%s%s", c.plaxtBaseUrl, r.RequestURI)
	request, err := http.NewRequest(http.MethodGet, plaxtUrl, nil)
	if err != nil {
		return
	}
	request.Header.Set(headerClientIdentity, client)
	go func() {
		_, _ = c.client.HTTPClient.Do(request)
	}()
}

func (c *PlexClient) disableTranscoding(r *http.Request) *http.Request {
	query := r.URL.Query()
	query.Del("maxVideoBitrate")
	query.Del("videoBitrate")
	query.Set("autoAdjustQuality", "0")
	query.Set("directPlay", "1")
	query.Set("directStream", "1")
	query.Set("directStreamAudio", "1")
	query.Set("videoQuality", "100")
	query.Set("videoResolution", "4096x2160")

	protocol := query.Get("protocol")
	switch protocol {
	case "http":
		query.Set("copyts", "0")
		query.Set("hasMDE", "0")
	}

	headers := r.Header
	if extraProfile := headers.Get(headerExtraProfile); extraProfile != "" {
		params := strings.Split(extraProfile, "+")
		i := 0
		for _, value := range params {
			if !strings.HasPrefix(value, "add-limitation") {
				params[i] = value
				i++
			}
		}
		headers.Set(headerExtraProfile, strings.Join(params[:i], "+"))
	}
	return cloneRequest(r, headers, query)
}
