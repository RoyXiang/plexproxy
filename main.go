package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/RoyXiang/plexproxy/handler"
	"github.com/gorilla/mux"
)

func newRouter() http.Handler {
	r := mux.NewRouter()

	refreshRouter := r.Methods(http.MethodPut).Subrouter()
	refreshRouter.Use(handler.RefreshMiddleware)
	refreshRouter.Path("/library/sections/{id}/refresh").HandlerFunc(handler.Handler)

	metadataRouter := r.Methods(http.MethodGet).Subrouter()
	metadataRouter.Use(handler.TrafficMiddleware)
	metadataRouter.Use(handler.CacheMiddleware)
	metadataRouter.PathPrefix("/library/collections/").HandlerFunc(handler.Handler)
	metadataRouter.PathPrefix("/library/metadata/").HandlerFunc(handler.Handler)
	metadataRouter.PathPrefix("/library/sections/").HandlerFunc(handler.Handler)

	getRouter := r.Methods(http.MethodGet).Subrouter()
	getRouter.Use(handler.TrafficMiddleware)
	getRouter.PathPrefix("/").HandlerFunc(handler.Handler)

	r.PathPrefix("/").HandlerFunc(handler.Handler)
	return r
}

func main() {
	srv := &http.Server{
		Addr:    "0.0.0.0:5000",
		Handler: newRouter(),
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
