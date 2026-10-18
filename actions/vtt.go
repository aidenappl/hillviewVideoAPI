package actions

import (
	"strconv"
	"strings"
)

// NumberVTTCues guarantees every cue has a sequential identifier line, so
// edited captions are stored in one consistent shape. Any existing identifiers are replaced with a
// contiguous 1..N numbering; header/NOTE/STYLE/REGION blocks pass through.
func NumberVTTCues(vtt string) string {
	normalized := strings.ReplaceAll(vtt, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	blocks := strings.Split(normalized, "\n\n")

	out := make([]string, 0, len(blocks))
	n := 0
	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}
		lines := strings.Split(strings.Trim(block, "\n"), "\n")
		head := strings.TrimSpace(lines[0])
		if strings.HasPrefix(head, "WEBVTT") ||
			strings.HasPrefix(head, "NOTE") ||
			strings.HasPrefix(head, "STYLE") ||
			strings.HasPrefix(head, "REGION") {
			out = append(out, strings.Join(lines, "\n"))
			continue
		}

		// Find the cue timing line; everything before it is a (discarded) id.
		timingIdx := -1
		for i, l := range lines {
			if strings.Contains(l, "-->") {
				timingIdx = i
				break
			}
		}
		if timingIdx == -1 {
			out = append(out, strings.Join(lines, "\n"))
			continue
		}
		n++
		cue := append([]string{strconv.Itoa(n)}, lines[timingIdx:]...)
		out = append(out, strings.Join(cue, "\n"))
	}

	result := strings.Join(out, "\n\n")
	if !strings.HasPrefix(strings.TrimSpace(result), "WEBVTT") {
		result = "WEBVTT\n\n" + result
	}
	return result + "\n"
}

// VTTToPlainText strips WebVTT structure (header, cue numbers, timestamps) down
// to readable transcript text, collapsing consecutive duplicate lines that
// auto-generated captions often produce.
func VTTToPlainText(vtt string) string {
	lines := strings.Split(strings.ReplaceAll(vtt, "\r\n", "\n"), "\n")
	var out []string
	var last string
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || line == "WEBVTT" {
			continue
		}
		if strings.Contains(line, "-->") {
			continue
		}
		// Bare cue index (e.g. "1", "2") — skip.
		if _, err := strconv.Atoi(line); err == nil {
			continue
		}
		if strings.HasPrefix(line, "NOTE") {
			continue
		}
		if line == last {
			continue
		}
		out = append(out, line)
		last = line
	}
	return strings.Join(out, " ")
}
