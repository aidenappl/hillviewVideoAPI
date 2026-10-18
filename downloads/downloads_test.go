package downloads

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestDownloadFilename(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"spaces and punctuation", "PAC Broadcast: Day 3!", "PAC-Broadcast-Day-3"},
		{"empty", "  ", "video"},
		{"unicode only", "日本語", "video"},
		{"length capped", strings.Repeat("a", 200), strings.Repeat("a", 120)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DownloadFilename(tt.title); got != tt.want {
				t.Errorf("DownloadFilename(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestContentDisposition(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		contains []string
	}{
		{"plain", "Morning Announcements", []string{`filename="Morning Announcements.mp4"`, `filename*=UTF-8''Morning%20Announcements.mp4`}},
		{"quotes stripped", `He said "hi"; ok`, []string{`filename="He said hi ok.mp4"`}},
		{"non-ascii fallback", "Café", []string{`filename="Caf_.mp4"`, `filename*=UTF-8''Caf%C3%A9.mp4`}},
		{"empty", "", []string{`filename="video.mp4"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContentDisposition(tt.title)
			if !strings.HasPrefix(got, "attachment; ") {
				t.Errorf("not an attachment: %q", got)
			}
			for _, c := range tt.contains {
				if !strings.Contains(got, c) {
					t.Errorf("ContentDisposition(%q) = %q, missing %q", tt.title, got, c)
				}
			}
		})
	}
}

func TestTarget(t *testing.T) {
	var gotKey, gotDisp string
	presign := func(key, disposition string) (string, error) {
		gotKey, gotDisp = key, disposition
		return "https://signed.example/" + key, nil
	}
	failing := func(string, string) (string, error) { return "", errors.New("no creds") }

	tests := []struct {
		name    string
		raw     string
		signer  Presigner
		want    string
		wantErr bool
	}{
		{"s3 object is presigned", "https://content.hillview.tv/videos/a%20b.mp4", presign, "https://signed.example/videos/a b.mp4", false},
		{"console plus is a space", "https://content.hillview.tv/videos/uploads/OK+Matilda+N3+S2.mp4", presign, "https://signed.example/videos/uploads/OK Matilda N3 S2.mp4", false},
		{"encoded plus stays a plus", "https://content.hillview.tv/videos/C%2B%2B.mp4", presign, "https://signed.example/videos/C++.mp4", false},
		{"cloudflare stream is no longer a download host", "https://customer-abc.cloudflarestream.com/uid/downloads/default.mp4", presign, "", true},
		{"mux gets download name", "https://stream.mux.com/pb1/highest.mp4", presign, "https://stream.mux.com/pb1/highest.mp4?download=My-Video", false},
		{"unknown host refused", "https://evil.example.com/x.mp4", presign, "", true},
		{"content without key refused", "https://content.hillview.tv/", presign, "", true},
		{"presign failure surfaces", "https://content.hillview.tv/v.mp4", failing, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Target(tt.raw, "My Video", tt.signer)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Target = %q, want %q", got, tt.want)
			}
		})
	}
	if !strings.Contains(gotDisp, `filename="My Video.mp4"`) {
		t.Errorf("presigner got key %q disposition %q", gotKey, gotDisp)
	}
	if _, err := url.Parse(gotKey); err != nil {
		t.Error(err)
	}
}
