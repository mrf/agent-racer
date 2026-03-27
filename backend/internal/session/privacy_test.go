package session

import (
	"regexp"
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
	f := &PrivacyFilter{}
	if !f.IsNoop() {
		t.Error("zero value filter should be noop")
	}

	notNoop := []struct {
		name   string
		filter PrivacyFilter
	}{
		{"MaskWorkingDirs", PrivacyFilter{MaskWorkingDirs: true}},
		{"MaskSessionIDs", PrivacyFilter{MaskSessionIDs: true}},
		{"MaskPIDs", PrivacyFilter{MaskPIDs: true}},
		{"MaskTmuxTargets", PrivacyFilter{MaskTmuxTargets: true}},
		{"AllowedPaths", PrivacyFilter{AllowedPaths: []string{"/foo"}}},
		{"BlockedPaths", PrivacyFilter{BlockedPaths: []string{"/bar"}}},
	}
	for _, tt := range notNoop {
		t.Run(tt.name, func(t *testing.T) {
			if tt.filter.IsNoop() {
				t.Errorf("filter with %s set should not be noop", tt.name)
			}
		})
	}
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

// TestPrivacyFilter_Apply_MaskWorkingDirs_SubagentsUnaffected documents that
// SubagentState has no WorkingDir field, so MaskWorkingDirs has no effect on
// subagents. If SubagentState gains a WorkingDir field in the future,
// PrivacyFilter.Apply must be updated to mask it.
func TestPrivacyFilter_Apply_MaskWorkingDirs_SubagentsUnaffected(t *testing.T) {
	original := &SessionState{
		ID:         "claude:abc123",
		WorkingDir: "/home/user/secret/project",
		Subagents: []SubagentState{
			{ID: "toolu_01", SessionID: "claude:abc123", Slug: "task-a"},
		},
	}

	f := &PrivacyFilter{MaskWorkingDirs: true}
	result := f.Apply(original)

	if result.WorkingDir != "project" {
		t.Errorf("masked WorkingDir = %q, want %q", result.WorkingDir, "project")
	}

	// Subagents must be preserved unchanged (SubagentState has no WorkingDir).
	if len(result.Subagents) != len(original.Subagents) {
		t.Errorf("subagent count changed: got %d, want %d", len(result.Subagents), len(original.Subagents))
	}
}

func TestPrivacyFilter_Apply_MaskSessionIDs_MasksSubagentSessionID(t *testing.T) {
	original := &SessionState{
		ID:         "claude:abc123",
		WorkingDir: "/home/user/project",
		Subagents: []SubagentState{
			{ID: "toolu_01", SessionID: "claude:abc123", Slug: "task-a"},
			{ID: "toolu_02", SessionID: "claude:abc123", Slug: "task-b"},
		},
	}

	f := &PrivacyFilter{MaskSessionIDs: true}
	result := f.Apply(original)

	if result.ID == original.ID {
		t.Error("parent session ID should have been masked")
	}

	for _, sa := range result.Subagents {
		if sa.SessionID == "claude:abc123" {
			t.Errorf("subagent SessionID should have been masked, got %q", sa.SessionID)
		}
		if sa.SessionID == "" {
			t.Error("masked subagent SessionID should not be empty")
		}
		// Subagent SessionID was the same as parent ID, so masked values must match.
		if sa.SessionID != result.ID {
			t.Errorf("subagent SessionID %q should match masked parent ID %q", sa.SessionID, result.ID)
		}
	}
}

func TestShortHash(t *testing.T) {
	a := shortHash("claude:abc123")
	b := shortHash("claude:abc123")
	if a != b {
		t.Errorf("shortHash not deterministic: %q vs %q", a, b)
	}

	c := shortHash("claude:different")
	if a == c {
		t.Error("different inputs should produce different hashes")
	}

	if !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(a) {
		t.Errorf("expected 12-char lowercase hex string, got %q", a)
	}
}

func TestPrivacyFilter_Apply_EmptyFields(t *testing.T) {
	t.Run("empty WorkingDir stays empty when masked", func(t *testing.T) {
		s := &SessionState{ID: "claude:1", WorkingDir: ""}
		f := &PrivacyFilter{MaskWorkingDirs: true}
		result := f.Apply(s)
		if result.WorkingDir != "" {
			t.Errorf("expected empty WorkingDir, got %q", result.WorkingDir)
		}
	})

	t.Run("empty ID stays empty when masked", func(t *testing.T) {
		s := &SessionState{ID: "", WorkingDir: "/home/user/project"}
		f := &PrivacyFilter{MaskSessionIDs: true}
		result := f.Apply(s)
		if result.ID != "" {
			t.Errorf("expected empty ID, got %q", result.ID)
		}
	})
}

func TestPrivacyFilter_Apply_OriginalUnmodified(t *testing.T) {
	original := &SessionState{
		ID:         "claude:abc123",
		WorkingDir: "/home/user/projects/myproject",
		PID:        42,
		TmuxTarget: "main:1.0",
		Subagents: []SubagentState{
			{ID: "toolu_01", SessionID: "claude:sub1", Slug: "worker"},
		},
	}

	f := &PrivacyFilter{
		MaskWorkingDirs: true,
		MaskSessionIDs:  true,
		MaskPIDs:        true,
		MaskTmuxTargets: true,
	}
	_ = f.Apply(original)

	if original.ID != "claude:abc123" {
		t.Errorf("original ID modified: %q", original.ID)
	}
	if original.WorkingDir != "/home/user/projects/myproject" {
		t.Errorf("original WorkingDir modified: %q", original.WorkingDir)
	}
	if original.PID != 42 {
		t.Errorf("original PID modified: %d", original.PID)
	}
	if original.TmuxTarget != "main:1.0" {
		t.Errorf("original TmuxTarget modified: %q", original.TmuxTarget)
	}
	if original.Subagents[0].SessionID != "claude:sub1" {
		t.Errorf("original subagent SessionID modified: %q", original.Subagents[0].SessionID)
	}
}

func TestPrivacyFilter_Apply_SubagentEmptySessionID(t *testing.T) {
	s := &SessionState{
		ID: "claude:abc123",
		Subagents: []SubagentState{
			{ID: "toolu_01", SessionID: "", Slug: "no-session"},
			{ID: "toolu_02", SessionID: "claude:abc123", Slug: "has-session"},
		},
	}

	f := &PrivacyFilter{MaskSessionIDs: true}
	result := f.Apply(s)

	if result.Subagents[0].SessionID != "" {
		t.Errorf("empty subagent SessionID should stay empty, got %q", result.Subagents[0].SessionID)
	}
	if result.Subagents[1].SessionID == "claude:abc123" {
		t.Error("non-empty subagent SessionID should be masked")
	}
}

func TestPrivacyFilter_Apply_NoSubagents(t *testing.T) {
	s := &SessionState{ID: "claude:abc123", Subagents: nil}
	f := &PrivacyFilter{MaskSessionIDs: true}
	result := f.Apply(s)

	if result.Subagents != nil {
		t.Errorf("expected nil subagents, got %v", result.Subagents)
	}
	if result.ID == "claude:abc123" {
		t.Error("ID should still be masked")
	}
}

func TestPrivacyFilter_Apply_NonSensitiveFieldsPreserved(t *testing.T) {
	s := &SessionState{
		ID:           "claude:abc123",
		Name:         "my-project",
		Source:       "claude",
		Activity:     Thinking,
		TokensUsed:   5000,
		Model:        "opus",
		WorkingDir:   "/home/user/project",
		MessageCount: 10,
		PID:          999,
		TmuxTarget:   "dev:0.1",
	}

	f := &PrivacyFilter{
		MaskWorkingDirs: true,
		MaskSessionIDs:  true,
		MaskPIDs:        true,
		MaskTmuxTargets: true,
	}
	result := f.Apply(s)

	// Non-sensitive fields must pass through unchanged.
	if result.Name != "my-project" {
		t.Errorf("Name changed: %q", result.Name)
	}
	if result.Source != "claude" {
		t.Errorf("Source changed: %q", result.Source)
	}
	if result.Activity != Thinking {
		t.Errorf("Activity changed: %v", result.Activity)
	}
	if result.TokensUsed != 5000 {
		t.Errorf("TokensUsed changed: %d", result.TokensUsed)
	}
	if result.Model != "opus" {
		t.Errorf("Model changed: %q", result.Model)
	}
	if result.MessageCount != 10 {
		t.Errorf("MessageCount changed: %d", result.MessageCount)
	}
}

func TestPrivacyFilter_FilterSlice_Empty(t *testing.T) {
	f := &PrivacyFilter{MaskPIDs: true}
	result := f.FilterSlice(nil)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}

	result = f.FilterSlice([]*SessionState{})
	if len(result) != 0 {
		t.Errorf("expected empty result for empty slice, got %d", len(result))
	}
}

