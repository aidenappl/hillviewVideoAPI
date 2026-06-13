package actions

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
