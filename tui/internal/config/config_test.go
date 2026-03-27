package config

import (
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
