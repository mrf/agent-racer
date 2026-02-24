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
	if cfg.Models["default"] != DefaultContextWindow {
		t.Errorf("Models[default] = %d, want default %d", cfg.Models["default"], DefaultContextWindow)
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
			want:   DefaultContextWindow,
		},
		{
			name:   "nil map falls back to hardcoded",
			models: nil,
			model:  "anything",
			want:   DefaultContextWindow,
		},
		{
			name:   "empty map falls back to hardcoded",
			models: map[string]int{},
			model:  "anything",
			want:   DefaultContextWindow,
		},
		// Prefix matching tests
		{
			name:   "prefix match with wildcard",
			models: map[string]int{"claude-*": 200000, "default": 128000},
			model:  "claude-opus-4-5-20251101",
			want:   200000,
		},
		{
			name:   "longest prefix wins",
			models: map[string]int{"gemini-*": 500000, "gemini-2.5-*": 1048576},
			model:  "gemini-2.5-pro",
			want:   1048576,
		},
		{
			name:   "shorter prefix matches when longer doesn't",
			models: map[string]int{"gemini-*": 500000, "gemini-2.5-*": 1048576},
			model:  "gemini-3-pro-preview",
			want:   500000,
		},
		{
			name:   "exact match beats prefix",
			models: map[string]int{"claude-*": 200000, "claude-opus-4-5": 300000},
			model:  "claude-opus-4-5",
			want:   300000,
		},
		{
			name:   "prefix with no trailing separator",
			models: map[string]int{"gemini-1.5-pro*": 2097152},
			model:  "gemini-1.5-pro-latest",
			want:   2097152,
		},
		{
			name:   "no prefix match falls to default",
			models: map[string]int{"claude-*": 200000, "default": 128000},
			model:  "gpt-4",
			want:   128000,
		},
		{
			name:   "gemini prefix replaces hardcoded switch",
			models: map[string]int{"gemini-2.5-*": 1048576, "gemini-2.0-*": 1048576, "gemini-3-*": 1000000, "gemini-1.5-pro*": 2097152, "gemini-1.5-flash*": 1048576, "default": 200000},
			model:  "gemini-2.5-pro",
			want:   1048576,
		},
		{
			name:   "gemini 1.5 pro prefix",
			models: map[string]int{"gemini-1.5-pro*": 2097152, "gemini-1.5-flash*": 1048576, "default": 200000},
			model:  "gemini-1.5-pro-latest",
			want:   2097152,
		},
		{
			name:   "gemini 3 prefix",
			models: map[string]int{"gemini-2.5-*": 1048576, "gemini-3-*": 1000000, "default": 200000},
			model:  "gemini-3-flash-preview",
			want:   1000000,
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

func TestGenerateToken(t *testing.T) {
	tok, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}
	if len(tok) != 32 { // 16 bytes = 32 hex chars
		t.Errorf("token length = %d, want 32", len(tok))
	}

	// Tokens should be unique.
	tok2, _ := GenerateToken()
	if tok == tok2 {
		t.Error("two generated tokens should not be identical")
	}
}

func TestDiffNoChanges(t *testing.T) {
	a := defaultConfig()
	b := defaultConfig()
	if changes := Diff(a, b); len(changes) != 0 {
		t.Errorf("Diff of identical configs = %v, want empty", changes)
	}
}

func TestDiffDetectsChanges(t *testing.T) {
	old := defaultConfig()
	new := defaultConfig()

	// Models
	new.Models["claude-opus-4-5"] = 300000
	delete(new.Models, "default")

	// Sources
	new.Sources.Codex = true

	// Privacy
	new.Privacy.MaskWorkingDirs = true
	new.Privacy.BlockedPaths = []string{"/tmp/secret"}

	// Token norm
	new.TokenNorm.TokensPerMessage = 3000

	changes := Diff(old, new)
	if len(changes) == 0 {
		t.Fatal("Diff should detect changes, got none")
	}

	// Check specific changes are present.
	found := map[string]bool{}
	for _, c := range changes {
		found[c] = true
	}

	want := []string{
		"models: added claude-opus-4-5=300000",
		"models: removed default",
		"sources.codex: false → true",
		"privacy.mask_working_dirs: false → true",
		"privacy.blocked_paths: [] → [/tmp/secret]",
		"token_normalization.tokens_per_message: 2000 → 3000",
	}
	for _, w := range want {
		if !found[w] {
			t.Errorf("Missing expected change: %q\nGot: %v", w, changes)
		}
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
