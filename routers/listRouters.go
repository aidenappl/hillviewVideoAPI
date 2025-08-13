package routers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/query"
	"github.com/hillview.tv/videoAPI/responder"
)

func HandlePlaylistLists(w http.ResponseWriter, r *http.Request) {
	limit := r.URL.Query().Get("limit")
	offset := r.URL.Query().Get("offset")
	sort := r.URL.Query().Get("sort")
	_ = r.URL.Query().Get("search")

	if len(limit) == 0 {
		http.Error(w, "missing limit param", http.StatusBadRequest)
		return
	}

	if len(offset) == 0 {
		http.Error(w, "missing offset param", http.StatusBadRequest)
		return
	}

	if len(sort) != 0 {
		sort = strings.ToLower(sort)
		if sort != "asc" && sort != "desc" {
			http.Error(w, "invalid sort param", http.StatusBadRequest)
			return
		}
	} else {
		sort = "desc"
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
		Sort:   &sort,
		Offset: offsetInt,
	})
	if err != nil {
		http.Error(w, "error querying playlists", http.StatusInternalServerError)
		return
	}

	responder.New(w, playlists, "Playlists retrieved successfully")
}

func HandleVideoLists(w http.ResponseWriter, r *http.Request) {
	limit := r.URL.Query().Get("limit")
	offset := r.URL.Query().Get("offset")
	sort := r.URL.Query().Get("sort")
	by := r.URL.Query().Get("by")
	searchQuery := r.URL.Query().Get("search")

	if len(limit) == 0 {
		http.Error(w, "missing limit param", http.StatusBadRequest)
		return
	}

	if len(offset) == 0 {
		http.Error(w, "missing offset param", http.StatusBadRequest)
		return
	}

	if len(sort) != 0 {
		sort = strings.ToLower(sort)
		if sort != "asc" && sort != "desc" {
			http.Error(w, "invalid sort param", http.StatusBadRequest)
			return
		}
	} else {
		sort = "desc"
	}

	if len(by) != 0 {
		by = strings.ToLower(by)
		if by != "date" && by != "views" {
			http.Error(w, "invalid by param", http.StatusBadRequest)
			return
		}
	} else {
		by = "date"
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
		By:     &by,
		Sort:   &sort,
	})
	if err != nil {
		http.Error(w, "failed to execute query: "+err.Error(), http.StatusInternalServerError)
		return
	}

	responder.New(w, videos, "Videos retrieved successfully")
}
