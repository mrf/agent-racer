package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig          `yaml:"server"`
	Monitor MonitorConfig         `yaml:"monitor"`
	Models  map[string]int        `yaml:"models"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

type MonitorConfig struct {
	PollInterval      time.Duration `yaml:"poll_interval"`
	SnapshotInterval  time.Duration `yaml:"snapshot_interval"`
	BroadcastThrottle time.Duration `yaml:"broadcast_throttle"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Server: ServerConfig{
			Port: 8080,
			Host: "0.0.0.0",
		},
		Monitor: MonitorConfig{
			PollInterval:      time.Second,
			SnapshotInterval:  5 * time.Second,
			BroadcastThrottle: 100 * time.Millisecond,
		},
		Models: map[string]int{
			"default": 200000,
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
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
