package routers

import (
	"errors"
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
		responder.ErrMissingBodyRequirement(w, "query")
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
		responder.ErrInvalidBodyField(w, "id or uuid", errors.New("either id or uuid must be provided"))
		return
	}

	// Get the video
	video, err := query.GetVideo(db.DB, query.GetVideoRequest{
		ID:   req.ID,
		UUID: req.UUID,
	})
	if err != nil {
		responder.SendError(w, http.StatusInternalServerError, "failed to execute query", err)
		return
	}

	if video == nil {
		responder.SendError(w, http.StatusNotFound, "video not found", nil)
		return
	}

	responder.New(w, video, "Video retrieved successfully")

}
