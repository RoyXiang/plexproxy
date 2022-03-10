package handler

import (
	"os"
	"sync"
	"time"

	"github.com/RoyXiang/plexproxy/common"
	"github.com/gorilla/websocket"
	"github.com/jrudio/go-plex-client"
)

func ListenToWebsocket(interrupt <-chan os.Signal) {
	if !plexClient.IsTokenSet() {
		<-interrupt
		return
	}
	events := plex.NewNotificationEvents()
	events.OnActivity(wsOnActivity)

	var wsWaitGroup sync.WaitGroup
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
			if !plexClient.TestReachability() {
				time.Sleep(time.Second)
				reconnect <- struct{}{}
				break
			}

			wsWaitGroup.Add(1)
			plexClient.SubscribeToNotifications(events, closeWebsocket, func(err error) {
				switch err.(type) {
				case *websocket.CloseError:
					wsWaitGroup.Done()
				}
			})
			logger.Println("Receiving notifications from Plex server through websocket...")
			go func() {
				wsWaitGroup.Wait()
				logger.Println("Websocket closed unexpectedly, reconnecting...")
				time.Sleep(time.Second)
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
	if isMetadataChanged {
		clearCachedMetadata("", 0)
	}
}
