package routers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/hillview.tv/videoAPI/actions"
	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/query"
	"github.com/hillview.tv/videoAPI/responder"
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
		responder.BadBody(w, err)
		return
	}

	if body.Title == nil || len(*body.Title) == 0 {
		responder.ErrMissingBodyRequirement(w, "title")
		return
	}

	if body.URL == nil || len(*body.URL) == 0 {
		responder.ErrMissingBodyRequirement(w, "url")
		return
	}

	if body.Description == nil || len(*body.Description) == 0 {
		responder.ErrMissingBodyRequirement(w, "description")
		return
	}

	// If the URL points at Cloudflare Stream, populate duration immediately.
	// A nil result (still processing / error) leaves the column NULL for the
	// background sync job to reconcile later.
	var duration *int
	if uid := actions.CloudflareID(*body.URL); uid != "" {
		duration, err = actions.GetCloudflareDuration(uid)
		if err != nil {
			log.Printf("create video: failed to fetch cloudflare duration: %v", err)
		}
	}

	upload, err := query.CreateVideo(db.DB, query.CreateVideoRequest{
		Title:       body.Title,
		Description: body.Description,
		URL:         body.URL,
		Thumbnail:   body.Thumbnail,
		Duration:    duration,
	})
	if err != nil {
		responder.SendError(w, http.StatusInternalServerError, "failed to create video", err)
		return
	}

	responder.New(w, upload)
}
