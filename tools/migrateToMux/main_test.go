package main

import "testing"

func TestMuxThumbnail(t *testing.T) {
	tests := []struct {
		name, thumb, want string
	}{
		{"stream with time", "https://customer-abc.cloudflarestream.com/uid/thumbnails/thumbnail.jpg?time=12s&width=1280&height=720", "https://image.mux.com/pb/thumbnail.jpg?height=720&time=12&width=1280"},
		{"stream fractional", "https://customer-abc.cloudflarestream.com/uid/thumbnails/thumbnail.jpg?time=1.5s", "https://image.mux.com/pb/thumbnail.jpg?height=720&time=1.5&width=1280"},
		{"videodelivery no time", "https://videodelivery.net/uid/thumbnails/thumbnail.jpg", "https://image.mux.com/pb/thumbnail.jpg?height=720&width=1280"},
		{"s3 kept", "https://content.hillview.tv/thumbnails/a.jpeg", "https://content.hillview.tv/thumbnails/a.jpeg"},
		{"empty kept", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := muxThumbnail(tt.thumb, "pb"); got != tt.want {
				t.Errorf("muxThumbnail = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsContentVideo(t *testing.T) {
	tests := []struct {
		u    string
		want bool
	}{
		{"https://content.hillview.tv/videos/uploads/UID1-1-ABC.mp4", true},
		{"https://content.hillview.tv/videos/a%20b.MOV", true},
		{"https://content.hillview.tv/thumbnails/a.jpeg", false},
		{"https://customer-abc.cloudflarestream.com/uid/downloads/default.mp4", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isContentVideo(tt.u); got != tt.want {
			t.Errorf("isContentVideo(%q) = %v, want %v", tt.u, got, tt.want)
		}
	}
}

func TestGroup(t *testing.T) {
	g := group{UID: "u", Rows: []row{
		{ID: 63, Status: 1, Caption: ""},
		{ID: 62, Status: 4, Caption: "ready", AssetID: "a1"},
	}}
	if g.archived() {
		t.Error("archived() = true with a live row")
	}
	if g.assetID() != "a1" {
		t.Errorf("assetID() = %q", g.assetID())
	}
	if cr := g.captionRow(); cr == nil || cr.ID != 62 {
		t.Errorf("captionRow() = %+v", cr)
	}
	if got := idList(g); got != "62,63" {
		t.Errorf("idList = %q", got)
	}
}
