package routers

import (
	"net/http"
	"strconv"

	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/query"
	"github.com/hillview.tv/videoAPI/responder"
)

func HandleSpotlightList(w http.ResponseWriter, r *http.Request) {
	q := query.ListSpotlightsRequest{}

	// get params
	params := r.URL.Query()
	limit := params.Get("limit")
	if limit == "" {
		responder.ParamError(w, "limit")
		return
	} else {
		l, err := strconv.Atoi(limit)
		if err != nil {
			responder.ParamError(w, "limit")
			return
		}
		q.Limit = &l
	}
	offset := params.Get("offset")
	if offset == "" {
		responder.ParamError(w, "offset")
		return
	} else {
		o, err := strconv.Atoi(offset)
		if err != nil {
			responder.ParamError(w, "offset")
			return
		}
		q.Offset = &o
	}

	spotlights, err := query.ListSpotlights(db.DB, q)
	if err != nil {
		responder.SendError(w, http.StatusInternalServerError, "failed to list spotlights", err)
		return
	}

	responder.New(w, spotlights)
}
