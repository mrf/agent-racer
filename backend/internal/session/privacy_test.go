package session

import (
	"testing"
)

func TestPrivacyFilter_IsAllowed(t *testing.T) {
	tests := []struct {
		name       string
		filter     PrivacyFilter
		workingDir string
		want       bool
	}{
		{
			name:       "empty filter allows everything",
			filter:     PrivacyFilter{},
			workingDir: "/home/user/project",
			want:       true,
		},
		{
			name:       "empty working dir always allowed",
			filter:     PrivacyFilter{BlockedPaths: []string{"/tmp/*"}},
			workingDir: "",
			want:       true,
		},
		{
			name:       "allowlist match direct",
			filter:     PrivacyFilter{AllowedPaths: []string{"/home/user/work/*"}},
			workingDir: "/home/user/work/myproject",
			want:       true,
		},
		{
			name:       "allowlist match nested",
			filter:     PrivacyFilter{AllowedPaths: []string{"/home/user/work/*"}},
			workingDir: "/home/user/work/deep/nested/path",
			want:       true,
		},
		{
			name:       "allowlist no match",
			filter:     PrivacyFilter{AllowedPaths: []string{"/home/user/work/*"}},
			workingDir: "/home/user/personal/diary",
			want:       false,
		},
		{
			name:       "blocklist match",
			filter:     PrivacyFilter{BlockedPaths: []string{"/tmp/*"}},
			workingDir: "/tmp/scratch",
			want:       false,
		},
		{
			name:       "blocklist match nested",
			filter:     PrivacyFilter{BlockedPaths: []string{"/tmp/*"}},
			workingDir: "/tmp/deep/nested",
			want:       false,
		},
		{
			name:       "blocklist no match",
			filter:     PrivacyFilter{BlockedPaths: []string{"/tmp/*"}},
			workingDir: "/home/user/project",
			want:       true,
		},
		{
			name: "allowlist passes but blocklist catches",
			filter: PrivacyFilter{
				AllowedPaths: []string{"/home/user/*"},
				BlockedPaths: []string{"/home/user/secret"},
			},
			workingDir: "/home/user/secret",
			want:       false,
		},
		{
			name: "multiple allowlist patterns",
			filter: PrivacyFilter{
				AllowedPaths: []string{"/home/user/work/*", "/home/user/projects/*"},
			},
			workingDir: "/home/user/projects/cool",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.IsAllowed(tt.workingDir)
			if got != tt.want {
				t.Errorf("IsAllowed(%q) = %v, want %v", tt.workingDir, got, tt.want)
			}
		})
	}
}

func TestPrivacyFilter_Apply(t *testing.T) {
	original := &SessionState{
		ID:         "claude:abc123",
		Name:       "myproject",
		WorkingDir: "/home/user/projects/myproject",
		PID:        12345,
		TmuxTarget: "main:2.0",
	}

	t.Run("mask working dirs", func(t *testing.T) {
		f := &PrivacyFilter{MaskWorkingDirs: true}
		result := f.Apply(original)
		if result.WorkingDir != "myproject" {
			t.Errorf("expected WorkingDir = %q, got %q", "myproject", result.WorkingDir)
		}
		// Original unchanged
		if original.WorkingDir != "/home/user/projects/myproject" {
			t.Error("original was modified")
		}
	})

	t.Run("mask session IDs", func(t *testing.T) {
		f := &PrivacyFilter{MaskSessionIDs: true}
		result := f.Apply(original)
		if result.ID == original.ID {
			t.Error("session ID should have been masked")
		}
		if len(result.ID) == 0 {
			t.Error("masked session ID should not be empty")
		}
	})

	t.Run("mask PIDs", func(t *testing.T) {
		f := &PrivacyFilter{MaskPIDs: true}
		result := f.Apply(original)
		if result.PID != 0 {
			t.Errorf("expected PID = 0, got %d", result.PID)
		}
	})

	t.Run("mask tmux targets", func(t *testing.T) {
		f := &PrivacyFilter{MaskTmuxTargets: true}
		result := f.Apply(original)
		if result.TmuxTarget != "" {
			t.Errorf("expected TmuxTarget = %q, got %q", "", result.TmuxTarget)
		}
	})

	t.Run("no masking is noop", func(t *testing.T) {
		f := &PrivacyFilter{}
		result := f.Apply(original)
		if result.ID != original.ID || result.WorkingDir != original.WorkingDir ||
			result.PID != original.PID || result.TmuxTarget != original.TmuxTarget {
			t.Error("no-op filter should not change any fields")
		}
	})

	t.Run("all masks combined", func(t *testing.T) {
		f := &PrivacyFilter{
			MaskWorkingDirs: true,
			MaskSessionIDs:  true,
			MaskPIDs:        true,
			MaskTmuxTargets: true,
		}
		result := f.Apply(original)
		if result.WorkingDir != "myproject" {
			t.Errorf("WorkingDir not masked: %q", result.WorkingDir)
		}
		if result.ID == original.ID {
			t.Error("session ID not masked")
		}
		if result.PID != 0 {
			t.Error("PID not masked")
		}
		if result.TmuxTarget != "" {
			t.Error("TmuxTarget not masked")
		}
	})
}

