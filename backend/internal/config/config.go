package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/agent-racer/backend/internal/session"
	"gopkg.in/yaml.v3"
)

// DefaultContextWindow is the fallback context window size (in tokens) used
// when no model-specific entry or "default" key is found in the config.
const DefaultContextWindow = 200000

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Monitor   MonitorConfig   `yaml:"monitor"`
	Sources   SourcesConfig   `yaml:"sources"`
	Models    map[string]int  `yaml:"models"`
	Sound     SoundConfig     `yaml:"sound"`
	TokenNorm TokenNormConfig `yaml:"token_normalization"`
	Privacy   PrivacyConfig   `yaml:"privacy"`
}

// PrivacyConfig controls what session metadata is exposed to connected clients.
type PrivacyConfig struct {
	// MaskWorkingDirs replaces full directory paths with just the last
	// path component (e.g. "/home/user/secret-project" → "secret-project").
	MaskWorkingDirs bool `yaml:"mask_working_dirs"`

	// MaskSessionIDs replaces composite session IDs with opaque short hashes.
	MaskSessionIDs bool `yaml:"mask_session_ids"`

	// MaskPIDs hides process IDs from broadcast data.
	MaskPIDs bool `yaml:"mask_pids"`

	// MaskTmuxTargets hides tmux pane locations from broadcast data.
	MaskTmuxTargets bool `yaml:"mask_tmux_targets"`

	// AllowedPaths is a list of glob patterns. When non-empty, only sessions
	// whose working directory matches at least one pattern are broadcast.
	AllowedPaths []string `yaml:"allowed_paths"`

	// BlockedPaths is a list of glob patterns. Sessions whose working
	// directory matches any pattern are excluded from broadcast.
	// BlockedPaths is evaluated after AllowedPaths.
	BlockedPaths []string `yaml:"blocked_paths"`
}

// NewPrivacyFilter converts the config into a session.PrivacyFilter.
func (p *PrivacyConfig) NewPrivacyFilter() *session.PrivacyFilter {
	return &session.PrivacyFilter{
		MaskWorkingDirs: p.MaskWorkingDirs,
		MaskSessionIDs:  p.MaskSessionIDs,
		MaskPIDs:        p.MaskPIDs,
		MaskTmuxTargets: p.MaskTmuxTargets,
		AllowedPaths:    p.AllowedPaths,
		BlockedPaths:    p.BlockedPaths,
	}
}

// TokenNormConfig controls how token counts are resolved for each agent
// source. Sources that report real usage data can use "usage" (the default
// for Claude, Codex, and Gemini). Sources without reliable token counts
// use "estimate" or "message_count" to derive progress from message counts.
type TokenNormConfig struct {
	// Strategies maps source names to their token strategy:
	//   "usage"         -- use real token counts from the source
	//   "estimate"      -- estimate tokens from message count
	//   "message_count" -- same as estimate (message-count heuristic)
	// A "default" key provides the fallback for unlisted sources.
	Strategies map[string]string `yaml:"strategies"`

	// TokensPerMessage is the estimated token cost per message for the
	// "estimate" and "message_count" strategies. Also used as a fallback
	// when a "usage" source has not yet reported token data.
	TokensPerMessage int `yaml:"tokens_per_message"`
}

type SourcesConfig struct {
	Claude bool `yaml:"claude"`
	Codex  bool `yaml:"codex"`
	Gemini bool `yaml:"gemini"`
}

type ServerConfig struct {
	Port           int      `yaml:"port"`
	Host           string   `yaml:"host"`
	AllowedOrigins []string `yaml:"allowed_origins"`
	AuthToken      string   `yaml:"auth_token"`
	MaxConnections int      `yaml:"max_connections"`
}

type MonitorConfig struct {
	PollInterval            time.Duration `yaml:"poll_interval"`
	SnapshotInterval        time.Duration `yaml:"snapshot_interval"`
	BroadcastThrottle       time.Duration `yaml:"broadcast_throttle"`
	SessionStaleAfter       time.Duration `yaml:"session_stale_after"`
	CompletionRemoveAfter   time.Duration `yaml:"completion_remove_after"`
	SessionEndDir           string        `yaml:"session_end_dir"`
	ChurningCPUThreshold    float64       `yaml:"churning_cpu_threshold"`
	ChurningRequiresNetwork bool          `yaml:"churning_requires_network"`
	HealthWarningThreshold  int           `yaml:"health_warning_threshold"`
}

type SoundConfig struct {
	Enabled       bool    `yaml:"enabled" json:"enabled"`
	MasterVolume  float64 `yaml:"master_volume" json:"master_volume"`
	AmbientVolume float64 `yaml:"ambient_volume" json:"ambient_volume"`
	SfxVolume     float64 `yaml:"sfx_volume" json:"sfx_volume"`
	EnableAmbient bool    `yaml:"enable_ambient" json:"enable_ambient"`
	EnableSfx     bool    `yaml:"enable_sfx" json:"enable_sfx"`
}

