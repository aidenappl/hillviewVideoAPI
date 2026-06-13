// Command backfillCaptions is a one-time / on-demand maintenance tool that drives
// Cloudflare Stream AI caption generation for existing videos and stores the
// resulting transcript in the video_captions table.
//
// It mirrors the in-app background job (background/SyncVideoCaptions.go) but is
// self-contained: it reads only the env vars it needs (no dependency on the
// app's env/db/actions packages).
//
// Required env:
//
//	DATABASE_DSN      same MySQL DSN the API uses
//	CLOUDFLARE_UID    Cloudflare account id
//	CLOUDFLARE_TOKEN  Cloudflare API token (CLOUDFLARE_AUTH_TOKEN also accepted)
//
// Run (captions are async — generation takes minutes, so run once to trigger,
// then again later to collect the finished transcripts; it's idempotent):
//
//	go run ./tools/backfillCaptions            # trigger + collect
//	go run ./tools/backfillCaptions -dry-run   # report only, no writes/calls that mutate
//
// Cloudflare generated captions: English only, videos must be < 2 hours.
package main

import (
	"database/sql"
	"encoding/json"
	"errors"
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

const (
	language        = "en"
	maxDurationSec  = 7200
	requestInterval = 1 * time.Second // generate endpoint is rate-limited tighter
	cfBase          = "https://api.cloudflare.com/client/v4/accounts"
)

// errRateLimited signals a transient 429 that survived all retries — retry later,
// don't treat as a permanent failure.
var errRateLimited = errors.New("rate limited")

var (
	accountUID string
	token      string
)

func mustEnv(key string) string {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		log.Fatalf("missing required env var %s", key)
	}
	return v
}

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

func cfDo(method, url string) (int, []byte, error) {
	const maxAttempts = 6
	backoff := 2 * time.Second
	client := &http.Client{Timeout: 20 * time.Second}
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequest(method, url, nil)
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err != nil {
			return 0, nil, err
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 12<<20))
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			wait := backoff
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, e := strconv.Atoi(strings.TrimSpace(ra)); e == nil && secs > 0 {
					wait = time.Duration(secs) * time.Second
				}
			}
			time.Sleep(wait)
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		return resp.StatusCode, body, nil
	}
	return 0, nil, errRateLimited
}

func generate(uid string) error {
	url := fmt.Sprintf("%s/%s/stream/%s/captions/%s/generate", cfBase, accountUID, uid, language)
	status, body, err := cfDo(http.MethodPost, url)
	if err != nil {
		return err
	}
	if status == http.StatusOK || status == http.StatusConflict {
		return nil
	}
	return fmt.Errorf("status %d: %s", status, strings.TrimSpace(string(body)))
}

