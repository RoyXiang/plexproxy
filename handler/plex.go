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
	BaseUrl          string
	Token            string
	PlaxtUrl         string
	RedirectWebApp   string
	DisableTranscode string
}

type PlexClient struct {
	proxy  *httputil.ReverseProxy
	client *plex.Plex

	plaxtUrl         string
	redirectWebApp   bool
	disableTranscode bool

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

	var redirectWebApp, disableTranscode bool
	if b, err := strconv.ParseBool(config.RedirectWebApp); err == nil {
		redirectWebApp = b
	} else {
		redirectWebApp = true
	}
	if b, err := strconv.ParseBool(config.DisableTranscode); err == nil {
		disableTranscode = b
	} else {
		disableTranscode = true
	}

	return &PlexClient{
		proxy:            proxy,
		client:           client,
		plaxtUrl:         plaxtUrl,
		redirectWebApp:   redirectWebApp,
		disableTranscode: disableTranscode,
		sections:         make(map[string]plex.Directory, 0),
		sessions:         make(map[string]sessionData),
		friends:          make(map[string]plexUser),
	}
}

func (u *plexUser) MarshalBinary() ([]byte, error) {
	return json.Marshal(u)
}

func (u *plexUser) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, u)
}

func (a sessionData) Equal(b sessionData) bool {
	if a.progress != b.progress {
		return false
	}
	if a.lastEvent != b.lastEvent {
		return false
	}
	if a.status != b.status {
		return false
	}
	if len(a.guids) != len(b.guids) {
		return false
	}
	return true
}

func (c *PlexClient) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.EscapedPath()
	switch {
	case path == "/video/:/transcode/universal/decision":
		if c.disableTranscode {
			r = c.disableTranscoding(r)
		}
	case strings.HasPrefix(path, "/web/"):
		if c.redirectWebApp && r.Method == http.MethodGet {
			http.Redirect(w, r, "https://app.plex.tv/desktop", http.StatusFound)
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

func (c *PlexClient) GetUser(token string) (user *plexUser) {
	if realUser, ok := c.friends[token]; ok {
		user = &realUser
		return
	}

	var err error
	ctx := context.Background()
	cacheKey := fmt.Sprintf("%s:token:%s", cachePrefixPlex, token)

	isCacheEnabled := redisClient != nil
	if isCacheEnabled {
		err = redisClient.Get(ctx, cacheKey).Scan(user)
		if err == nil {
			c.friends[token] = *user
			return
		}
	}

	response := c.GetSharedServers()
	if response == nil {
		return
	}
	for _, friend := range response.Friends {
		realUser := plexUser{
			Id:       friend.UserId,
			Username: friend.Username,
		}
		if friend.AccessToken == token {
			user = &realUser
		}
		c.friends[friend.AccessToken] = realUser
		if isCacheEnabled {
			key := fmt.Sprintf("%s:token:%s", cachePrefixPlex, friend.AccessToken)
			redisClient.Set(ctx, key, &realUser, 0)
		}
	}
	if user != nil {
		return
	}

	info := c.GetAccountInfo(token)
	if info.ID > 0 {
		realUser := c.friends[token]
		user = &realUser
		if isCacheEnabled {
			redisClient.Set(ctx, cacheKey, user, 0)
		}
	}
	return
}

func (c *PlexClient) GetSharedServers() *plex.SharedServersResponse {
	c.mu.RLock()
	defer c.mu.RUnlock()

	identifier := c.getServerIdentifier()
	if identifier == "" {
		return nil
	}
	response, err := c.client.GetSharedServers(identifier)
	if err != nil {
		return nil
	}
	return &response
}

func (c *PlexClient) GetAccountInfo(token string) (user plex.UserPlexTV) {
	c.mu.Lock()
	originalToken := token
	defer func() {
		c.client.Token = originalToken
		c.mu.Unlock()
	}()

	var err error
	c.client.Token = token
	user, err = c.client.MyAccount()
	if err == nil {
		c.friends[token] = plexUser{
			Id:       user.ID,
			Username: user.Username,
		}
	}
	return
}

func (c *PlexClient) syncTimelineWithPlaxt(r *http.Request) {
	if c.plaxtUrl == "" || !c.IsTokenSet() {
		return
	}

	token := r.Header.Get(headerToken)
	clientUuid := r.Header.Get(headerClientIdentity)
	ratingKey := r.URL.Query().Get("ratingKey")
	playbackTime := r.URL.Query().Get("time")
	state := r.URL.Query().Get("state")
	if token == "" || clientUuid == "" || ratingKey == "" || playbackTime == "" || state == "" {
		return
	}

	user := c.GetUser(token)
	if user == nil {
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

	progress := int(math.Round(float64(viewOffset) / float64(session.metadata.Duration) * 100.0))
	if progress == 0 {
		if session.progress >= watchedThreshold {
			// time would become 0 once a playback session was finished
			progress = 100
		} else if session.status != sessionUnplayed {
			return
		}
	}

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
	} else {
		externalGuids = session.guids
	}

	var event string
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
		go clearCachedMetadata(ratingKey, token)
	} else if event == webhookEventStop {
		go clearCachedMetadata(ratingKey, token)
	} else if session.status == sessionUnplayed {
		session.status = sessionPlaying
	}

	session.lastEvent = event
	session.progress = progress
	if !session.Equal(c.sessions[sessionKey]) {
		c.sessions[sessionKey] = session
	} else {
		return
	}

	webhook := plexhooks.PlexResponse{
		Event: event,
		Owner: true,
		User:  false,
		Account: plexhooks.Account{
			Id:    user.Id,
			Title: user.Username,
			Thumb: session.metadata.User.Thumb,
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

	user := c.GetUser(c.client.Token)
	if user == nil {
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
	cacheKey := fmt.Sprintf("%s:%s?%s=%s&%s=%d", cachePrefixMetadata, path, headerAccept, "json", headerUserId, user.Id)
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
