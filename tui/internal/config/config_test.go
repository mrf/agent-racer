package config

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrDefault_MissingFile(t *testing.T) {
	cfg, warn := LoadOrDefault("/nonexistent/config.yaml")
	if warn != nil {
		t.Fatalf("expected no warning for missing file, got: %v", warn)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected default host 127.0.0.1, got %s", cfg.Server.Host)
	}
}

func TestLoadOrDefault_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("server:\n  port: 9999\n  host: 10.0.0.1\n  auth_token: secret\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, warn := LoadOrDefault(path)
	if warn != nil {
		t.Fatalf("expected no warning for valid config, got: %v", warn)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("expected port 9999, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "10.0.0.1" {
		t.Errorf("expected host 10.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.AuthToken != "secret" {
		t.Errorf("expected auth_token secret, got %s", cfg.Server.AuthToken)
	}
}

func TestLoadOrDefault_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("server:\n  port: [invalid\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, warn := LoadOrDefault(path)
	if warn == nil {
		t.Fatal("expected warning for invalid YAML, got nil")
	}
	// Should still return usable defaults.
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
}

func TestLoadOrDefault_UnreadableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("server:\n  port: 1234\n"), 0000); err != nil {
		t.Fatal(err)
	}

	cfg, warn := LoadOrDefault(path)
	if warn == nil {
		t.Fatal("expected warning for unreadable file, got nil")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
}

func TestLoad_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("server:\n  port: 3000\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 3000 {
		t.Errorf("expected port 3000, got %d", cfg.Server.Port)
	}
}

func TestWebSocketURL(t *testing.T) {
	cfg := &Config{Server: ServerConfig{Host: "10.0.0.1", Port: 9090}}
	got := cfg.WebSocketURL()
	want := "ws://10.0.0.1:9090/ws"
	if got != want {
		t.Errorf("WebSocketURL() = %q, want %q", got, want)
	}
}

func TestDefaultConfigPath(t *testing.T) {
	path := DefaultConfigPath()
	if path == "" {
		t.Skip("could not determine home directory")
	}
	if filepath.Base(path) != "config.yaml" {
		t.Errorf("expected config.yaml, got %s", filepath.Base(path))
	}
}

func TestWebSocketURLPlain(t *testing.T) {
	cfg := defaultConfig()
	if got := cfg.WebSocketURL(); got != "ws://127.0.0.1:8080/ws" {
		t.Errorf("WebSocketURL() = %q, want ws://127.0.0.1:8080/ws", got)
	}
}

func TestWebSocketURLTLS(t *testing.T) {
	cfg := defaultConfig()
	cfg.Server.TLS = true
	if got := cfg.WebSocketURL(); got != "wss://127.0.0.1:8080/ws" {
		t.Errorf("WebSocketURL() = %q, want wss://127.0.0.1:8080/ws", got)
	}
}

func TestTLSConfigDisabled(t *testing.T) {
	cfg := defaultConfig()
	tlsCfg, err := cfg.TLSConfig()
	if err != nil {
		t.Fatalf("TLSConfig() error: %v", err)
	}
	if tlsCfg != nil {
		t.Error("TLSConfig() should return nil when TLS is disabled")
	}
}

func TestTLSConfigSkipVerify(t *testing.T) {
	cfg := defaultConfig()
	cfg.Server.TLS = true
	cfg.Server.TLSSkipVerify = true
	tlsCfg, err := cfg.TLSConfig()
	if err != nil {
		t.Fatalf("TLSConfig() error: %v", err)
	}
	if tlsCfg == nil {
		t.Fatal("TLSConfig() returned nil, want non-nil")
	}
	if !tlsCfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify = false, want true")
	}
}

func TestTLSConfigMinVersion(t *testing.T) {
	cfg := defaultConfig()
	cfg.Server.TLS = true
	cfg.Server.TLSSkipVerify = true
	tlsCfg, err := cfg.TLSConfig()
	if err != nil {
		t.Fatalf("TLSConfig() error: %v", err)
	}
	if tlsCfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %#x, want %#x (TLS 1.2)", tlsCfg.MinVersion, tls.VersionTLS12)
	}
}

func TestTLSConfigCACert(t *testing.T) {
	// Write a self-signed PEM cert to a temp file.
	// This is a minimal valid PEM block (not a real cert, but enough to test parsing failure).
	dir := t.TempDir()
	certPath := filepath.Join(dir, "ca.pem")

	// Test with invalid cert content.
	if err := os.WriteFile(certPath, []byte("not a cert"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.Server.TLS = true
	cfg.Server.TLSCACert = certPath
	_, err := cfg.TLSConfig()
	if err == nil {
		t.Error("TLSConfig() should fail with invalid cert")
	}
}

func TestTLSConfigCACertMissing(t *testing.T) {
	cfg := defaultConfig()
	cfg.Server.TLS = true
	cfg.Server.TLSCACert = "/nonexistent/ca.pem"
	_, err := cfg.TLSConfig()
	if err == nil {
		t.Error("TLSConfig() should fail when CA cert file is missing")
	}
}

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		host     string
		loopback bool
	}{
		{"127.0.0.1", true},
		{"localhost", true},
		{"::1", true},
		{"192.168.1.1", false},
		{"example.com", false},
		{"0.0.0.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.Server.Host = tt.host
			if got := cfg.IsLoopback(); got != tt.loopback {
				t.Errorf("IsLoopback() with host %q = %v, want %v", tt.host, got, tt.loopback)
			}
		})
	}
}

func TestLoadWithTLSFields(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `server:
  host: 10.0.0.5
  port: 9090
  tls: true
  tls_ca_cert: /etc/ssl/ca.pem
  tls_skip_verify: false
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !cfg.Server.TLS {
		t.Error("TLS = false, want true")
	}
	if cfg.Server.TLSCACert != "/etc/ssl/ca.pem" {
		t.Errorf("TLSCACert = %q, want /etc/ssl/ca.pem", cfg.Server.TLSCACert)
	}
	if cfg.Server.TLSSkipVerify {
		t.Error("TLSSkipVerify = true, want false")
	}
}
