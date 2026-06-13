package background

import (
	"context"
	"log"
	"time"

	"github.com/hillview.tv/videoAPI/actions"
	"github.com/hillview.tv/videoAPI/db"
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

func reconcileVideoDurations() {
	rows, err := db.DB.Query(
		"SELECT id, url FROM videos WHERE (duration IS NULL OR duration = 0) AND url LIKE '%cloudflare%' LIMIT ?",
		maxPerRun,
	)
	if err != nil {
		log.Printf("duration sync: query failed: %v", err)
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
		if err != nil {
			log.Printf("duration sync: video %d cloudflare error: %v", t.id, err)
			continue
		}
		if duration == nil {
			// still processing — try again next pass
			continue
		}
		if _, err := db.DB.Exec("UPDATE videos SET duration = ? WHERE id = ?", *duration, t.id); err != nil {
			log.Printf("duration sync: video %d update failed: %v", t.id, err)
			continue
		}
		updated++
	}

	if updated > 0 {
		log.Printf("duration sync: updated %d/%d videos", updated, len(targets))
	}
}
