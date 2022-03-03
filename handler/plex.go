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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/RoyXiang/plexproxy/common"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jrudio/go-plex-client"
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
	friends          map[string]plexUser

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

	if w.(middleware.WrapResponseWriter).Status() == http.StatusOK {
		query := r.URL.Query()
		switch path {
		case "/:/scrobble", "/:/unscrobble":
			go clearCachedMetadata(query.Get("key"), r.Header.Get(headerToken))
		case "/:/timeline":
			go c.syncTimelineWithPlaxt(r)
		}
	}
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

	isCacheEnabled := redisClient != nil
	if isCacheEnabled {
		id, err = redisClient.Get(ctx, cacheKey).Int()
		if err == nil {
			return id
		}
	}

	// refresh the list of friends
	c.findFriend("")
	for _, friend := range c.friends {
		if friend.Token == token {
			if id64, err := friend.Id.Int64(); err == nil {
				id = int(id64)
				if !isCacheEnabled {
					break
				}
			}
		}
		if isCacheEnabled {
			key := fmt.Sprintf("%s:token:%s", cachePrefixPlex, friend.Token)
			redisClient.Set(ctx, key, friend.Id, 0)
		}
	}
	if id > 0 {
		return
	}

	user := c.GetAccountInfo(token)
	if user.ID > 0 {
		id = user.ID
		if isCacheEnabled {
			redisClient.Set(ctx, cacheKey, user.ID, 0)
		}
	}
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

func (c *PlexClient) findFriend(id string) (isFound bool) {
	if id != "" {
		for _, friend := range c.friends {
			if friend.Id.String() == id {
				isFound = true
				return
			}
		}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	identifier := c.getServerIdentifier()
	if identifier == "" {
		return
	}
	response, err := c.client.GetSharedServers(identifier)
	if err != nil {
		return
	}

	c.friends = make(map[string]plexUser, len(response.Friends))
	for _, friend := range response.Friends {
		userId := strconv.Itoa(friend.UserId)
		c.friends[userId] = plexUser{
			Id:       json.Number(userId),
			Username: friend.Username,
			Token:    friend.AccessToken,
		}
		if userId == id {
			isFound = true
		}
	}
	return
}

func (c *PlexClient) syncTimelineWithPlaxt(r *http.Request) {
	if c.plaxtUrl == "" || !c.IsTokenSet() {
		return
	}

	clientUuid := r.Header.Get(headerClientIdentity)
	ratingKey := r.URL.Query().Get("ratingKey")
	playbackTime := r.URL.Query().Get("time")
	state := r.URL.Query().Get("state")
	if clientUuid == "" || ratingKey == "" || playbackTime == "" || state == "" {
		return
	}

	var viewOffset int
	var err error
	if viewOffset, err = strconv.Atoi(playbackTime); err != nil {
		return
	}

	sessionKey := c.getPlayerSession(clientUuid, ratingKey)
	if sessionKey == "" {
		return
	}
	lockKey := fmt.Sprintf("plex:session:%s", sessionKey)
	if ml.TryLock(lockKey, time.Second) {
		defer ml.Unlock(lockKey)
	} else {
		return
	}
	session := c.sessions[sessionKey]
	sessionChanged := false

	serverIdentifier := c.getServerIdentifier()
	if serverIdentifier == "" {
		return
	}
	var section plex.Directory
	sectionId := session.metadata.LibrarySectionID.String()
	if c.getLibrarySection(sectionId) {
		section = c.sections[sectionId]
		if section.Type != "show" && section.Type != "movie" {
			return
		}
	} else {
		return
	}

	externalGuids := make([]plexhooks.ExternalGuid, 0)
	if session.guids == nil {
		metadata := c.getMetadata(ratingKey)
		if metadata == nil {
			return
		} else if metadata.MediaContainer.Metadata[0].OriginalTitle != "" {
			session.metadata.Title = metadata.MediaContainer.Metadata[0].OriginalTitle
		}
		for _, guid := range metadata.MediaContainer.Metadata[0].AltGUIDs {
			externalGuids = append(externalGuids, plexhooks.ExternalGuid{
				Id: guid.ID,
			})
		}
		session.guids = externalGuids
		sessionChanged = true
	} else {
		externalGuids = session.guids
	}

	var event string
	progress := int(math.Round(float64(viewOffset) / float64(session.metadata.Duration) * 100.0))
	switch state {
	case "playing":
		if session.status == sessionPlaying {
			if progress >= 100 {
				event = webhookEventScrobble
			} else {
				event = webhookEventResume
			}
		} else {
			event = webhookEventPlay
		}
	case "paused":
		if progress >= watchedThreshold && session.status == sessionPlaying {
			event = webhookEventScrobble
		} else {
			event = webhookEventPause
		}
	case "stopped":
		if progress >= watchedThreshold && session.status == sessionPlaying {
			event = webhookEventScrobble
		} else {
			event = webhookEventStop
		}
	}
	if event == "" || session.status == sessionWatched {
		return
	} else if event == webhookEventScrobble {
		session.status = sessionWatched
		sessionChanged = true
		go clearCachedMetadata(ratingKey, r.Header.Get(headerToken))
	} else if event == webhookEventStop {
		go clearCachedMetadata(ratingKey, r.Header.Get(headerToken))
	} else if session.status == sessionUnplayed {
		session.status = sessionPlaying
		sessionChanged = true
	}
	if sessionChanged || event != session.lastEvent {
		session.lastEvent = event
		c.sessions[sessionKey] = session
	} else {
		return
	}

	userId := session.metadata.User.ID
	username := session.metadata.User.Title
	if c.findFriend(userId) {
		username = c.friends[userId].Username
	}

	webhook := plexhooks.PlexResponse{
		Event: event,
		Owner: true,
		User:  false,
		Account: plexhooks.Account{
			Title: username,
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
			ViewOffset:         viewOffset,
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

func (c *PlexClient) getPlayerSession(playerIdentifier, ratingKey string) (sessionKey string) {
	for key, data := range c.sessions {
		if data.metadata.Player.MachineIdentifier == playerIdentifier && data.metadata.RatingKey == ratingKey {
			sessionKey = key
			return
		}
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
				guids:    nil,
				status:   sessionUnplayed,
			}
		}
		if session.Player.MachineIdentifier == playerIdentifier && session.RatingKey == ratingKey {
			sessionKey = session.SessionKey
		}
	}
	for key := range c.sessions {
		if _, ok := keys[key]; !ok {
			delete(c.sessions, key)
		}
	}
	return
}

func (c *PlexClient) getMetadata(ratingKey string) *plex.MediaMetadata {
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
	isFromCache := false
	if redisClient != nil {
		b, err := redisClient.Get(context.Background(), cacheKey).Bytes()
		if err == nil {
			reader := bufio.NewReader(bytes.NewReader(b))
			resp, _ = http.ReadResponse(reader, req)
			isFromCache = true
		}
	}
	if resp == nil {
		resp, err = c.client.HTTPClient.Do(req)
		if err != nil {
			return nil
		}
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil
	} else if !isFromCache {
		writeToCache(cacheKey, resp, cacheTtlMetadata)
	}

	var result plex.MediaMetadata
	_ = json.NewDecoder(resp.Body).Decode(&result)
	return &result
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
