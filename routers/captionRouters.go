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
	id       int
	uuid     string
	url      string
	provider streamurl.Provider
	cfid     string
}

// resolveVideo loads a video by uuid. Captions exist only for Cloudflare
// Stream and Mux videos.
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
	v := captionVideo{
		id:       video.ID,
		uuid:     video.UUID,
		url:      video.URL,
		provider: streamurl.ProviderOf(video.URL),
		cfid:     actions.CloudflareID(video.URL),
	}
	if v.provider == streamurl.ProviderNone {
		responder.SendError(w, http.StatusBadRequest, "video is not a Cloudflare Stream or Mux video")
		return captionVideo{}, false
	}
	return v, true
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
// plain-text transcript) in our DB and replaces the provider's caption track —
// uploaded to Cloudflare, or fetched by Mux from /captions/{uuid}/en.vtt.
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

	// Cloudflare requires sequential cue identifiers — enforce on every PUT,
	// regardless of which client/path produced the VTT.
	vtt := actions.NumberVTTCues(*body.VTT)
	plain := actions.VTTToPlainText(vtt)

	switch v.provider {
	case streamurl.ProviderMux:
		// Mux fetches the track from our DB, so save first.
		if err := query.UpsertCaption(db.DB, v.id, captionLanguage, "ready", &vtt, &plain); err != nil {
			responder.SendError(w, http.StatusInternalServerError, "failed to save caption", err)
			return
		}
		if err := actions.ReplaceMuxCaption(v.id, v.url, captionFileURL(v.uuid)); err != nil {
			sendMuxError(w, "failed to replace caption on Mux", err)
			return
		}
	default:
		if err := actions.PutCaptions(v.cfid, captionLanguage, vtt); err != nil {
			responder.SendError(w, http.StatusBadGateway, "failed to upload caption to Cloudflare", err)
			return
		}
		if err := query.UpsertCaption(db.DB, v.id, captionLanguage, "ready", &vtt, &plain); err != nil {
			responder.SendError(w, http.StatusInternalServerError, "failed to save caption", err)
			return
		}
	}
	responder.New(w, query.Caption{Language: captionLanguage, Status: "ready", VTT: &vtt})
}

// HandleRegenerateCaption discards the current caption and re-runs the
// provider's AI generation. The background sync job (and, for Mux, the
// track.ready webhook) collects the result when it's ready.
func HandleRegenerateCaption(w http.ResponseWriter, r *http.Request) {
	v, ok := resolveVideo(w, r)
	if !ok {
		return
	}
	switch v.provider {
	case streamurl.ProviderMux:
		// Mark it in progress first so a fast track.ready webhook is collected.
		if err := query.UpsertCaption(db.DB, v.id, captionLanguage, "inprogress", nil, nil); err != nil {
			responder.SendError(w, http.StatusInternalServerError, "failed to update caption status", err)
			return
		}
		if err := actions.RegenerateMuxCaption(v.id, v.url); err != nil {
			sendMuxError(w, "failed to start caption generation", err)
			return
		}
	default:
		if err := actions.DeleteCaptions(v.cfid, captionLanguage); err != nil {
			responder.SendError(w, http.StatusBadGateway, "failed to clear existing caption", err)
			return
		}
		if err := actions.GenerateCaptions(v.cfid, captionLanguage); err != nil {
			responder.SendError(w, http.StatusBadGateway, "failed to start caption generation", err)
			return
		}
		if err := query.UpsertCaption(db.DB, v.id, captionLanguage, "inprogress", nil, nil); err != nil {
			responder.SendError(w, http.StatusInternalServerError, "failed to update caption status", err)
			return
		}
	}
	responder.New(w, query.Caption{Language: captionLanguage, Status: "inprogress"})
}
