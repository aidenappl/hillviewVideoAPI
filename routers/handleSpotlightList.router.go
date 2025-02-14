package routers

import (
	"encoding/json"
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
		json.NewEncoder(w).Encode(responder.Error("missing limit"))
		return
	} else {
		l, err := strconv.Atoi(limit)
		if err != nil {
			json.NewEncoder(w).Encode(responder.Error("invalid limit"))
			return
		}
		q.Limit = &l
	}
	offset := params.Get("offset")
	if offset == "" {
		json.NewEncoder(w).Encode(responder.Error("missing offset"))
		return
	} else {
		o, err := strconv.Atoi(offset)
		if err != nil {
			json.NewEncoder(w).Encode(responder.Error("invalid offset"))
			return
		}
		q.Offset = &o
	}

	spotlights, err := query.ListSpotlights(db.DB, q)
	if err != nil {
		json.NewEncoder(w).Encode(responder.Error(err.Error()))
		return
	}

	json.NewEncoder(w).Encode(responder.New(spotlights))
}
