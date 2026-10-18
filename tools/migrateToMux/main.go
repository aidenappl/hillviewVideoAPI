// Command migrateToMux moves the video library from Cloudflare Stream to Mux.
//
// Videos are grouped by Stream uid (several rows can share one Stream video,
// e.g. 62/63); each group becomes one Mux asset.
//
// Live groups (any row not archived):
//
//  1. pick a source Mux can fetch: the S3 original on content.hillview.tv (a
//     row's download_url, or Stream's meta["downloaded-from"]), else Stream's
//     own MP4 download (requested on the first -apply run, used on a later one)
//  2. create the asset (basic, 1080p, highest MP4; English captions only when
//     no row has a stored caption) and store mux_asset_id / mux_playback_id on
//     the rows — they keep playing from Stream meanwhile
//  3. once the asset is ready: re-upload the stored caption as a text track,
//     then swap each row: legacy_url = old url, url = stream.mux.com,
//     download_url = the Mux MP4, Stream thumbnails → image.mux.com. Rows
//     with no stored caption are set "inprogress" for the caption sync.
//
// Archived groups (every row status 4), with -archive: the MP4 is kept in
// S3 — an existing content.hillview.tv download_url, or Stream's MP4 copied to
// content.hillview.tv/archive/{uid}.mp4 — and the rows become
// legacy_url = old url, url = the S3 MP4. They are not uploaded to Mux.
//
// Nothing is changed without -apply. Re-running resumes: in-flight assets are
// polled, pending Stream MP4s are picked up. Stream itself is never modified
// apart from requesting MP4 downloads; deleting Stream videos is a separate step.
//
//	go run ./tools/migrateToMux                         # plan (dry run)
//	go run ./tools/migrateToMux -apply -limit 3         # pilot: 3 new assets
//	go run ./tools/migrateToMux -apply                  # everything
//	go run ./tools/migrateToMux -archive                # plan the archive
//	go run ./tools/migrateToMux -archive -apply         # copy archived MP4s to S3
//
// Env: DATABASE_DSN, CLOUDFLARE_UID, CLOUDFLARE_TOKEN, MUX_TOKEN_ID,
// MUX_TOKEN_SECRET, PUBLIC_API_URL (default https://api.hillview.tv/video/v1.1),
// and AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY for -archive -apply.
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
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	_ "github.com/go-sql-driver/mysql"
	"github.com/hillview.tv/videoAPI/mux"
	"github.com/hillview.tv/videoAPI/streamurl"
)

const (
	CONTENT_HOST          = "content.hillview.tv"
	CONTENT_BUCKET        = "content.hillview.tv"
	CONTENT_BUCKET_REGION = "us-west-1"
	ARCHIVE_PREFIX        = "archive/"
	STATUS_ARCHIVED       = 4
)

type row struct {
	ID          int
	UUID        string
	Title       string
	Status      int
	URL         string
	DownloadURL string
	Thumbnail   string
	Duration    int
	AssetID     string
	PlaybackID  string
	Caption     string // ready, inprogress, error or ""
}

type group struct {
	UID  string
	Rows []row
}

func (g group) archived() bool {
	for _, r := range g.Rows {
		if r.Status != STATUS_ARCHIVED {
			return false
		}
	}
	return true
}

func (g group) assetID() string {
	for _, r := range g.Rows {
		if r.AssetID != "" {
			return r.AssetID
		}
	}
	return ""
}

// captionRow is the row whose stored caption is re-uploaded to Mux, or nil.
func (g group) captionRow() *row {
	for i := range g.Rows {
		if g.Rows[i].Caption == "ready" {
			return &g.Rows[i]
		}
	}
	return nil
}

func (g group) ids() []any {
	ids := make([]any, len(g.Rows))
	for i, r := range g.Rows {
		ids[i] = r.ID
	}
	return ids
}

type tool struct {
	db        *sql.DB
	mux       *mux.Client
	cfAccount string
	cfToken   string
	publicAPI string
	apply     bool
	http      *http.Client
}

