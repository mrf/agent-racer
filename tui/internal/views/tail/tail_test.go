package tail

import (
	"strings"
	"testing"
	"time"

	"github.com/agent-racer/tui/internal/client"
)

func makeSession() *client.SessionState {
	return &client.SessionState{
		ID:       "sess-123",
		Name:     "my-session",
		Slug:     "my-slug",
		Activity: client.ActivityThinking,
	}
}

func makeEntries(n int) []client.TailEntry {
	entries := make([]client.TailEntry, n)
	for i := 0; i < n; i++ {
		entries[i] = client.TailEntry{
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			Activity:  "thinking",
			Summary:   "thinking about stuff",
		}
	}
	return entries
}

func TestNew(t *testing.T) {
	s := makeSession()
	m := New(s)
	if m.SessionID != "sess-123" {
		t.Errorf("SessionID = %q, want %q", m.SessionID, "sess-123")
	}
	// Prefers Slug over Name
	if m.SessionName != "my-slug" {
		t.Errorf("SessionName = %q, want %q", m.SessionName, "my-slug")
	}
	if m.Activity != "thinking" {
		t.Errorf("Activity = %q, want %q", m.Activity, "thinking")
	}
	if !m.autoTail {
		t.Error("should start with autoTail enabled")
	}
}

func TestNew_NoSlug(t *testing.T) {
	s := &client.SessionState{
		ID:       "sess-456",
		Name:     "named-session",
		Activity: client.ActivityIdle,
	}
	m := New(s)
	if m.SessionName != "named-session" {
		t.Errorf("SessionName = %q, want %q", m.SessionName, "named-session")
	}
}

func TestUpdate_TailDataMsg(t *testing.T) {
	s := makeSession()
	m := New(s)

	entries := makeEntries(5)
	m, cmd := m.Update(TailDataMsg{Entries: entries, Offset: 1000})
	if len(m.entries) != 5 {
		t.Errorf("entries len = %d, want 5", len(m.entries))
	}
	if m.pollOff != 1000 {
		t.Errorf("pollOff = %d, want 1000", m.pollOff)
	}
	if cmd == nil {
		t.Error("should return a cmd to schedule next poll")
	}
}

func TestUpdate_TailDataMsg_AppendAndCap(t *testing.T) {
	s := makeSession()
	m := New(s)

	// Add near-max entries
	m, _ = m.Update(TailDataMsg{Entries: makeEntries(maxEntries - 5), Offset: 100})
	if len(m.entries) != maxEntries-5 {
		t.Fatalf("entries len = %d, want %d", len(m.entries), maxEntries-5)
	}

	// Add more to exceed cap
	m, _ = m.Update(TailDataMsg{Entries: makeEntries(20), Offset: 200})
	if len(m.entries) != maxEntries {
		t.Errorf("entries should be capped at %d, got %d", maxEntries, len(m.entries))
	}
}

