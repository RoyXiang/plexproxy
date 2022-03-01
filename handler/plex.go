package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/RoyXiang/plexproxy/common"
	"github.com/jrudio/go-plex-client"
	"github.com/tidwall/gjson"
	"github.com/xanderstrike/plexhooks"
)

type PlexConfig struct {
	BaseUrl  string
	Token    string
	PlaxtUrl string
}

type PlexClient struct {
	proxy  *httputil.ReverseProxy
	client *plex.Plex

	plaxtBaseUrl string
	plaxtUrl     string

	serverIdentifier *string
	sections         map[string]plex.Directory
	sessions         map[string]sessionData

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

	var plaxtUrl string
	u, err = url.Parse(config.PlaxtUrl)
	if err == nil && strings.HasSuffix(u.Path, "/api") && u.Query().Get("id") != "" {
		plaxtUrl = u.String()
	}

	return &PlexClient{
		proxy:    proxy,
		client:   client,
		plaxtUrl: plaxtUrl,
		sections: make(map[string]plex.Directory, 0),
		sessions: make(map[string]sessionData),
	}
}

func (c *PlexClient) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.EscapedPath()
	switch {
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

	identifier := c.getServerIdentifier()
	if identifier == "" {
		return
	}
	response, _ = c.client.GetSharedServers(identifier)
	return
}

func (c *PlexClient) GetAccountInfo(token string) (user plex.UserPlexTV) {
	if c.client.Token != token {
		c.mu.Lock()
		originalToken := token
		defer func() {
			c.client.Token = originalToken
			c.mu.Unlock()
		}()
	} else {
		c.mu.RLock()
		defer c.mu.RUnlock()
	}

	c.client.Token = token
	user, _ = c.client.MyAccount()
	return
}

func (c *PlexClient) ScrobbleToPlaxt(notification plex.PlaySessionStateNotification) {
	if c.plaxtUrl == "" {
		return
	}

	lockKey := fmt.Sprintf("plex:session:%s", notification.SessionKey)
	if ml.TryLock(lockKey, time.Second) {
		defer ml.Unlock(lockKey)
	} else {
		return
	}

	serverIdentifier := c.getServerIdentifier()
	if serverIdentifier == "" {
		return
	}
	var session sessionData
	if c.getPlayerSession(notification.SessionKey) {
		session = c.sessions[notification.SessionKey]
	} else {
		return
	}
	var section plex.Directory
	sectionId := session.metadata.LibrarySectionID.String()
	if c.getLibrarySection(sectionId) {
		section = c.sections[sectionId]
	} else {
		return
	}

	var externalGuids []plexhooks.ExternalGuid
	if session.status == sessionUnplayed && strings.HasPrefix(session.metadata.GUID, "plex://") {
		metadata := c.getMetadata(notification.RatingKey)
		if metadata == nil {
			return
		}
		guidValue := gjson.Get(string(metadata), "MediaContainer.Metadata.0.Guid")
		guidResult := guidValue.Array()
		for _, guid := range guidResult {
			externalGuids = append(externalGuids, plexhooks.ExternalGuid{
				Id: guid.Get("id").String(),
			})
		}
		session.guids = externalGuids
	} else {
		externalGuids = session.guids
	}

	var event string
	progress := int(math.Round(float64(notification.ViewOffset) / float64(session.metadata.Duration) * 100.0))
	switch notification.State {
	case "playing":
		if session.status == sessionPlaying {
			event = "media.resume"
		} else {
			event = "media.play"
		}
	case "paused":
		if progress >= watchedThreshold && session.status == sessionPlaying {
			event = "media.scrobble"
		} else {
			event = "media.pause"
		}
	case "stopped":
		if progress >= watchedThreshold && session.status == sessionPlaying {
			event = "media.scrobble"
		} else {
			event = "media.stop"
		}
	}
	if event == "" {
		return
	} else if event == "media.scrobble" {
		session.status = sessionWatched
		c.sessions[notification.SessionKey] = session
		go clearCachedMetadata()
	} else if session.status == sessionUnplayed {
		session.status = sessionPlaying
		c.sessions[notification.SessionKey] = session
	}

	webhook := plexhooks.PlexResponse{
		Event: event,
		Owner: true,
		User:  false,
		Account: plexhooks.Account{
			Title: session.metadata.User.Title,
		},
		Server: plexhooks.Server{
			Uuid: serverIdentifier,
		},
		Player: plexhooks.Player{
			Uuid: session.metadata.Player.MachineIdentifier,
		},
		Metadata: plexhooks.Metadata{
			LibrarySectionType: section.Type,
			RatingKey:          session.metadata.RatingKey,
			Guid:               session.metadata.GUID,
			ExternalGuid:       externalGuids,
			Title:              session.metadata.Title,
			Year:               session.metadata.Year,
			Duration:           session.metadata.Duration,
			ViewOffset:         int(notification.ViewOffset),
		},
	}
	b, _ := json.Marshal(webhook)
	_, _ = c.client.HTTPClient.Post(c.plaxtUrl, "application/json", bytes.NewBuffer(b))
}

