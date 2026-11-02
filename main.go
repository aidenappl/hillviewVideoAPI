package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/hillview.tv/videoAPI/background"
	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/env"
	"github.com/hillview.tv/videoAPI/middleware"
	"github.com/hillview.tv/videoAPI/routers"
	"github.com/hillview.tv/videoAPI/telemetry"
)

func FilterDir(dir string) (*[]string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	res := &[]string{}
	for _, f := range files {
		if !f.IsDir() && strings.HasPrefix(f.Name(), "multipart-") {
			*res = append(*res, f.Name())
			os.Remove(filepath.Join(dir, f.Name()))
		}
	}

	if len(*res) == 0 {
		return nil, nil
	}
	return res, nil
}

func main() {
	telemetry.Init(telemetry.Config{
		Service:   "hillview-video-api",
		IngestURL: env.MonitorIngestURL,
		APIKey:    env.MonitorAPIKey,
		Env:       env.MonitorEnv,
		SpoolDir:  env.MonitorSpoolDir,
		Stdout:    env.MonitorStdout,
	})

	primary := mux.NewRouter()

	// Healthcheck Endpoint
	primary.HandleFunc("/healthcheck", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodGet)

	// Ping DB
	if err := db.PingDB(); err != nil {
		telemetry.Fatal("service.startup.db_unreachable", err, nil)
		log.Fatalf("❌ Failed to connect to the database: %v", err)
	} else {
		log.Println("✅ Connected to the database successfully")
	}

	// Define the API Endpoints
	r := primary.PathPrefix("/video/v1.1").Subrouter()

	// Request ID Middleware
	r.Use(middleware.RequestIDMiddleware)

	// Monitor: one http.request.end event per request, panics recovered
	r.Use(telemetry.Middleware)

	// Logging of requests
	r.Use(middleware.LoggingMiddleware)

	// Adding response headers
	r.Use(middleware.MuxHeaderMiddleware)

	// Track & Update Last Active
	r.Use(middleware.TokenHandlers)

	// Add context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Clear all temporary files
	filenames, err := FilterDir("/tmp")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("🧹 Cleanup! [Removing temporary files]")
	if filenames != nil {
		for _, filename := range *filenames {
			fmt.Println("   > removing:" + filename)
		}
	} else {
		fmt.Println("   > Done. no files to remove")
	}

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

	upload.Handle("/thumbnail", middleware.AccessTokenMiddleware(http.HandlerFunc(routers.HandleThumbnailUpload))).Methods(http.MethodPost)

	upload.Handle("/mux/upload", middleware.AccessTokenMiddleware(http.HandlerFunc(routers.HandleMuxCreateUpload))).Methods(http.MethodPost)
	upload.Handle("/mux/{id}", middleware.AccessTokenMiddleware(http.HandlerFunc(routers.HandleMuxUploadStatus))).Methods(http.MethodGet)

	// Create Queries

	create := r.PathPrefix("/create").Subrouter()

	create.Handle("/video", middleware.AccessTokenMiddleware(http.HandlerFunc(routers.HandleVideoCreate))).Methods(http.MethodPost)

	// Caption admin (edit / regenerate) — keyed by video uuid
	captions := r.PathPrefix("/captions").Subrouter()
	captions.HandleFunc("/{uuid}/en.vtt", routers.HandleCaptionFile).Methods(http.MethodGet) // public: Mux fetches edited captions here
	captions.Handle("/{uuid}", middleware.AccessTokenMiddleware(http.HandlerFunc(routers.HandleGetCaption))).Methods(http.MethodGet)
	captions.Handle("/{uuid}", middleware.AccessTokenMiddleware(http.HandlerFunc(routers.HandlePutCaption))).Methods(http.MethodPut)
	captions.Handle("/{uuid}/regenerate", middleware.AccessTokenMiddleware(http.HandlerFunc(routers.HandleRegenerateCaption))).Methods(http.MethodPost)

	// V2.1 Endpoints
	r.HandleFunc("/recordView/{query}", routers.HandleRecordView).Methods(http.MethodPost)
	r.HandleFunc("/recordDownload/{query}", routers.HandleRecordDownload).Methods(http.MethodPost)
	r.HandleFunc("/download/{query}", routers.HandleDownloadVideo).Methods(http.MethodGet)
	r.HandleFunc("/video/{query}", routers.HandleGetVideo).Methods(http.MethodGet)
	r.HandleFunc("/newsletter", routers.HandlePostNewsletter).Methods(http.MethodPost)
	r.HandleFunc("/newsletter/unsubscribe", routers.HandleUnsubscribeNewsletter).Methods(http.MethodPost)
	r.HandleFunc("/spotlight", routers.HandleSpotlightList).Methods(http.MethodGet)

	// Mux webhooks — public, authenticated by Mux-Signature
	r.HandleFunc("/webhooks/mux", routers.HandleMuxWebhook).Methods(http.MethodPost)

	// Launch API Listener
	fmt.Printf("✅ Hillview Video Provider API running on port %s\n", env.Port)

	// Add the health check polling
	telemetry.Go(ctx, "background.healthcheck", background.StartHealthCheckPolling)

	// Reconcile video durations with Mux (fallback for the asset.ready webhook)
	telemetry.Go(ctx, "background.duration_sync", background.StartVideoDurationSync)

	// Collect Mux AI captions/transcripts (fallback for the track webhooks)
	telemetry.Go(ctx, "background.caption_sync", background.StartVideoCaptionSync)

	headersOk := handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Origin", "Authorization", "Cookies", "Accept", "Cookie", "X-CSRF-Token"})
	originsOk := handlers.AllowedOrigins([]string{"https://team.hillview.tv", "https://hillview.tv", "https://live.hillview.tv", "http://localhost:3000", "http://localhost:3001", "http://localhost:3002"})
	exposedOk := handlers.ExposedHeaders([]string{"X-Request-ID"})
	allowCredentials := handlers.AllowCredentials()
	methodsOk := handlers.AllowedMethods([]string{"GET", "HEAD", "POST", "PUT", "OPTIONS"})

	// No Read/WriteTimeout: /upload takes whole video files, and the old
	// ListenAndServe had no limit at all. Only the header read is bounded.
	server := &http.Server{
		Addr:              ":" + env.Port,
		Handler:           handlers.CORS(originsOk, headersOk, exposedOk, methodsOk, allowCredentials)(primary),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	if err := telemetry.ServeWithGracefulShutdown(server, stop); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
