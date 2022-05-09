package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/hillview.tv/videoAPI/env"
	"github.com/hillview.tv/videoAPI/middleware"
	"github.com/hillview.tv/videoAPI/routers"
)

func main() {
	primary := mux.NewRouter()

	// Healthcheck Endpoint

	primary.HandleFunc("/healthcheck", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodGet)

	// Define the API Endpoints

	r := primary.PathPrefix("/video/v1.1").Subrouter()

	// Logging of requests
	r.Use(middleware.LoggingMiddleware)

	// Adding response headers
	r.Use(middleware.MuxHeaderMiddleware)

	// Track & Update Last Active
	r.Use(middleware.TokenHandlers)

	// List Queries

	list := r.PathPrefix("/list").Subrouter()

	list.HandleFunc("/videos", routers.HandleVideoLists).Methods(http.MethodGet)
	list.HandleFunc("/playlists", routers.HandlePlaylistLists).Methods(http.MethodGet)

	// Read Queries

	read := r.PathPrefix("/read").Subrouter()

	read.HandleFunc("/videoByID/{id}", routers.HandleVideoRead).Methods(http.MethodGet)
	read.HandleFunc("/playlist", routers.HandlePlaylistRead).Methods(http.MethodGet)

	// Upload Queries

	upload := r.PathPrefix("/upload").Subrouter()

	upload.Handle("/video", middleware.AccessTokenMiddleware(http.HandlerFunc(routers.HandleVideoUpload))).Methods(http.MethodPost)
	upload.Handle("/thumbnail", middleware.AccessTokenMiddleware(http.HandlerFunc(routers.HandleThumbnailUpload))).Methods(http.MethodPost)

	// Create Queries

	create := r.PathPrefix("/create").Subrouter()

	create.HandleFunc("/video", routers.HandleVideoCreate).Methods(http.MethodPost)

	// Launch API Listener
	fmt.Printf("✅ Hillview Video Provider API running on port %s\n", env.Port)

	headersOk := handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Origin", "Authorization", "Accept", "X-CSRF-Token"})
	originsOk := handlers.AllowedOrigins([]string{"*"})
	methodsOk := handlers.AllowedMethods([]string{"GET", "HEAD", "POST", "PUT", "OPTIONS"})
	log.Fatal(http.ListenAndServe(":"+env.Port, handlers.CORS(originsOk, headersOk, methodsOk)(primary)))
}