func Load(path string) (*Config, error) {
	cfg := defaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if cfg.Monitor.SessionEndDir == "" {
		cfg.Monitor.SessionEndDir = filepath.Join(defaultStateDir(), "agent-racer", "session-end")
	}

	return cfg, nil
}

// LoadOrDefault loads config from the given path, or returns default config if path doesn't exist
func LoadOrDefault(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return defaultConfig(), nil
	}
	return Load(path)
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:           8080,
			Host:           "127.0.0.1",
			MaxConnections: 1000,
		},
		Monitor: MonitorConfig{
			PollInterval:            time.Second,
			SnapshotInterval:        5 * time.Second,
			BroadcastThrottle:       100 * time.Millisecond,
			SessionStaleAfter:       2 * time.Minute,
			CompletionRemoveAfter:   90 * time.Second,
			SessionEndDir:           filepath.Join(defaultStateDir(), "agent-racer", "session-end"),
			ChurningCPUThreshold:    15.0,
			ChurningRequiresNetwork: false,
			HealthWarningThreshold:  3,
		},
		Sources: SourcesConfig{
			Claude: true,
			Codex:  false,
			Gemini: false,
		},
		Models: map[string]int{
			"default": DefaultContextWindow,
		},
		Sound: SoundConfig{
			Enabled:       true,
			MasterVolume:  1.0,
			AmbientVolume: 1.0,
			SfxVolume:     1.0,
			EnableAmbient: true,
			EnableSfx:     true,
		},
		TokenNorm: TokenNormConfig{
			Strategies: map[string]string{
				"claude":  "usage",
				"codex":   "usage",
				"gemini":  "usage",
				"default": "estimate",
			},
			TokensPerMessage: 2000,
		},
	}
}

// MaxContextTokens resolves the context window size for a model.
// Resolution order: exact match → longest prefix match → "default" key → DefaultContextWindow.
// Config keys ending with "*" are treated as prefix patterns (e.g. "claude-*"
// matches "claude-opus-4-5-20251101"). The longest matching prefix wins.
func (c *Config) MaxContextTokens(model string) int {
	// 1. Exact match
	if n, ok := c.Models[model]; ok {
		return n
	}

	// 2. Longest prefix match (keys ending with *)
	bestLen := 0
	bestVal := 0
	for key, val := range c.Models {
		if !strings.HasSuffix(key, "*") {
			continue
		}
		prefix := strings.TrimSuffix(key, "*")
		if strings.HasPrefix(model, prefix) && len(prefix) > bestLen {
			bestLen = len(prefix)
			bestVal = val
		}
	}
	if bestLen > 0 {
		return bestVal
	}

	// 3. "default" key
	if n, ok := c.Models["default"]; ok {
		return n
	}
	return DefaultContextWindow
}

// TokenStrategy returns the configured token normalization strategy for the
// given source name. It checks the per-source strategies map first, then
// the "default" key, and falls back to "estimate" if neither is configured.
func (c *Config) TokenStrategy(source string) string {
	if s, ok := c.TokenNorm.Strategies[source]; ok {
		return s
	}
	if s, ok := c.TokenNorm.Strategies["default"]; ok {
		return s
	}
	return "estimate"
}

func defaultStateDir() string {
	if value := os.Getenv("XDG_STATE_HOME"); value != "" {
		return value
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".local", "state")
}

