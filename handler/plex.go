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

	"github.com/RoyXiang/plexproxy/common"
	"github.com/jrudio/go-plex-client"
	"github.com/xanderstrike/plexhooks"
)

type PlexConfig struct {
	BaseUrl          string
	Token            string
	PlaxtUrl         string
	RedirectWebApp   string
	DisableTranscode string
	NoRequestLogs    string
}

type PlexClient struct {
	proxy  *httputil.ReverseProxy
	client *plex.Plex

	plaxtUrl         string
	redirectWebApp   bool
	disableTranscode bool
	NoRequestLogs    bool

	serverIdentifier *string
	sections         map[string]plex.Directory
	sessions         map[string]sessionData
	friends          map[string]plexUser

	mu sync.RWMutex

	MulLock common.MultipleLock
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

	var redirectWebApp, disableTranscode, noRequestLogs bool
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
	if b, err := strconv.ParseBool(config.NoRequestLogs); err == nil {
		noRequestLogs = b
	} else {
		noRequestLogs = false
	}

	return &PlexClient{
		proxy:            proxy,
		client:           client,
		plaxtUrl:         plaxtUrl,
		redirectWebApp:   redirectWebApp,
		disableTranscode: disableTranscode,
		NoRequestLogs:    noRequestLogs,
		sections:         make(map[string]plex.Directory, 0),
		sessions:         make(map[string]sessionData),
		friends:          make(map[string]plexUser),
		MulLock:          common.NewMultipleLock(),
	}
}

func (u *plexUser) MarshalBinary() ([]byte, error) {
	return json.Marshal(u)
}

func (u *plexUser) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, u)
}

func (a sessionData) Check(b sessionData) (bool, bool) {
	if a.status != b.status {
		return true, true
	}
	if a.progress != b.progress {
		if a.status == sessionPlaying {
			return true, false
		}
		return true, true
	}
	if a.lastEvent != b.lastEvent {
		return true, false
	}
	if len(a.guids) != len(b.guids) {
		return true, false
	}
	return false, false
}

