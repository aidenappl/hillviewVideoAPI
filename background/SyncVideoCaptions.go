package background

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/hillview.tv/videoAPI/actions"
	"github.com/hillview.tv/videoAPI/db"
)

const (
	// captionSyncInterval controls how often we reconcile captions with Cloudflare.
	captionSyncInterval = 10 * time.Minute
	// captionLanguage — Cloudflare generated captions only support English in beta.
	captionLanguage = "en"
	// maxCaptionDurationSec — Cloudflare can't generate captions for videos > 2h.
	maxCaptionDurationSec = 7200
	// captionsPerRun caps Cloudflare calls per phase per tick.
	captionsPerRun = 20
	// captionRequestInterval throttles individual Cloudflare calls. The generate
	// endpoint (Workers AI) is rate-limited tighter than reads, so pace gently.
	captionRequestInterval = 1 * time.Second
)

// StartVideoCaptionSync drives AI caption generation + transcript capture:
//
//	Phase 1 (trigger): eligible Cloudflare videos (duration known, ≤2h) with no
//	  caption row → POST generate, insert a row as "inprogress".
//	Phase 2 (collect): rows still "inprogress" → poll Cloudflare; when "ready",
//	  download the VTT, store it + a plain-text transcript; on "error", mark it.
//
// Cloudflare owns transcription (Workers AI); this job orchestrates and persists
// the result so the transcript is available for SEO and the player gets captions.
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
	triggerCaptionGeneration()
	collectReadyCaptions()
}

// Phase 1: trigger generation for eligible videos that have no caption row yet.
func triggerCaptionGeneration() {
	rows, err := db.DB.Query(
		`SELECT v.id, v.url
		   FROM videos v
		   LEFT JOIN video_captions vc
		     ON vc.video_id = v.id AND vc.language = ?
		  WHERE vc.id IS NULL
		    AND v.url LIKE '%cloudflare%'
		    AND v.duration IS NOT NULL AND v.duration > 0 AND v.duration <= ?
		  LIMIT ?`,
		captionLanguage, maxCaptionDurationSec, captionsPerRun,
	)
	if err != nil {
		log.Printf("caption sync: trigger query failed: %v", err)
		return
	}

	type target struct {
		id  int
		uid string
	}
	var targets []target
	for rows.Next() {
		var id int
		var url string
		if err := rows.Scan(&id, &url); err != nil {
			continue
		}
		if uid := actions.CloudflareID(url); uid != "" {
			targets = append(targets, target{id: id, uid: uid})
		}
	}
	rows.Close()

	for i, t := range targets {
		if i > 0 {
			time.Sleep(captionRequestInterval)
		}
		err := actions.GenerateCaptions(t.uid, captionLanguage)
		if errors.Is(err, actions.ErrRateLimited) {
			// transient — leave no row so it's retried next tick
			continue
		}
		status := "inprogress"
		if err != nil {
			log.Printf("caption sync: generate failed for video %d: %v", t.id, err)
			status = "error"
		}
		if _, err := db.DB.Exec(
			`INSERT INTO video_captions (video_id, language, status)
			 VALUES (?, ?, ?)
			 ON DUPLICATE KEY UPDATE status = VALUES(status)`,
			t.id, captionLanguage, status,
		); err != nil {
			log.Printf("caption sync: insert row failed for video %d: %v", t.id, err)
		}
	}
	if len(targets) > 0 {
		log.Printf("caption sync: triggered generation for %d videos", len(targets))
	}
}

// Phase 2: poll in-progress captions; store transcript when ready.
func collectReadyCaptions() {
	rows, err := db.DB.Query(
		`SELECT vc.video_id, v.url
		   FROM video_captions vc
		   JOIN videos v ON v.id = vc.video_id
		  WHERE vc.language = ? AND vc.status = 'inprogress'
		  LIMIT ?`,
		captionLanguage, captionsPerRun,
	)
	if err != nil {
		log.Printf("caption sync: collect query failed: %v", err)
		return
	}

	type target struct {
		id  int
		uid string
	}
	var targets []target
	for rows.Next() {
		var id int
		var url string
		if err := rows.Scan(&id, &url); err != nil {
			continue
		}
		if uid := actions.CloudflareID(url); uid != "" {
			targets = append(targets, target{id: id, uid: uid})
		}
	}
	rows.Close()

	collected := 0
	for i, t := range targets {
		if i > 0 {
			time.Sleep(captionRequestInterval)
		}
		status, err := actions.CaptionStatus(t.uid, captionLanguage)
		if err != nil {
			log.Printf("caption sync: status check failed for video %d: %v", t.id, err)
			continue
		}
		switch status {
		case "ready":
			vtt, err := actions.FetchVTT(t.uid, captionLanguage)
			if err != nil {
				log.Printf("caption sync: fetch vtt failed for video %d: %v", t.id, err)
				continue
			}
			plain := actions.VTTToPlainText(vtt)
			if _, err := db.DB.Exec(
				`UPDATE video_captions SET status = 'ready', vtt = ?, plain_text = ?
				 WHERE video_id = ? AND language = ?`,
				vtt, plain, t.id, captionLanguage,
			); err != nil {
				log.Printf("caption sync: save failed for video %d: %v", t.id, err)
				continue
			}
			collected++
		case "error":
			db.DB.Exec(
				`UPDATE video_captions SET status = 'error' WHERE video_id = ? AND language = ?`,
				t.id, captionLanguage,
			)
		default:
			// still inprogress — try again next tick
		}
	}
	if collected > 0 {
		log.Printf("caption sync: stored transcripts for %d videos", collected)
	}
}
