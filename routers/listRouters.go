package routers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/query"
)

func HandlePlaylistLists(w http.ResponseWriter, r *http.Request) {
	limit := r.URL.Query().Get("limit")
	offset := r.URL.Query().Get("offset")
	_ = r.URL.Query().Get("search")

	if len(limit) == 0 {
		http.Error(w, "missing limit param", http.StatusBadRequest)
		return
	}

	if len(offset) == 0 {
		http.Error(w, "missing offset param", http.StatusBadRequest)
		return
	}

	limitInt, err := strconv.ParseUint(string(limit), 10, 64)
	if err != nil {
		http.Error(w, "invalid limit param", http.StatusBadRequest)
		return
	}

	offsetInt, err := strconv.ParseUint(string(offset), 10, 64)
	if err != nil {
		http.Error(w, "invalid offset param", http.StatusBadRequest)
		return
	}

	playlists, err := query.ListPlaylists(db.DB, query.ListPlaylistsRequest{
		Limit:  limitInt,
		Offset: offsetInt,
	})
	if err != nil {
		http.Error(w, "error querying playlists", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(playlists)
}

func HandleVideoLists(w http.ResponseWriter, r *http.Request) {
	limit := r.URL.Query().Get("limit")
	offset := r.URL.Query().Get("offset")
	searchQuery := r.URL.Query().Get("search")

	if len(limit) == 0 {
		http.Error(w, "missing limit param", http.StatusBadRequest)
		return
	}

	if len(offset) == 0 {
		http.Error(w, "missing offset param", http.StatusBadRequest)
		return
	}

	var search *string
	if len(searchQuery) > 0 {
		search = &searchQuery
	}

	limitInt, err := strconv.ParseUint(string(limit), 10, 64)
	if err != nil {
		http.Error(w, "failed to convert string to int: "+err.Error(), http.StatusInternalServerError)
		return
	}

	offsetInt, err := strconv.ParseUint(string(offset), 10, 64)
	if err != nil {
		http.Error(w, "failed to convert string to int: "+err.Error(), http.StatusInternalServerError)
		return
	}

	videos, err := query.ListVideos(db.DB, query.ListVideosRequest{
		Limit:  &limitInt,
		Offset: &offsetInt,
		Search: search,
	})
	if err != nil {
		http.Error(w, "failed to execute query: "+err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(videos)
}
