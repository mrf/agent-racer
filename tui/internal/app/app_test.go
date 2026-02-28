package app

import (
	"strings"
	"testing"
	"time"

	"github.com/agent-racer/tui/internal/client"
)

func TestClassifyZone(t *testing.T) {
	tests := []struct {
		name     string
		session  *client.SessionState
		expected Zone
	}{
		{
			name:     "complete → parked",
			session:  &client.SessionState{Activity: client.ActivityComplete},
			expected: ZoneParked,
		},
		{
			name:     "errored → parked",
			session:  &client.SessionState{Activity: client.ActivityErrored},
			expected: ZoneParked,
		},
		{
			name:     "lost → parked",
			session:  &client.SessionState{Activity: client.ActivityLost},
			expected: ZoneParked,
		},
		{
			name:     "thinking → racing",
			session:  &client.SessionState{Activity: client.ActivityThinking},
			expected: ZoneRacing,
		},
		{
			name:     "tool_use → racing",
			session:  &client.SessionState{Activity: client.ActivityToolUse},
			expected: ZoneRacing,
		},
		{
			name: "idle with fresh data → racing",
			session: &client.SessionState{
				Activity:           client.ActivityIdle,
				LastDataReceivedAt: time.Now(),
			},
			expected: ZoneRacing,
		},
		{
			name: "idle with stale data → pit",
			session: &client.SessionState{
				Activity:           client.ActivityIdle,
				LastDataReceivedAt: time.Now().Add(-60 * time.Second),
			},
			expected: ZonePit,
		},
		{
			name:     "idle with zero time → pit",
			session:  &client.SessionState{Activity: client.ActivityIdle},
			expected: ZonePit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyZone(tt.session)
			if got != tt.expected {
				t.Errorf("classifyZone() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestSessionDisplayName(t *testing.T) {
	tests := []struct {
		name    string
		session *client.SessionState
		want    string
	}{
		{
			name:    "uses Name first",
			session: &client.SessionState{Name: "my-session", Slug: "slug", ID: "12345678"},
			want:    "my-session",
		},
		{
			name:    "falls back to Slug",
			session: &client.SessionState{Slug: "my-slug", ID: "12345678"},
			want:    "my-slug",
		},
		{
			name:    "falls back to ID prefix",
			session: &client.SessionState{ID: "12345678abcdef"},
			want:    "12345678",
		},
		{
			name:    "truncates long names",
			session: &client.SessionState{Name: "this-is-a-very-long-session-name-that-exceeds-limit"},
			want:    "this-is-a-very-long-ses…",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sessionDisplayName(tt.session)
			if got != tt.want {
				t.Errorf("sessionDisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestActivityGlyph(t *testing.T) {
	tests := map[string]string{
		"thinking": "●>",
		"tool_use": "⚙>",
		"idle":     "○",
		"waiting":  "◌",
		"starting": "◎",
		"complete": "✓",
		"errored":  "✗",
		"lost":     "?",
		"unknown":  "·",
	}
	for activity, expected := range tests {
		got := activityGlyph(activity)
		if got != expected {
			t.Errorf("activityGlyph(%q) = %q, want %q", activity, got, expected)
		}
	}
}

func TestRenderEmptyZoneMessages(t *testing.T) {
	m := New(nil, nil)
	m.width = 100

	racing := m.renderEmptyZone(ZoneRacing)
	if !strings.Contains(racing, "No active sessions") {
		t.Error("racing empty should mention 'No active sessions'")
	}

	pit := m.renderEmptyZone(ZonePit)
	if !strings.Contains(pit, "Pit lane empty") {
		t.Error("pit empty should mention 'Pit lane empty'")
	}

	parked := m.renderEmptyZone(ZoneParked)
	if !strings.Contains(parked, "No completed sessions") {
		t.Error("parked empty should mention 'No completed sessions'")
	}
}

func TestRenderEmptyZoneCompact(t *testing.T) {
	m := New(nil, nil)
	m.width = 40 // below breakpointCompact

	racing := m.renderEmptyZone(ZoneRacing)
	if !strings.Contains(racing, "(empty)") {
		t.Error("compact empty should show '(empty)'")
	}
}

func TestDisconnectOverlay(t *testing.T) {
	m := New(nil, nil)
	m.width = 80
	m.height = 24
	m.connected = false

	v := m.View()
	if !strings.Contains(v, "DISCONNECTED") {
		t.Error("disconnect overlay should contain 'DISCONNECTED'")
	}
	if !strings.Contains(v, "Reconnecting") {
		t.Error("disconnect overlay should contain 'Reconnecting'")
	}
}

func TestRenderContextBar(t *testing.T) {
	// Just check it doesn't panic at edge values.
	renderContextBar(0.0)
	renderContextBar(0.5)
	renderContextBar(0.8)
	renderContextBar(1.0)
	renderContextBar(1.5) // over 100%
}

func TestRebuildOrder(t *testing.T) {
	m := New(nil, nil)
	m.sessions = map[string]*client.SessionState{
		"a": {ID: "a", Activity: client.ActivityComplete, ContextUtilization: 0.5},
		"b": {ID: "b", Activity: client.ActivityThinking, ContextUtilization: 0.9},
		"c": {ID: "c", Activity: client.ActivityThinking, ContextUtilization: 0.3},
	}
	m.rebuildOrder()

	// Racing sessions (b, c) should come before parked (a).
	if len(m.order) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(m.order))
	}
	// b (racing, 0.9) should be first.
	if m.order[0] != "b" {
		t.Errorf("expected first to be 'b', got %q", m.order[0])
	}
	// c (racing, 0.3) second.
	if m.order[1] != "c" {
		t.Errorf("expected second to be 'c', got %q", m.order[1])
	}
	// a (parked) last.
	if m.order[2] != "a" {
		t.Errorf("expected third to be 'a', got %q", m.order[2])
	}
}