func TestPrivacyFilter_FilterSlice_AllBlocked(t *testing.T) {
	sessions := []*SessionState{
		{ID: "claude:1", WorkingDir: "/tmp/a"},
		{ID: "claude:2", WorkingDir: "/tmp/b"},
	}

	f := &PrivacyFilter{BlockedPaths: []string{"/tmp/*"}}
	result := f.FilterSlice(sessions)
	if len(result) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(result))
	}
}

func TestPrivacyFilter_FilterSlice_OriginalUnmodified(t *testing.T) {
	sessions := []*SessionState{
		{ID: "claude:1", WorkingDir: "/home/user/project", PID: 100},
		{ID: "claude:2", WorkingDir: "/tmp/scratch", PID: 200},
	}

	f := &PrivacyFilter{MaskPIDs: true, BlockedPaths: []string{"/tmp/*"}}
	_ = f.FilterSlice(sessions)

	if len(sessions) != 2 {
		t.Fatalf("original slice length changed: %d", len(sessions))
	}
	if sessions[0].PID != 100 {
		t.Errorf("original session PID modified: %d", sessions[0].PID)
	}
	if sessions[1].WorkingDir != "/tmp/scratch" {
		t.Errorf("original blocked session modified: %q", sessions[1].WorkingDir)
	}
}

func TestPrivacyFilter_FilterSlice_EmptyWorkingDirAllowed(t *testing.T) {
	sessions := []*SessionState{
		{ID: "claude:1", WorkingDir: ""},
		{ID: "claude:2", WorkingDir: "/allowed/project"},
	}

	f := &PrivacyFilter{AllowedPaths: []string{"/allowed/*"}}
	result := f.FilterSlice(sessions)

	// Empty WorkingDir is always allowed per IsAllowed contract.
	if len(result) != 2 {
		t.Fatalf("expected 2 sessions (empty dir always allowed), got %d", len(result))
	}
}
