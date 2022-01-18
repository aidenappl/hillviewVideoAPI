package routers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/query"
)

func HandleVideoRead(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	id := params["id"]

	if len(id) == 0 {
		http.Error(w, "missing id param", http.StatusBadRequest)
		return
	}

	idInt, err := strconv.Atoi(id)
	if err != nil {
		http.Error(w, "failed to convert string to int: "+err.Error(), http.StatusInternalServerError)
		return
	}

	video, err := query.GetVideo(db.DB, query.GetVideoRequest{
		ID: &idInt,
	})
	if err != nil {
		http.Error(w, "failed to execute query: "+err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(video)
}
