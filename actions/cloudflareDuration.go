package actions

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hillview.tv/videoAPI/env"
)

// CloudflareID derives the Stream UID from a video's stored url, e.g.
// https://customer-xxxx.cloudflarestream.com/{uid}/manifest/video.m3u8 -> {uid}.
// Returns "" when the url is not a Cloudflare Stream url.
func CloudflareID(url string) string {
	if !strings.Contains(url, "cloudflare") {
		return ""
	}
	parts := strings.Split(url, "/")
	if len(parts) < 3 {
		return ""
	}
	return parts[len(parts)-3]
}

type cloudflareStreamResponse struct {
	Result struct {
		Duration float64 `json:"duration"`
	} `json:"result"`
	Success bool `json:"success"`
}

// GetCloudflareDuration fetches a video's duration (seconds) from the Cloudflare
// Stream API by its UID. Returns nil when Cloudflare has no duration yet (still
// processing) so callers can leave the column NULL and let the sync job retry.
// Retries on HTTP 429, honoring Retry-After.
func GetCloudflareDuration(uid string) (*int, error) {
	endpoint := fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/accounts/%s/stream/%s",
		env.CloudflareUID, uid,
	)

	client := &http.Client{Timeout: 15 * time.Second}
	const maxAttempts = 4
	backoff := 2 * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequest("GET", endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+env.CloudflareToken)

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			wait := backoff
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, e := strconv.Atoi(strings.TrimSpace(ra)); e == nil && secs > 0 {
					wait = time.Duration(secs) * time.Second
				}
			}
			time.Sleep(wait)
			backoff *= 2
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("cloudflare returned status %d: %s",
				resp.StatusCode, strings.TrimSpace(string(payload)))
		}

		var body cloudflareStreamResponse
		if err := json.Unmarshal(payload, &body); err != nil {
			return nil, err
		}
		if !body.Success || body.Result.Duration <= 0 {
			return nil, nil
		}

		seconds := int(body.Result.Duration + 0.5)
		return &seconds, nil
	}

	return nil, fmt.Errorf("cloudflare rate limited after %d attempts", maxAttempts)
}
