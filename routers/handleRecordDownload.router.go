package routers

import (
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/query"
)

type HandleRecordDownloadRequest struct {
	UUID *string `json:"uuid"`
	ID   *int    `json:"id"`
}

func HandleRecordDownload(w http.ResponseWriter, r *http.Request) {
	var req HandleRecordDownloadRequest
	q := mux.Vars(r)["query"]

	if q != "" {
		// check if int or string
		intID, err := strconv.Atoi(q)
		if err != nil {
			req.UUID = &q
		} else {
			req.ID = &intID
		}
	} else {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// get video
	video, err := query.GetVideo(db.DB, query.GetVideoRequest{
		UUID: req.UUID,
		ID:   req.ID,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// check if video exists
	if video == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// update video
	err = query.RecordDownload(db.DB, query.RecordDownloadRequest{
		ID: video.ID,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
