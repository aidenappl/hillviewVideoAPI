// Package streamurl recognises the Mux urls stored in the videos table
// (https://stream.mux.com/{playback_id}.m3u8) and extracts their playback id.
// It has no dependencies on env or the database, so it is unit-tested directly.
//
// Videos moved here from Cloudflare Stream in October 2026; the Stream account
// is empty, so old Stream urls (archived rows, videos.legacy_url) no longer
// play and are not recognised.
package streamurl

import "strings"

// muxHost prefixes every Mux HLS url.
const muxHost = "https://stream.mux.com/"

// MuxSQL matches the rows whose url is a Mux url. It names the column `url`.
const MuxSQL = "(url LIKE 'https://stream.mux.com/%')"

// IsMux reports whether a stored url points at Mux.
func IsMux(url string) bool {
	return strings.HasPrefix(url, muxHost)
}

// MuxPlaybackID returns the playback id in a stored Mux url, or "" when it is
// not a Mux url. Query strings are ignored.
func MuxPlaybackID(url string) string {
	if !IsMux(url) {
		return ""
	}
	rest := strings.TrimPrefix(url, muxHost)
	if i := strings.IndexAny(rest, "?#"); i >= 0 {
		rest = rest[:i]
	}
	id := strings.TrimSuffix(rest, ".m3u8")
	if id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

// MuxURL is the HLS url stored for a Mux playback id.
func MuxURL(playbackID string) string {
	return muxHost + playbackID + ".m3u8"
}
