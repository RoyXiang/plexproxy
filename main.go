package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/RoyXiang/plexproxy/common"
	"github.com/RoyXiang/plexproxy/handler"
)

func main() {
	srv := &http.Server{
		Addr:    "0.0.0.0:5000",
		Handler: handler.NewRouter(),
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			common.GetLogger().Println(err)
		}
	}()
	common.GetLogger().Println("Server started on :5000")

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-c

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	_ = srv.Shutdown(ctx)

	common.GetLogger().Println("Shutting down...")
	os.Exit(0)
}
