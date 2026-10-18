package mux

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func testClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New("id", "secret")
	c.BaseURL = srv.URL
	c.Backoff = time.Millisecond
	return c
}

func TestNotConfigured(t *testing.T) {
	c := New("", "")
	if _, err := c.GetAsset("x"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func TestCreateAssetSendsSettings(t *testing.T) {
	var got AssetSettings
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/assets" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if u, p, ok := r.BasicAuth(); !ok || u != "id" || p != "secret" {
			t.Errorf("basic auth = %q %q %v", u, p, ok)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data":{"id":"asset1","status":"preparing","playback_ids":[{"id":"pb1","policy":"public"}]}}`))
	})

	a, err := c.CreateAsset(NewAssetSettings("https://content.hillview.tv/a.mp4", "42", true))
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != "asset1" || a.PublicPlaybackID() != "pb1" {
		t.Errorf("asset = %+v", a)
	}
	if got.VideoQuality != "basic" || got.MaxResolutionTier != "1080p" || got.Passthrough != "42" {
		t.Errorf("settings = %+v", got)
	}
	if len(got.StaticRenditions) != 1 || got.StaticRenditions[0].Resolution != "highest" {
		t.Errorf("static renditions = %+v", got.StaticRenditions)
	}
	if len(got.Inputs) != 1 || len(got.Inputs[0].GeneratedSubtitles) != 1 || got.Inputs[0].GeneratedSubtitles[0].LanguageCode != "en" {
		t.Errorf("inputs = %+v", got.Inputs)
	}
}

func TestNewAssetSettingsWithoutCaptions(t *testing.T) {
	s := NewAssetSettings("u", "1", false)
	if len(s.Inputs[0].GeneratedSubtitles) != 0 {
		t.Errorf("generated subtitles = %+v, want none", s.Inputs[0].GeneratedSubtitles)
	}
}

func TestStatusMapping(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   error
	}{
		{"not found", http.StatusNotFound, ErrNotFound},
		{"rate limited", http.StatusTooManyRequests, ErrRateLimited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			})
			c.MaxAttempts = 2
			if _, err := c.GetAsset("x"); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("api error", func(t *testing.T) {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":{"messages":["bad"]}}`))
		})
		_, err := c.GetAsset("x")
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadRequest {
			t.Fatalf("err = %v, want APIError 400", err)
		}
	})
}

func TestRetryThenSucceed(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"data":{"id":"u1","status":"asset_created","asset_id":"a1"}}`))
	})
	u, err := c.GetUpload("u1")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || u.AssetID != "a1" {
		t.Errorf("calls = %d, upload = %+v", calls, u)
	}
}

func TestAssetIDForPlayback(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/playback-ids/pb1" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Write([]byte(`{"data":{"id":"pb1","policy":"public","object":{"type":"asset","id":"asset9"}}}`))
	})
	id, err := c.AssetIDForPlayback("pb1")
	if err != nil || id != "asset9" {
		t.Fatalf("AssetIDForPlayback = %q, %v", id, err)
	}
}

func TestDeleteMissingIsSuccess(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if err := c.DeleteTrack("a", "t"); err != nil {
		t.Errorf("DeleteTrack = %v", err)
	}
	if err := c.DeleteAsset("a"); err != nil {
		t.Errorf("DeleteAsset = %v", err)
	}
}

func TestAssetHelpers(t *testing.T) {
	var a Asset
	raw := `{"id":"a","status":"ready","duration":125.6,
		"playback_ids":[{"id":"signed1","policy":"signed"},{"id":"pub1","policy":"public"}],
		"tracks":[{"id":"v","type":"video"},{"id":"au","type":"audio"},{"id":"tx","type":"text","language_code":"en","text_source":"generated_vod","status":"ready"}],
		"static_renditions":{"files":[{"id":"s","name":"highest.mp4","status":"ready"}]}}`
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatal(err)
	}
	if got := a.PublicPlaybackID(); got != "pub1" {
		t.Errorf("PublicPlaybackID = %q", got)
	}
	if tr := a.AudioTrack(); tr == nil || tr.ID != "au" {
		t.Errorf("AudioTrack = %+v", tr)
	}
	if tr := a.TextTrack("en"); tr == nil || tr.ID != "tx" {
		t.Errorf("TextTrack = %+v", tr)
	}
	if !a.StaticRenditionReady("highest.mp4") {
		t.Error("StaticRenditionReady = false")
	}
	if d := a.DurationSeconds(); d == nil || *d != 126 {
		t.Errorf("DurationSeconds = %v", d)
	}
}

func TestURLs(t *testing.T) {
	d := 120
	tests := []struct{ got, want string }{
		{VTTURL("pb", "tr"), "https://stream.mux.com/pb/text/tr.vtt"},
		{ThumbnailURL("pb", &d), "https://image.mux.com/pb/thumbnail.jpg?time=60"},
		{ThumbnailURL("pb", nil), "https://image.mux.com/pb/thumbnail.jpg"},
		{DownloadURL("pb", "Morning-News"), "https://stream.mux.com/pb/highest.mp4?download=Morning-News"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("got %q, want %q", tt.got, tt.want)
		}
	}
}

func sign(secret, ts string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyWebhook(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ts := strconv.FormatInt(now.Unix(), 10)
	body := []byte(`{"type":"video.asset.ready"}`)
	good := "t=" + ts + ",v1=" + sign("whsec", ts, body)

	old := strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10)

	tests := []struct {
		name   string
		secret string
		header string
		body   []byte
		want   error
	}{
		{"valid", "whsec", good, body, nil},
		{"tampered body", "whsec", good, []byte(`{"type":"x"}`), ErrBadSignature},
		{"wrong secret", "other", good, body, ErrBadSignature},
		{"stale", "whsec", "t=" + old + ",v1=" + sign("whsec", old, body), body, ErrStaleWebhook},
		{"missing v1", "whsec", "t=" + ts, body, ErrBadSignature},
		{"garbage", "whsec", "nonsense", body, ErrBadSignature},
		{"unconfigured", "", good, body, ErrNotConfigured},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := VerifyWebhook(tt.secret, tt.header, tt.body, now); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}
