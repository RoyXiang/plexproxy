package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

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
	sections         map[string]*plex.Directory
	sessions         map[string]*sessionData
	users            map[string]*plexUser

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
		sections:         make(map[string]*plex.Directory, 0),
		sessions:         make(map[string]*sessionData),
		users:            make(map[string]*plexUser),
		MulLock:          common.NewMultipleLock(),
	}
}

func (u *plexUser) MarshalBinary() ([]byte, error) {
	return json.Marshal(u)
}

func (u *plexUser) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, u)
}

func (a sessionData) Check(b sessionData) bool {
	if a.status != b.status {
		return true
	}
	if a.progress != b.progress {
		if a.status == sessionPlaying {
			return false
		}
		return true
	}
	return false
}

func (c *PlexClient) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.EscapedPath()
	if c.redirectWebApp && strings.HasPrefix(path, "/web/") && r.Method == http.MethodGet {
		http.Redirect(w, r, "https://app.plex.tv/desktop", http.StatusFound)
		return
	}

	// If it is an authorized request
	if user := r.Context().Value(userCtxKey); user != nil {
		switch path {
		case "/:/timeline":
			go c.syncTimelineWithPlaxt(r, user.(*plexUser))
		case "/video/:/transcode/universal/decision":
			if c.disableTranscode {
				r = c.disableTranscoding(r)
			}
		}
	}

	c.proxy.ServeHTTP(w, r)
}

func (c *PlexClient) IsTokenSet() bool {
	c.MulLock.RLock(lockKeyToken)
	defer c.MulLock.RUnlock(lockKeyToken)

	return c.client.Token != ""
}

func (c *PlexClient) GetUser(token string) (user *plexUser) {
	if user = c.searchUser(token); user != nil {
		return
	}
	c.fetchUsers(token)
	user = c.searchUser(token)
	return
}

func (c *PlexClient) searchUser(token string) *plexUser {
	c.MulLock.RLock(lockKeyUsers)
	defer c.MulLock.RUnlock(lockKeyUsers)

	if user, ok := c.users[token]; ok {
		return user
	}
	return nil
}

func (c *PlexClient) fetchUsers(token string) {
	c.MulLock.Lock(lockKeyUsers)
	defer c.MulLock.Unlock(lockKeyUsers)

	var user plexUser
	ctx := context.Background()
	cacheKey := fmt.Sprintf("%s:token:%s", cachePrefixPlex, token)
	isCacheEnabled := redisClient != nil

	if isCacheEnabled {
		err := redisClient.Get(ctx, cacheKey).Scan(&user)
		if err == nil {
			c.users[token] = &user
			return
		}
	}

	userInfo := c.GetAccountInfo(token)
	if userInfo.ID > 0 {
		user = plexUser{
			Id:       userInfo.ID,
			Username: userInfo.Username,
		}
		if isCacheEnabled {
			redisClient.Set(ctx, cacheKey, &user, 0).Val()
		}
		c.users[token] = &user
		return
	}

	response := c.GetSharedServers()
	if response != nil {
		isFriend := false
		for _, friend := range response.Friends {
			if friend.AccessToken == token {
				isFriend = true
			}
			user = plexUser{
				Id:       friend.UserId,
				Username: friend.Username,
			}
			if isCacheEnabled {
				redisClient.Set(ctx, cacheKey, &user, 0).Val()
			}
			c.users[token] = &user
		}
		if isFriend {
			return
		}
	}
}

