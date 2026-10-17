package streamurl

import "testing"

func TestID(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"customer subdomain", "https://customer-nakrsdfbtn3mdz5z.cloudflarestream.com/30a81e2078e321a9784b97d6a7074398/manifest/video.m3u8", "30a81e2078e321a9784b97d6a7074398"},
		{"videodelivery.net", "https://videodelivery.net/e7e125f35c62ae1a7b9a3eb354e56cb9/manifest/video.m3u8", "e7e125f35c62ae1a7b9a3eb354e56cb9"},
		{"s3 is not stream", "https://content.hillview.tv/videos/uploads/a.mp4", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ID(tt.url); got != tt.want {
				t.Errorf("ID(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestQualified(t *testing.T) {
	want := "(v.url LIKE '%cloudflare%' OR v.url LIKE '%videodelivery.net%')"
	if got := Qualified("v"); got != want {
		t.Errorf("Qualified(v) = %q, want %q", got, want)
	}
}
