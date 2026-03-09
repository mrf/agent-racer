package track

import (
	"regexp"
	"strings"
	"testing"

	"github.com/agent-racer/tui/internal/client"
	"github.com/charmbracelet/lipgloss"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestRenderRacingLineKeepsFinishAlignedAcrossVariableNameAndGlyphWidths(t *testing.T) {
	width := 96
	burnHist := []float64{0.5, 1.0, 2.0}
	sessions := []*client.SessionState{
		{
			ID:                 "short-id",
			Name:               "Ada",
			Source:             "claude",
			Model:              "claude-sonnet-4",
			Activity:           client.ActivityIdle,
			ContextUtilization: 1.0,
			TokensUsed:         123000,
			BurnRatePerMinute:  3.2,
		},
		{
			ID:                 "long-id",
			Name:               "Longer session name for truncation",
			Source:             "claude",
			Model:              "claude-sonnet-4",
			Activity:           client.ActivityThinking,
			ContextUtilization: 1.0,
			TokensUsed:         123000,
			BurnRatePerMinute:  3.2,
		},
	}

	expectedPctColumn := -1
	for i := 0; i < len(sessions); i++ {
		line := renderRacingLine(i, sessions[i], false, width, burnHist, 0, false)
		plain := stripANSI(line)
		pctIndex := strings.Index(plain, " 100%")
		if pctIndex == -1 {
			t.Fatalf("rendered line missing percent block: %q", plain)
		}
		pctColumn := lipgloss.Width(plain[:pctIndex])
		if expectedPctColumn == -1 {
			expectedPctColumn = pctColumn
			continue
		}
		if pctColumn != expectedPctColumn {
			t.Fatalf("percent block column = %d, want %d for line %q", pctColumn, expectedPctColumn, plain)
		}
	}
}

func TestDisplayNameTruncatesToConfiguredDisplayWidth(t *testing.T) {
	s := &client.SessionState{
		ID:   "wide-id",
		Name: strings.Repeat("A\u540d", 12),
	}

	name := displayName(s, nameWidth)
	if lipgloss.Width(name) > nameWidth {
		t.Fatalf("display width = %d, want <= %d for %q", lipgloss.Width(name), nameWidth, name)
	}
}

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}
