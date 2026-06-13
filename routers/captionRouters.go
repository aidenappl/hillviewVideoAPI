package routers

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/hillview.tv/videoAPI/actions"
	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/query"
	"github.com/hillview.tv/videoAPI/responder"
)

const captionLanguage = "en"

// resolveVideo loads a video by uuid and returns it plus its Cloudflare UID.
func resolveVideo(w http.ResponseWriter, r *http.Request) (id int, uid string, ok bool) {
	uuid := mux.Vars(r)["uuid"]
	if uuid == "" {
		responder.ErrMissingBodyRequirement(w, "uuid")
		return 0, "", false
	}
	video, err := query.GetVideo(db.DB, query.GetVideoRequest{UUID: &uuid})
	if err != nil {
		responder.SendError(w, http.StatusInternalServerError, "failed to load video", err)
		return 0, "", false
	}
	if video == nil {
		responder.SendError(w, http.StatusNotFound, "video not found")
		return 0, "", false
	}
	cfid := actions.CloudflareID(video.URL)
	if cfid == "" {
		responder.SendError(w, http.StatusBadRequest, "video is not a Cloudflare Stream video")
		return 0, "", false
	}
	return video.ID, cfid, true
}

// HandleGetCaption returns the stored caption (status + VTT) for a video.
func HandleGetCaption(w http.ResponseWriter, r *http.Request) {
	id, _, ok := resolveVideo(w, r)
	if !ok {
		return
	}
	caption, err := query.GetCaption(db.DB, id, captionLanguage)
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

// HandlePutCaption saves an edited WebVTT caption: uploads it to Cloudflare and
// stores it (with a derived plain-text transcript) in our DB.
func HandlePutCaption(w http.ResponseWriter, r *http.Request) {
	id, uid, ok := resolveVideo(w, r)
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

	if err := actions.PutCaptions(uid, captionLanguage, *body.VTT); err != nil {
		responder.SendError(w, http.StatusBadGateway, "failed to upload caption to Cloudflare", err)
		return
	}

	plain := actions.VTTToPlainText(*body.VTT)
	if err := query.UpsertCaption(db.DB, id, captionLanguage, "ready", body.VTT, &plain); err != nil {
		responder.SendError(w, http.StatusInternalServerError, "failed to save caption", err)
		return
	}
	responder.New(w, query.Caption{Language: captionLanguage, Status: "ready", VTT: body.VTT})
}

// HandleRegenerateCaption discards the current caption and re-runs Cloudflare's
// AI generation. The background sync job collects the result when it's ready.
func HandleRegenerateCaption(w http.ResponseWriter, r *http.Request) {
	id, uid, ok := resolveVideo(w, r)
	if !ok {
		return
	}
	if err := actions.DeleteCaptions(uid, captionLanguage); err != nil {
		responder.SendError(w, http.StatusBadGateway, "failed to clear existing caption", err)
		return
	}
	if err := actions.GenerateCaptions(uid, captionLanguage); err != nil {
		responder.SendError(w, http.StatusBadGateway, "failed to start caption generation", err)
		return
	}
	if err := query.UpsertCaption(db.DB, id, captionLanguage, "inprogress", nil, nil); err != nil {
		responder.SendError(w, http.StatusInternalServerError, "failed to update caption status", err)
		return
	}
	responder.New(w, query.Caption{Language: captionLanguage, Status: "inprogress"})
}
