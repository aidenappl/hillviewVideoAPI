// Package mux is a small client for the Mux Video API: direct uploads,
// assets, text tracks, generated captions and static MP4 renditions, plus
// webhook signature verification. It has no dependency on env, so it is
// unit-tested against httptest servers; Default builds the production client.
package mux

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.mux.com/video/v1"

// Asset settings every Hillview asset is created with: free basic encoding,
// capped at 1080p so storage and delivery bill at 1080p rates, with an MP4
// for downloads.
const (
	VideoQuality      = "basic"
	MaxResolutionTier = "1080p"
	CaptionLanguage   = "en"
	CaptionName       = "English (generated)"
	StaticRendition   = "highest"
)

var (
	// ErrNotConfigured means MUX_TOKEN_ID / MUX_TOKEN_SECRET are unset.
	ErrNotConfigured = errors.New("mux is not configured")
	// ErrNotFound is a Mux 404: the asset, upload or track does not exist.
	ErrNotFound = errors.New("mux resource not found")
	// ErrRateLimited is a 429 that survived every retry. Try again later.
	ErrRateLimited = errors.New("mux rate limited")
)

// Client calls the Mux Video API with a token id/secret pair.
type Client struct {
	BaseURL     string
	TokenID     string
	TokenSecret string
	HTTP        *http.Client
	// MaxAttempts bounds retries on 429; Backoff is the first wait.
	MaxAttempts int
	Backoff     time.Duration
}

// New builds a client for the production API.
func New(tokenID, tokenSecret string) *Client {
	return &Client{
		BaseURL:     defaultBaseURL,
		TokenID:     tokenID,
		TokenSecret: tokenSecret,
		HTTP:        &http.Client{Timeout: 30 * time.Second},
		MaxAttempts: 5,
		Backoff:     2 * time.Second,
	}
}

// APIError is a non-2xx answer from Mux.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("mux returned status %d: %s", e.Status, e.Body)
}

// do sends one request, retrying on 429 (honouring Retry-After), and decodes
// the `data` envelope into out when out is non-nil.
func (c *Client) do(method, path string, in, out any) error {
	if c.TokenID == "" || c.TokenSecret == "" {
		return ErrNotConfigured
	}

	var payload []byte
	if in != nil {
		var err error
		if payload, err = json.Marshal(in); err != nil {
			return err
		}
	}

	attempts := c.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	backoff := c.Backoff

	for attempt := 1; attempt <= attempts; attempt++ {
		req, err := http.NewRequest(method, c.BaseURL+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.SetBasicAuth(c.TokenID, c.TokenSecret)
		if in != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.HTTP.Do(req)
		if err != nil {
			return err
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			wait := backoff
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, e := strconv.Atoi(strings.TrimSpace(ra)); e == nil && secs > 0 {
					wait = time.Duration(secs) * time.Second
				}
			}
			if attempt < attempts {
				time.Sleep(wait)
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}

		if resp.StatusCode == http.StatusNotFound {
			return ErrNotFound
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return &APIError{Status: resp.StatusCode, Body: strings.TrimSpace(string(body))}
		}
		if out == nil || len(body) == 0 {
			return nil
		}

		envelope := struct {
			Data json.RawMessage `json:"data"`
		}{}
		if err := json.Unmarshal(body, &envelope); err != nil {
			return err
		}
		return json.Unmarshal(envelope.Data, out)
	}
	return ErrRateLimited
}

// GeneratedSubtitles asks Mux to caption an input with Whisper.
type GeneratedSubtitles struct {
	LanguageCode string `json:"language_code"`
	Name         string `json:"name"`
}

// Input is one asset input: the video url, or for direct uploads only the
// caption settings.
type Input struct {
	URL                string               `json:"url,omitempty"`
	GeneratedSubtitles []GeneratedSubtitles `json:"generated_subtitles,omitempty"`
}

// StaticRenditionRequest names one MP4/M4A rendition to prepare.
type StaticRenditionRequest struct {
	Resolution string `json:"resolution"`
}

// AssetSettings is the body of POST /assets and an upload's new_asset_settings.
type AssetSettings struct {
	Inputs            []Input                  `json:"inputs,omitempty"`
	PlaybackPolicies  []string                 `json:"playback_policies"`
	VideoQuality      string                   `json:"video_quality"`
	MaxResolutionTier string                   `json:"max_resolution_tier"`
	StaticRenditions  []StaticRenditionRequest `json:"static_renditions,omitempty"`
	Passthrough       string                   `json:"passthrough,omitempty"`
}

// NewAssetSettings is the Hillview default: public playback, basic quality,
// 1080p, an MP4 rendition, and English captions unless captions is false (the
// video already has edited captions to restore instead).
func NewAssetSettings(inputURL, passthrough string, captions bool) AssetSettings {
	input := Input{URL: inputURL}
	if captions {
		input.GeneratedSubtitles = []GeneratedSubtitles{{LanguageCode: CaptionLanguage, Name: CaptionName}}
	}
	return AssetSettings{
		Inputs:            []Input{input},
		PlaybackPolicies:  []string{"public"},
		VideoQuality:      VideoQuality,
		MaxResolutionTier: MaxResolutionTier,
		StaticRenditions:  []StaticRenditionRequest{{Resolution: StaticRendition}},
		Passthrough:       passthrough,
	}
}

// PlaybackID is one way to play an asset.
type PlaybackID struct {
	ID     string `json:"id"`
	Policy string `json:"policy"`
}

// Track is a video, audio or text track on an asset.
type Track struct {
	ID           string  `json:"id"`
	AssetID      string  `json:"asset_id,omitempty"`
	Type         string  `json:"type"`
	Status       string  `json:"status"`
	TextType     string  `json:"text_type,omitempty"`
	TextSource   string  `json:"text_source,omitempty"`
	LanguageCode string  `json:"language_code,omitempty"`
	Name         string  `json:"name,omitempty"`
	Duration     float64 `json:"duration,omitempty"`
}

// StaticRenditionFile is one prepared MP4/M4A.
type StaticRenditionFile struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Ext        string `json:"ext"`
	Status     string `json:"status"`
	Resolution string `json:"resolution"`
}

