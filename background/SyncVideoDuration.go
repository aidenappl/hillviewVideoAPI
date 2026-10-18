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
	"github.com/hillview.tv/videoAPI/mux"
	"github.com/hillview.tv/videoAPI/streamurl"
	"github.com/hillview.tv/videoAPI/telemetry"
)

const (
	// syncInterval controls how often we reconcile local duration against Mux.
	syncInterval = 5 * time.Minute
	// maxPerRun caps how many videos we reconcile per tick.
	maxPerRun = 40
	// requestInterval throttles individual Mux calls.
	requestInterval = 400 * time.Millisecond
)

// StartVideoDurationSync periodically fills videos.duration from Mux. It is
// the fallback for the video.asset.ready webhook: it targets Mux videos whose
// duration IS NULL or 0 — videos saved after the webhook fired, videos whose
// url was edited (the edit path nulls duration), and missed deliveries.
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

// missingOnMux holds videos Mux has no asset for. They are left out of later
// passes — otherwise the same dead rows fill every batch of maxPerRun and newly
// uploaded videos behind them never get a duration — and reported once per
// process instead of on every tick. A restart retries them.
var (
	missingMu    sync.Mutex
	missingOnMux = map[int]bool{}
)

func reconcileVideoDurations() {
	missingMu.Lock()
	skip := make([]any, 0, len(missingOnMux))
	for id := range missingOnMux {
		skip = append(skip, id)
	}
	missingMu.Unlock()

	// Archived videos (status 4) are never shown again.
	q := "SELECT id, url, status FROM videos WHERE (duration IS NULL OR duration = 0) AND " + streamurl.MuxSQL + " AND status <> 4"
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
		id     int
		url    string
		status int
	}
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.url, &t.status); err != nil {
			log.Printf("duration sync: scan failed: %v", err)
			telemetry.Error(context.Background(), "duration_sync.scan.failed", err, nil)
			continue
		}
		targets = append(targets, t)
	}
	rows.Close()

	updated := 0
	for i, t := range targets {
		// Throttle every request (not just successful ones).
		if i > 0 {
			time.Sleep(requestInterval)
		}

		// SyncMuxDuration stores the duration itself; nil while Mux is still
		// processing.
		duration, err := actions.SyncMuxDuration(t.id, t.url)
		if errors.Is(err, mux.ErrNotFound) {
			missingMu.Lock()
			missingOnMux[t.id] = true
			missingMu.Unlock()
			log.Printf("duration sync: video %d not found on Mux; skipping until restart", t.id)
			telemetry.Warn(context.Background(), "duration_sync.mux.missing", err, map[string]any{"video_id": t.id, "video_status": t.status})
			continue
		}
		if err != nil {
			log.Printf("duration sync: video %d mux error: %v", t.id, err)
			telemetry.Warn(context.Background(), "duration_sync.mux.failed", err, map[string]any{"video_id": t.id})
			continue
		}
		if duration != nil {
			updated++
		}
	}

	if updated > 0 {
		log.Printf("duration sync: updated %d/%d videos", updated, len(targets))
	}
}