// Diff compares two configs and returns human-readable descriptions of what changed.
// Only sections that are safe to reload at runtime are compared (models, privacy,
// sources, token normalization, monitor timings, sound).
func Diff(old, new *Config) []string {
	var changes []string

	// Models
	for k, v := range new.Models {
		if ov, ok := old.Models[k]; !ok {
			changes = append(changes, fmt.Sprintf("models: added %s=%d", k, v))
		} else if ov != v {
			changes = append(changes, fmt.Sprintf("models: %s changed %d → %d", k, ov, v))
		}
	}
	for k := range old.Models {
		if _, ok := new.Models[k]; !ok {
			changes = append(changes, fmt.Sprintf("models: removed %s", k))
		}
	}

	// Sources
	if old.Sources.Claude != new.Sources.Claude {
		changes = append(changes, fmt.Sprintf("sources.claude: %v → %v", old.Sources.Claude, new.Sources.Claude))
	}
	if old.Sources.Codex != new.Sources.Codex {
		changes = append(changes, fmt.Sprintf("sources.codex: %v → %v", old.Sources.Codex, new.Sources.Codex))
	}
	if old.Sources.Gemini != new.Sources.Gemini {
		changes = append(changes, fmt.Sprintf("sources.gemini: %v → %v", old.Sources.Gemini, new.Sources.Gemini))
	}

	// Privacy
	if old.Privacy.MaskWorkingDirs != new.Privacy.MaskWorkingDirs {
		changes = append(changes, fmt.Sprintf("privacy.mask_working_dirs: %v → %v", old.Privacy.MaskWorkingDirs, new.Privacy.MaskWorkingDirs))
	}
	if old.Privacy.MaskSessionIDs != new.Privacy.MaskSessionIDs {
		changes = append(changes, fmt.Sprintf("privacy.mask_session_ids: %v → %v", old.Privacy.MaskSessionIDs, new.Privacy.MaskSessionIDs))
	}
	if old.Privacy.MaskPIDs != new.Privacy.MaskPIDs {
		changes = append(changes, fmt.Sprintf("privacy.mask_pids: %v → %v", old.Privacy.MaskPIDs, new.Privacy.MaskPIDs))
	}
	if old.Privacy.MaskTmuxTargets != new.Privacy.MaskTmuxTargets {
		changes = append(changes, fmt.Sprintf("privacy.mask_tmux_targets: %v → %v", old.Privacy.MaskTmuxTargets, new.Privacy.MaskTmuxTargets))
	}
	if !slices.Equal(old.Privacy.AllowedPaths, new.Privacy.AllowedPaths) {
		changes = append(changes, fmt.Sprintf("privacy.allowed_paths: %v → %v", old.Privacy.AllowedPaths, new.Privacy.AllowedPaths))
	}
	if !slices.Equal(old.Privacy.BlockedPaths, new.Privacy.BlockedPaths) {
		changes = append(changes, fmt.Sprintf("privacy.blocked_paths: %v → %v", old.Privacy.BlockedPaths, new.Privacy.BlockedPaths))
	}

	// Token normalization
	if old.TokenNorm.TokensPerMessage != new.TokenNorm.TokensPerMessage {
		changes = append(changes, fmt.Sprintf("token_normalization.tokens_per_message: %d → %d", old.TokenNorm.TokensPerMessage, new.TokenNorm.TokensPerMessage))
	}
	for k, v := range new.TokenNorm.Strategies {
		if ov, ok := old.TokenNorm.Strategies[k]; !ok {
			changes = append(changes, fmt.Sprintf("token_normalization.strategies: added %s=%s", k, v))
		} else if ov != v {
			changes = append(changes, fmt.Sprintf("token_normalization.strategies: %s changed %s → %s", k, ov, v))
		}
	}
	for k := range old.TokenNorm.Strategies {
		if _, ok := new.TokenNorm.Strategies[k]; !ok {
			changes = append(changes, fmt.Sprintf("token_normalization.strategies: removed %s", k))
		}
	}

	// Monitor timings
	if old.Monitor.SessionStaleAfter != new.Monitor.SessionStaleAfter {
		changes = append(changes, fmt.Sprintf("monitor.session_stale_after: %s → %s", old.Monitor.SessionStaleAfter, new.Monitor.SessionStaleAfter))
	}
	if old.Monitor.CompletionRemoveAfter != new.Monitor.CompletionRemoveAfter {
		changes = append(changes, fmt.Sprintf("monitor.completion_remove_after: %s → %s", old.Monitor.CompletionRemoveAfter, new.Monitor.CompletionRemoveAfter))
	}
	if old.Monitor.ChurningCPUThreshold != new.Monitor.ChurningCPUThreshold {
		changes = append(changes, fmt.Sprintf("monitor.churning_cpu_threshold: %.1f → %.1f", old.Monitor.ChurningCPUThreshold, new.Monitor.ChurningCPUThreshold))
	}
	if old.Monitor.ChurningRequiresNetwork != new.Monitor.ChurningRequiresNetwork {
		changes = append(changes, fmt.Sprintf("monitor.churning_requires_network: %v → %v", old.Monitor.ChurningRequiresNetwork, new.Monitor.ChurningRequiresNetwork))
	}
	if old.Monitor.HealthWarningThreshold != new.Monitor.HealthWarningThreshold {
		changes = append(changes, fmt.Sprintf("monitor.health_warning_threshold: %d → %d", old.Monitor.HealthWarningThreshold, new.Monitor.HealthWarningThreshold))
	}

	// Sound
	if old.Sound != new.Sound {
		changes = append(changes, "sound: configuration changed")
	}

	return changes
}

func defaultConfigDir() string {
	if value := os.Getenv("XDG_CONFIG_HOME"); value != "" {
		return value
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".config")
}

// DefaultConfigPath returns the default XDG-compliant config file path
func DefaultConfigPath() string {
	return filepath.Join(defaultConfigDir(), "agent-racer", "config.yaml")
}