func TestPrivacyFilter_FilterSlice(t *testing.T) {
	sessions := []*SessionState{
		{ID: "claude:1", WorkingDir: "/home/user/work/project-a", PID: 100},
		{ID: "claude:2", WorkingDir: "/home/user/personal/diary", PID: 200},
		{ID: "claude:3", WorkingDir: "/tmp/scratch", PID: 300},
	}

	f := &PrivacyFilter{
		MaskPIDs:     true,
		BlockedPaths: []string{"/tmp/*"},
	}

	result := f.FilterSlice(sessions)

	// /home/user/work/project-a -> not blocked -> included
	// /home/user/personal/diary -> not blocked -> included
	// /tmp/scratch -> blocked by /tmp/* -> excluded
	if len(result) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(result))
	}

	for _, s := range result {
		if s.PID != 0 {
			t.Errorf("PID should be masked, got %d for %s", s.PID, s.ID)
		}
		if s.WorkingDir == "/tmp/scratch" {
			t.Error("blocked session should not be in result")
		}
	}
}

func TestPrivacyFilter_FilterSlice_AllowAndBlock(t *testing.T) {
	sessions := []*SessionState{
		{ID: "claude:1", WorkingDir: "/home/user/work/project-a"},
		{ID: "claude:2", WorkingDir: "/home/user/work/secret-project"},
		{ID: "claude:3", WorkingDir: "/other/path"},
	}

	f := &PrivacyFilter{
		AllowedPaths: []string{"/home/user/work/*"},
		BlockedPaths: []string{"/home/user/work/secret-*"},
	}

	result := f.FilterSlice(sessions)

	// project-a: allowed by /home/user/work/*, not blocked -> included
	// secret-project: allowed by /home/user/work/*, but blocked by /home/user/work/secret-* -> excluded
	// /other/path: not in allowlist -> excluded
	if len(result) != 1 {
		t.Fatalf("expected 1 session, got %d", len(result))
	}
	if result[0].ID != "claude:1" {
		t.Errorf("expected claude:1, got %s", result[0].ID)
	}
}

func TestPrivacyFilter_IsNoop(t *testing.T) {
	t.Run("zero value is noop", func(t *testing.T) {
		f := &PrivacyFilter{}
		if !f.IsNoop() {
			t.Error("zero value filter should be noop")
		}
	})

	t.Run("with masking is not noop", func(t *testing.T) {
		f := &PrivacyFilter{MaskPIDs: true}
		if f.IsNoop() {
			t.Error("filter with masking should not be noop")
		}
	})

	t.Run("with paths is not noop", func(t *testing.T) {
		f := &PrivacyFilter{AllowedPaths: []string{"/foo/*"}}
		if f.IsNoop() {
			t.Error("filter with allowed paths should not be noop")
		}
	})
}

func TestMatchPathOrParent_WindowsPaths(t *testing.T) {
	// On Windows, filepath.Dir(`C:\`) returns `C:\` (the drive root is its own
	// parent). The loop must terminate via the p == filepath.Dir(p) condition
	// rather than the old p == "/" check, which only covers Unix roots.
	//
	// These tests use forward-slash paths so they run on any platform, but the
	// termination logic is the same: p == filepath.Dir(p) fires at every root.
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{
			name:    "drive-root pattern matches child",
			pattern: "/",
			path:    "/project",
			want:    false, // "/" is excluded as the loop stops before checking the root
		},
		{
			name:    "exact path match",
			pattern: "/home/user/project",
			path:    "/home/user/project",
			want:    true,
		},
		{
			name:    "parent glob matches nested path",
			pattern: "/home/user/*",
			path:    "/home/user/work/src",
			want:    true,
		},
		{
			name:    "no match returns false without infinite loop",
			pattern: "/other/*",
			path:    "/home/user/project",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchPathOrParent(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("matchPathOrParent(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

func TestShortHash_Deterministic(t *testing.T) {
	a := shortHash("claude:abc123")
	b := shortHash("claude:abc123")
	if a != b {
		t.Errorf("shortHash not deterministic: %q vs %q", a, b)
	}

	c := shortHash("claude:different")
	if a == c {
		t.Error("different inputs should produce different hashes")
	}
}
