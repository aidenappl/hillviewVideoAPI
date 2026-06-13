// Command auditCloudflareVideos lists every video in the Cloudflare Stream
// account and compares it against the hillview `videos` table to find orphans:
//
//	Cloudflare-orphans : exist in Cloudflare but NO DB row references them.
//	                     These cost storage money and are deletion candidates.
//	DB-orphans         : DB rows whose Cloudflare asset no longer exists (broken).
//
// Self-contained — reads only the env vars it needs:
//
//	DATABASE_DSN      same MySQL DSN the API uses
//	CLOUDFLARE_UID    Cloudflare account id
//	CLOUDFLARE_TOKEN  Cloudflare API token (CLOUDFLARE_AUTH_TOKEN also accepted)
//
// Reports by default. Deletion is destructive and OFF unless you pass -delete,
// and never touches videos younger than -min-age-days (default 7) so an
// in-flight upload that hasn't been saved to the DB yet is never destroyed.
//
//	go run ./tools/auditCloudflareVideos                  # report only
//	go run ./tools/auditCloudflareVideos -delete          # delete aged CF-orphans
//	go run ./tools/auditCloudflareVideos -min-age-days 30 # stricter age cutoff
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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// autoUploadName matches the file-upload flow's generated names, e.g.
// "UID12-1643395336-XVIMMWFYFN" or "UID21-1644261582-TXIPZPEHPS.mp4".
// A human-given name (a play title, a .mov/.m4v with words) does NOT match —
// those are treated as named content and protected from bulk deletion.
var autoUploadName = regexp.MustCompile(`^UID\d+-\d+-[A-Z0-9]{6,}(\.[A-Za-z0-9]+)?$`)

func isAutoUploadName(name string) bool {
	return autoUploadName.MatchString(strings.TrimSpace(name))
}

const cfBase = "https://api.cloudflare.com/client/v4/accounts"

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
	client := &http.Client{Timeout: 30 * time.Second}
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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
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
	return 0, nil, fmt.Errorf("rate limited")
}

type cfVideo struct {
	UID      string  `json:"uid"`
	Created  string  `json:"created"`
	Size     int64   `json:"size"`
	Duration float64 `json:"duration"`
	Meta     struct {
		Name string `json:"name"`
	} `json:"meta"`
	Status struct {
		State string `json:"state"`
	} `json:"status"`
}

