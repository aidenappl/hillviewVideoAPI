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

func HandleVideoRead(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	id := params["id"]

	if len(id) == 0 {
		responder.ErrMissingBodyRequirement(w, "id")
		return
	}

	idInt, err := strconv.Atoi(id)
	if err != nil {
		responder.ErrInvalidBodyField(w, "id", errors.New("id is invalid"))
		return
	}

	video, err := query.GetVideo(db.DB, query.GetVideoRequest{
		ID: &idInt,
	})
	if err != nil {
		responder.SendError(w, http.StatusInternalServerError, "failed to execute query", err)
		return
	}

	responder.New(w, video, "Video retrieved successfully")
}

func HandlePlaylistRead(w http.ResponseWriter, r *http.Request) {
	var route *string

	id := r.URL.Query().Get("id")
	queryRoute := r.URL.Query().Get("route")

	if len(queryRoute) != 0 {
		route = &queryRoute
	}

	var idInt *int

	if len(id) != 0 {
		intrn, err := strconv.Atoi(id)
		if err != nil {
			responder.ErrInvalidBodyField(w, "id", errors.New("failed to convert string to int: "+err.Error()))
			return
		}
		idInt = &intrn
	}

	playlist, err := query.GetPlaylist(db.DB, query.GetPlaylistRequest{
		ID:    idInt,
		Route: route,
	})
	if err != nil {
		responder.SendError(w, http.StatusInternalServerError, "failed to execute query", err)
		return
	}

	responder.New(w, playlist, "Playlist retrieved successfully")
}
