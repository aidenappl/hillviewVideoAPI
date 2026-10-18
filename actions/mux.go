package actions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/env"
	"github.com/hillview.tv/videoAPI/mux"
	"github.com/hillview.tv/videoAPI/query"
	"github.com/hillview.tv/videoAPI/streamurl"
	"github.com/hillview.tv/videoAPI/telemetry"
)

// Mux is the production Mux client. Without MUX_TOKEN_ID / MUX_TOKEN_SECRET
// every call returns mux.ErrNotConfigured.
var Mux = mux.New(env.MuxTokenID, env.MuxTokenSecret)

// ErrNotMux means a video's url is not a Mux url.
var ErrNotMux = errors.New("video is not a Mux video")

// Caption statuses stored in video_captions.status.
const (
	CaptionReady      = "ready"
	CaptionInProgress = "inprogress"
	CaptionError      = "error"
)

// ResolveMuxAsset loads the Mux asset a video plays. The url is the source of
// truth: the asset id stored on the row is used while its playback id still
// matches the url, and otherwise looked up from the playback id and stored.
func ResolveMuxAsset(videoID int, url string) (*mux.Asset, error) {
	pid := streamurl.MuxPlaybackID(url)
	if pid == "" {
		return nil, ErrNotMux
	}

	assetID, storedPID, err := query.GetVideoMuxIDs(db.DB, videoID)
	if err != nil {
		return nil, err
	}
	if assetID == "" || storedPID != pid {
		if assetID, err = Mux.AssetIDForPlayback(pid); err != nil {
			return nil, err
		}
		// A second row playing the same asset trips the unique key; the asset
		// still resolves, so report it and carry on.
		if err := query.SetVideoMuxIDs(db.DB, videoID, assetID, pid); err != nil {
			telemetry.Warn(context.Background(), "mux.ids.store_failed", err, map[string]any{"video_id": videoID, "mux_asset_id": assetID})
		}
	}
	return Mux.GetAsset(assetID)
}

// SyncMuxDuration stores a Mux video's duration once the asset is ready. It
// returns nil while Mux is still processing.
func SyncMuxDuration(videoID int, url string) (*int, error) {
	asset, err := ResolveMuxAsset(videoID, url)
	if err != nil {
		return nil, err
	}
	if asset.Status != "ready" {
		return nil, nil
	}
	seconds := asset.DurationSeconds()
	if seconds == nil {
		return nil, nil
	}
	if _, err := db.DB.Exec("UPDATE videos SET duration = ? WHERE id = ?", *seconds, videoID); err != nil {
		return nil, fmt.Errorf("failed to store duration: %w", err)
	}
	return seconds, nil
}

// SyncMuxCaption moves a Mux video's English caption one step forward and
// stores the result in video_captions: the generated track's VTT once it is
// ready, "inprogress" while it is being made (requesting generation when the
// asset has no English track), "error" when Mux failed. Callers run it only for
// videos with no caption row or an "inprogress" one, so edited captions are
// never overwritten.
func SyncMuxCaption(videoID int, url string) (string, error) {
	asset, err := ResolveMuxAsset(videoID, url)
	if err != nil {
		return "", err
	}

	status := CaptionInProgress
	reason := ""
	var vtt, plain *string

	switch {
	case asset.Status == "errored":
		status, reason = CaptionError, "asset_errored"
	case asset.Status != "ready":
		// Captions are generated after ingest; check again later.
	default:
		track := asset.TextTrack(captionLang)
		switch {
		case track == nil:
			audio := asset.AudioTrack()
			if audio == nil {
				status, reason = CaptionError, "no_audio"
				break
			}
			if err := Mux.GenerateCaptions(asset.ID, audio.ID); err != nil {
				return "", err
			}
		case track.Status == "ready":
			body, err := FetchMuxVTT(asset.PublicPlaybackID(), track.ID)
			if err != nil {
				return "", err
			}
			text := VTTToPlainText(body)
			status, vtt, plain = CaptionReady, &body, &text
		case track.Status == "errored":
			status, reason = CaptionError, "track_errored"
		}
	}

	if status == CaptionError {
		log.Printf("caption sync: video %d caption failed on Mux: %s", videoID, reason)
		telemetry.Warn(context.Background(), "caption_sync.mux.error", nil, map[string]any{"video_id": videoID, "mux_asset_id": asset.ID, "reason": reason})
	}

	if err := query.UpsertCaption(db.DB, videoID, captionLang, status, vtt, plain); err != nil {
		return "", err
	}
	return status, nil
}

// ReplaceMuxCaption swaps a Mux video's English text tracks for one Mux fetches
// from vttURL (the edited caption, served from our DB).
func ReplaceMuxCaption(videoID int, url, vttURL string) error {
	asset, err := ResolveMuxAsset(videoID, url)
	if err != nil {
		return err
	}
	if err := deleteMuxTextTracks(asset); err != nil {
		return err
	}
	_, err = Mux.CreateTextTrack(asset.ID, vttURL, captionLang, "English")
	return err
}

// RegenerateMuxCaption drops a Mux video's English text tracks and asks Mux to
// generate new ones. The caption sync collects the result.
func RegenerateMuxCaption(videoID int, url string) error {
	asset, err := ResolveMuxAsset(videoID, url)
	if err != nil {
		return err
	}
	audio := asset.AudioTrack()
	if audio == nil {
		return errors.New("mux asset has no audio track")
	}
	if err := deleteMuxTextTracks(asset); err != nil {
		return err
	}
	return Mux.GenerateCaptions(asset.ID, audio.ID)
}

func deleteMuxTextTracks(asset *mux.Asset) error {
	for _, t := range asset.Tracks {
		if t.Type == "text" && t.LanguageCode == captionLang {
			if err := Mux.DeleteTrack(asset.ID, t.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// FetchMuxVTT downloads a text track's WebVTT from its public url.
func FetchMuxVTT(playbackID, trackID string) (string, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(mux.VTTURL(playbackID, trackID))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 12<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch mux vtt status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return string(body), nil
}

// captionLang is the only caption language Hillview stores.
const captionLang = mux.CaptionLanguage
