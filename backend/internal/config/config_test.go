package config

import "testing"

func TestTokenStrategy(t *testing.T) {
	cfg := defaultConfig()

	tests := []struct {
		source string
		want   string
	}{
		{"claude", "usage"},
		{"codex", "usage"},
		{"gemini", "usage"},
		{"default", "estimate"},
		{"unknown_source", "estimate"}, // falls through to "default" key
	}

	for _, tt := range tests {
		got := cfg.TokenStrategy(tt.source)
		if got != tt.want {
			t.Errorf("TokenStrategy(%q) = %q, want %q", tt.source, got, tt.want)
		}
	}
}

func TestTokenStrategyEmptyStrategies(t *testing.T) {
	cfg := &Config{
		TokenNorm: TokenNormConfig{
			Strategies: nil, // no strategies configured
		},
	}

	// Should fall back to hardcoded "estimate".
	got := cfg.TokenStrategy("claude")
	if got != "estimate" {
		t.Errorf("TokenStrategy with nil map = %q, want %q", got, "estimate")
	}
}

func TestTokenStrategyPartialOverride(t *testing.T) {
	cfg := &Config{
		TokenNorm: TokenNormConfig{
			Strategies: map[string]string{
				"claude": "message_count",
				// no "default" key
			},
		},
	}

	if got := cfg.TokenStrategy("claude"); got != "message_count" {
		t.Errorf("TokenStrategy(claude) = %q, want %q", got, "message_count")
	}

	// Unknown source with no "default" key falls back to hardcoded "estimate".
	if got := cfg.TokenStrategy("codex"); got != "estimate" {
		t.Errorf("TokenStrategy(codex) = %q, want %q", got, "estimate")
	}
}

func TestDefaultConfigTokenNorm(t *testing.T) {
	cfg := defaultConfig()

	if cfg.TokenNorm.TokensPerMessage != 2000 {
		t.Errorf("TokensPerMessage = %d, want 2000", cfg.TokenNorm.TokensPerMessage)
	}

	if len(cfg.TokenNorm.Strategies) != 4 {
		t.Errorf("len(Strategies) = %d, want 4", len(cfg.TokenNorm.Strategies))
	}
}
