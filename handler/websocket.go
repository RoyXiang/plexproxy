package handler

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/RoyXiang/plexproxy/common"
	"github.com/gorilla/websocket"
	"github.com/jrudio/go-plex-client"
)

func ListenToWebsocket(interrupt <-chan os.Signal) {
	if plexToken == "" {
		<-interrupt
		return
	}
	plexClient, err := plex.New(plexBaseUrl, plexToken)
	if err != nil {
		<-interrupt
		return
	}
	events := plex.NewNotificationEvents()
	events.OnActivity(wsOnActivity)

	var wsWaitGroup sync.WaitGroup
	var isReadClosed, isWriteClosed bool
	logger := common.GetLogger()
	closeWebsocket := make(chan os.Signal, 1)
	reconnect := make(chan struct{}, 1)
	reconnect <- struct{}{}
	logger.Println("Connecting to Plex server through websocket...")

socket:
	for {
		select {
		case <-reconnect:
			// wait for Plex server until it is online
			_, err = plexClient.GetLibraries()
			if err != nil {
				time.Sleep(time.Second)
				reconnect <- struct{}{}
				break
			}

			wsWaitGroup.Add(2)
			plexClient.SubscribeToNotifications(events, closeWebsocket, func(err error) {
				switch err.(type) {
				case *websocket.CloseError:
					if !isReadClosed {
						wsWaitGroup.Done()
						isReadClosed = true
					}
				}
				switch err.Error() {
				case "use of closed network connection":
					if !isWriteClosed {
						closeWebsocket <- os.Interrupt
						wsWaitGroup.Done()
						isWriteClosed = true
					}
				}
			})
			logger.Println("Receiving notifications from Plex server through websocket...")
			go func() {
				wsWaitGroup.Wait()
				logger.Println("Websocket closed unexpectedly, reconnecting...")
				time.Sleep(time.Second)
				isReadClosed, isWriteClosed = false, false
				reconnect <- struct{}{}
			}()
		case signal := <-interrupt:
			closeWebsocket <- signal
			break socket
		}
	}
}

func wsOnActivity(n plex.NotificationContainer) {
	isMetadataChanged := false
	for _, a := range n.ActivityNotification {
		if a.Event != "ended" {
			continue
		}
		switch a.Activity.Type {
		case "library.update.section", "library.refresh.items", "media.generate.intros":
			isMetadataChanged = true
		}
	}
	if isMetadataChanged && redisClient != nil {
		mu.Lock()
		defer mu.Unlock()

		ctx := context.Background()
		keys := redisClient.Keys(ctx, fmt.Sprintf("%s:*", cachePrefixMetadata)).Val()
		if len(keys) > 0 {
			redisClient.Del(ctx, keys...).Val()
		}
	}
}
