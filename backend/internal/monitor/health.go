package monitor

import (
	"regexp"
	"strings"
)

// absPathRe matches absolute file paths (e.g. /home/user/.claude/projects/...).
var absPathRe = regexp.MustCompile(`/(?:[^\s:]+/)*[^\s:]+`)

// sanitizeHealthError strips internal details from error strings before
// they are broadcast to WebSocket clients. File paths are replaced with
// <path> and panic details are reduced to "internal error".
func sanitizeHealthError(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "panic:") {
		return "internal error"
	}
	return absPathRe.ReplaceAllString(raw, "<path>")
}
