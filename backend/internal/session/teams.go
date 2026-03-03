package session

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"
)

// teamPalette is a curated list of distinct, vibrant colors for dark backgrounds.
// Selected to avoid confusion with model colors (purple for Opus, blue for Sonnet,
// green for Haiku, dark-blue for Codex).
var teamPalette = []string{
	"#e74c3c", // red
	"#e67e22", // orange
	"#f1c40f", // yellow
	"#1abc9c", // teal
	"#3498db", // sky blue
	"#9b59b6", // purple
	"#ff6b6b", // salmon
	"#48dbfb", // cyan
	"#ff9ff3", // pink
	"#feca57", // gold
	"#54a0ff", // periwinkle
	"#01cbc6", // dark cyan
	"#ff6b81", // rose
	"#a29bfe", // lavender
	"#fd79a8", // hot pink
	"#6c5ce7", // indigo
}

// TeamInfo aggregates sessions sharing the same project (working-directory basename).
type TeamInfo struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Color           string   `json:"color"`
	MemberIDs       []string `json:"memberIds"`
	SessionCount    int      `json:"sessionCount"`
	ActiveCount     int      `json:"activeCount"`
	TotalTokens     int      `json:"totalTokens"`
	AvgBurnRate     float64  `json:"avgBurnRate"`
	CompletionCount int      `json:"completionCount"`
	ErrorCount      int      `json:"errorCount"`
}

// teamColor returns a deterministic color for the given team name by hashing it
// and indexing into the palette.
func teamColor(name string) string {
	h := sha256.Sum256([]byte(name))
	return teamPalette[int(h[0])%len(teamPalette)]
}

// teamID returns a stable short hex ID derived from the team name.
func teamID(name string) string {
	h := sha256.Sum256([]byte(name))
	return fmt.Sprintf("%x", h[:4])
}

// ComputeTeams groups sessions by their working-directory basename and returns
// one TeamInfo per unique project. Teams are sorted by total tokens descending.
// Sessions with an empty or unparseable WorkingDir are grouped under "unknown".
func ComputeTeams(sessions []*SessionState) []TeamInfo {
	type entry struct {
		name    string
		members []*SessionState
	}
	byName := make(map[string]*entry)

	for _, s := range sessions {
		name := filepath.Base(s.WorkingDir)
		if name == "" || name == "." {
			name = "unknown"
		}
		e, ok := byName[name]
		if !ok {
			e = &entry{name: name}
			byName[name] = e
		}
		e.members = append(e.members, s)
	}

	teams := make([]TeamInfo, 0, len(byName))
	for _, e := range byName {
		var totalTokens, activeCount, completionCount, errorCount int
		var burnRateSum float64
		var burnRateCount int
		memberIDs := make([]string, 0, len(e.members))

		for _, m := range e.members {
			memberIDs = append(memberIDs, m.ID)
			totalTokens += m.TokensUsed
			if !m.IsTerminal() {
				activeCount++
				if m.BurnRatePerMinute > 0 {
					burnRateSum += m.BurnRatePerMinute
					burnRateCount++
				}
			}
			switch m.Activity {
			case Complete:
				completionCount++
			case Errored:
				errorCount++
			}
		}

		var avgBurnRate float64
		if burnRateCount > 0 {
			avgBurnRate = burnRateSum / float64(burnRateCount)
		}

		teams = append(teams, TeamInfo{
			ID:              teamID(e.name),
			Name:            e.name,
			Color:           teamColor(e.name),
			MemberIDs:       memberIDs,
			SessionCount:    len(e.members),
			ActiveCount:     activeCount,
			TotalTokens:     totalTokens,
			AvgBurnRate:     avgBurnRate,
			CompletionCount: completionCount,
			ErrorCount:      errorCount,
		})
	}

	sort.Slice(teams, func(i, j int) bool {
		return teams[i].TotalTokens > teams[j].TotalTokens
	})

	return teams
}
