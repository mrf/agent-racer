package detail

import (
	"strings"
	"testing"
	"time"

	"github.com/agent-racer/tui/internal/client"
)

func makeSession() *client.SessionState {
	now := time.Now()
	return &client.SessionState{
		ID:                 "sess-abcdefgh-1234",
		Name:               "fix-login-bug",
		Slug:               "fix-login",
		Source:             "claude",
		Activity:           client.ActivityThinking,
		Model:              "claude-opus-4-6",
		TokensUsed:         50000,
		MaxContextTokens:   200000,
		ContextUtilization: 0.25,
		CurrentTool:        "Read",
		WorkingDir:         "/home/user/projects/app",
		Branch:             "feature/login-fix",
		StartedAt:          now.Add(-10 * time.Minute),
		LastActivityAt:     now.Add(-5 * time.Second),
		MessageCount:       42,
		ToolCallCount:      15,
		CompactionCount:    1,
		PID:                12345,
		TmuxTarget:         "main:1",
		BurnRatePerMinute:  500,
	}
}

func TestNew(t *testing.T) {
	s := makeSession()
	m := New(s)
	if m.Session != s {
		t.Error("New should store session pointer")
	}
	if m.FocusMode {
		t.Error("FocusMode should default to false")
	}
}

func TestView_NilSession(t *testing.T) {
	m := Model{Session: nil}
	view := m.View()
	if view != "" {
		t.Error("View with nil session should return empty string")
	}
}

func TestView_BasicSession(t *testing.T) {
	s := makeSession()
	m := New(s)
	view := m.View()

	checks := []struct {
		label    string
		contains string
	}{
		{"session name", "fix-login-bug"},
		{"source", "claude"},
		{"model", "opus"},
		{"activity", "thinking"},
		{"tool", "Read"},
		{"branch", "feature/login-fix"},
		{"working dir", "/home/user/projects/app"},
		{"tmux target", "main:1"},
		{"PID", "12345"},
		{"burn rate", "500 tok/min"},
		{"messages", "42 msgs"},
		{"tool calls", "15 tool calls"},
		{"compactions", "1 compactions"},
	}

	for _, check := range checks {
		if !strings.Contains(view, check.contains) {
			t.Errorf("view should contain %s (%q)", check.label, check.contains)
		}
	}
}

func TestView_CompletedSession(t *testing.T) {
	s := makeSession()
	completed := time.Now().Add(-2 * time.Minute)
	s.Activity = client.ActivityComplete
	s.CompletedAt = &completed
	s.LastAssistantText = "All done!"

	m := New(s)
	view := m.View()

	if !strings.Contains(view, "Summary") {
		t.Error("completed session should show 'Summary' label")
	}
	// Check for core content that survives glamour markdown rendering.
	if !strings.Contains(view, "done") {
		t.Error("completed session should show assistant text content")
	}
}

func TestView_ErroredSession(t *testing.T) {
	s := makeSession()
	s.Activity = client.ActivityErrored
	s.LastAssistantText = "something went wrong"

	m := New(s)
	view := m.View()

	if !strings.Contains(view, "Error") {
		t.Error("errored session should show 'Error' label")
	}
}

func TestView_Subagents(t *testing.T) {
	s := makeSession()
	s.Subagents = []client.SubagentState{
		{
			ID:           "sub-1",
			Slug:         "test-runner",
			Model:        "claude-haiku-4-5",
			Activity:     client.ActivityToolUse,
			CurrentTool:  "Bash",
			TokensUsed:   5000,
			MessageCount: 8,
		},
	}

	m := New(s)
	view := m.View()

	if !strings.Contains(view, "Subagents (1)") {
		t.Error("should show subagent count")
	}
	if !strings.Contains(view, "test-runner") {
		t.Error("should show subagent slug")
	}
}

func TestView_FocusError(t *testing.T) {
	s := makeSession()
	m := New(s)
	m.FocusError = "tmux not found"
	view := m.View()

	if !strings.Contains(view, "Focus error: tmux not found") {
		t.Error("should show focus error")
	}
}

