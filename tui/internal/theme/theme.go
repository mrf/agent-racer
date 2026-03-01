// Package theme provides the Lip Gloss color palette and reusable styles
// for the Agent Racer TUI. It is a leaf package with no internal imports
// to avoid import cycles.
package theme

import "github.com/charmbracelet/lipgloss"

// Model colors.
var (
	ColorOpus    = lipgloss.Color("#a855f7")
	ColorSonnet4 = lipgloss.Color("#3b82f6")
	ColorSonnet45 = lipgloss.Color("#06b6d4")
	ColorHaiku   = lipgloss.Color("#22c55e")
	ColorGemini  = lipgloss.Color("#4285f4")
	ColorCodex   = lipgloss.Color("#10b981")
	ColorDefault = lipgloss.Color("#9ca3af")
)

// Activity colors.
var (
	ColorThinking = lipgloss.Color("#2563eb")
	ColorToolUse  = lipgloss.Color("#d97706")
	ColorWaiting  = lipgloss.Color("#854d0e")
	ColorIdle     = lipgloss.Color("#4b5563")
	ColorStarting = lipgloss.Color("#7c3aed")
	ColorComplete = lipgloss.Color("#16a34a")
	ColorErrored  = lipgloss.Color("#dc2626")
	ColorLost     = lipgloss.Color("#374151")
)

// Source badge colors.
var (
	ColorSourceClaude = lipgloss.Color("#a855f7")
	ColorSourceCodex  = lipgloss.Color("#10b981")
	ColorSourceGemini = lipgloss.Color("#4285f4")
)

// Context bar thresholds.
var (
	ColorContextLow  = lipgloss.Color("#22c55e") // <50%
	ColorContextMid  = lipgloss.Color("#d97706") // 50-80%
	ColorContextHigh = lipgloss.Color("#dc2626") // >80%
)

// Tier colors.
var (
	ColorBronze   = lipgloss.Color("#d97706")
	ColorSilver   = lipgloss.Color("#9ca3af")
	ColorGold     = lipgloss.Color("#f59e0b")
	ColorPlatinum = lipgloss.Color("#67e8f9")
)

// UI chrome colors.
var (
	ColorBorder  = lipgloss.Color("#4b5563")
	ColorDimmed  = lipgloss.Color("#6b7280")
	ColorBright  = lipgloss.Color("#f9fafb")
	ColorBg      = lipgloss.Color("#111827")
	ColorHealthy = lipgloss.Color("#22c55e")
	ColorWarning = lipgloss.Color("#d97706")
	ColorDanger  = lipgloss.Color("#dc2626")
)

// ModelColor returns the Lip Gloss color for a model name.
func ModelColor(model string) lipgloss.Color {
	switch {
	case contains(model, "opus"):
		return ColorOpus
	case contains(model, "sonnet") && contains(model, "4-5"):
		return ColorSonnet45
	case contains(model, "sonnet"):
		return ColorSonnet4
	case contains(model, "haiku"):
		return ColorHaiku
	case contains(model, "gemini"):
		return ColorGemini
	case contains(model, "codex"):
		return ColorCodex
	default:
		return ColorDefault
	}
}

// ActivityColor returns the Lip Gloss color for an activity string.
func ActivityColor(activity string) lipgloss.Color {
	switch activity {
	case "thinking":
		return ColorThinking
	case "tool_use":
		return ColorToolUse
	case "waiting":
		return ColorWaiting
	case "idle":
		return ColorIdle
	case "starting":
		return ColorStarting
	case "complete":
		return ColorComplete
	case "errored":
		return ColorErrored
	case "lost":
		return ColorLost
	default:
		return ColorDefault
	}
}

// SourceBadge returns a colored badge string for a source name.
func SourceBadge(source string) string {
	switch source {
	case "claude":
		return lipgloss.NewStyle().Foreground(ColorSourceClaude).Render("[C]")
	case "codex":
		return lipgloss.NewStyle().Foreground(ColorSourceCodex).Render("[X]")
	case "gemini":
		return lipgloss.NewStyle().Foreground(ColorSourceGemini).Render("[G]")
	default:
		return lipgloss.NewStyle().Foreground(ColorDefault).Render("[?]")
	}
}

// TierColor returns the color for a tier name.
func TierColor(tier string) lipgloss.Color {
	switch tier {
	case "bronze":
		return ColorBronze
	case "silver":
		return ColorSilver
	case "gold":
		return ColorGold
	case "platinum":
		return ColorPlatinum
	default:
		return ColorDefault
	}
}

// ContextBarColor returns the color for a context utilization percentage.
func ContextBarColor(pct float64) lipgloss.Color {
	switch {
	case pct > 0.8:
		return ColorContextHigh
	case pct > 0.5:
		return ColorContextMid
	default:
		return ColorContextLow
	}
}

// Reusable styles.
var (
	StyleBorder = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder)

	StyleHeader = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorBright)

	StyleDimmed = lipgloss.NewStyle().
		Foreground(ColorDimmed)

	StyleSelected = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorBright)
)

// ActivityGlyph returns a Unicode glyph representing an activity state.
func ActivityGlyph(activity string) string {
	switch activity {
	case "thinking":
		return "●>"
	case "tool_use":
		return "⚙>"
	case "idle":
		return "○"
	case "waiting":
		return "◌"
	case "starting":
		return "◎"
	case "complete":
		return "✓"
	case "errored":
		return "✗"
	case "lost":
		return "?"
	default:
		return "·"
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
