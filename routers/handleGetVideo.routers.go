package routers

import (
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/query"
	"github.com/hillview.tv/videoAPI/responder"
)

type GetVideoRequest struct {
	ID   *int
	UUID *string
}

func HandleGetVideo(w http.ResponseWriter, r *http.Request) {
	var req GetVideoRequest

	// Get the query params
	q := mux.Vars(r)["query"]

	// Convert the query params to the correct type
	if q == "" {
		http.Error(w, "missing 'query' param", http.StatusBadRequest)
		return
	} else {
		idInt, err := strconv.Atoi(q)
		if err != nil {
			req.UUID = &q
		} else {
			req.ID = &idInt
		}
	}

	// Check if the request is valid
	if req.ID == nil && req.UUID == nil {
		http.Error(w, "missing 'id' or 'uuid' query param", http.StatusBadRequest)
		return
	}

	// Get the video
	video, err := query.GetVideo(db.DB, query.GetVideoRequest{
		ID:   req.ID,
		UUID: req.UUID,
	})
	if err != nil {
		http.Error(w, "failed to execute query: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if video == nil {
		http.Error(w, "video not found", http.StatusNotFound)
		return
	}

	responder.New(w, video, "Video retrieved successfully")

}