func TestView_FooterVariants(t *testing.T) {
	t.Run("normal_with_tmux", func(t *testing.T) {
		m := New(makeSession())
		if !strings.Contains(m.View(), "[f] focus/split") {
			t.Error("normal footer should show focus/split hint")
		}
	})

	t.Run("no_tmux_target", func(t *testing.T) {
		s := makeSession()
		s.TmuxTarget = ""
		if !strings.Contains(New(s).View(), "no tmux target") {
			t.Error("no-tmux footer should indicate no target")
		}
	})

	t.Run("focus_mode_no_split", func(t *testing.T) {
		m := New(makeSession())
		m.FocusMode = true
		m.CanSplit = false
		if !strings.Contains(m.View(), "[f] focus window") {
			t.Error("focus mode without split should show focus window")
		}
	})

	t.Run("focus_mode_with_split", func(t *testing.T) {
		m := New(makeSession())
		m.FocusMode = true
		m.CanSplit = true
		if !strings.Contains(m.View(), "[s] split side-by-side") {
			t.Error("focus mode with split should show split option")
		}
	})
}

func TestView_NoOptionalFields(t *testing.T) {
	s := &client.SessionState{
		ID:       "minimal-session",
		Source:   "claude",
		Activity: client.ActivityStarting,
		Model:    "claude-sonnet-4-6",
	}
	m := New(s)
	// Should not panic with missing fields
	view := m.View()
	if view == "" {
		t.Error("view should not be empty for minimal session")
	}
}

func TestDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		session  client.SessionState
		expected string
	}{
		{"prefers name", client.SessionState{ID: "abcdefgh-1234", Name: "my-session", Slug: "my-slug"}, "my-session"},
		{"falls back to slug", client.SessionState{ID: "abcdefgh-1234", Slug: "my-slug"}, "my-slug"},
		{"falls back to short ID", client.SessionState{ID: "abcdefgh-1234"}, "abcdefgh"},
		{"short ID as-is", client.SessionState{ID: "abc"}, "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DisplayName(&tt.session)
			if got != tt.expected {
				t.Errorf("DisplayName() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s    string
		max  int
		want string
	}{
		{"short", 10, "short"},
		{"exactly", 7, "exactly"},
		{"this is long", 8, "this is…"},
	}
	for _, tt := range tests {
		got := truncate(tt.s, tt.max)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
		}
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{500, "500"},
		{1500, "1.5k"},
		{50000, "50.0k"},
		{1500000, "1.5M"},
	}
	for _, tt := range tests {
		got := formatTokens(tt.n)
		if got != tt.want {
			t.Errorf("formatTokens(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestFormatAge(t *testing.T) {
	now := time.Now()

	got := formatAge(now.Add(-30 * time.Second))
	if !strings.Contains(got, "s ago") {
		t.Errorf("30s ago should contain 's ago', got %q", got)
	}

	got = formatAge(now.Add(-5 * time.Minute))
	if !strings.Contains(got, "m") {
		t.Errorf("5m ago should contain 'm', got %q", got)
	}

	got = formatAge(now.Add(-2 * time.Hour))
	if !strings.Contains(got, "h") {
		t.Errorf("2h ago should contain 'h', got %q", got)
	}
}

func TestRenderBar(t *testing.T) {
	// 0% bar
	bar := renderBar(0.0, 10, "#22c55e")
	if !strings.Contains(bar, "░") {
		t.Error("0% bar should have empty blocks")
	}

	// 100% bar
	bar = renderBar(1.0, 10, "#22c55e")
	if !strings.Contains(bar, "█") {
		t.Error("100% bar should have filled blocks")
	}

	// Clamped above 1.0
	bar = renderBar(1.5, 10, "#22c55e")
	if bar == "" {
		t.Error("over-1.0 bar should not be empty")
	}

	// Clamped below 0
	bar = renderBar(-0.5, 10, "#22c55e")
	if bar == "" {
		t.Error("negative bar should not be empty")
	}
}
