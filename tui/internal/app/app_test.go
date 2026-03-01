package app

import (
	"strings"
	"testing"
	"time"

	"github.com/agent-racer/tui/internal/client"
	"github.com/agent-racer/tui/internal/views/track"
)

func TestClassifyZone(t *testing.T) {
	tests := []struct {
		name     string
		session  *client.SessionState
		expected track.Zone
	}{
		{
			name:     "complete → parked",
			session:  &client.SessionState{Activity: client.ActivityComplete},
			expected: track.ZoneParked,
		},
		{
			name:     "errored → parked",
			session:  &client.SessionState{Activity: client.ActivityErrored},
			expected: track.ZoneParked,
		},
		{
			name:     "lost → parked",
			session:  &client.SessionState{Activity: client.ActivityLost},
			expected: track.ZoneParked,
		},
		{
			name:     "thinking → racing",
			session:  &client.SessionState{Activity: client.ActivityThinking},
			expected: track.ZoneRacing,
		},
		{
			name:     "tool_use → racing",
			session:  &client.SessionState{Activity: client.ActivityToolUse},
			expected: track.ZoneRacing,
		},
		{
			name: "idle with fresh data → racing",
			session: &client.SessionState{
				Activity:           client.ActivityIdle,
				LastDataReceivedAt: time.Now(),
			},
			expected: track.ZoneRacing,
		},
		{
			name: "idle with stale data → pit",
			session: &client.SessionState{
				Activity:           client.ActivityIdle,
				LastDataReceivedAt: time.Now().Add(-60 * time.Second),
			},
			expected: track.ZonePit,
		},
		{
			name:     "idle with zero time → pit",
			session:  &client.SessionState{Activity: client.ActivityIdle},
			expected: track.ZonePit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := track.Classify(tt.session)
			if got != tt.expected {
				t.Errorf("Classify() = %d, want %d", got, tt.expected)
			}
		})
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
