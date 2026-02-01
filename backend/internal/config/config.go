package config

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig   `yaml:"server"`
	Monitor MonitorConfig  `yaml:"monitor"`
	Models  map[string]int `yaml:"models"`
}

type ServerConfig struct {
	Port           int      `yaml:"port"`
	Host           string   `yaml:"host"`
	AllowedOrigins []string `yaml:"allowed_origins"`
	AuthToken      string   `yaml:"auth_token"`
}

type MonitorConfig struct {
	PollInterval          time.Duration `yaml:"poll_interval"`
	SnapshotInterval      time.Duration `yaml:"snapshot_interval"`
	BroadcastThrottle     time.Duration `yaml:"broadcast_throttle"`
	SessionStaleAfter     time.Duration `yaml:"session_stale_after"`
	CompletionRemoveAfter time.Duration `yaml:"completion_remove_after"`
	SessionEndDir         string        `yaml:"session_end_dir"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Server: ServerConfig{
			Port: 8080,
			Host: "127.0.0.1",
		},
		Monitor: MonitorConfig{
			PollInterval:          time.Second,
			SnapshotInterval:      5 * time.Second,
			BroadcastThrottle:     100 * time.Millisecond,
			SessionStaleAfter:     2 * time.Minute,
			CompletionRemoveAfter: 8 * time.Second,
			SessionEndDir:         filepath.Join(defaultStateDir(), "agent-racer", "session-end"),
		},
		Models: map[string]int{
			"default": 200000,
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if cfg.Monitor.SessionEndDir == "" {
		cfg.Monitor.SessionEndDir = filepath.Join(defaultStateDir(), "agent-racer", "session-end")
	}

	return cfg, nil
}

func (c *Config) MaxContextTokens(model string) int {
	if n, ok := c.Models[model]; ok {
		return n
	}
	if n, ok := c.Models["default"]; ok {
		return n
	}
	return 200000
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
