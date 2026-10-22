package streamurl

import "testing"

func TestMuxPlaybackID(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"hls", "https://stream.mux.com/a4nOgmxGWg6gULfcBbAa00gXyfcwPnAFldF8RdsNyk8M.m3u8", "a4nOgmxGWg6gULfcBbAa00gXyfcwPnAFldF8RdsNyk8M"},
		{"query string", "https://stream.mux.com/abc123.m3u8?max_resolution=720p", "abc123"},
		{"static rendition is not a stored url", "https://stream.mux.com/abc123/highest.mp4", ""},
		{"old stream url", "https://videodelivery.net/e7e125f35c62ae1a7b9a3eb354e56cb9/manifest/video.m3u8", ""},
		{"s3", "https://content.hillview.tv/videos/uploads/a.mp4", ""},
		{"bare host", "https://stream.mux.com/", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MuxPlaybackID(tt.url); got != tt.want {
				t.Errorf("MuxPlaybackID(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestMuxURL(t *testing.T) {
	u := MuxURL("abc123")
	if u != "https://stream.mux.com/abc123.m3u8" || !IsMux(u) || MuxPlaybackID(u) != "abc123" {
		t.Errorf("MuxURL round trip failed: %q", u)
	}
}