func TestUpdate_TailDataMsg_Error(t *testing.T) {
	s := makeSession()
	m := New(s)

	m, cmd := m.Update(TailDataMsg{Err: &testError{"poll failed"}})
	// Should schedule retry
	if cmd == nil {
		t.Error("error should schedule retry poll")
	}
	if len(m.entries) != 0 {
		t.Error("entries should be unchanged on error")
	}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestUpdate_TailDataMsg_EmptyEntries(t *testing.T) {
	s := makeSession()
	m := New(s)
	m, _ = m.Update(TailDataMsg{Entries: makeEntries(3), Offset: 100})

	m, _ = m.Update(TailDataMsg{Entries: nil, Offset: 200})
	if len(m.entries) != 3 {
		t.Errorf("entries should be unchanged with nil entries, got %d", len(m.entries))
	}
	if m.pollOff != 200 {
		t.Errorf("pollOff should update even with empty entries, got %d", m.pollOff)
	}
}

func TestScrollUp(t *testing.T) {
	s := makeSession()
	m := New(s)
	m, _ = m.Update(TailDataMsg{Entries: makeEntries(20), Offset: 100})

	m.ScrollUp(5)
	if m.offset != 5 {
		t.Errorf("offset = %d, want 5", m.offset)
	}
	if m.autoTail {
		t.Error("ScrollUp should disable autoTail")
	}
}

func TestScrollUp_Capped(t *testing.T) {
	s := makeSession()
	m := New(s)
	m, _ = m.Update(TailDataMsg{Entries: makeEntries(10), Offset: 100})

	m.ScrollUp(100)
	if m.offset != 9 { // max is len-1
		t.Errorf("offset = %d, want 9", m.offset)
	}
}

func TestScrollUp_EmptyEntries(t *testing.T) {
	s := makeSession()
	m := New(s)
	m.ScrollUp(5)
	if m.offset != 0 {
		t.Errorf("offset with no entries = %d, want 0", m.offset)
	}
}

func TestScrollDown(t *testing.T) {
	s := makeSession()
	m := New(s)
	m, _ = m.Update(TailDataMsg{Entries: makeEntries(20), Offset: 100})

	m.ScrollUp(10)
	m.ScrollDown(3)
	if m.offset != 7 {
		t.Errorf("offset = %d, want 7", m.offset)
	}
	if m.autoTail {
		t.Error("should not re-enable autoTail yet")
	}
}

func TestScrollDown_ReEnablesAutoTail(t *testing.T) {
	s := makeSession()
	m := New(s)
	m, _ = m.Update(TailDataMsg{Entries: makeEntries(20), Offset: 100})

	m.ScrollUp(5)
	m.ScrollDown(10) // overshoots to 0
	if m.offset != 0 {
		t.Errorf("offset = %d, want 0", m.offset)
	}
	if !m.autoTail {
		t.Error("scrolling to bottom should re-enable autoTail")
	}
}

func TestJumpToBottom(t *testing.T) {
	s := makeSession()
	m := New(s)
	m, _ = m.Update(TailDataMsg{Entries: makeEntries(20), Offset: 100})

	m.ScrollUp(10)
	m.JumpToBottom()
	if m.offset != 0 {
		t.Errorf("offset = %d, want 0", m.offset)
	}
	if !m.autoTail {
		t.Error("JumpToBottom should re-enable autoTail")
	}
}

func TestPollOffset(t *testing.T) {
	s := makeSession()
	m := New(s)
	if m.PollOffset() != 0 {
		t.Errorf("initial PollOffset = %d, want 0", m.PollOffset())
	}
	m, _ = m.Update(TailDataMsg{Entries: makeEntries(5), Offset: 500})
	if m.PollOffset() != 500 {
		t.Errorf("PollOffset = %d, want 500", m.PollOffset())
	}
}

func TestUpdateActivity(t *testing.T) {
	s := makeSession()
	m := New(s)
	m.UpdateActivity("tool_use")
	if m.Activity != "tool_use" {
		t.Errorf("Activity = %q, want %q", m.Activity, "tool_use")
	}
}

func TestView_Empty(t *testing.T) {
	s := makeSession()
	m := New(s)
	view := m.View(80, 30)

	if !strings.Contains(view, "Waiting for data") {
		t.Error("empty view should show 'Waiting for data'")
	}
	if !strings.Contains(view, "my-slug") {
		t.Error("should show session name")
	}
	if !strings.Contains(view, "0 entries") {
		t.Error("should show 0 entries")
	}
}

func TestView_WithEntries(t *testing.T) {
	s := makeSession()
	m := New(s)
	m, _ = m.Update(TailDataMsg{Entries: makeEntries(5), Offset: 100})

	view := m.View(80, 30)
	if !strings.Contains(view, "thinking about stuff") {
		t.Error("should show entry summary")
	}
	if !strings.Contains(view, "5 entries") {
		t.Error("should show entry count")
	}
}

func TestView_AutoTailIndicator(t *testing.T) {
	s := makeSession()
	m := New(s)
	m, _ = m.Update(TailDataMsg{Entries: makeEntries(5), Offset: 100})

	// Auto-tail enabled
	view := m.View(80, 30)
	if !strings.Contains(view, "LIVE") {
		t.Error("should show LIVE indicator when auto-tailing")
	}

	// Auto-tail disabled
	m.ScrollUp(2)
	view = m.View(80, 30)
	if strings.Contains(view, "LIVE") {
		t.Error("should not show LIVE indicator when scrolled up")
	}
}

func TestView_ScrollIndicator(t *testing.T) {
	s := makeSession()
	m := New(s)
	m, _ = m.Update(TailDataMsg{Entries: makeEntries(5), Offset: 100})

	m.ScrollUp(3)
	view := m.View(80, 30)
	if !strings.Contains(view, "3 more") {
		t.Error("should show scroll indicator with offset count")
	}
}

func TestView_NarrowDimensions(t *testing.T) {
	s := makeSession()
	m := New(s)
	m, _ = m.Update(TailDataMsg{Entries: makeEntries(5), Offset: 100})

	// Very narrow - should not panic
	view := m.View(20, 8)
	if view == "" {
		t.Error("narrow view should not be empty")
	}
}

func TestView_FooterContent(t *testing.T) {
	s := makeSession()
	m := New(s)
	view := m.View(80, 30)

	if !strings.Contains(view, "j/k:scroll") {
		t.Error("should contain scroll help")
	}
	if !strings.Contains(view, "esc:close") {
		t.Error("should contain close help")
	}
}

func TestActivityGlyphAndColor(t *testing.T) {
	tests := []struct {
		activity string
		want     string
	}{
		{"thinking", "●>"},
		{"tool_use", "⚙>"},
		{"tool_result", "←"},
		{"text", "··"},
		{"subagent", "◈"},
		{"compact", "⟲"},
		{"system", "◇"},
		{"unknown", "·"},
	}
	for _, tt := range tests {
		t.Run(tt.activity, func(t *testing.T) {
			glyph, color := activityGlyphAndColor(tt.activity)
			if glyph != tt.want {
				t.Errorf("glyph = %q, want %q", glyph, tt.want)
			}
			if string(color) == "" {
				t.Error("color should not be empty")
			}
		})
	}
}
