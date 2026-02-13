package config

import (
	"os"
	"path/filepath"
	"testing"
)

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

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	yaml := `
server:
  port: 9090
  host: "0.0.0.0"
sources:
  claude: true
  codex: true
models:
  default: 128000
  claude-opus-4-5: 200000
privacy:
  mask_working_dirs: true
  blocked_paths:
    - "/tmp/secret"
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "0.0.0.0")
	}
	if !cfg.Sources.Codex {
		t.Error("Sources.Codex = false, want true")
	}
	if cfg.Models["default"] != 128000 {
		t.Errorf("Models[default] = %d, want 128000", cfg.Models["default"])
	}
	if cfg.Models["claude-opus-4-5"] != 200000 {
		t.Errorf("Models[claude-opus-4-5] = %d, want 200000", cfg.Models["claude-opus-4-5"])
	}
	if !cfg.Privacy.MaskWorkingDirs {
		t.Error("Privacy.MaskWorkingDirs = false, want true")
	}
	if len(cfg.Privacy.BlockedPaths) != 1 || cfg.Privacy.BlockedPaths[0] != "/tmp/secret" {
		t.Errorf("Privacy.BlockedPaths = %v, want [/tmp/secret]", cfg.Privacy.BlockedPaths)
	}

	// Defaults should still be applied for unspecified fields.
	if cfg.Monitor.PollInterval == 0 {
		t.Error("Monitor.PollInterval should have default, got 0")
	}
	if cfg.Sound.MasterVolume != 1.0 {
		t.Errorf("Sound.MasterVolume = %f, want 1.0", cfg.Sound.MasterVolume)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("Load() on missing file should return error")
	}
}

func TestLoadOrDefaultMissingFile(t *testing.T) {
	cfg, err := LoadOrDefault("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("LoadOrDefault() error: %v", err)
	}

	// Should return defaults.
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want default 8080", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Server.Host = %q, want default %q", cfg.Server.Host, "127.0.0.1")
	}
	if !cfg.Sources.Claude {
		t.Error("Sources.Claude = false, want default true")
	}
	if cfg.Sources.Codex {
		t.Error("Sources.Codex = true, want default false")
	}
	if cfg.Models["default"] != 200000 {
		t.Errorf("Models[default] = %d, want default 200000", cfg.Models["default"])
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(cfgPath, []byte(":::not valid yaml"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("Load() with invalid YAML should return error")
	}
}

func TestNewPrivacyFilter(t *testing.T) {
	pc := PrivacyConfig{
		MaskWorkingDirs: true,
		MaskSessionIDs:  true,
		MaskPIDs:        false,
		MaskTmuxTargets: true,
		AllowedPaths:    []string{"/home/user/*"},
		BlockedPaths:    []string{"/home/user/secret"},
	}

	pf := pc.NewPrivacyFilter()

	if !pf.MaskWorkingDirs {
		t.Error("MaskWorkingDirs not copied")
	}
	if !pf.MaskSessionIDs {
		t.Error("MaskSessionIDs not copied")
	}
	if pf.MaskPIDs {
		t.Error("MaskPIDs should be false")
	}
	if !pf.MaskTmuxTargets {
		t.Error("MaskTmuxTargets not copied")
	}
	if len(pf.AllowedPaths) != 1 || pf.AllowedPaths[0] != "/home/user/*" {
		t.Errorf("AllowedPaths = %v, want [/home/user/*]", pf.AllowedPaths)
	}
	if len(pf.BlockedPaths) != 1 || pf.BlockedPaths[0] != "/home/user/secret" {
		t.Errorf("BlockedPaths = %v, want [/home/user/secret]", pf.BlockedPaths)
	}
}

func TestNewPrivacyFilterZeroValue(t *testing.T) {
	pc := PrivacyConfig{}
	pf := pc.NewPrivacyFilter()

	if !pf.IsNoop() {
		t.Error("zero-value PrivacyConfig should produce a noop filter")
	}
}

func TestMaxContextTokens(t *testing.T) {
	tests := []struct {
		name   string
		models map[string]int
		model  string
		want   int
	}{
		{
			name:   "exact match",
			models: map[string]int{"claude-opus-4-5": 200000, "default": 128000},
			model:  "claude-opus-4-5",
			want:   200000,
		},
		{
			name:   "falls back to default key",
			models: map[string]int{"claude-opus-4-5": 200000, "default": 128000},
			model:  "unknown-model",
			want:   128000,
		},
		{
			name:   "no default key falls back to hardcoded",
			models: map[string]int{"claude-opus-4-5": 200000},
			model:  "unknown-model",
			want:   200000,
		},
		{
			name:   "nil map falls back to hardcoded",
			models: nil,
			model:  "anything",
			want:   200000,
		},
		{
			name:   "empty map falls back to hardcoded",
			models: map[string]int{},
			model:  "anything",
			want:   200000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Models: tt.models}
			got := cfg.MaxContextTokens(tt.model)
			if got != tt.want {
				t.Errorf("MaxContextTokens(%q) = %d, want %d", tt.model, got, tt.want)
			}
		})
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
