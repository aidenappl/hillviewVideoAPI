package routers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/query"
)

func HandleVideoLists(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	limit := params["limit"]

	if len(limit) == 0 {
		http.Error(w, "missing limit param", http.StatusBadRequest)
		return
	}

	limitInt, err := strconv.ParseUint(string(limit), 10, 64)
	if err != nil {
		http.Error(w, "failed to convert string to int: "+err.Error(), http.StatusInternalServerError)
		return
	}

	videos, err := query.ListVideos(db.DB, query.ListVideosRequest{
		Limit: &limitInt,
	})
	if err != nil {
		http.Error(w, "failed to execute query: "+err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(videos)
}
