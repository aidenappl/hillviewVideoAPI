package actions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hillview.tv/videoAPI/env"
)

const cfBase = "https://api.cloudflare.com/client/v4/accounts"

// ErrRateLimited signals a transient Cloudflare 429 that survived all retries.
// Callers should treat it as "try again later", NOT as a permanent failure.
var ErrRateLimited = errors.New("cloudflare rate limited")

// cfDo performs a Cloudflare API request with the Bearer token and retries on 429.
// Returns the response status code and the (size-limited) body.
func cfDo(method, url string) (int, []byte, error) {
	const maxAttempts = 6
	backoff := 2 * time.Second
	client := &http.Client{Timeout: 20 * time.Second}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequest(method, url, nil)
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Authorization", "Bearer "+env.CloudflareToken)

		resp, err := client.Do(req)
		if err != nil {
			return 0, nil, err
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 12<<20)) // captions can be large
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
	return 0, nil, ErrRateLimited
}

// GenerateCaptions asks Cloudflare to AI-generate captions for a video+language.
// It is async — Cloudflare returns immediately and processes in the background.
// A 409 (already exists) is treated as success.
func GenerateCaptions(uid, lang string) error {
	url := fmt.Sprintf("%s/%s/stream/%s/captions/%s/generate", cfBase, env.CloudflareUID, uid, lang)
	status, body, err := cfDo(http.MethodPost, url)
	if err != nil {
		return err
	}
	if status == http.StatusOK || status == http.StatusConflict {
		return nil
	}
	return fmt.Errorf("generate captions status %d: %s", status, strings.TrimSpace(string(body)))
}

// CaptionStatus returns Cloudflare's status for a language ("ready", "inprogress",
// "error") or "" if no caption for that language exists yet.
func CaptionStatus(uid, lang string) (string, error) {
	url := fmt.Sprintf("%s/%s/stream/%s/captions", cfBase, env.CloudflareUID, uid)
	status, body, err := cfDo(http.MethodGet, url)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("list captions status %d: %s", status, strings.TrimSpace(string(body)))
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
		if c.Language == lang {
			return c.Status, nil
		}
	}
	return "", nil
}

// FetchVTT downloads the WebVTT caption file for a video+language.
func FetchVTT(uid, lang string) (string, error) {
	url := fmt.Sprintf("%s/%s/stream/%s/captions/%s/vtt", cfBase, env.CloudflareUID, uid, lang)
	status, body, err := cfDo(http.MethodGet, url)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("fetch vtt status %d: %s", status, strings.TrimSpace(string(body)))
	}
	return string(body), nil
}

// PutCaptions uploads a (corrected) WebVTT file for a video+language. This marks
// the caption as human-edited on Cloudflare (generated=false).
func PutCaptions(uid, lang, vtt string) error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", lang+".vtt")
	if err != nil {
		return err
	}
	if _, err := fw.Write([]byte(vtt)); err != nil {
		return err
	}
	mw.Close()

	url := fmt.Sprintf("%s/%s/stream/%s/captions/%s", cfBase, env.CloudflareUID, uid, lang)
	req, err := http.NewRequest(http.MethodPut, url, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+env.CloudflareToken)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("put captions status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// DeleteCaptions removes a video's caption for a language. A 404 is treated as
// success (nothing to delete).
func DeleteCaptions(uid, lang string) error {
	url := fmt.Sprintf("%s/%s/stream/%s/captions/%s", cfBase, env.CloudflareUID, uid, lang)
	status, body, err := cfDo(http.MethodDelete, url)
	if err != nil {
		return err
	}
	if status == http.StatusOK || status == http.StatusNoContent || status == http.StatusNotFound {
		return nil
	}
	return fmt.Errorf("delete captions status %d: %s", status, strings.TrimSpace(string(body)))
}

// NumberVTTCues guarantees every cue has a sequential identifier line, which
// Cloudflare Stream requires. Any existing identifiers are replaced with a
// contiguous 1..N numbering; header/NOTE/STYLE/REGION blocks pass through.
func NumberVTTCues(vtt string) string {
	normalized := strings.ReplaceAll(vtt, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	blocks := strings.Split(normalized, "\n\n")

	out := make([]string, 0, len(blocks))
	n := 0
	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}
		lines := strings.Split(strings.Trim(block, "\n"), "\n")
		head := strings.TrimSpace(lines[0])
		if strings.HasPrefix(head, "WEBVTT") ||
			strings.HasPrefix(head, "NOTE") ||
			strings.HasPrefix(head, "STYLE") ||
			strings.HasPrefix(head, "REGION") {
			out = append(out, strings.Join(lines, "\n"))
			continue
		}

		// Find the cue timing line; everything before it is a (discarded) id.
		timingIdx := -1
		for i, l := range lines {
			if strings.Contains(l, "-->") {
				timingIdx = i
				break
			}
		}
		if timingIdx == -1 {
			out = append(out, strings.Join(lines, "\n"))
			continue
		}
		n++
		cue := append([]string{strconv.Itoa(n)}, lines[timingIdx:]...)
		out = append(out, strings.Join(cue, "\n"))
	}

	result := strings.Join(out, "\n\n")
	if !strings.HasPrefix(strings.TrimSpace(result), "WEBVTT") {
		result = "WEBVTT\n\n" + result
	}
	return result + "\n"
}

// VTTToPlainText strips WebVTT structure (header, cue numbers, timestamps) down
// to readable transcript text, collapsing consecutive duplicate lines that
// auto-generated captions often produce.
func VTTToPlainText(vtt string) string {
	lines := strings.Split(strings.ReplaceAll(vtt, "\r\n", "\n"), "\n")
	var out []string
	var last string
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || line == "WEBVTT" {
			continue
		}
		if strings.Contains(line, "-->") {
			continue
		}
		// Bare cue index (e.g. "1", "2") — skip.
		if _, err := strconv.Atoi(line); err == nil {
			continue
		}
		if strings.HasPrefix(line, "NOTE") {
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
