package theme

import (
	"testing"
)

func TestModelColor(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{"opus", "claude-opus-4-6", string(ColorOpus)},
		{"sonnet-4-6", "claude-sonnet-4-6", string(ColorSonnet46)},
		{"sonnet-4-5", "claude-sonnet-4-5", string(ColorSonnet45)},
		{"sonnet-4-generic", "claude-sonnet-4-20250514", string(ColorSonnet4)},
		{"haiku", "claude-haiku-4-5", string(ColorHaiku)},
		{"gemini", "gemini-2.5-pro", string(ColorGemini)},
		{"codex", "codex-mini", string(ColorCodex)},
		{"unknown", "unknown-model", string(ColorDefault)},
		{"empty", "", string(ColorDefault)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(ModelColor(tt.model))
			if got != tt.want {
				t.Errorf("ModelColor(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestActivityColor(t *testing.T) {
	tests := []struct {
		name     string
		activity string
		want     string
	}{
		{"thinking", "thinking", string(ColorThinking)},
		{"tool_use", "tool_use", string(ColorToolUse)},
		{"waiting", "waiting", string(ColorWaiting)},
		{"idle", "idle", string(ColorIdle)},
		{"starting", "starting", string(ColorStarting)},
		{"complete", "complete", string(ColorComplete)},
		{"errored", "errored", string(ColorErrored)},
		{"lost", "lost", string(ColorLost)},
		{"unknown", "unknown", string(ColorDefault)},
		{"empty", "", string(ColorDefault)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(ActivityColor(tt.activity))
			if got != tt.want {
				t.Errorf("ActivityColor(%q) = %q, want %q", tt.activity, got, tt.want)
			}
		})
	}
}

func TestSourceBadge(t *testing.T) {
	sources := []string{"claude", "codex", "gemini", "unknown"}
	for _, source := range sources {
		t.Run(source, func(t *testing.T) {
			if got := SourceBadge(source); got == "" {
				t.Errorf("SourceBadge(%q) returned empty string", source)
			}
		})
	}
}

func TestTierColor(t *testing.T) {
	tests := []struct {
		tier string
		want string
	}{
		{"bronze", string(ColorBronze)},
		{"silver", string(ColorSilver)},
		{"gold", string(ColorGold)},
		{"platinum", string(ColorPlatinum)},
		{"unknown", string(ColorDefault)},
	}
	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			got := string(TierColor(tt.tier))
			if got != tt.want {
				t.Errorf("TierColor(%q) = %q, want %q", tt.tier, got, tt.want)
			}
		})
	}
}

func TestContextBarColor(t *testing.T) {
	tests := []struct {
		name string
		pct  float64
		want string
	}{
		{"low", 0.3, string(ColorContextLow)},
		{"mid", 0.6, string(ColorContextMid)},
		{"high", 0.9, string(ColorContextHigh)},
		{"zero", 0.0, string(ColorContextLow)},
		{"boundary_50", 0.5, string(ColorContextLow)},
		{"boundary_80", 0.8, string(ColorContextMid)},
		{"above_80", 0.81, string(ColorContextHigh)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(ContextBarColor(tt.pct))
			if got != tt.want {
				t.Errorf("ContextBarColor(%f) = %q, want %q", tt.pct, got, tt.want)
			}
		})
	}
}

func TestActivityGlyph(t *testing.T) {
	tests := []struct {
		name     string
		activity string
		want     string
	}{
		{"thinking", "thinking", "●>"},
		{"tool_use", "tool_use", "⚙>"},
		{"idle", "idle", "○"},
		{"waiting", "waiting", "◌"},
		{"starting", "starting", "◎"},
		{"complete", "complete", "✓"},
		{"errored", "errored", "✗"},
		{"lost", "lost", "?"},
		{"unknown", "unknown", "·"},
		{"empty", "", "·"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ActivityGlyph(tt.activity)
			if got != tt.want {
				t.Errorf("ActivityGlyph(%q) = %q, want %q", tt.activity, got, tt.want)
			}
		})
	}
}

func TestSpinnerOrFallback(t *testing.T) {
	if got := SpinnerOrFallback("⠋"); got != "⠋" {
		t.Errorf("SpinnerOrFallback with view = %q, want %q", got, "⠋")
	}
	if got := SpinnerOrFallback(""); got != "○" {
		t.Errorf("SpinnerOrFallback empty = %q, want %q", got, "○")
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		s, sub string
		want   bool
	}{
		{"claude-opus-4-6", "opus", true},
		{"claude-sonnet-4-6", "sonnet", true},
		{"claude-sonnet-4-6", "4-6", true},
		{"hello", "xyz", false},
		{"short", "longer-than-s", false},
		{"", "", true},
		{"abc", "", true},
	}
	for _, tt := range tests {
		got := contains(tt.s, tt.sub)
		if got != tt.want {
			t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.sub, got, tt.want)
		}
	}
}

func TestStylesRenderWithoutPanic(t *testing.T) {
	_ = StyleBorder.Render("test")
	_ = StyleHeader.Render("test")
	_ = StyleDimmed.Render("test")
	_ = StyleSelected.Render("test")
}
