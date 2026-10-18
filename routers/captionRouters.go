package routers

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/hillview.tv/videoAPI/actions"
	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/query"
	"github.com/hillview.tv/videoAPI/responder"
	"github.com/hillview.tv/videoAPI/streamurl"
)

const captionLanguage = "en"

// captionVideo is the video a caption request is about.
type captionVideo struct {
	id   int
	uuid string
	url  string
}

// resolveVideo loads a video by uuid. Captions exist only for Mux videos.
func resolveVideo(w http.ResponseWriter, r *http.Request) (captionVideo, bool) {
	uuid := mux.Vars(r)["uuid"]
	if uuid == "" {
		responder.ErrMissingBodyRequirement(w, "uuid")
		return captionVideo{}, false
	}
	video, err := query.GetVideo(db.DB, query.GetVideoRequest{UUID: &uuid})
	if err != nil {
		responder.SendError(w, http.StatusInternalServerError, "failed to load video", err)
		return captionVideo{}, false
	}
	if video == nil {
		responder.SendError(w, http.StatusNotFound, "video not found")
		return captionVideo{}, false
	}
	if !streamurl.IsMux(video.URL) {
		responder.SendError(w, http.StatusBadRequest, "video is not a Mux video")
		return captionVideo{}, false
	}
	return captionVideo{id: video.ID, uuid: video.UUID, url: video.URL}, true
}

// HandleGetCaption returns the stored caption (status + VTT) for a video.
func HandleGetCaption(w http.ResponseWriter, r *http.Request) {
	v, ok := resolveVideo(w, r)
	if !ok {
		return
	}
	caption, err := query.GetCaption(db.DB, v.id, captionLanguage)
	if err != nil {
		responder.SendError(w, http.StatusInternalServerError, "failed to load caption", err)
		return
	}
	if caption == nil {
		responder.New(w, query.Caption{Language: captionLanguage, Status: "none"})
		return
	}
	responder.New(w, caption)
}

type putCaptionRequest struct {
	VTT *string `json:"vtt"`
}

// HandlePutCaption saves an edited WebVTT caption: stores it (with a derived
// plain-text transcript) in our DB, then replaces Mux's English track with one
// Mux fetches from /captions/{uuid}/en.vtt.
func HandlePutCaption(w http.ResponseWriter, r *http.Request) {
	v, ok := resolveVideo(w, r)
	if !ok {
		return
	}
	var body putCaptionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		responder.BadBody(w, err)
		return
	}
	if body.VTT == nil || len(*body.VTT) == 0 {
		responder.ErrMissingBodyRequirement(w, "vtt")
		return
	}

	vtt := actions.NumberVTTCues(*body.VTT)
	plain := actions.VTTToPlainText(vtt)

	// Mux fetches the track from our DB, so save first.
	if err := query.UpsertCaption(db.DB, v.id, captionLanguage, actions.CaptionReady, &vtt, &plain); err != nil {
		responder.SendError(w, http.StatusInternalServerError, "failed to save caption", err)
		return
	}
	if err := actions.ReplaceMuxCaption(v.id, v.url, captionFileURL(v.uuid)); err != nil {
		sendMuxError(w, "failed to replace caption on Mux", err)
		return
	}
	responder.New(w, query.Caption{Language: captionLanguage, Status: actions.CaptionReady, VTT: &vtt})
}

// HandleRegenerateCaption discards the current caption and asks Mux to
// generate a new one. The track.ready webhook (or the caption sync) collects
// the result.
func HandleRegenerateCaption(w http.ResponseWriter, r *http.Request) {
	v, ok := resolveVideo(w, r)
	if !ok {
		return
	}
	// Mark it in progress first so a fast track.ready webhook is collected.
	if err := query.UpsertCaption(db.DB, v.id, captionLanguage, actions.CaptionInProgress, nil, nil); err != nil {
		responder.SendError(w, http.StatusInternalServerError, "failed to update caption status", err)
		return
	}
	if err := actions.RegenerateMuxCaption(v.id, v.url); err != nil {
		sendMuxError(w, "failed to start caption generation", err)
		return
	}
	responder.New(w, query.Caption{Language: captionLanguage, Status: actions.CaptionInProgress})
}
