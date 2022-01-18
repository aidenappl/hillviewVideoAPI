package routers

import (
	"encoding/json"
	"net/http"

	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/query"
)

type VideoCreateRequest struct {
	Title       *string `json:"title"`
	URL         *string `json:"url"`
	Description *string `json:"description"`
	Thumbnail   *string `json:"thumbnail"`
}

func HandleVideoCreate(w http.ResponseWriter, r *http.Request) {
	body := VideoCreateRequest{}
	err := json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if body.Title == nil || len(*body.Title) == 0 {
		http.Error(w, "Missing title", http.StatusBadRequest)
		return
	}

	if body.URL == nil || len(*body.URL) == 0 {
		http.Error(w, "Missing URL", http.StatusBadRequest)
		return
	}

	if body.Description == nil || len(*body.Description) == 0 {
		http.Error(w, "Missing description", http.StatusBadRequest)
		return
	}

	upload, err := query.CreateVideo(db.DB, query.CreateVideoRequest{
		Title:       body.Title,
		Description: body.Description,
		URL:         body.URL,
		Thumbnail:   body.Thumbnail,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(upload)
}
