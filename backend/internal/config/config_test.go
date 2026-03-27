package config

import (
	"os"
	"path/filepath"
	"strings"
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

	cfg, warnings, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("Load() unexpected warnings: %v", warnings)
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
	_, _, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("Load() on missing file should return error")
	}
}

func TestLoadOrDefaultMissingFile(t *testing.T) {
	cfg, _, err := LoadOrDefault("/nonexistent/path/config.yaml")
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
	// Privacy defaults should mask working dirs, PIDs, and tmux targets.
	if !cfg.Privacy.MaskWorkingDirs {
		t.Error("Privacy.MaskWorkingDirs = false, want default true")
	}
	if !cfg.Privacy.MaskPIDs {
		t.Error("Privacy.MaskPIDs = false, want default true")
	}
	if !cfg.Privacy.MaskTmuxTargets {
		t.Error("Privacy.MaskTmuxTargets = false, want default true")
	}
	if cfg.Privacy.MaskSessionIDs {
		t.Error("Privacy.MaskSessionIDs = true, want default false")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(cfgPath, []byte(":::not valid yaml"), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err := Load(cfgPath)
	if err == nil {
		t.Fatal("Load() with invalid YAML should return error")
	}
}

// loadYAML writes yamlData to a temp file and returns the loaded Config and warnings.
func loadYAML(t *testing.T, yamlData string) (*Config, []string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yamlData), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, warnings, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	return cfg, warnings
}

// containsWarning reports whether any warning string contains substr.
func containsWarning(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

func TestLoadUnknownTopLevelKey(t *testing.T) {
	cfg, warnings := loadYAML(t, `
server:
  port: 9090
servr:
  port: 8080
`)

	// Valid key should still be parsed.
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
	}

	if !containsWarning(warnings, "servr") {
		t.Errorf("expected warning mentioning 'servr', got: %v", warnings)
	}
}

func TestLoadUnknownNestedKey(t *testing.T) {
	cfg, warnings := loadYAML(t, `
monitor:
  poll_interval: 2s
  poll_intervl: 3s
`)

	if cfg.Monitor.PollInterval != 2*1000000000 {
		t.Errorf("Monitor.PollInterval = %v, want 2s", cfg.Monitor.PollInterval)
	}

	if !containsWarning(warnings, "poll_intervl") {
		t.Errorf("expected warning mentioning 'poll_intervl', got: %v", warnings)
	}
}

func TestLoadMultipleUnknownKeys(t *testing.T) {
	_, warnings := loadYAML(t, `
foo: bar
server:
  port: 9090
  baz: true
`)

	if len(warnings) < 2 {
		t.Errorf("expected at least 2 warnings, got %d: %v", len(warnings), warnings)
	}
}

func TestLoadValidConfigNoWarnings(t *testing.T) {
	_, warnings := loadYAML(t, `
server:
  port: 9090
  host: "0.0.0.0"
sources:
  claude: true
models:
  default: 128000
`)

	if len(warnings) != 0 {
		t.Errorf("expected no warnings for valid config, got: %v", warnings)
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
			name:   "glob match with trailing wildcard",
			models: map[string]int{"claude-*": 200000, "default": 128000},
			model:  "claude-opus-4-5-20251101",
			want:   200000,
		},
		{
			name:   "glob match with embedded wildcard",
			models: map[string]int{"gpt-*-codex": 272000, "default": 128000},
			model:  "gpt-5.2-codex",
			want:   272000,
		},
		{
			name:   "more specific glob wins",
			models: map[string]int{"gemini-*": 500000, "gemini-2.5-*": 1048576},
			model:  "gemini-2.5-pro",
			want:   1048576,
		},
		{
			name:   "less specific glob matches when longer one does not",
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
			name:   "glob with no trailing separator",
			models: map[string]int{"gemini-1.5-pro*": 2097152},
			model:  "gemini-1.5-pro-latest",
			want:   2097152,
		},
		{
			name:   "claude 4.6 specific glob beats generic claude glob",
			models: map[string]int{"claude-*-4-6*": 1000000, "claude-*": 200000},
			model:  "claude-opus-4-6",
			want:   1000000,
		},
		{
			name:   "claude 4.6 with date suffix matches specific glob",
			models: map[string]int{"claude-*-4-6*": 1000000, "claude-*": 200000},
			model:  "claude-sonnet-4-6-20260301",
			want:   1000000,
		},
		{
			name:   "older claude model falls back to generic glob",
			models: map[string]int{"claude-*-4-6*": 1000000, "claude-*": 200000},
			model:  "claude-opus-4-5-20251101",
			want:   200000,
		},
		{
			name:   "no glob match falls to default",
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

func TestDefaultConfigIncludesCodexFallbacks(t *testing.T) {
	cfg := defaultConfig()

	if got := cfg.MaxContextTokens("gpt-5.4"); got != 258400 {
		t.Errorf("MaxContextTokens(gpt-5.4) = %d, want 258400", got)
	}
	if got := cfg.MaxContextTokens("gpt-5.2-codex"); got != 272000 {
		t.Errorf("MaxContextTokens(gpt-5.2-codex) = %d, want 272000", got)
	}
	if got := cfg.MaxContextTokens("codex-mini-latest"); got != 200000 {
		t.Errorf("MaxContextTokens(codex-mini-latest) = %d, want 200000", got)
	}
}

func TestNormalizeAuthToken(t *testing.T) {
	if got := NormalizeAuthToken("  abc123  "); got != "abc123" {
		t.Errorf("NormalizeAuthToken() = %q, want %q", got, "abc123")
	}
}

func TestIsWeakAuthToken(t *testing.T) {
	tests := []struct {
		token string
		want  bool
	}{
		{token: "dev", want: true},
		{token: " DEV ", want: true},
		{token: "test", want: true},
		{token: "changeme", want: true},
		{token: "default", want: true},
		{token: "", want: false},
		{token: "  ", want: false},
		{token: "my-actual-secret-token", want: false},
	}

	for _, tt := range tests {
		if got := IsWeakAuthToken(tt.token); got != tt.want {
			t.Errorf("IsWeakAuthToken(%q) = %v, want %v", tt.token, got, tt.want)
		}
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
	new.Privacy.MaskWorkingDirs = false
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
		"privacy.mask_working_dirs: true → false",
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

func TestDiffDetectsPollIntervalChange(t *testing.T) {
	old := defaultConfig()
	new := defaultConfig()
	new.Monitor.PollInterval = 2 * 1000000000 // 2 seconds

	changes := Diff(old, new)
	if len(changes) == 0 {
		t.Fatal("Diff should detect PollInterval change")
	}

	found := false
	for _, c := range changes {
		if c == "monitor.poll_interval: 1s → 2s" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'monitor.poll_interval: 1s → 2s' in changes: %v", changes)
	}
}

func TestDiffDetectsBroadcastThrottleChange(t *testing.T) {
	old := defaultConfig()
	new := defaultConfig()
	new.Monitor.BroadcastThrottle = 200000000 // 200ms

	changes := Diff(old, new)
	if len(changes) == 0 {
		t.Fatal("Diff should detect BroadcastThrottle change")
	}

	found := false
	for _, c := range changes {
		if c == "monitor.broadcast_throttle: 100ms → 200ms" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'monitor.broadcast_throttle: 100ms → 200ms' in changes: %v", changes)
	}
}

func TestDiffDetectsSnapshotIntervalChange(t *testing.T) {
	old := defaultConfig()
	new := defaultConfig()
	new.Monitor.SnapshotInterval = 10 * 1000000000 // 10 seconds

	changes := Diff(old, new)
	if len(changes) == 0 {
		t.Fatal("Diff should detect SnapshotInterval change")
	}

	found := false
	for _, c := range changes {
		if c == "monitor.snapshot_interval: 5s → 10s" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'monitor.snapshot_interval: 5s → 10s' in changes: %v", changes)
	}
}

func TestDiffDetectsStatsEventBufferChange(t *testing.T) {
	old := defaultConfig()
	new := defaultConfig()
	new.Monitor.StatsEventBuffer = 512

	changes := Diff(old, new)
	if len(changes) == 0 {
		t.Fatal("Diff should detect StatsEventBuffer change")
	}

	found := false
	for _, c := range changes {
		if c == "monitor.stats_event_buffer: 256 → 512" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'monitor.stats_event_buffer: 256 → 512' in changes: %v", changes)
	}
}

func TestDiffDetectsMultipleMonitorChanges(t *testing.T) {
	old := defaultConfig()
	new := defaultConfig()

	// Change multiple monitor fields
	new.Monitor.PollInterval = 2 * 1000000000
	new.Monitor.BroadcastThrottle = 150000000
	new.Monitor.SnapshotInterval = 10 * 1000000000
	new.Monitor.StatsEventBuffer = 512
	new.Monitor.SessionStaleAfter = 3 * 60 * 1000000000

	changes := Diff(old, new)
	if len(changes) == 0 {
		t.Fatal("Diff should detect multiple changes")
	}

	expectedCount := 5
	if len(changes) < expectedCount {
		t.Errorf("Expected at least %d changes, got %d: %v", expectedCount, len(changes), changes)
	}

	expectedChanges := []string{
		"monitor.poll_interval: 1s → 2s",
		"monitor.snapshot_interval: 5s → 10s",
		"monitor.broadcast_throttle: 100ms → 150ms",
		"monitor.session_stale_after: 2m0s → 3m0s",
		"monitor.stats_event_buffer: 256 → 512",
	}

	found := map[string]bool{}
	for _, c := range changes {
		found[c] = true
	}

	for _, expected := range expectedChanges {
		if !found[expected] {
			t.Errorf("Missing expected change: %q\nGot: %v", expected, changes)
		}
	}
}

func TestDiffDetectsGamificationChanges(t *testing.T) {
	old := defaultConfig()
	new := defaultConfig()
	new.Gamification.BattlePass.Enabled = true
	new.Gamification.BattlePass.Season = "2026-03"

	changes := Diff(old, new)
	if len(changes) == 0 {
		t.Fatal("Diff should detect gamification changes, got none")
	}

	found := map[string]bool{}
	for _, c := range changes {
		found[c] = true
	}

	want := []string{
		"gamification.battle_pass.enabled: false → true",
		"gamification.battle_pass.season:  → 2026-03",
	}
	for _, w := range want {
		if !found[w] {
			t.Errorf("Missing expected change: %q\nGot: %v", w, changes)
		}
	}
}

func TestDiffDetectsReplayChanges(t *testing.T) {
	old := defaultConfig()
	new := defaultConfig()
	new.Replay.Enabled = false
	new.Replay.RetentionDays = 30

	changes := Diff(old, new)
	if len(changes) == 0 {
		t.Fatal("Diff should detect replay changes, got none")
	}

	found := map[string]bool{}
	for _, c := range changes {
		found[c] = true
	}

	want := []string{
		"replay.enabled: true → false",
		"replay.retention_days: 7 → 30",
	}
	for _, w := range want {
		if !found[w] {
			t.Errorf("Missing expected change: %q\nGot: %v", w, changes)
		}
	}
}

func TestDiffDetectsTrackChange(t *testing.T) {
	old := defaultConfig()
	new := defaultConfig()
	new.Track.Active = "oval"

	changes := Diff(old, new)
	if len(changes) == 0 {
		t.Fatal("Diff should detect Track.Active change")
	}

	found := false
	for _, c := range changes {
		if c == "track.active:  → oval" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'track.active:  → oval' in changes: %v", changes)
	}
}

func TestServerConfigTLSEnabled(t *testing.T) {
	tests := []struct {
		name string
		cert string
		key  string
		want bool
	}{
		{"both empty", "", "", false},
		{"cert only", "/path/cert.pem", "", false},
		{"key only", "", "/path/key.pem", false},
		{"both set", "/path/cert.pem", "/path/key.pem", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := ServerConfig{TLSCert: tt.cert, TLSKey: tt.key}
			if got := sc.TLSEnabled(); got != tt.want {
				t.Errorf("TLSEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestServerConfigScheme(t *testing.T) {
	plain := ServerConfig{}
	if s := plain.Scheme(); s != "http" {
		t.Errorf("Scheme() = %q, want %q", s, "http")
	}

	tls := ServerConfig{TLSCert: "c.pem", TLSKey: "k.pem"}
	if s := tls.Scheme(); s != "https" {
		t.Errorf("Scheme() = %q, want %q", s, "https")
	}
}
