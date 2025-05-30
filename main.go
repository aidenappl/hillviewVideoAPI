package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/hillview.tv/videoAPI/env"
	"github.com/hillview.tv/videoAPI/middleware"
	"github.com/hillview.tv/videoAPI/routers"
)

func FilterDir(dir string) (*[]string, error) {
	files, err := ioutil.ReadDir(dir)
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

	upload.Handle("/video", middleware.AccessTokenMiddleware(http.HandlerFunc(routers.HandleVideoUpload))).Methods(http.MethodPost)
	upload.Handle("/thumbnail", middleware.AccessTokenMiddleware(http.HandlerFunc(routers.HandleThumbnailUpload))).Methods(http.MethodPost)

	upload.Handle("/cf/{id}", middleware.AccessTokenMiddleware(http.HandlerFunc(routers.HandleCloudflareStatus))).Methods(http.MethodGet)
	upload.Handle("/cf/{id}/download", middleware.AccessTokenMiddleware(http.HandlerFunc(routers.HandleCloudflareDownloadStatus))).Methods(http.MethodGet)
	upload.Handle("/cf/{id}", middleware.AccessTokenMiddleware(http.HandlerFunc(routers.HandleCloudflareUpdate))).Methods(http.MethodPost)
	upload.Handle("/cf/upload", middleware.AccessTokenMiddleware(http.HandlerFunc(routers.HandleCloudflareUpload))).Methods(http.MethodPost)
	upload.Handle("/cf/{id}/generateDownload", middleware.AccessTokenMiddleware(http.HandlerFunc(routers.HandleCloudflareDownload))).Methods(http.MethodPost)

	// Create Queries

	create := r.PathPrefix("/create").Subrouter()

	create.HandleFunc("/video", routers.HandleVideoCreate).Methods(http.MethodPost)

	// V2.1 Endpoints
	r.HandleFunc("/recordView/{query}", routers.HandleRecordView).Methods(http.MethodPost)
	r.HandleFunc("/recordDownload/{query}", routers.HandleRecordDownload).Methods(http.MethodPost)
	r.HandleFunc("/video/{query}", routers.HandleGetVideo).Methods(http.MethodGet)
	r.HandleFunc("/newsletter", routers.HandlePostNewsletter).Methods(http.MethodPost)
	r.HandleFunc("/newsletter/unsubscribe", routers.HandleUnsubscribeNewsletter).Methods(http.MethodPost)
	r.HandleFunc("/spotlight", routers.HandleSpotlightList).Methods(http.MethodGet)

	// Launch API Listener
	fmt.Printf("✅ Hillview Video Provider API running on port %s\n", env.Port)

	headersOk := handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Origin", "Authorization", "Cookies", "Accept", "Cookie", "X-CSRF-Token", "Tus-Resumable", "Upload-Length", "Upload-Metadata"})
	originsOk := handlers.AllowedOrigins([]string{"*"})
	allowCredentials := handlers.AllowCredentials()
	methodsOk := handlers.AllowedMethods([]string{"GET", "HEAD", "POST", "PUT", "OPTIONS"})
	log.Fatal(http.ListenAndServe(":"+env.Port, handlers.CORS(originsOk, headersOk, methodsOk, allowCredentials)(primary)))
}