// Asset is a Mux asset. Status is preparing, ready or errored.
type Asset struct {
	ID               string       `json:"id"`
	Status           string       `json:"status"`
	Duration         float64      `json:"duration"`
	Passthrough      string       `json:"passthrough"`
	PlaybackIDs      []PlaybackID `json:"playback_ids"`
	Tracks           []Track      `json:"tracks"`
	StaticRenditions *struct {
		Files []StaticRenditionFile `json:"files"`
	} `json:"static_renditions,omitempty"`
}

// PublicPlaybackID returns the asset's public playback id, or "".
func (a *Asset) PublicPlaybackID() string {
	for _, p := range a.PlaybackIDs {
		if p.Policy == "public" {
			return p.ID
		}
	}
	return ""
}

// AudioTrack returns the asset's first audio track, which generated captions
// are requested against.
func (a *Asset) AudioTrack() *Track {
	for i := range a.Tracks {
		if a.Tracks[i].Type == "audio" {
			return &a.Tracks[i]
		}
	}
	return nil
}

// TextTrack returns the asset's text track for a language, or nil.
func (a *Asset) TextTrack(lang string) *Track {
	for i := range a.Tracks {
		if a.Tracks[i].Type == "text" && a.Tracks[i].LanguageCode == lang {
			return &a.Tracks[i]
		}
	}
	return nil
}

// StaticRenditionReady reports whether the named MP4 (e.g. highest.mp4) is
// ready to download.
func (a *Asset) StaticRenditionReady(name string) bool {
	if a.StaticRenditions == nil {
		return false
	}
	for _, f := range a.StaticRenditions.Files {
		if f.Name == name && f.Status == "ready" {
			return true
		}
	}
	return false
}

// DurationSeconds rounds the asset duration to whole seconds, or nil while
// Mux has not reported one.
func (a *Asset) DurationSeconds() *int {
	if a.Duration <= 0 {
		return nil
	}
	s := int(a.Duration + 0.5)
	return &s
}

