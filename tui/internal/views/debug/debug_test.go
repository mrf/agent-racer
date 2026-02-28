package debug

import (
	"strings"
	"testing"
)

func TestAddEntry(t *testing.T) {
	m := New()
	m.Add("ws", "connected")
	if len(m.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m.Entries))
	}
	if m.Entries[0].Kind != "ws" {
		t.Errorf("expected kind 'ws', got %q", m.Entries[0].Kind)
	}
}

func TestMaxEntries(t *testing.T) {
	m := New()
	for i := 0; i < maxEntries+50; i++ {
		m.Add("ws", "msg")
	}
	if len(m.Entries) != maxEntries {
		t.Errorf("expected %d entries, got %d", maxEntries, len(m.Entries))
	}
}

func TestScrollUpDown(t *testing.T) {
	m := New()
	for i := 0; i < 20; i++ {
		m.Add("ws", "msg")
	}
	if m.Offset != 0 {
		t.Fatal("expected offset 0 after adds")
	}

	m.ScrollUp(5)
	if m.Offset != 5 {
		t.Errorf("expected offset 5, got %d", m.Offset)
	}

	m.ScrollDown(3)
	if m.Offset != 2 {
		t.Errorf("expected offset 2, got %d", m.Offset)
	}

	m.ScrollDown(10) // shouldn't go below 0
	if m.Offset != 0 {
		t.Errorf("expected offset 0, got %d", m.Offset)
	}
}

func TestScrollUpCapped(t *testing.T) {
	m := New()
	for i := 0; i < 5; i++ {
		m.Add("ws", "msg")
	}
	m.ScrollUp(100)
	if m.Offset != 4 { // max is len-1
		t.Errorf("expected offset 4, got %d", m.Offset)
	}
}

func TestViewEmpty(t *testing.T) {
	m := New()
	v := m.View(80, 20)
	if !strings.Contains(v, "No events") {
		t.Error("empty view should show 'No events' message")
	}
}

func TestViewWithEntries(t *testing.T) {
	m := New()
	m.Add("ws", "connected")
	m.Add("err", "timeout")
	v := m.View(80, 20)
	if !strings.Contains(v, "connected") {
		t.Error("view should contain 'connected'")
	}
	if !strings.Contains(v, "timeout") {
		t.Error("view should contain 'timeout'")
	}
}

func TestAddResetsScroll(t *testing.T) {
	m := New()
	for i := 0; i < 10; i++ {
		m.Add("ws", "msg")
	}
	m.ScrollUp(5)
	m.Add("ws", "new")
	if m.Offset != 0 {
		t.Error("adding entry should reset scroll to 0")
	}
}
