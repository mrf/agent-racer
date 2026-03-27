package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateLogPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	// Create a temp file inside a simulated Claude projects dir.
	claudeDir := filepath.Join(home, ".claude", "projects")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	tmpFile, err := os.CreateTemp(claudeDir, "test-session-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	_ = tmpFile.Close()
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	t.Run("valid path in claude projects", func(t *testing.T) {
		if err := ValidateLogPath(tmpFile.Name()); err != nil {
			t.Errorf("expected valid path, got error: %v", err)
		}
	})

	t.Run("relative path rejected", func(t *testing.T) {
		if err := ValidateLogPath("relative/path.jsonl"); err == nil {
			t.Error("expected error for relative path")
		}
	})

	t.Run("path outside allowed dirs rejected", func(t *testing.T) {
		// Create a temp file outside allowed dirs.
		outside, err := os.CreateTemp(t.TempDir(), "outside-*.jsonl")
		if err != nil {
			t.Fatal(err)
		}
		_ = outside.Close()

		if err := ValidateLogPath(outside.Name()); err == nil {
			t.Error("expected error for path outside allowed dirs")
		}
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		// Construct a path with .. that would escape the allowed dir.
		traversal := filepath.Join(claudeDir, "..", "..", "..", "etc", "passwd")
		if err := ValidateLogPath(traversal); err == nil {
			t.Error("expected error for path traversal")
		}
	})

	t.Run("nonexistent path rejected", func(t *testing.T) {
		missing := filepath.Join(claudeDir, "nonexistent-session.jsonl")
		if err := ValidateLogPath(missing); err == nil {
			t.Error("expected error for nonexistent path")
		}
	})

	t.Run("directory rejected", func(t *testing.T) {
		if err := ValidateLogPath(claudeDir); err == nil {
			t.Error("expected error for directory path")
		}
	})
}
