package status

import (
	"strings"
	"testing"

	"github.com/agent-racer/tui/internal/client"
)

func TestNew(t *testing.T) {
	m := New()
	if m.Connected {
		t.Error("New() should default to not connected")
	}
	if m.SourceHealth == nil {
		t.Error("New() should initialize SourceHealth map")
	}
}

func TestSetCounts(t *testing.T) {
	m := New()
	m.SetCounts(5, 3, 2)
	if m.Racing != 5 {
		t.Errorf("Racing = %d, want 5", m.Racing)
	}
	if m.Pit != 3 {
		t.Errorf("Pit = %d, want 3", m.Pit)
	}
	if m.Parked != 2 {
		t.Errorf("Parked = %d, want 2", m.Parked)
	}
}

func TestView_Connected(t *testing.T) {
	m := New()
	m.Connected = true
	m.Width = 80
	m.SetCounts(3, 1, 2)

	view := m.View()
	if !strings.Contains(view, "Connected") {
		t.Error("connected view should contain 'Connected'")
	}
	if !strings.Contains(view, "3 racing") {
		t.Error("should show racing count")
	}
	if !strings.Contains(view, "1 pit") {
		t.Error("should show pit count")
	}
	if !strings.Contains(view, "2 parked") {
		t.Error("should show parked count")
	}
}

func TestView_Disconnected(t *testing.T) {
	m := New()
	m.Connected = false
	m.Width = 80

	view := m.View()
	if !strings.Contains(view, "Connecting") {
		t.Error("disconnected view should contain 'Connecting'")
	}
}

func TestView_DisconnectedWithSpinner(t *testing.T) {
	m := New()
	m.Connected = false
	m.Width = 80
	m.SpinnerView = "⠋"

	view := m.View()
	if !strings.Contains(view, "⠋") {
		t.Error("should show spinner when provided")
	}
}

func TestView_DisconnectedFallback(t *testing.T) {
	m := New()
	m.Connected = false
	m.Width = 80
	m.SpinnerView = ""

	view := m.View()
	if !strings.Contains(view, "○") {
		t.Error("should show fallback glyph when no spinner")
	}
}

func TestView_NarrowWidth(t *testing.T) {
	m := New()
	m.Connected = true
	m.Width = 20 // below 40 threshold

	// Should not panic, uses minimum width of 40
	view := m.View()
	if view == "" {
		t.Error("narrow view should not be empty")
	}
}

func TestView_WithSourceHealth(t *testing.T) {
	m := New()
	m.Connected = true
	m.Width = 120
	m.SourceHealth = map[string]client.SourceHealthPayload{
		"claude": {Source: "claude", Status: client.StatusHealthy},
		"gemini": {Source: "gemini", Status: client.StatusDegraded},
	}

	view := m.View()
	if !strings.Contains(view, "claude") {
		t.Error("should show claude source health")
	}
	if !strings.Contains(view, "gemini") {
		t.Error("should show gemini source health")
	}
	if !strings.Contains(view, "healthy") {
		t.Error("should show healthy status")
	}
	if !strings.Contains(view, "degraded") {
		t.Error("should show degraded status")
	}
}

func TestView_SourceHealthStatuses(t *testing.T) {
	statuses := []client.SourceHealthStatus{
		client.StatusHealthy,
		client.StatusDegraded,
		client.StatusFailed,
		"unknown",
	}
	for _, s := range statuses {
		t.Run(string(s), func(t *testing.T) {
			m := New()
			m.Connected = true
			m.Width = 80
			m.SourceHealth["test"] = client.SourceHealthPayload{Source: "test", Status: s}
			if view := m.View(); view == "" {
				t.Errorf("view with status %q should not be empty", s)
			}
		})
	}
}

func TestView_ZeroCounts(t *testing.T) {
	m := New()
	m.Connected = true
	m.Width = 80
	m.SetCounts(0, 0, 0)

	view := m.View()
	if !strings.Contains(view, "0 racing") {
		t.Error("should show 0 racing")
	}
	if !strings.Contains(view, "0 pit") {
		t.Error("should show 0 pit")
	}
	if !strings.Contains(view, "0 parked") {
		t.Error("should show 0 parked")
	}
}
