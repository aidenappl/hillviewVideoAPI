package routers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/query"
)

type GetVideoRequest struct {
	ID   *int
	UUID *string
}

func HandleGetVideo(w http.ResponseWriter, r *http.Request) {
	var req GetVideoRequest

	// Get the query params
	id := r.URL.Query().Get("id")
	uuid := r.URL.Query().Get("uuid")

	// Check if the id is valid
	if len(id) != 0 {
		intID, err := strconv.Atoi(id)
		if err != nil {
			http.Error(w, "failed to convert string to int: "+err.Error(), http.StatusInternalServerError)
			return
		}

		req.ID = &intID
	}

	// Check if the uuid is valid
	if len(uuid) != 0 {
		req.UUID = &uuid
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

	json.NewEncoder(w).Encode(video)

}
