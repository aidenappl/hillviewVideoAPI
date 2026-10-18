package background

import (
	"context"
	"log"
	"time"

	"github.com/hillview.tv/videoAPI/actions"
	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/streamurl"
	"github.com/hillview.tv/videoAPI/telemetry"
)

const (
	// captionSyncInterval controls how often we reconcile captions with Mux.
	captionSyncInterval = 10 * time.Minute
	// captionLanguage is the only caption language Hillview stores.
	captionLanguage = "en"
	// captionsPerRun caps Mux calls per phase per tick.
	captionsPerRun = 20
	// captionRequestInterval throttles individual Mux calls.
	captionRequestInterval = 1 * time.Second
)

// StartVideoCaptionSync is the fallback for the Mux track webhooks. Each tick
// it advances captions for Mux videos (actions.SyncMuxCaption):
//
//	Phase 1 (trigger): Mux videos with no caption row → generation is requested
//	  when the asset has no English track; the row becomes "inprogress"
//	  (or "ready" straight away when the track already exists).
//	Phase 2 (collect): rows still "inprogress" → the VTT and a plain-text
//	  transcript are stored once Mux's track is ready; "error" when it failed.
//
// Mux owns transcription (Whisper); this job persists the result so the
// transcript is available for SEO and the caption editor.
func StartVideoCaptionSync(ctx context.Context) {
	ticker := time.NewTicker(captionSyncInterval)
	defer ticker.Stop()

	reconcileCaptions()

	for {
		select {
		case <-ticker.C:
			reconcileCaptions()
		case <-ctx.Done():
			log.Println("🚦 Stopping video caption sync...")
			return
		}
	}
}

func reconcileCaptions() {
	// Phase 1: Mux videos that have never had a caption row.
	syncMuxCaptions(captionTargets(
		`SELECT v.id, v.url
		   FROM videos v
		   LEFT JOIN video_captions vc
		     ON vc.video_id = v.id AND vc.language = ?
		  WHERE vc.id IS NULL
		    AND v.status <> 4
		    AND `+streamurl.MuxSQL+`
		  LIMIT ?`,
		"trigger",
	), "trigger")

	// Phase 2: captions Mux is still producing.
	syncMuxCaptions(captionTargets(
		`SELECT v.id, v.url
		   FROM video_captions vc
		   JOIN videos v ON v.id = vc.video_id
		  WHERE vc.language = ? AND vc.status = 'inprogress'
		    AND `+streamurl.MuxSQL+`
		  LIMIT ?`,
		"collect",
	), "collect")
}

type muxTarget struct {
	id  int
	url string
}

// captionTargets runs a query taking (language, limit) and returning (id, url).
func captionTargets(q, stage string) []muxTarget {
	rows, err := db.DB.Query(q, captionLanguage, captionsPerRun)
	if err != nil {
		log.Printf("caption sync: %s query failed: %v", stage, err)
		telemetry.Error(context.Background(), "caption_sync.query.failed", err, map[string]any{"stage": stage})
		return nil
	}
	defer rows.Close()

	var targets []muxTarget
	for rows.Next() {
		var t muxTarget
		if err := rows.Scan(&t.id, &t.url); err != nil {
			continue
		}
		targets = append(targets, t)
	}
	return targets
}

// syncMuxCaptions advances each Mux video's caption one step (see
// actions.SyncMuxCaption). Failures are retried next tick.
func syncMuxCaptions(targets []muxTarget, stage string) {
	stored := 0
	for i, t := range targets {
		if i > 0 {
			time.Sleep(captionRequestInterval)
		}
		status, err := actions.SyncMuxCaption(t.id, t.url)
		if err != nil {
			log.Printf("caption sync: mux %s failed for video %d: %v", stage, t.id, err)
			telemetry.Warn(context.Background(), "caption_sync.mux.failed", err, map[string]any{"video_id": t.id, "stage": stage})
			continue
		}
		if status == actions.CaptionReady {
			stored++
		}
	}
	if stored > 0 {
		log.Printf("caption sync: stored Mux transcripts for %d videos", stored)
	}
}
