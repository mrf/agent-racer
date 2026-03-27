package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ServerConfig mirrors the server section of the shared config.yaml.
// Only the fields the TUI needs for connecting to the backend are included.
type ServerConfig struct {
	Port      int    `yaml:"port"`
	Host      string `yaml:"host"`
	AuthToken string `yaml:"auth_token"`
}

// Config holds the subset of agent-racer configuration relevant to the TUI.
type Config struct {
	Server ServerConfig `yaml:"server"`
}

// Load reads a config file and returns the parsed Config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := defaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// LoadOrDefault loads config from path, or returns defaults if the file
// does not exist. If the file exists but cannot be read or parsed, it
// returns the default config and a non-nil warning error.
func LoadOrDefault(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return defaultConfig(), nil
	}
	cfg, err := Load(path)
	if err != nil {
		return defaultConfig(), fmt.Errorf("could not load %s: %w (using defaults)", path, err)
	}
	return cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: 8080,
			Host: "127.0.0.1",
		},
	}
}

// DefaultConfigPath returns the default XDG-compliant config file path.
func DefaultConfigPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "agent-racer", "config.yaml")
}

// WebSocketURL builds the WebSocket URL from host and port.
func (c *Config) WebSocketURL() string {
	return fmt.Sprintf("ws://%s:%d/ws", c.Server.Host, c.Server.Port)
}