// listAllCloudflareVideos paginates the Stream list endpoint (asc by created).
func listAllCloudflareVideos() ([]cfVideo, error) {
	seen := map[string]bool{}
	var all []cfVideo
	after := ""
	for {
		url := fmt.Sprintf("%s/%s/stream?asc=true&limit=1000", cfBase, accountUID)
		if after != "" {
			url += "&after=" + after
		}
		status, body, err := cfDo(http.MethodGet, url)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("list status %d: %s", status, strings.TrimSpace(string(body)))
		}
		var parsed struct {
			Result  []cfVideo `json:"result"`
			Success bool      `json:"success"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, err
		}
		added := 0
		for _, v := range parsed.Result {
			if seen[v.UID] {
				continue
			}
			seen[v.UID] = true
			all = append(all, v)
			added++
		}
		if len(parsed.Result) < 1000 || added == 0 {
			break
		}
		after = parsed.Result[len(parsed.Result)-1].Created
	}
	return all, nil
}

func deleteCloudflareVideo(uid string) error {
	url := fmt.Sprintf("%s/%s/stream/%s", cfBase, accountUID, uid)
	status, body, err := cfDo(http.MethodDelete, url)
	if err != nil {
		return err
	}
	if status == http.StatusOK || status == http.StatusNoContent {
		return nil
	}
	return fmt.Errorf("status %d: %s", status, strings.TrimSpace(string(body)))
}

func mib(bytes int64) float64 { return float64(bytes) / (1024 * 1024) }

func main() {
	doDelete := flag.Bool("delete", false, "delete aged AUTO-NAMED Cloudflare-orphans only (DESTRUCTIVE; skips named productions)")
	deleteUIDs := flag.String("delete-uids", "", "comma-separated Cloudflare UIDs to delete explicitly (overrides name/age guards — use for reviewed named videos)")
	minAgeDays := flag.Int("min-age-days", 7, "never bulk-delete Cloudflare videos newer than this many days")
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

	// 1. Build the set of Cloudflare UIDs referenced anywhere in the videos table.
	rows, err := database.Query("SELECT id, url FROM videos")
	if err != nil {
		log.Fatalf("query videos: %v", err)
	}
	dbUIDs := map[string]bool{}
	dbURLByUID := map[string]int{}
	dbCloudflareRows := 0
	for rows.Next() {
		var id int
		var url string
		if err := rows.Scan(&id, &url); err != nil {
			continue
		}
		if uid := cloudflareID(url); uid != "" {
			dbUIDs[uid] = true
			dbURLByUID[uid] = id
			dbCloudflareRows++
		}
	}
	rows.Close()

	// 2. Pull every Cloudflare video.
	cfVideos, err := listAllCloudflareVideos()
	if err != nil {
		log.Fatalf("list cloudflare videos: %v", err)
	}

	// 3. Diff.
	cfUIDs := map[string]bool{}
	var orphans []cfVideo // in Cloudflare, not in DB
	for _, v := range cfVideos {
		cfUIDs[v.UID] = true
		if !dbUIDs[v.UID] {
			orphans = append(orphans, v)
		}
	}
	var dbOrphans []int // DB rows whose Cloudflare asset is gone
	for uid, id := range dbURLByUID {
		if !cfUIDs[uid] {
			dbOrphans = append(dbOrphans, id)
		}
	}
	sort.Ints(dbOrphans)

	cutoff := time.Now().AddDate(0, 0, -*minAgeDays)

	fmt.Println("================ Cloudflare ↔ DB audit ================")
	fmt.Printf("Cloudflare videos:        %d\n", len(cfVideos))
	fmt.Printf("DB rows w/ Cloudflare URL: %d\n", dbCloudflareRows)
	fmt.Printf("Cloudflare-orphans (in CF, not in DB): %d\n", len(orphans))
	fmt.Printf("DB-orphans (in DB, asset gone in CF):  %d  %v\n", len(dbOrphans), dbOrphans)
	fmt.Println("-------------------------------------------------------")

	// Categorize orphans. Only SHORT, aged, auto-named files are bulk-deletable —
	// long auto-named files are likely raw event/play masters and are protected
	// alongside custom-named content (deletable only via explicit -delete-uids).
	const bulkMaxMinutes = 25
	var autoAged []cfVideo   // auto-named, short, aged — safe bulk candidates
	var namedAll []cfVideo   // custom-named — review individually
	var longAuto []cfVideo   // auto-named but long — likely masters, review individually
	var recentAuto []cfVideo // auto-named but too recent
	cfByUID := map[string]cfVideo{}
	var orphanMinutes float64
	var orphanBytes int64

	for _, v := range orphans {
		cfByUID[v.UID] = v
		orphanMinutes += v.Duration / 60
		orphanBytes += v.Size
		created, _ := time.Parse(time.RFC3339, v.Created)
		switch {
		case !isAutoUploadName(v.Meta.Name):
			namedAll = append(namedAll, v)
		case !created.Before(cutoff):
			recentAuto = append(recentAuto, v)
		case v.Duration/60 >= bulkMaxMinutes:
			longAuto = append(longAuto, v)
		default:
			autoAged = append(autoAged, v)
		}
	}

	printGroup := func(title string, vids []cfVideo) {
		var min float64
		var bytes int64
		fmt.Printf("\n### %s (%d)\n", title, len(vids))
		for _, v := range vids {
			created, _ := time.Parse(time.RFC3339, v.Created)
			min += v.Duration / 60
			bytes += v.Size
			fmt.Printf("  %s  state=%-8s  created=%s  %6.1f min  %8.1f MiB  %q\n",
				v.UID, v.Status.State, created.Format("2006-01-02"), v.Duration/60, mib(v.Size), v.Meta.Name)
		}
		if len(vids) > 0 {
			fmt.Printf("  → %.0f min (%.1f GiB) ≈ $%.2f/mo\n", min, float64(bytes)/(1<<30), min/1000*5)
		}
	}

	printGroup("NAMED — review individually, NOT bulk-deleted", namedAll)
	printGroup("LONG auto-named (≥25 min) — likely masters, review individually", longAuto)
	printGroup("SHORT auto-named, aged — bulk-delete candidates (-delete)", autoAged)
	printGroup("AUTO-NAMED, too recent — skipped", recentAuto)

	fmt.Println("\n-------------------------------------------------------")
	fmt.Printf("All orphans: %.0f min (%.1f GiB) ≈ $%.2f/mo at $5/1000 min stored\n",
		orphanMinutes, float64(orphanBytes)/(1<<30), orphanMinutes/1000*5)

	// Explicit reviewed delete by UID list — overrides name/age guards.
	if *deleteUIDs != "" {
		uids := strings.Split(*deleteUIDs, ",")
		fmt.Printf("\n⚠️  Deleting %d explicitly-listed Cloudflare videos (irreversible)...\n", len(uids))
		deleted, failed := 0, 0
		for i, raw := range uids {
			uid := strings.TrimSpace(raw)
			if uid == "" {
				continue
			}
			if i > 0 {
				time.Sleep(300 * time.Millisecond)
			}
			if err := deleteCloudflareVideo(uid); err != nil {
				log.Printf("delete %s failed: %v", uid, err)
				failed++
				continue
			}
			fmt.Printf("  deleted %s (%q)\n", uid, cfByUID[uid].Meta.Name)
			deleted++
		}
		fmt.Printf("done: %d deleted, %d failed\n", deleted, failed)
		return
	}

	if !*doDelete {
		fmt.Println("\nReport only.")
		fmt.Println("  • Bulk-delete the aged AUTO-NAMED orphans:  -delete")
		fmt.Println("  • Delete specific reviewed videos by UID:   -delete-uids uid1,uid2,...")
		fmt.Println("  Named productions are never bulk-deleted — remove those only via -delete-uids.")
		return
	}

	fmt.Printf("\n⚠️  Bulk-deleting %d aged AUTO-NAMED orphans (irreversible; named productions skipped)...\n", len(autoAged))
	deleted, failed := 0, 0
	for i, v := range autoAged {
		if i > 0 {
			time.Sleep(300 * time.Millisecond)
		}
		if err := deleteCloudflareVideo(v.UID); err != nil {
			log.Printf("delete %s failed: %v", v.UID, err)
			failed++
			continue
		}
		fmt.Printf("  deleted %s (%q)\n", v.UID, v.Meta.Name)
		deleted++
	}
	fmt.Printf("done: %d deleted, %d failed\n", deleted, failed)
}