func (c *PlexClient) GetSharedServers() *plex.SharedServersResponse {
	if !c.IsTokenSet() {
		return nil
	}

	c.MulLock.RLock(lockKeyToken)
	defer c.MulLock.RUnlock(lockKeyToken)

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
	c.MulLock.Lock(lockKeyToken)
	originalToken := c.client.Token
	defer func() {
		c.client.Token = originalToken
		c.MulLock.Unlock(lockKeyToken)
	}()

	c.client.Token = token
	user, _ = c.client.MyAccount()
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

	session := c.getPlayerSession(clientUuid, ratingKey)
	if session == nil || session.status == sessionWatched {
		return
	}
	lockKey := fmt.Sprintf("plex:session:%d:%s:%s", user.Id, clientUuid, ratingKey)
	c.MulLock.Lock(lockKey)
	defer c.MulLock.Unlock(lockKey)

	originalSession := *session
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
	}
	session.lastEvent = event
	session.progress = progress
	shouldScrobble := session.Check(originalSession)
	if !shouldScrobble {
		return
	}

	serverIdentifier := c.getServerIdentifier()
	if serverIdentifier == "" {
		return
	}
	var section *plex.Directory
	sectionId := session.metadata.LibrarySectionID.String()
	if section = c.getLibrarySection(sectionId); session == nil || (section.Type != "show" && section.Type != "movie") {
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
	resp, err := c.client.DownloadClient.Post(c.plaxtUrl, "application/json", bytes.NewBuffer(b))
	if err != nil {
		common.GetLogger().Printf("Failed on sending webhook to Plaxt: %s", err.Error())
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
		identity, err := c.client.GetServerIdentity()
		if err != nil {
			common.GetLogger().Printf("Failed to get server identifier: %s", err.Error())
			return ""
		}
		c.serverIdentifier = &identity.MediaContainer.MachineIdentifier
	}
	return *c.serverIdentifier
}

func (c *PlexClient) getLibrarySection(sectionKey string) (section *plex.Directory) {
	if section = c.searchLibrarySection(sectionKey); section != nil {
		return
	}
	c.fetchLibrarySections()
	section = c.searchLibrarySection(sectionKey)
	return
}

func (c *PlexClient) searchLibrarySection(sectionKey string) *plex.Directory {
	c.MulLock.RLock(lockKeySections)
	defer c.MulLock.RUnlock(lockKeySections)

	if section, ok := c.sections[sectionKey]; ok {
		return section
	}
	return nil
}

func (c *PlexClient) fetchLibrarySections() {
	c.MulLock.Lock(lockKeySections)
	c.MulLock.RLock(lockKeyToken)
	defer func() {
		c.MulLock.RUnlock(lockKeyToken)
		c.MulLock.Unlock(lockKeySections)
	}()

	sections, err := c.client.GetLibraries()
	if err != nil {
		common.GetLogger().Printf("Failed to fetch library sections: %s", err.Error())
		return
	}

	c.sections = make(map[string]*plex.Directory, len(sections.MediaContainer.Directory))
	for _, section := range sections.MediaContainer.Directory {
		c.sections[section.Key] = &section
	}
}

func (c *PlexClient) getPlayerSession(playerIdentifier, ratingKey string) (session *sessionData) {
	if session = c.searchPlayerSession(playerIdentifier, ratingKey); session != nil {
		return session
	}
	c.fetchPlayerSessions()
	session = c.searchPlayerSession(playerIdentifier, ratingKey)
	return
}

func (c *PlexClient) searchPlayerSession(playerIdentifier, ratingKey string) *sessionData {
	c.MulLock.RLock(lockKeySessions)
	c.MulLock.RUnlock(lockKeySessions)

	key := playerIdentifier + "-" + ratingKey
	if session, ok := c.sessions[key]; ok {
		return session
	}
	return nil
}

func (c *PlexClient) fetchPlayerSessions() {
	c.MulLock.Lock(lockKeySessions)
	c.MulLock.RLock(lockKeyToken)
	defer func() {
		c.MulLock.RUnlock(lockKeyToken)
		c.MulLock.Unlock(lockKeySessions)
	}()

	sessions, err := c.client.GetSessions()
	if err != nil {
		common.GetLogger().Printf("Failed to fetch playback sessions: %s", err.Error())
		return
	}

	keys := make(map[string]struct{}, len(sessions.MediaContainer.Metadata))
	for _, session := range sessions.MediaContainer.Metadata {
		key := session.Player.MachineIdentifier + "-" + session.RatingKey
		keys[key] = emptyStruct
		if _, ok := c.sessions[key]; !ok {
			c.sessions[key] = &sessionData{
				metadata: session,
				guids:    nil,
				status:   sessionUnplayed,
			}
		}
	}
	for key := range c.sessions {
		if _, ok := keys[key]; !ok {
			delete(c.sessions, key)
		}
	}
}

func (c *PlexClient) getMetadata(ratingKey string) *plex.MediaMetadata {
	c.MulLock.RLock(lockKeyToken)
	defer c.MulLock.RUnlock(lockKeyToken)

	metadata, err := c.client.GetMetadata(ratingKey)
	if err != nil {
		common.GetLogger().Printf("Failed to parse metadata of item %s: %s", ratingKey, err.Error())
		return nil
	}
	return &metadata
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
