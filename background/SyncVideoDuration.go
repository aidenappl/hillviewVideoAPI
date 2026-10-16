package background

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/hillview.tv/videoAPI/actions"
	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/telemetry"
)

const (
	// syncInterval controls how often we reconcile local duration against Cloudflare.
	syncInterval = 5 * time.Minute
	// maxPerRun caps how many videos we reconcile per tick so we never burst the
	// Cloudflare API (large backlogs are cleared with the standalone backfill tool).
	maxPerRun = 40
	// requestInterval throttles individual Cloudflare calls (~2.5/s, well under the limit).
	requestInterval = 400 * time.Millisecond
)

// StartVideoDurationSync periodically reconciles videos.duration with Cloudflare.
// It targets rows where duration IS NULL or 0 and the url is a Cloudflare Stream
// url — which covers freshly created videos, videos whose url was edited (the
// edit path nulls duration), and videos that were still processing on the last
// pass (Cloudflare reports 0 until processing finishes). Cloudflare is the
// source of truth; this job keeps the local column in agreement with it.
func StartVideoDurationSync(ctx context.Context) {
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()

	// Run once shortly after boot, then on every tick.
	reconcileVideoDurations()

	for {
		select {
		case <-ticker.C:
			reconcileVideoDurations()
		case <-ctx.Done():
			log.Println("🚦 Stopping video duration sync...")
			return
		}
	}
}

// missingOnCloudflare holds videos Cloudflare answered 404 for. They are
// left out of later passes — otherwise the same dead rows fill every batch of
// maxPerRun and newly uploaded videos behind them never get a duration — and
// reported once per process instead of on every tick. A restart retries them.
var (
	missingMu           sync.Mutex
	missingOnCloudflare = map[int]bool{}
)

func reconcileVideoDurations() {
	missingMu.Lock()
	skip := make([]any, 0, len(missingOnCloudflare))
	for id := range missingOnCloudflare {
		skip = append(skip, id)
	}
	missingMu.Unlock()

	q := "SELECT id, url FROM videos WHERE (duration IS NULL OR duration = 0) AND url LIKE '%cloudflare%'"
	args := []any{}
	if len(skip) > 0 {
		q += " AND id NOT IN (?" + strings.Repeat(",?", len(skip)-1) + ")"
		args = append(args, skip...)
	}
	q += " ORDER BY id DESC LIMIT ?"
	args = append(args, maxPerRun)

	rows, err := db.DB.Query(q, args...)
	if err != nil {
		log.Printf("duration sync: query failed: %v", err)
		telemetry.Error(context.Background(), "duration_sync.query.failed", err, nil)
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
			log.Printf("duration sync: scan failed: %v", err)
			telemetry.Error(context.Background(), "duration_sync.scan.failed", err, nil)
			continue
		}
		if uid := actions.CloudflareID(url); uid != "" {
			targets = append(targets, target{id: id, uid: uid})
		}
	}
	rows.Close()

	if len(targets) == 0 {
		return
	}

	updated := 0
	for i, t := range targets {
		// Throttle every request (not just successful ones) to stay under the limit.
		if i > 0 {
			time.Sleep(requestInterval)
		}

		duration, err := actions.GetCloudflareDuration(t.uid)
		if errors.Is(err, actions.ErrStreamNotFound) {
			missingMu.Lock()
			missingOnCloudflare[t.id] = true
			missingMu.Unlock()
			log.Printf("duration sync: video %d not found on Cloudflare Stream; skipping until restart", t.id)
			telemetry.Warn(context.Background(), "duration_sync.cloudflare.missing", err, map[string]any{"video_id": t.id, "cloudflare_uid": t.uid})
			continue
		}
		if err != nil {
			log.Printf("duration sync: video %d cloudflare error: %v", t.id, err)
			telemetry.Warn(context.Background(), "duration_sync.cloudflare.failed", err, map[string]any{"video_id": t.id})
			continue
		}
		if duration == nil {
			// still processing — try again next pass
			continue
		}
		if _, err := db.DB.Exec("UPDATE videos SET duration = ? WHERE id = ?", *duration, t.id); err != nil {
			log.Printf("duration sync: video %d update failed: %v", t.id, err)
			telemetry.Error(context.Background(), "duration_sync.update.failed", err, map[string]any{"video_id": t.id})
			continue
		}
		updated++
	}

	if updated > 0 {
		log.Printf("duration sync: updated %d/%d videos", updated, len(targets))
	}
}
