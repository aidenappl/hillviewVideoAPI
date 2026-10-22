package routers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/hillview.tv/videoAPI/actions"
	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/env"
	"github.com/hillview.tv/videoAPI/middleware"
	muxvideo "github.com/hillview.tv/videoAPI/mux"
	"github.com/hillview.tv/videoAPI/query"
	"github.com/hillview.tv/videoAPI/responder"
	"github.com/hillview.tv/videoAPI/streamurl"
	"github.com/hillview.tv/videoAPI/telemetry"
)

// DEFAULT_UPLOAD_ORIGIN is the browser origin Mux allows to PUT the file when
// the request carries no Origin header.
const DEFAULT_UPLOAD_ORIGIN = "https://team.hillview.tv"

type MuxUploadResponse struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// HandleMuxCreateUpload answers POST /upload/mux/upload with a Mux direct
// upload: a resumable PUT url the browser sends the file to (UpChunk). The
// asset is created with the Hillview defaults (mux.NewAssetSettings).
func HandleMuxCreateUpload(w http.ResponseWriter, r *http.Request) {
	claims := middleware.WithClaimsValue(r.Context())
	if claims == nil {
		responder.SendError(w, http.StatusUnauthorized, "Missing Authorization token")
		return
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = DEFAULT_UPLOAD_ORIGIN
	}

	upload, err := actions.Mux.CreateUpload("user:"+claims.Subject, origin)
	if err != nil {
		sendMuxError(w, "failed to create Mux upload", err)
		return
	}
	responder.New(w, MuxUploadResponse{ID: upload.ID, URL: upload.URL})
}