func main() {
	apply := flag.Bool("apply", false, "make changes (default: plan only)")
	archive := flag.Bool("archive", false, "handle archived videos (S3 copies) instead of live ones")
	limit := flag.Int("limit", 0, "create at most this many new Mux assets this run (0 = all)")
	wait := flag.Duration("wait", 2*time.Hour, "how long to wait for in-flight assets before exiting")
	only := flag.String("uid", "", "only this Stream uid")
	flag.Parse()

	t := &tool{
		cfAccount: mustEnv("CLOUDFLARE_UID"),
		cfToken:   envOr("CLOUDFLARE_TOKEN", os.Getenv("CLOUDFLARE_AUTH_TOKEN")),
		publicAPI: envOr("PUBLIC_API_URL", "https://api.hillview.tv/video/v1.1"),
		apply:     *apply,
		http:      &http.Client{Timeout: 30 * time.Second},
	}
	if t.cfToken == "" {
		log.Fatal("CLOUDFLARE_TOKEN is not set")
	}
	db, err := sql.Open("mysql", mustEnv("DATABASE_DSN"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	t.db = db

	groups, err := t.loadGroups()
	if err != nil {
		log.Fatal(err)
	}
	if *only != "" {
		var kept []group
		for _, g := range groups {
			if g.UID == *only {
				kept = append(kept, g)
			}
		}
		groups = kept
	}

	if !*apply {
		fmt.Println("DRY RUN — nothing will change. Pass -apply to migrate.")
	}

	if *archive {
		t.runArchive(groups)
		return
	}

	t.mux = mux.New(os.Getenv("MUX_TOKEN_ID"), os.Getenv("MUX_TOKEN_SECRET"))
	if *apply && (t.mux.TokenID == "" || t.mux.TokenSecret == "") {
		log.Fatal("MUX_TOKEN_ID / MUX_TOKEN_SECRET are not set")
	}
	t.runLive(groups, *limit, *wait)
}

// loadGroups reads every row that still plays from Stream, grouped by uid.
func (t *tool) loadGroups() ([]group, error) {
	rows, err := t.db.Query(`
		SELECT v.id, v.uuid, v.title, v.status, v.url,
		       IFNULL(v.download_url, ''), IFNULL(v.thumbnail, ''), IFNULL(v.duration, 0),
		       IFNULL(v.mux_asset_id, ''), IFNULL(v.mux_playback_id, ''), IFNULL(vc.status, '')
		  FROM videos v
		  LEFT JOIN video_captions vc ON vc.video_id = v.id AND vc.language = 'en'
		 WHERE ` + streamurl.Qualified("v") + `
		 ORDER BY v.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byUID := map[string]*group{}
	var order []string
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ID, &r.UUID, &r.Title, &r.Status, &r.URL, &r.DownloadURL, &r.Thumbnail, &r.Duration, &r.AssetID, &r.PlaybackID, &r.Caption); err != nil {
			return nil, err
		}
		uid := streamurl.ID(r.URL)
		if uid == "" {
			continue
		}
		g, ok := byUID[uid]
		if !ok {
			g = &group{UID: uid}
			byUID[uid] = g
			order = append(order, uid)
		}
		g.Rows = append(g.Rows, r)
	}
	groups := make([]group, 0, len(order))
	for _, uid := range order {
		groups = append(groups, *byUID[uid])
	}
	return groups, rows.Err()
}

// ─── Live groups → Mux ──────────────────────────────────────────────────────

type source struct {
	Kind string // s3, stream_mp4, stream_mp4_pending, missing
	URL  string
}

func (t *tool) runLive(groups []group, limit int, wait time.Duration) {
	counts := map[string]int{}
	minutes := map[string]int{}
	var missing, pending []string
	var inflight []group
	created := 0

	for _, g := range groups {
		if g.archived() {
			continue
		}
		if g.assetID() != "" {
			inflight = append(inflight, g)
			counts["in_flight"]++
			continue
		}

		src, err := t.chooseSource(g)
		if err != nil {
			log.Printf("uid %s: %v", g.UID, err)
			counts["error"]++
			continue
		}
		counts[src.Kind]++
		minutes[src.Kind] += g.Rows[0].Duration / 60
		label := fmt.Sprintf("%s (videos %s)", g.UID, idList(g))
		switch src.Kind {
		case "missing":
			missing = append(missing, label)
			continue
		case "stream_mp4_pending":
			pending = append(pending, label)
			continue
		}

		if !t.apply || (limit > 0 && created >= limit) {
			continue
		}
		asset, err := t.createAsset(g, src)
		if err != nil {
			log.Printf("uid %s: create asset: %v", g.UID, err)
			counts["error"]++
			continue
		}
		created++
		for i := range g.Rows {
			g.Rows[i].AssetID = asset.ID
			g.Rows[i].PlaybackID = asset.PublicPlaybackID()
		}
		inflight = append(inflight, g)
		log.Printf("uid %s → asset %s from %s", g.UID, asset.ID, src.Kind)
		time.Sleep(300 * time.Millisecond)
	}

	fmt.Println()
	fmt.Println("Live videos (by Stream video):")
	for _, k := range []string{"s3", "stream_mp4", "stream_mp4_pending", "missing", "in_flight", "error"} {
		if counts[k] > 0 {
			fmt.Printf("  %-20s %4d  (~%d min)\n", k, counts[k], minutes[k])
		}
	}
	if len(pending) > 0 {
		verb := "would be requested"
		if t.apply {
			verb = "requested; re-run once Stream has prepared them"
		}
		fmt.Printf("\nStream MP4s %s (%d):\n  %s\n", verb, len(pending), strings.Join(pending, "\n  "))
	}
	if len(missing) > 0 {
		fmt.Printf("\nMissing — gone from Stream and no S3 original (%d), left untouched:\n  %s\n", len(missing), strings.Join(missing, "\n  "))
	}
	if t.apply {
		fmt.Printf("\nCreated %d new assets this run.\n", created)
		t.finish(inflight, wait)
	}
}

// chooseSource finds a file Mux can ingest for a group. On -apply it also asks
// Stream to prepare an MP4 when there is nothing else.
func (t *tool) chooseSource(g group) (source, error) {
	for _, r := range g.Rows {
		if isContentVideo(r.DownloadURL) && t.reachable(r.DownloadURL) {
			return source{Kind: "s3", URL: r.DownloadURL}, nil
		}
	}

	info, err := t.streamVideo(g.UID)
	if errors.Is(err, errStreamNotFound) {
		return source{Kind: "missing"}, nil
	}
	if err != nil {
		return source{}, err
	}
	if from := info.Meta["downloaded-from"]; isContentVideo(from) && t.reachable(from) {
		return source{Kind: "s3", URL: from}, nil
	}

	mp4, ready, err := t.streamDownload(g.UID)
	if err != nil {
		return source{}, err
	}
	if ready {
		return source{Kind: "stream_mp4", URL: mp4}, nil
	}
	if t.apply {
		if err := t.requestStreamDownload(g.UID); err != nil {
			return source{}, err
		}
	}
	return source{Kind: "stream_mp4_pending"}, nil
}

func (t *tool) createAsset(g group, src source) (*mux.Asset, error) {
	captions := g.captionRow() == nil
	settings := mux.NewAssetSettings(src.URL, "stream:"+g.UID, captions)
	asset, err := t.mux.CreateAsset(settings)
	if err != nil {
		return nil, err
	}
	q := "UPDATE videos SET mux_asset_id = ?, mux_playback_id = ? WHERE id IN (?" + strings.Repeat(",?", len(g.Rows)-1) + ")"
	args := append([]any{asset.ID, asset.PublicPlaybackID()}, g.ids()...)
	if _, err := t.db.Exec(q, args...); err != nil {
		return nil, fmt.Errorf("store asset ids: %w", err)
	}
	return asset, nil
}

// finish polls in-flight assets and swaps their rows over as each is ready.
func (t *tool) finish(inflight []group, wait time.Duration) {
	if len(inflight) == 0 {
		return
	}
	fmt.Printf("Waiting for %d assets (up to %s)...\n", len(inflight), wait)
	deadline := time.Now().Add(wait)
	swapped, failed := 0, 0
	for len(inflight) > 0 && time.Now().Before(deadline) {
		var still []group
		for _, g := range inflight {
			asset, err := t.mux.GetAsset(g.assetID())
			if err != nil {
				log.Printf("uid %s: get asset: %v", g.UID, err)
				still = append(still, g)
				continue
			}
			switch asset.Status {
			case "ready":
				if err := t.swap(g, asset); err != nil {
					log.Printf("uid %s: swap: %v", g.UID, err)
					failed++
					continue
				}
				swapped++
				log.Printf("uid %s: videos %s now play from Mux (%s)", g.UID, idList(g), asset.PublicPlaybackID())
			case "errored":
				failed++
				log.Printf("uid %s: Mux could not ingest the asset; clearing its ids so a re-run retries", g.UID)
				t.db.Exec("UPDATE videos SET mux_asset_id = NULL, mux_playback_id = NULL WHERE mux_asset_id = ?", asset.ID)
			default:
				still = append(still, g)
			}
			time.Sleep(200 * time.Millisecond)
		}
		inflight = still
		if len(inflight) > 0 {
			time.Sleep(20 * time.Second)
		}
	}
	fmt.Printf("\nSwapped %d, failed %d, still processing %d (re-run to continue).\n", swapped, failed, len(inflight))
}

// swap re-uploads the stored caption, then points the group's rows at Mux in
// one transaction.
func (t *tool) swap(g group, asset *mux.Asset) error {
	pid := asset.PublicPlaybackID()
	if pid == "" {
		return errors.New("asset has no public playback id")
	}

	if cr := g.captionRow(); cr != nil && asset.TextTrack(mux.CaptionLanguage) == nil {
		vttURL := t.publicAPI + "/captions/" + cr.UUID + "/" + mux.CaptionLanguage + ".vtt"
		if _, err := t.mux.CreateTextTrack(asset.ID, vttURL, mux.CaptionLanguage, "English"); err != nil {
			return fmt.Errorf("upload caption: %w", err)
		}
	}

	tx, err := t.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, r := range g.Rows {
		duration := r.Duration
		if duration <= 0 {
			if d := asset.DurationSeconds(); d != nil {
				duration = *d
			}
		}
		if _, err := tx.Exec(
			`UPDATE videos
			    SET legacy_url = url, url = ?, download_url = ?, thumbnail = ?,
			        duration = NULLIF(?, 0), mux_asset_id = ?, mux_playback_id = ?
			  WHERE id = ? AND url = ?`,
			streamurl.MuxURL(pid), mux.DownloadURL(pid, ""), muxThumbnail(r.Thumbnail, pid),
			duration, asset.ID, pid, r.ID, r.URL,
		); err != nil {
			return err
		}
		if r.Caption != "ready" {
			// The caption sync collects Mux's generated (or the sibling's
			// uploaded) track for this row.
			if _, err := tx.Exec(
				`INSERT INTO video_captions (video_id, language, status) VALUES (?, 'en', 'inprogress')
				 ON DUPLICATE KEY UPDATE status = 'inprogress', vtt = NULL, plain_text = NULL`,
				r.ID,
			); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// ─── Archived groups → S3 ───────────────────────────────────────────────────

func (t *tool) runArchive(groups []group) {
	var have, copyNow, pendingDL, missing []string
	var uploader *s3manager.Uploader
	if t.apply {
		sess, err := session.NewSession(aws.NewConfig().WithRegion(CONTENT_BUCKET_REGION).WithS3ForcePathStyle(true))
		if err != nil {
			log.Fatal(err)
		}
		uploader = s3manager.NewUploader(sess, func(u *s3manager.Uploader) { u.PartSize = 64 << 20 })
	}

	for _, g := range groups {
		if !g.archived() {
			continue
		}
		label := fmt.Sprintf("%s (videos %s)", g.UID, idList(g))

		archiveURL := ""
		for _, r := range g.Rows {
			if isContentVideo(r.DownloadURL) && t.reachable(r.DownloadURL) {
				archiveURL = r.DownloadURL
				break
			}
		}
		if archiveURL != "" {
			have = append(have, label)
			if t.apply {
				t.pointArchived(g, archiveURL)
			}
			continue
		}

		info, err := t.streamVideo(g.UID)
		if errors.Is(err, errStreamNotFound) {
			missing = append(missing, label)
			continue
		}
		if err != nil {
			log.Printf("uid %s: %v", g.UID, err)
			continue
		}
		if from := info.Meta["downloaded-from"]; isContentVideo(from) && t.reachable(from) {
			have = append(have, label)
			if t.apply {
				t.pointArchived(g, from)
			}
			continue
		}

		mp4, ready, err := t.streamDownload(g.UID)
		if err != nil {
			log.Printf("uid %s: %v", g.UID, err)
			continue
		}
		if !ready {
			pendingDL = append(pendingDL, label)
			if t.apply {
				if err := t.requestStreamDownload(g.UID); err != nil {
					log.Printf("uid %s: request download: %v", g.UID, err)
				}
			}
			continue
		}
		copyNow = append(copyNow, label)
		if !t.apply {
			continue
		}
		key := ARCHIVE_PREFIX + g.UID + ".mp4"
		if err := t.copyToS3(uploader, mp4, key); err != nil {
			log.Printf("uid %s: copy to S3: %v", g.UID, err)
			continue
		}
		t.pointArchived(g, "https://"+CONTENT_HOST+"/"+key)
	}

	fmt.Println("\nArchived videos (by Stream video):")
	fmt.Printf("  already in S3        %4d\n  copy Stream MP4      %4d\n  Stream MP4 pending   %4d\n  missing (nothing)    %4d\n", len(have), len(copyNow), len(pendingDL), len(missing))
	if len(missing) > 0 {
		fmt.Printf("\nGone from Stream with no S3 copy — nothing to keep, left untouched:\n  %s\n", strings.Join(missing, "\n  "))
	}
	if len(pendingDL) > 0 {
		fmt.Printf("\nStream MP4s pending (re-run once ready):\n  %s\n", strings.Join(pendingDL, "\n  "))
	}
}

func (t *tool) pointArchived(g group, archiveURL string) {
	q := "UPDATE videos SET legacy_url = url, url = ? WHERE id IN (?" + strings.Repeat(",?", len(g.Rows)-1) + ") AND " + streamurl.SQL
	args := append([]any{archiveURL}, g.ids()...)
	if _, err := t.db.Exec(q, args...); err != nil {
		log.Printf("uid %s: point at archive: %v", g.UID, err)
		return
	}
	log.Printf("uid %s: videos %s → %s", g.UID, idList(g), archiveURL)
}

func (t *tool) copyToS3(uploader *s3manager.Uploader, src, key string) error {
	client := &http.Client{} // no timeout: files are large
	resp, err := client.Get(src)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", src, resp.StatusCode)
	}
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket:      aws.String(CONTENT_BUCKET),
		Key:         aws.String(key),
		Body:        resp.Body,
		ContentType: aws.String("video/mp4"),
	})
	return err
}

// ─── Cloudflare Stream ──────────────────────────────────────────────────────

var errStreamNotFound = errors.New("not found on Cloudflare Stream")

type streamInfo struct {
	Meta map[string]string `json:"meta"`
}

func (t *tool) streamVideo(uid string) (*streamInfo, error) {
	var out struct {
		Result streamInfo `json:"result"`
	}
	if err := t.cf(http.MethodGet, "/stream/"+uid, &out); err != nil {
		return nil, err
	}
	return &out.Result, nil
}

// streamDownload reports the Stream MP4 url and whether it is ready.
func (t *tool) streamDownload(uid string) (string, bool, error) {
	var out struct {
		Result struct {
			Default *struct {
				Status string `json:"status"`
				URL    string `json:"url"`
			} `json:"default"`
		} `json:"result"`
	}
	if err := t.cf(http.MethodGet, "/stream/"+uid+"/downloads", &out); err != nil {
		return "", false, err
	}
	d := out.Result.Default
	if d == nil {
		return "", false, nil
	}
	return d.URL, d.Status == "ready", nil
}

func (t *tool) requestStreamDownload(uid string) error {
	return t.cf(http.MethodPost, "/stream/"+uid+"/downloads", nil)
}

// cf calls the Stream API, retrying 429s.
func (t *tool) cf(method, path string, out any) error {
	endpoint := "https://api.cloudflare.com/client/v4/accounts/" + t.cfAccount + path
	backoff := 2 * time.Second
	for attempt := 0; attempt < 6; attempt++ {
		req, err := http.NewRequest(method, endpoint, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+t.cfToken)
		resp, err := t.http.Do(req)
		if err != nil {
			return err
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			wait := backoff
			if secs, e := strconv.Atoi(resp.Header.Get("Retry-After")); e == nil && secs > 0 {
				wait = time.Duration(secs) * time.Second
			}
			time.Sleep(wait)
			backoff *= 2
			continue
		case resp.StatusCode == http.StatusNotFound:
			return errStreamNotFound
		case resp.StatusCode < 200 || resp.StatusCode > 299:
			return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(body)))
		}
		if out == nil {
			return nil
		}
		return json.Unmarshal(body, out)
	}
	return fmt.Errorf("%s %s: rate limited", method, path)
}

// reachable reports whether a public url answers a HEAD with 200.
func (t *tool) reachable(u string) bool {
	resp, err := t.http.Head(u)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// ─── Helpers (unit-tested) ──────────────────────────────────────────────────

// isContentVideo reports whether u is a video file in the content bucket.
func isContentVideo(u string) bool {
	p, err := url.Parse(u)
	if err != nil || p.Hostname() != CONTENT_HOST {
		return false
	}
	ext := strings.ToLower(p.Path)
	return strings.HasSuffix(ext, ".mp4") || strings.HasSuffix(ext, ".m4v") || strings.HasSuffix(ext, ".mov")
}

// muxThumbnail converts a Stream thumbnail url to the same frame on Mux
// (Stream's ?time=12s → ?time=12); any other thumbnail is kept.
func muxThumbnail(thumb, playbackID string) string {
	if !strings.Contains(thumb, "/thumbnails/") || !(strings.Contains(thumb, "cloudflarestream.com") || strings.Contains(thumb, "videodelivery.net")) {
		return thumb
	}
	out := "https://image.mux.com/" + playbackID + "/thumbnail.jpg"
	params := url.Values{}
	if p, err := url.Parse(thumb); err == nil {
		if ts := p.Query().Get("time"); ts != "" {
			if d, err := time.ParseDuration(ts); err == nil {
				params.Set("time", strconv.FormatFloat(d.Seconds(), 'f', -1, 64))
			} else if f, err := strconv.ParseFloat(ts, 64); err == nil {
				params.Set("time", strconv.FormatFloat(f, 'f', -1, 64))
			}
		}
	}
	params.Set("width", "1280")
	params.Set("height", "720")
	return out + "?" + params.Encode()
}

func idList(g group) string {
	ids := make([]int, len(g.Rows))
	for i, r := range g.Rows {
		ids[i] = r.ID
	}
	sort.Ints(ids)
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = strconv.Itoa(id)
	}
	return strings.Join(s, ",")
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s is not set", key)
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