// CreateAsset ingests a video from a url Mux can fetch.
func (c *Client) CreateAsset(settings AssetSettings) (*Asset, error) {
	var a Asset
	if err := c.do(http.MethodPost, "/assets", settings, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// GetAsset fetches an asset by id.
func (c *Client) GetAsset(id string) (*Asset, error) {
	var a Asset
	if err := c.do(http.MethodGet, "/assets/"+id, nil, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// AssetIDForPlayback returns the asset a playback id plays. Live stream
// playback ids are rejected.
func (c *Client) AssetIDForPlayback(playbackID string) (string, error) {
	var p struct {
		Object struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"object"`
	}
	if err := c.do(http.MethodGet, "/playback-ids/"+playbackID, nil, &p); err != nil {
		return "", err
	}
	if p.Object.Type != "asset" || p.Object.ID == "" {
		return "", fmt.Errorf("playback id %s plays a %q, not an asset", playbackID, p.Object.Type)
	}
	return p.Object.ID, nil
}

// DeleteAsset deletes an asset. A missing asset counts as deleted.
func (c *Client) DeleteAsset(id string) error {
	if err := c.do(http.MethodDelete, "/assets/"+id, nil, nil); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	return nil
}

// Upload is a direct upload. Status is waiting, asset_created, errored,
// cancelled or timed_out; AssetID is set once asset_created.
type Upload struct {
	ID      string `json:"id"`
	URL     string `json:"url"`
	Status  string `json:"status"`
	AssetID string `json:"asset_id"`
	Timeout int    `json:"timeout"`
}

type createUploadRequest struct {
	NewAssetSettings AssetSettings `json:"new_asset_settings"`
	CORSOrigin       string        `json:"cors_origin"`
	Timeout          int           `json:"timeout,omitempty"`
}

// CreateUpload returns a resumable PUT url the browser uploads the file to.
// corsOrigin is the page origin doing the upload.
func (c *Client) CreateUpload(passthrough, corsOrigin string) (*Upload, error) {
	settings := NewAssetSettings("", passthrough, true)
	var u Upload
	err := c.do(http.MethodPost, "/uploads", createUploadRequest{
		NewAssetSettings: settings,
		CORSOrigin:       corsOrigin,
		Timeout:          3600,
	}, &u)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUpload fetches a direct upload by id.
func (c *Client) GetUpload(id string) (*Upload, error) {
	var u Upload
	if err := c.do(http.MethodGet, "/uploads/"+id, nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// GenerateCaptions asks Mux to caption an existing asset's audio track.
func (c *Client) GenerateCaptions(assetID, audioTrackID string) error {
	body := struct {
		GeneratedSubtitles []GeneratedSubtitles `json:"generated_subtitles"`
	}{[]GeneratedSubtitles{{LanguageCode: CaptionLanguage, Name: CaptionName}}}
	return c.do(http.MethodPost, "/assets/"+assetID+"/tracks/"+audioTrackID+"/generate-subtitles", body, nil)
}

type createTrackRequest struct {
	URL            string `json:"url"`
	Type           string `json:"type"`
	TextType       string `json:"text_type"`
	LanguageCode   string `json:"language_code"`
	Name           string `json:"name"`
	ClosedCaptions bool   `json:"closed_captions"`
}

// CreateTextTrack adds a caption track from a WebVTT url Mux can fetch. The
// asset must be ready.
func (c *Client) CreateTextTrack(assetID, vttURL, lang, name string) (*Track, error) {
	var t Track
	err := c.do(http.MethodPost, "/assets/"+assetID+"/tracks", createTrackRequest{
		URL:            vttURL,
		Type:           "text",
		TextType:       "subtitles",
		LanguageCode:   lang,
		Name:           name,
		ClosedCaptions: true,
	}, &t)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// DeleteTrack removes a track. A missing track counts as deleted.
func (c *Client) DeleteTrack(assetID, trackID string) error {
	if err := c.do(http.MethodDelete, "/assets/"+assetID+"/tracks/"+trackID, nil, nil); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	return nil
}

// CreateStaticRendition prepares an MP4 for an existing asset.
func (c *Client) CreateStaticRendition(assetID string) error {
	return c.do(http.MethodPost, "/assets/"+assetID+"/static-renditions", StaticRenditionRequest{Resolution: StaticRendition}, nil)
}

// Public urls for a playback id.

// VTTURL is the WebVTT for a text track.
func VTTURL(playbackID, trackID string) string {
	return "https://stream.mux.com/" + playbackID + "/text/" + trackID + ".vtt"
}

// ThumbnailURL is a poster frame, taken at the middle of the video when
// duration is known (matching Stream's thumbnailTimestampPct 0.5).
func ThumbnailURL(playbackID string, duration *int) string {
	u := "https://image.mux.com/" + playbackID + "/thumbnail.jpg"
	if duration != nil && *duration > 1 {
		u += "?time=" + strconv.Itoa(*duration/2)
	}
	return u
}

// DownloadURL is the MP4 rendition, saved by the browser as <filename>.mp4.
// filename must already be safe (see downloads.DownloadFilename).
func DownloadURL(playbackID, filename string) string {
	u := "https://stream.mux.com/" + playbackID + "/" + StaticRendition + ".mp4"
	if filename != "" {
		u += "?download=" + filename
	}
	return u
}
