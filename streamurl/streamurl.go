// Package streamurl recognises the streaming urls stored in the videos table
// and extracts their provider ids. It has no dependencies on env or the
// database, so it is unit-tested directly.
//
// Videos play from Mux. Cloudflare Stream urls remain only on archived rows
// (and in videos.legacy_url) from before the October 2026 migration:
//
//   - Cloudflare Stream, in both url forms: the customer subdomain
//     (customer-….cloudflarestream.com, newer uploads) and videodelivery.net
//     (2022 uploads). The uid sits at the same path position in each.
//   - Mux: https://stream.mux.com/{playback_id}.m3u8
package streamurl

import "strings"

// Provider names a video's streaming host.
type Provider string

const (
	ProviderNone       Provider = ""
	ProviderCloudflare Provider = "cloudflare"
	ProviderMux        Provider = "mux"
)

// muxHost prefixes every Mux HLS url.
const muxHost = "https://stream.mux.com/"

// SQL matches the rows whose url is a Stream url, in either form ID accepts.
// It names the column `url`; qualify it with Qualified for a joined query.
const SQL = "(url LIKE '%cloudflare%' OR url LIKE '%videodelivery.net%')"

// MuxSQL matches the rows whose url is a Mux url. It names the column `url`.
const MuxSQL = "(url LIKE 'https://stream.mux.com/%')"

// Qualified is SQL against a table alias, e.g. Qualified("v").
func Qualified(alias string) string {
	return strings.ReplaceAll(SQL, "url", alias+".url")
}

// ProviderOf reports which provider serves a stored url.
func ProviderOf(url string) Provider {
	switch {
	case IsMux(url):
		return ProviderMux
	case IsStream(url):
		return ProviderCloudflare
	default:
		return ProviderNone
	}
}

// IsStream reports whether a stored url points at Cloudflare Stream.
func IsStream(url string) bool {
	return strings.Contains(url, "cloudflare") || strings.Contains(url, "videodelivery.net")
}

// IsMux reports whether a stored url points at Mux.
func IsMux(url string) bool {
	return strings.HasPrefix(url, muxHost)
}

// ID returns the Stream uid in a stored url, or "" when it is not a Stream url.
func ID(url string) string {
	if !IsStream(url) {
		return ""
	}
	parts := strings.Split(url, "/")
	if len(parts) < 3 {
		return ""
	}
	return parts[len(parts)-3]
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