func (c *PlexClient) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.EscapedPath()
	if c.redirectWebApp && strings.HasPrefix(path, "/web/") && r.Method == http.MethodGet {
		http.Redirect(w, r, "https://app.plex.tv/desktop", http.StatusFound)
		return
	}

	if token := r.Header.Get(headerToken); token != "" {
		// If it is an authorized request
		if user := c.GetUser(token); user != nil {
			switch path {
			case "/:/scrobble", "/:/unscrobble":
				ratingKey := r.URL.Query().Get("key")
				if ratingKey != "" {
					go clearCachedMetadata(ratingKey, user.Id)
				}
			case "/:/timeline":
				go c.syncTimelineWithPlaxt(r, user)
			case "/video/:/transcode/universal/decision":
				if c.disableTranscode {
					r = c.disableTranscoding(r)
				}
			}
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

func (c *PlexClient) GetUser(token string) *plexUser {
	if realUser, ok := c.friends[token]; ok {
		return &realUser
	}

	var err error
	ctx := context.Background()
	cacheKey := fmt.Sprintf("%s:token:%s", cachePrefixPlex, token)

	isCacheEnabled := redisClient != nil
	if isCacheEnabled {
		var result plexUser
		err = redisClient.Get(ctx, cacheKey).Scan(&result)
		if err == nil {
			c.friends[token] = result
			return &result
		}
	}

	c.MulLock.Lock(lockKeyFriends)
	defer c.MulLock.Unlock(lockKeyFriends)

	response := c.GetSharedServers()
	if response == nil {
		return nil
	}
	var user *plexUser
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
		return user
	}

	info := c.GetAccountInfo(token)
	if info.ID > 0 {
		realUser := c.friends[token]
		user = &realUser
		if isCacheEnabled {
			redisClient.Set(ctx, cacheKey, user, 0)
		}
	}
	return user
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
		common.GetLogger().Printf("Failed to get friend list: %s", err.Error())
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

func (c *PlexClient) syncTimelineWithPlaxt(r *http.Request, user *plexUser) {
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
	c.MulLock.Lock(lockKey)
	defer c.MulLock.Unlock(lockKey)

	session := c.sessions[sessionKey]
	if session.status == sessionWatched {
		return
	}
	progress := int(math.Round(float64(viewOffset) / float64(session.metadata.Duration) * 100.0))
	if progress == 0 {
		if session.progress >= watchedThreshold {
			// time would become 0 once a playback session was finished
			progress = 100
			viewOffset = session.metadata.Duration
		} else if session.status != sessionUnplayed {
			return
		}
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
	var threshold int
	switch state {
	case "playing":
		threshold = 100
		if session.status == sessionUnplayed || session.status == sessionStopped {
			event = webhookEventPlay
		} else {
			event = webhookEventResume
		}
	case "paused":
		threshold = watchedThreshold
		event = webhookEventPause
	case "stopped":
		threshold = watchedThreshold
		event = webhookEventStop
	}
	if event == "" {
		return
	} else if progress >= threshold {
		event = webhookEventScrobble
	}
	switch event {
	case webhookEventPlay, webhookEventResume:
		session.status = sessionPlaying
	case webhookEventPause:
		session.status = sessionPaused
	case webhookEventStop, webhookEventScrobble:
		session.status = sessionStopped
		go clearCachedMetadata(ratingKey, user.Id)
	}
	session.lastEvent = event
	session.progress = progress
	shouldUpdate, shouldScrobble := session.Check(c.sessions[sessionKey])
	if shouldUpdate {
		defer func() {
			c.sessions[sessionKey] = session
		}()
	}
	if !shouldScrobble {
		return
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
	resp, err := c.client.HTTPClient.Post(c.plaxtUrl, "application/json", bytes.NewBuffer(b))
	if err != nil {
		return
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	if event == webhookEventScrobble && resp.StatusCode == http.StatusOK {
		session.status = sessionWatched
	}
}

func (c *PlexClient) getServerIdentifier() string {
	if c.serverIdentifier == nil {
		c.mu.RLock()
		defer c.mu.RUnlock()

		identity, err := c.client.GetServerIdentity()
		if err != nil {
			common.GetLogger().Printf("Failed to get server identifier: %s", err.Error())
			return ""
		}
		c.serverIdentifier = &identity.MediaContainer.MachineIdentifier
	}
	return *c.serverIdentifier
}

func (c *PlexClient) getLibrarySection(sectionKey string) (isFound bool) {
	if _, ok := c.sections[sectionKey]; ok {
		isFound = true
		return
	}

	c.MulLock.Lock(lockKeySections)
	c.mu.RLock()
	defer func() {
		c.mu.RUnlock()
		c.MulLock.Unlock(lockKeySections)
	}()

	sections, err := c.client.GetLibraries()
	if err != nil {
		common.GetLogger().Printf("Failed to get library sections: %s", err.Error())
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

	c.MulLock.Lock(lockKeySessions)
	c.mu.RLock()
	defer func() {
		c.mu.RUnlock()
		c.MulLock.Unlock(lockKeySessions)
	}()

	sessions, err := c.client.GetSessions()
	if err != nil {
		common.GetLogger().Printf("Failed to get playback sessions: %s", err.Error())
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
			common.GetLogger().Printf("Failed to parse metadata of item %s: %s", ratingKey, err.Error())
			return nil
		}
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		common.GetLogger().Printf("Failed to get metadata of item %s (status: %d)", ratingKey, resp.StatusCode)
		return nil
	} else if !isFromCache {
		writeToCache(cacheKey, resp, cacheTtlMetadata)
	}

	var result plex.MediaMetadata
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		common.GetLogger().Printf("Failed to parse metadata of item %s: %s", ratingKey, err.Error())
		return nil
	}
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
