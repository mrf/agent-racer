package dashboard

import (
	"strings"
	"testing"

	"github.com/agent-racer/tui/internal/client"
)

func TestLeaderboardHidesActivityForTerminalSessions(t *testing.T) {
	m := New()
	m.Width = 120

	sessions := map[string]*client.SessionState{
		"active": {
			ID:                 "active",
			Activity:           client.ActivityThinking,
			Model:              "claude-sonnet-4-6",
			ContextUtilization: 0.5,
		},
		"done": {
			ID:                 "done",
			Activity:           client.ActivityComplete,
			Model:              "claude-sonnet-4-6",
			ContextUtilization: 0.8,
		},
		"gone": {
			ID:                 "gone",
			Activity:           client.ActivityLost,
			Model:              "claude-sonnet-4-6",
			ContextUtilization: 0.3,
		},
	}
	m.SetSessions(sessions)

	view := m.View()

	if !strings.Contains(view, "thinking") {
		t.Error("active session should show 'thinking' in leaderboard")
	}

	// Terminal activities should not appear as activity text in the leaderboard.
	// The word "complete" appears in the data but should not be rendered as
	// an activity indicator for terminal sessions.
	lines := strings.Split(view, "\n")
	for _, line := range lines {
		if !strings.Contains(line, "sonnet") {
			continue
		}
		if strings.Contains(line, "done") || strings.Contains(line, "gone") {
			// These are terminal session rows.
			if strings.Contains(line, "complete") {
				t.Errorf("terminal session row should not show 'complete' activity: %s", line)
			}
			if strings.Contains(line, "lost") {
				t.Errorf("terminal session row should not show 'lost' activity: %s", line)
			}
		}
	}
}