func captionStatus(uid string) (string, error) {
	url := fmt.Sprintf("%s/%s/stream/%s/captions", cfBase, accountUID, uid)
	status, body, err := cfDo(http.MethodGet, url)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("status %d: %s", status, strings.TrimSpace(string(body)))
	}
	var parsed struct {
		Result []struct {
			Language string `json:"language"`
			Status   string `json:"status"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	for _, c := range parsed.Result {
		if c.Language == language {
			return c.Status, nil
		}
	}
	return "", nil
}

func fetchVTT(uid string) (string, error) {
	url := fmt.Sprintf("%s/%s/stream/%s/captions/%s/vtt", cfBase, accountUID, uid, language)
	status, body, err := cfDo(http.MethodGet, url)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("status %d: %s", status, strings.TrimSpace(string(body)))
	}
	return string(body), nil
}

func vttToPlainText(vtt string) string {
	lines := strings.Split(strings.ReplaceAll(vtt, "\r\n", "\n"), "\n")
	var out []string
	var last string
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || line == "WEBVTT" || strings.Contains(line, "-->") || strings.HasPrefix(line, "NOTE") {
			continue
		}
		if _, err := strconv.Atoi(line); err == nil {
			continue
		}
		if line == last {
			continue
		}
		out = append(out, line)
		last = line
	}
	return strings.Join(out, " ")
}

func main() {
	dryRun := flag.Bool("dry-run", false, "report what would happen without calling Cloudflare or writing")
	flag.Parse()

	dsn := mustEnv("DATABASE_DSN")
	accountUID = mustEnv("CLOUDFLARE_UID")
	token = os.Getenv("CLOUDFLARE_TOKEN")
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

	triggerPhase(database, *dryRun)
	collectPhase(database, *dryRun)
}

type target struct {
	id  int
	uid string
}

func loadTargets(rows *sql.Rows) []target {
	var targets []target
	for rows.Next() {
		var id int
		var url string
		if err := rows.Scan(&id, &url); err != nil {
			continue
		}
		if uid := cloudflareID(url); uid != "" {
			targets = append(targets, target{id: id, uid: uid})
		}
	}
	rows.Close()
	return targets
}

func triggerPhase(database *sql.DB, dry bool) {
	// Pick videos with no caption row yet, plus any previously marked 'error'
	// (e.g. an earlier transient failure) so re-runs heal them.
	rows, err := database.Query(
		`SELECT v.id, v.url FROM videos v
		   LEFT JOIN video_captions vc ON vc.video_id = v.id AND vc.language = ?
		  WHERE (vc.id IS NULL OR vc.status = 'error') AND v.url LIKE '%cloudflare%'
		    AND v.duration IS NOT NULL AND v.duration > 0 AND v.duration <= ?`,
		language, maxDurationSec,
	)
	if err != nil {
		log.Fatalf("trigger query: %v", err)
	}
	targets := loadTargets(rows)
	log.Printf("phase 1 (generate): %d videos to (re)trigger (dry-run=%v)", len(targets), dry)

	triggered, skipped := 0, 0
	for i, t := range targets {
		if dry {
			log.Printf("video %d: would generate captions", t.id)
			continue
		}
		if i > 0 {
			time.Sleep(requestInterval)
		}
		err := generate(t.uid)
		if errors.Is(err, errRateLimited) {
			// transient — leave the row untouched so the next run retries it
			log.Printf("video %d: rate limited, will retry on next run", t.id)
			skipped++
			continue
		}
		status := "inprogress"
		if err != nil {
			log.Printf("video %d: generate failed: %v", t.id, err)
			status = "error"
		}
		if _, err := database.Exec(
			`INSERT INTO video_captions (video_id, language, status) VALUES (?, ?, ?)
			 ON DUPLICATE KEY UPDATE status = VALUES(status)`,
			t.id, language, status,
		); err != nil {
			log.Printf("video %d: insert failed: %v", t.id, err)
			continue
		}
		if status == "inprogress" {
			triggered++
		}
	}
	log.Printf("phase 1 done: triggered %d, rate-limited (will retry) %d", triggered, skipped)
}

func collectPhase(database *sql.DB, dry bool) {
	rows, err := database.Query(
		`SELECT vc.video_id, v.url FROM video_captions vc
		   JOIN videos v ON v.id = vc.video_id
		  WHERE vc.language = ? AND vc.status = 'inprogress'`,
		language,
	)
	if err != nil {
		log.Fatalf("collect query: %v", err)
	}
	targets := loadTargets(rows)
	log.Printf("phase 2 (collect): %d captions in progress (dry-run=%v)", len(targets), dry)
	if dry {
		return
	}

	stored, pending, failed := 0, 0, 0
	for i, t := range targets {
		if i > 0 {
			time.Sleep(requestInterval)
		}
		status, err := captionStatus(t.uid)
		if err != nil {
			log.Printf("video %d: status check failed: %v", t.id, err)
			failed++
			continue
		}
		switch status {
		case "ready":
			vtt, err := fetchVTT(t.uid)
			if err != nil {
				log.Printf("video %d: fetch vtt failed: %v", t.id, err)
				failed++
				continue
			}
			plain := vttToPlainText(vtt)
			if _, err := database.Exec(
				`UPDATE video_captions SET status='ready', vtt=?, plain_text=? WHERE video_id=? AND language=?`,
				vtt, plain, t.id, language,
			); err != nil {
				log.Printf("video %d: save failed: %v", t.id, err)
				failed++
				continue
			}
			log.Printf("video %d: transcript stored (%d chars)", t.id, len(plain))
			stored++
		case "error":
			database.Exec(`UPDATE video_captions SET status='error' WHERE video_id=? AND language=?`, t.id, language)
			log.Printf("video %d: cloudflare reported error", t.id)
			failed++
		default:
			pending++
		}
	}
	log.Printf("phase 2 done: %d stored, %d still processing, %d failed", stored, pending, failed)
}
