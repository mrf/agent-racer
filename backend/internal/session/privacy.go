package session

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
)

// PrivacyFilter applies masking and path-based filtering to session state
// before it is broadcast to clients. The zero value is a no-op filter.
type PrivacyFilter struct {
	MaskWorkingDirs bool
	MaskSessionIDs  bool
	MaskPIDs        bool
	MaskTmuxTargets bool
	AllowedPaths    []string
	BlockedPaths    []string
}

// IsAllowed reports whether a session with the given working directory should
// be broadcast. An empty working directory is always allowed (the session
// hasn't resolved its path yet). When AllowedPaths is non-empty, the path
// must match at least one pattern. If it passes the allowlist, it must not
// match any BlockedPaths pattern.
func (f *PrivacyFilter) IsAllowed(workingDir string) bool {
	if workingDir == "" {
		return true
	}

	if len(f.AllowedPaths) > 0 {
		allowed := false
		for _, pattern := range f.AllowedPaths {
			if matchPathOrParent(pattern, workingDir) {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}

	for _, pattern := range f.BlockedPaths {
		if matchPathOrParent(pattern, workingDir) {
			return false
		}
	}

	return true
}

// matchPathOrParent checks if pattern matches path or any of its parent
// directories. This allows patterns like "/home/user/*" to match deeply
// nested paths like "/home/user/work/project-a" because the parent
// "/home/user/work" matches the glob.
func matchPathOrParent(pattern, path string) bool {
	for p := path; p != "." && p != "" && p != filepath.Dir(p); p = filepath.Dir(p) {
		if matched, _ := filepath.Match(pattern, p); matched {
			return true
		}
	}
	return false
}

// Apply returns a copy of the session state with sensitive fields masked
// according to the filter configuration. The original state is never modified.
func (f *PrivacyFilter) Apply(s *SessionState) *SessionState {
	masked := *s

	if f.MaskWorkingDirs && masked.WorkingDir != "" {
		masked.WorkingDir = filepath.Base(masked.WorkingDir)
	}

	if f.MaskSessionIDs && masked.ID != "" {
		masked.ID = shortHash(masked.ID)
	}

	if f.MaskPIDs {
		masked.PID = 0
	}

	if f.MaskTmuxTargets {
		masked.TmuxTarget = ""
	}

	return &masked
}

// FilterSlice returns a new slice containing only the allowed sessions,
// with privacy masking applied to each. The original slice is not modified.
func (f *PrivacyFilter) FilterSlice(sessions []*SessionState) []*SessionState {
	result := make([]*SessionState, 0, len(sessions))
	for _, s := range sessions {
		if !f.IsAllowed(s.WorkingDir) {
			continue
		}
		result = append(result, f.Apply(s))
	}
	return result
}

// IsNoop reports whether the filter does nothing (no masking, no path filtering).
func (f *PrivacyFilter) IsNoop() bool {
	return !f.MaskWorkingDirs && !f.MaskSessionIDs && !f.MaskPIDs && !f.MaskTmuxTargets &&
		len(f.AllowedPaths) == 0 && len(f.BlockedPaths) == 0
}

// shortHash returns a truncated SHA-256 hex digest for an opaque identifier.
func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:6])
}