func (c *PlexClient) getServerIdentifier() string {
	if c.serverIdentifier == nil {
		c.mu.RLock()
		defer c.mu.RUnlock()

		identity, err := c.client.GetServerIdentity()
		if err == nil {
			c.serverIdentifier = &identity.MediaContainer.MachineIdentifier
		}
	}
	return *c.serverIdentifier
}

func (c *PlexClient) getLibrarySection(sectionKey string) (isFound bool) {
	if _, ok := c.sections[sectionKey]; ok {
		isFound = true
		return
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	sections, err := c.client.GetLibraries()
	if err != nil {
		return
	}

	c.sections = make(map[string]plex.Directory, len(sections.MediaContainer.Directory))
	for _, section := range sections.MediaContainer.Directory {
		c.sections[section.Key] = section
		if sectionKey == section.Key {
			isFound = true
		}
	}
	return
}

func (c *PlexClient) getPlayerSession(sessionKey string) (isFound bool) {
	if _, ok := c.sessions[sessionKey]; ok {
		isFound = true
		return
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	sessions, err := c.client.GetSessions()
	if err != nil {
		return
	}

	keys := make(map[string]struct{}, len(sessions.MediaContainer.Metadata))
	for _, session := range sessions.MediaContainer.Metadata {
		keys[session.SessionKey] = emptyStruct
		if _, ok := c.sessions[session.SessionKey]; !ok {
			c.sessions[session.SessionKey] = sessionData{
				metadata: session,
				status:   sessionUnplayed,
			}
		}
		if sessionKey == session.SessionKey {
			isFound = true
		}
	}
	for key := range c.sessions {
		if _, ok := keys[key]; !ok {
			delete(c.sessions, key)
		}
	}
	return
}

func (c *PlexClient) getMetadata(ratingKey string) []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()

	userId := c.GetUserId(c.client.Token)
	if userId <= 0 {
		return nil
	}

	path := fmt.Sprintf("/library/metadata/%s", ratingKey)
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s%s", c.client.URL, path), nil)
	if err != nil {
		return nil
	}
	req.Header.Set(headerToken, c.client.Token)
	req.Header.Set(headerAccept, "application/json")

	var resp *http.Response
	cacheKey := fmt.Sprintf("%s:%s?%s=%s&%s=%d", cachePrefixMetadata, path, headerAccept, "json", headerUserId, userId)
	if redisClient != nil {
		b, err := redisClient.Get(context.Background(), cacheKey).Bytes()
		if err == nil {
			reader := bufio.NewReader(bytes.NewReader(b))
			resp, _ = http.ReadResponse(reader, req)
		}
	}
	if resp == nil {
		resp, err = c.client.HTTPClient.Do(req)
		writeToCache(cacheKey, resp, cacheTtlMetadata)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	bodyBytes, _ := io.ReadAll(resp.Body)
	return bodyBytes
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
