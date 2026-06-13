// Command backfillDuration is a one-time maintenance tool that populates the
// videos.duration column for existing rows by querying the Cloudflare Stream
// API for each video's cloudflare_id.
//
// It is NOT wired into the server and never runs automatically. It is
// self-contained: it reads only the env vars it needs (no dependency on the
// main app's env/db packages, so you don't need AWS/JWT/Sendgrid vars set).
//
// Required env:
//
//	DATABASE_DSN      same MySQL DSN the API uses
//	CLOUDFLARE_UID    Cloudflare account id
//	CLOUDFLARE_TOKEN  Cloudflare API token with Stream Read (same as the API uses;
//	                  CLOUDFLARE_AUTH_TOKEN is also accepted)
//
// Run:
//
//	go run ./tools/backfillDuration            # apply
//	go run ./tools/backfillDuration -dry-run   # report only, no writes
//
// It is idempotent: it only touches rows where duration IS NULL or 0, and skips
// videos with no resolvable cloudflare_id.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Cloudflare's API rate limit is ~1200 req / 5 min (~4/s). Stay well under it.
const requestInterval = 400 * time.Millisecond

type cloudflareStreamResponse struct {
	Result struct {
		Duration float64 `json:"duration"`
	} `json:"result"`
	Success bool `json:"success"`
}

func mustEnv(key string) string {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		log.Fatalf("missing required env var %s", key)
	}
	return v
}

// cloudflareID derives the Stream UID from a video's stored url, e.g.
// https://customer-xxxx.cloudflarestream.com/{uid}/manifest/video.m3u8
func cloudflareID(url string) string {
	if !strings.Contains(url, "cloudflare") {
		return ""
	}
	parts := strings.Split(url, "/")
	if len(parts) < 3 {
		return ""
	}
	return parts[len(parts)-3]
}

func fetchDuration(client *http.Client, accountUID, token, uid string) (int, error) {
	const maxAttempts = 5
	backoff := 2 * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequest(
			"GET",
			"https://api.cloudflare.com/client/v4/accounts/"+accountUID+"/stream/"+uid,
			nil,
		)
		if err != nil {
			return 0, err
		}
		req.Header.Set("Authorization", "Bearer "+token)

		res, err := client.Do(req)
		if err != nil {
			return 0, err
		}
		payload, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		res.Body.Close()

		// Rate limited — honor Retry-After when present, else exponential backoff.
		if res.StatusCode == http.StatusTooManyRequests {
			wait := backoff
			if ra := res.Header.Get("Retry-After"); ra != "" {
				if secs, e := strconv.Atoi(strings.TrimSpace(ra)); e == nil && secs > 0 {
					wait = time.Duration(secs) * time.Second
				}
			}
			log.Printf("  rate limited (attempt %d/%d), waiting %s", attempt, maxAttempts, wait)
			time.Sleep(wait)
			backoff *= 2
			continue
		}

		if res.StatusCode != http.StatusOK {
			return 0, fmt.Errorf("status %d: %s", res.StatusCode, strings.TrimSpace(string(payload)))
		}

		var body cloudflareStreamResponse
		if err := json.Unmarshal(payload, &body); err != nil {
			return 0, err
		}
		if !body.Success {
			return 0, nil
		}
		return int(body.Result.Duration + 0.5), nil
	}

	return 0, fmt.Errorf("still rate limited after %d attempts", maxAttempts)
}

func main() {
	dryRun := flag.Bool("dry-run", false, "report what would change without writing")
	flag.Parse()

	dsn := mustEnv("DATABASE_DSN")
	accountUID := mustEnv("CLOUDFLARE_UID")
	// The API names this CLOUDFLARE_TOKEN; some .env files use CLOUDFLARE_AUTH_TOKEN.
	token := os.Getenv("CLOUDFLARE_TOKEN")
	if token == "" {
		token = os.Getenv("CLOUDFLARE_AUTH_TOKEN")
	}
	if token == "" {
		log.Fatalf("missing required env var CLOUDFLARE_TOKEN (or CLOUDFLARE_AUTH_TOKEN)")
	}

	database, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()
	if err := database.Ping(); err != nil {
		log.Fatalf("connect database: %v", err)
	}

	rows, err := database.Query(
		"SELECT id, url FROM videos WHERE (duration IS NULL OR duration = 0) AND url LIKE '%cloudflare%'",
	)
	if err != nil {
		log.Fatalf("query videos: %v", err)
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
			rows.Close()
			log.Fatalf("scan row: %v", err)
		}
		if uid := cloudflareID(url); uid != "" {
			targets = append(targets, target{id: id, uid: uid})
		}
	}
	rows.Close()

	log.Printf("found %d videos to backfill (dry-run=%v)", len(targets), *dryRun)

	client := &http.Client{Timeout: 15 * time.Second}
	updated, skipped, failed := 0, 0, 0

	for i, t := range targets {
		// Throttle every request (not just successful ones) to stay under the
		// Cloudflare rate limit. No need to wait before the very first call.
		if i > 0 {
			time.Sleep(requestInterval)
		}

		duration, err := fetchDuration(client, accountUID, token, t.uid)
		if err != nil {
			log.Printf("video %d (%s): cloudflare error: %v", t.id, t.uid, err)
			failed++
			continue
		}
		if duration <= 0 {
			log.Printf("video %d (%s): no duration reported, skipping", t.id, t.uid)
			skipped++
			continue
		}
		if *dryRun {
			log.Printf("video %d: would set duration = %ds", t.id, duration)
			updated++
			continue
		}
		if _, err := database.Exec("UPDATE videos SET duration = ? WHERE id = ?", duration, t.id); err != nil {
			log.Printf("video %d: update failed: %v", t.id, err)
			failed++
			continue
		}
		log.Printf("video %d: duration = %ds", t.id, duration)
		updated++
	}

	log.Printf("done: %d %s, %d skipped, %d failed",
		updated, map[bool]string{true: "would update", false: "updated"}[*dryRun], skipped, failed)
}
