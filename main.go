package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/RoyXiang/plexproxy/handler"
)

func main() {
	srv := &http.Server{
		Addr:    "0.0.0.0:5000",
		Handler: handler.NewRouter(),
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()
	log.Println("Server started on :5000")

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	<-c

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	_ = srv.Shutdown(ctx)

	log.Println("Shutting down...")
	os.Exit(0)
}
