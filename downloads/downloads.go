// Package downloads turns a video's stored download URL into a link the
// browser saves as "<video title>.mp4". It has no dependencies on env or the
// database, so it is unit-tested directly.
package downloads

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const (
	// CONTENT_BUCKET is the S3 bucket behind https://content.hillview.tv.
	CONTENT_BUCKET        = "content.hillview.tv"
	CONTENT_BUCKET_REGION = "us-west-1"
	CONTENT_HOST          = "content.hillview.tv"
	// MUX_STREAM_HOST serves Mux static renditions: /{playback_id}/highest.mp4.
	MUX_STREAM_HOST = "stream.mux.com"
)

// Presigner signs a GET for an object in CONTENT_BUCKET with the given
// Content-Disposition override.
type Presigner func(key string, disposition string) (string, error)

var cloudflareFilenameDisallowed = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// Target builds the redirect for a stored download URL.
func Target(raw string, title string, presign Presigner) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse download url: %w", err)
	}

	switch {
	case u.Hostname() == CONTENT_HOST:
		key, err := objectKey(u)
		if err != nil {
			return "", fmt.Errorf("download url %q: %w", raw, err)
		}
		if key == "" {
			return "", fmt.Errorf("download url %q has no object key", raw)
		}
		return presign(key, ContentDisposition(title))
	case strings.HasSuffix(u.Hostname(), ".cloudflarestream.com"):
		// Stream sets Content-Disposition itself from ?filename=, and appends .mp4.
		qs := u.Query()
		qs.Set("filename", CloudflareFilename(title))
		u.RawQuery = qs.Encode()
		return u.String(), nil
	case u.Hostname() == MUX_STREAM_HOST:
		// Mux sets Content-Disposition from ?download= and appends the extension.
		qs := u.Query()
		qs.Set("download", CloudflareFilename(title))
		u.RawQuery = qs.Encode()
		return u.String(), nil
	default:
		return "", fmt.Errorf("download url host %q is not a known content host", u.Hostname())
	}
}

// objectKey is the S3 key a content.hillview.tv URL names. The stored URLs
// come from the S3 console, which writes a space as a literal "+" — and S3
// reads a "+" in the path the same way — so "+" is a space here, while a
// percent-encoded "%2B" stays a real plus.
func objectKey(u *url.URL) (string, error) {
	escaped := strings.ReplaceAll(strings.TrimPrefix(u.EscapedPath(), "/"), "+", "%20")
	return url.PathUnescape(escaped)
}

// ContentDisposition is an attachment header per RFC 6266: an ASCII-only
// fallback filename plus a UTF-8 filename*.
func ContentDisposition(title string) string {
	base := baseName(title)
	ascii := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e {
			return '_'
		}
		return r
	}, base)
	encoded := strings.ReplaceAll(url.QueryEscape(base), "+", "%20")
	return fmt.Sprintf(`attachment; filename="%s.mp4"; filename*=UTF-8''%s.mp4`, ascii, encoded)
}

// baseName strips control and quoting characters from a title.
func baseName(title string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r < 0x20:
			return -1
		case strings.ContainsRune(`/\"';%`, r):
			return -1
		}
		return r
	}, title)
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	cleaned = strings.TrimRight(cleaned, ".")
	if runes := []rune(cleaned); len(runes) > 150 {
		cleaned = string(runes[:150])
	}
	if cleaned == "" {
		return "video"
	}
	return cleaned
}

// CloudflareFilename fits Stream's rule: [A-Za-z0-9_-], at most 120 chars.
// Mux's ?download= gets the same safe name.
func CloudflareFilename(title string) string {
	name := cloudflareFilenameDisallowed.ReplaceAllString(strings.TrimSpace(title), "-")
	name = strings.Trim(name, "-")
	if len(name) > 120 {
		name = strings.TrimRight(name[:120], "-")
	}
	if name == "" {
		return "video"
	}
	return name
}
