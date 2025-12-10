package routers

import (
	"errors"
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
		responder.ErrMissingBodyRequirement(w, "limit")
		return
	}

	if len(offset) == 0 {
		responder.ErrMissingBodyRequirement(w, "offset")
		return
	}

	if len(sort) != 0 {
		sort = strings.ToLower(sort)
		if sort != "asc" && sort != "desc" {
			responder.ErrInvalidBodyField(w, "invalid sort param", errors.New("sort must be 'asc' or 'desc'"))
			return
		}
	} else {
		sort = "desc"
	}

	limitInt, err := strconv.ParseUint(string(limit), 10, 64)
	if err != nil {
		responder.ErrInvalidBodyField(w, "limit", errors.New("invalid limit param"))
		return
	}

	offsetInt, err := strconv.ParseUint(string(offset), 10, 64)
	if err != nil {
		responder.ErrInvalidBodyField(w, "offset", errors.New("invalid offset param"))
		return
	}

	playlists, err := query.ListPlaylists(db.DB, query.ListPlaylistsRequest{
		Limit:  limitInt,
		Sort:   &sort,
		Offset: offsetInt,
	})
	if err != nil {
		responder.SendError(w, http.StatusInternalServerError, "error querying playlists", err)
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
		responder.ErrMissingBodyRequirement(w, "limit")
		return
	}

	if len(offset) == 0 {
		responder.ErrMissingBodyRequirement(w, "offset")
		return
	}

	if len(sort) != 0 {
		sort = strings.ToLower(sort)
		if sort != "asc" && sort != "desc" {
			responder.ErrInvalidBodyField(w, "invalid sort param", errors.New("sort must be 'asc' or 'desc'"))
			return
		}
	} else {
		sort = "desc"
	}

	if len(by) != 0 {
		by = strings.ToLower(by)
		if by != "date" && by != "views" {
			responder.ErrInvalidBodyField(w, "invalid by param", errors.New("by must be 'date' or 'views'"))
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
		responder.ErrInvalidBodyField(w, "limit", errors.New("failed to convert string to int: "+err.Error()))
		return
	}

	offsetInt, err := strconv.ParseUint(string(offset), 10, 64)
	if err != nil {
		responder.ErrInvalidBodyField(w, "offset", errors.New("failed to convert string to int: "+err.Error()))
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
		responder.SendError(w, http.StatusInternalServerError, "failed to execute query", err)
		return
	}

	responder.New(w, videos, "Videos retrieved successfully")
}
