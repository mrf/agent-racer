package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ServerConfig mirrors the server section of the shared config.yaml.
// Only the fields the TUI needs for connecting to the backend are included.
type ServerConfig struct {
	Port          int    `yaml:"port"`
	Host          string `yaml:"host"`
	AuthToken     string `yaml:"auth_token"`
	TLS           bool   `yaml:"tls"`
	TLSCACert     string `yaml:"tls_ca_cert"`
	TLSSkipVerify bool   `yaml:"tls_skip_verify"`
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
// does not exist.
func LoadOrDefault(path string) *Config {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return defaultConfig()
	}
	cfg, err := Load(path)
	if err != nil {
		return defaultConfig()
	}
	return cfg
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
	scheme := "ws"
	if c.Server.TLS {
		scheme = "wss"
	}
	return fmt.Sprintf("%s://%s:%d/ws", scheme, c.Server.Host, c.Server.Port)
}

// TLSConfig builds a *tls.Config from the server settings. Returns nil if
// TLS is not enabled.
func (c *Config) TLSConfig() (*tls.Config, error) {
	if !c.Server.TLS {
		return nil, nil
	}

	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if c.Server.TLSSkipVerify {
		tlsCfg.InsecureSkipVerify = true
		return tlsCfg, nil
	}

	if c.Server.TLSCACert != "" {
		pem, err := os.ReadFile(c.Server.TLSCACert)
		if err != nil {
			return nil, fmt.Errorf("reading TLS CA cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("tls_ca_cert: no valid certificates found in %s", c.Server.TLSCACert)
		}
		tlsCfg.RootCAs = pool
	}

	return tlsCfg, nil
}

// IsLoopback reports whether the configured host resolves to a loopback address.
func (c *Config) IsLoopback() bool {
	host := c.Server.Host
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
