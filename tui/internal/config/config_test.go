package config

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
)

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
