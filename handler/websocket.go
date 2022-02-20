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
	events.OnActivity(func(n plex.NotificationContainer) {
		isUpdateEnded := false
		for _, a := range n.ActivityNotification {
			if a.Event == "ended" && a.Activity.Type == "library.update.section" {
				isUpdateEnded = true
				break
			}
		}
		if isUpdateEnded {
			mu.Lock()
			defer mu.Unlock()

			ctx := context.Background()
			keys := redisClient.Keys(ctx, fmt.Sprintf("%s:*", cachePrefixMetadata)).Val()
			if len(keys) > 0 {
				redisClient.Del(ctx, keys...).Val()
			}
		}
	})

	var wsWaitGroup sync.WaitGroup
	var isReadClosed, isWriteClosed bool
	logger := common.GetLogger()
	closeWebsocket := make(chan os.Signal, 1)
	reconnect := make(chan struct{}, 1)
	reconnect <- struct{}{}

socket:
	for {
		select {
		case <-reconnect:
			logger.Println("Connecting to Plex server through websocket...")
			for {
				// wait for Plex server until it is online
				_, err = plexClient.GetLibraries()
				if err == nil {
					break
				}
				time.Sleep(time.Second)
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
			go func() {
				wsWaitGroup.Wait()
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