// MuxUploadStatus is what the team dashboard polls after an upload: status is
// waiting (file not finished), preparing, ready or errored. The playback
// fields are set once Mux has created the asset.
type MuxUploadStatus struct {
	Status      string `json:"status"`
	AssetID     string `json:"asset_id,omitempty"`
	PlaybackID  string `json:"playback_id,omitempty"`
	URL         string `json:"url,omitempty"`
	Thumbnail   string `json:"thumbnail,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
	Duration    *int   `json:"duration,omitempty"`
}

// HandleMuxUploadStatus answers GET /upload/mux/{id} for a direct upload id.
func HandleMuxUploadStatus(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if id == "" {
		responder.ErrMissingBodyRequirement(w, "id")
		return
	}

	upload, err := actions.Mux.GetUpload(id)
	if err != nil {
		sendMuxError(w, "failed to fetch Mux upload", err)
		return
	}

	switch upload.Status {
	case "errored", "cancelled", "timed_out":
		responder.New(w, MuxUploadStatus{Status: "errored"})
		return
	}
	if upload.AssetID == "" {
		responder.New(w, MuxUploadStatus{Status: "waiting"})
		return
	}

	asset, err := actions.Mux.GetAsset(upload.AssetID)
	if err != nil {
		sendMuxError(w, "failed to fetch Mux asset", err)
		return
	}

	status := MuxUploadStatus{Status: asset.Status, AssetID: asset.ID}
	if pid := asset.PublicPlaybackID(); pid != "" {
		duration := asset.DurationSeconds()
		status.PlaybackID = pid
		status.URL = streamurl.MuxURL(pid)
		status.Thumbnail = muxvideo.ThumbnailURL(pid, duration)
		status.DownloadURL = muxvideo.DownloadURL(pid, "")
		status.Duration = duration
	}
	responder.New(w, status)
}

// HandleMuxWebhook answers POST /webhooks/mux. Mux signs every delivery
// (Mux-Signature); unsigned or stale ones are refused. Asset and track events
// run the same reconcile steps as the background syncs for every video that
// plays the asset, so a video saved before Mux finished is caught up there
// instead. Failures answer 500 so Mux retries (for up to 24h).
func HandleMuxWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		responder.SendError(w, http.StatusBadRequest, "failed to read body", err)
		return
	}
	if err := muxvideo.VerifyWebhook(env.MuxWebhookSecret, r.Header.Get(muxvideo.SignatureHeader), body, time.Now()); err != nil {
		responder.SendError(w, http.StatusUnauthorized, "invalid webhook signature", err)
		return
	}

	var event muxvideo.Event
	if err := json.Unmarshal(body, &event); err != nil {
		responder.BadBody(w, err)
		return
	}

	var assetID string
	switch event.Type {
	case "video.asset.ready", "video.asset.errored":
		assetID = event.Object.ID
	case "video.asset.track.ready", "video.asset.track.errored":
		var track muxvideo.Track
		if err := json.Unmarshal(event.Data, &track); err != nil {
			responder.BadBody(w, err)
			return
		}
		if track.Type != "text" {
			responder.New(w, nil, "ignored")
			return
		}
		assetID = track.AssetID
	default:
		responder.New(w, nil, "ignored")
		return
	}
	if assetID == "" {
		responder.New(w, nil, "ignored")
		return
	}

	asset, err := actions.Mux.GetAsset(assetID)
	if errors.Is(err, muxvideo.ErrNotFound) {
		responder.New(w, nil, "asset deleted")
		return
	}
	if err != nil {
		sendMuxError(w, "failed to fetch Mux asset", err)
		return
	}
	pid := asset.PublicPlaybackID()
	if pid == "" {
		responder.New(w, nil, "ignored")
		return
	}

	videos, err := query.FindVideosByMuxAsset(db.DB, assetID, streamurl.MuxURL(pid))
	if err != nil {
		responder.SendError(w, http.StatusInternalServerError, "failed to find videos", err)
		return
	}

	for _, v := range videos {
		// tools/migrateToMux stores the asset ids before the row's url moves
		// to Mux; skip rows that are not on Mux yet.
		if !streamurl.IsMux(v.URL) {
			continue
		}
		switch event.Type {
		case "video.asset.ready":
			if _, err := actions.SyncMuxDuration(v.ID, v.URL); err != nil {
				sendMuxError(w, "failed to sync duration", err)
				return
			}
		case "video.asset.errored":
			telemetry.Warn(r.Context(), "mux.asset.errored", nil, map[string]any{"video_id": v.ID, "mux_asset_id": assetID})
		default:
			caption, err := query.GetCaption(db.DB, v.ID, captionLanguage)
			if err != nil {
				responder.SendError(w, http.StatusInternalServerError, "failed to load caption", err)
				return
			}
			if caption != nil && caption.Status != actions.CaptionInProgress {
				continue // ready (possibly hand-edited) or error: leave it
			}
			if _, err := actions.SyncMuxCaption(v.ID, v.URL); err != nil {
				sendMuxError(w, "failed to sync caption", err)
				return
			}
		}
	}

	responder.New(w, map[string]int{"videos": len(videos)})
}

// HandleCaptionFile answers GET /captions/{uuid}/en.vtt with a video's stored
// caption. Mux fetches edited captions from here when they replace a track.
func HandleCaptionFile(w http.ResponseWriter, r *http.Request) {
	uuid := mux.Vars(r)["uuid"]
	vtt, err := query.GetCaptionVTTByVideoUUID(db.DB, uuid, captionLanguage)
	if err != nil {
		responder.SendError(w, http.StatusInternalServerError, "failed to load caption", err)
		return
	}
	if vtt == nil {
		responder.SendError(w, http.StatusNotFound, "caption not found")
		return
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(*vtt))
}

// captionFileURL is the public url of a video's stored caption.
func captionFileURL(uuid string) string {
	return env.PublicAPIURL + "/captions/" + uuid + "/" + captionLanguage + ".vtt"
}

// sendMuxError maps Mux client errors onto responses: unconfigured is 503,
// a missing resource 404, anything else 502.
func sendMuxError(w http.ResponseWriter, msg string, err error) {
	switch {
	case errors.Is(err, muxvideo.ErrNotConfigured):
		responder.SendError(w, http.StatusServiceUnavailable, "Mux is not configured", err)
	case errors.Is(err, muxvideo.ErrNotFound):
		responder.SendError(w, http.StatusNotFound, msg, err)
	case errors.Is(err, actions.ErrCaptionBusy):
		responder.SendError(w, http.StatusConflict, "the previous caption change is still processing; try again in a moment", err)
	default:
		responder.SendError(w, http.StatusBadGateway, msg, err)
	}
}
