// Package achievements provides the achievements modal overlay for the TUI.
package achievements

import (
	"fmt"
	"strings"
	"time"

	"github.com/agent-racer/tui/internal/client"
	"github.com/agent-racer/tui/internal/theme"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// categories defines the tab order for the panel.
var categories = []string{
	"Session Milestones",
	"Source Diversity",
	"Model Collection",
	"Performance & Endurance",
	"Spectacle",
	"Streaks",
}

// LoadedMsg is returned when the /api/achievements fetch completes.
type LoadedMsg struct {
	Items []client.AchievementResponse
	Err   error
}

// FetchCmd returns a Bubble Tea command that fetches achievements via HTTP.
func FetchCmd(h *client.HTTPClient) tea.Cmd {
	return func() tea.Msg {
		items, err := h.GetAchievements()
		return LoadedMsg{Items: items, Err: err}
	}
}

// Model holds the achievements panel state.
type Model struct {
	items       []client.AchievementResponse
	activeTab   int
	scroll      int
	loading     bool
	fetchErr    string
}

// New returns a Model in loading state.
func New() Model {
	return Model{loading: true}
}

// ApplyUnlock marks an achievement as unlocked when the WS notification arrives.
func (m *Model) ApplyUnlock(id string) {
	for i := 0; i < len(m.items); i++ {
		if m.items[i].ID == id {
			m.items[i].Unlocked = true
			if m.items[i].UnlockedAt == nil {
				now := time.Now()
				m.items[i].UnlockedAt = &now
			}
			return
		}
	}
}

// Update processes key messages forwarded from the parent when this overlay is active.
func (m Model) Update(msg tea.KeyMsg) Model {
	switch msg.String() {
	case "left", "h":
		if m.activeTab > 0 {
			m.activeTab--
			m.scroll = 0
		}
	case "right", "l":
		if m.activeTab < len(categories)-1 {
			m.activeTab++
			m.scroll = 0
		}
	case "tab":
		m.activeTab = (m.activeTab + 1) % len(categories)
		m.scroll = 0
	case "j", "down":
		m.scroll++
	case "k", "up":
		if m.scroll > 0 {
			m.scroll--
		}
	}
	return m
}

// ApplyLoaded stores fetched achievements.
func (m *Model) ApplyLoaded(msg LoadedMsg) {
	m.loading = false
	if msg.Err != nil {
		m.fetchErr = msg.Err.Error()
	} else {
		m.items = msg.Items
		m.fetchErr = ""
	}
}

// ViewOverlay renders the achievements panel centered in a terminal of size w×h.
func (m Model) ViewOverlay(w, h int) string {
	mw := clamp(w-8, 60, 110)
	mh := max(h-4, 16)

	inner := m.renderInner(mw-4, mh-2)

	box := lipgloss.NewStyle().
		Width(mw).
		Height(mh).
		Padding(1, 2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.ColorBorder).
		Render(inner)

	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) renderInner(w, h int) string {
	var b strings.Builder

	// Title row.
	title := lipgloss.NewStyle().Bold(true).Foreground(theme.ColorBright).Render("ACHIEVEMENTS")
	b.WriteString(title + "\n\n")

	if m.loading {
		b.WriteString(theme.StyleDimmed.Render("Loading..."))
		return b.String()
	}
	if m.fetchErr != "" {
		errStr := lipgloss.NewStyle().Foreground(theme.ColorDanger).Render("Error: " + m.fetchErr)
		b.WriteString(errStr)
		return b.String()
	}

	// Category tab bar.
	var tabs []string
	for i, cat := range categories {
		shortName := tabShortName(cat)
		if i == m.activeTab {
			tabs = append(tabs, lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.ColorBright).
				Underline(true).
				Render(shortName))
		} else {
			tabs = append(tabs, theme.StyleDimmed.Render(shortName))
		}
	}
	b.WriteString(strings.Join(tabs, "  ") + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(theme.ColorBorder).Render(strings.Repeat("─", w)) + "\n")

	// Filter to active category.
	filtered := filterByCategory(m.items, categories[m.activeTab])

	// Summary count.
	unlocked := countUnlocked(filtered)
	summary := fmt.Sprintf("%d / %d unlocked", unlocked, len(filtered))
	b.WriteString(theme.StyleDimmed.Render(summary) + "\n\n")

	// Scrollable achievement rows (each occupies 2 lines: name + description).
	linesAvail := h - 7 // title(2) + tabs(1) + divider(1) + summary(2)
	maxItems := max(linesAvail/2, 1)

	start := clamp(m.scroll, 0, max(len(filtered)-1, 0))

	shown := 0
	for i := start; i < len(filtered) && shown < maxItems; i++ {
		a := filtered[i]
		badge := tierBadge(a.Tier)

		var lockGlyph string
		var nameStyle lipgloss.Style
		if a.Unlocked {
			lockGlyph = lipgloss.NewStyle().Foreground(theme.ColorComplete).Render("✓")
			nameStyle = lipgloss.NewStyle().Foreground(theme.ColorBright)
		} else {
			lockGlyph = theme.StyleDimmed.Render("○")
			nameStyle = theme.StyleDimmed
		}

		nameLine := lockGlyph + " " + badge + " " + nameStyle.Render(a.Name)
		descLine := theme.StyleDimmed.Render("    " + truncate(a.Description, w-5))

		b.WriteString(nameLine + "\n")
		b.WriteString(descLine + "\n")
		shown++
	}

	if len(filtered) == 0 {
		b.WriteString(theme.StyleDimmed.Render("No achievements in this category."))
	}

	// Scroll indicator.
	remaining := len(filtered) - start - shown
	if remaining > 0 {
		b.WriteString("\n" + theme.StyleDimmed.Render(fmt.Sprintf("↓ %d more (j/k to scroll)", remaining)))
	}

	// Help row pinned at bottom.
	help := theme.StyleDimmed.Render("←/→ tab  j/k scroll  esc close")
	b.WriteString("\n\n" + help)

	return b.String()
}

// tierBadge returns a compact colored badge for a tier name.
func tierBadge(tier string) string {
	color := theme.TierColor(tier)
	var label string
	switch tier {
	case "bronze":
		label = "[B]"
	case "silver":
		label = "[S]"
	case "gold":
		label = "[G]"
	case "platinum":
		label = "[P]"
	default:
		label = "[?]"
	}
	return lipgloss.NewStyle().Foreground(color).Bold(true).Render(label)
}

// tabShortName returns a shortened label for a category, used in the tab bar.
func tabShortName(cat string) string {
	switch cat {
	case "Performance & Endurance":
		return "Perf & Endurance"
	default:
		return cat
	}
}

func filterByCategory(items []client.AchievementResponse, cat string) []client.AchievementResponse {
	var out []client.AchievementResponse
	for i := 0; i < len(items); i++ {
		if items[i].Category == cat {
			out = append(out, items[i])
		}
	}
	return out
}

func countUnlocked(items []client.AchievementResponse) int {
	n := 0
	for i := 0; i < len(items); i++ {
		if items[i].Unlocked {
			n++
		}
	}
	return n
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
