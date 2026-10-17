// Package streamurl recognises Cloudflare Stream urls in the videos table and
// extracts their uid. It has no dependencies on env or the database, so it is
// unit-tested directly.
//
// Both url forms are stored: the customer subdomain
// (customer-….cloudflarestream.com, newer uploads) and videodelivery.net
// (2022 uploads). The uid sits at the same path position in each.
package streamurl

import "strings"

// SQL matches the rows whose url is a Stream url, in either form ID accepts.
// It names the column `url`; qualify it with Qualified for a joined query.
const SQL = "(url LIKE '%cloudflare%' OR url LIKE '%videodelivery.net%')"

// Qualified is SQL against a table alias, e.g. Qualified("v").
func Qualified(alias string) string {
	return strings.ReplaceAll(SQL, "url", alias+".url")
}

// IsStream reports whether a stored url points at Cloudflare Stream.
func IsStream(url string) bool {
	return strings.Contains(url, "cloudflare") || strings.Contains(url, "videodelivery.net")
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
