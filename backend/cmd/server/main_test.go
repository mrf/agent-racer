package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/monitor"
	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}
	return path
}

func newReloadTestBroadcaster(t *testing.T) (*session.Store, *ws.Broadcaster) {
	t.Helper()
	store := session.NewStore()
	b := ws.NewBroadcaster(store, 50*time.Millisecond, 10*time.Second, 0)
	t.Cleanup(func() { b.Stop() })
	return store, b
}

// TestApplyConfigReload_PrivacyFilterUpdated verifies that the broadcaster's
// privacy filter takes effect when the config file enables PID masking.
func TestApplyConfigReload_PrivacyFilterUpdated(t *testing.T) {
	_, b := newReloadTestBroadcaster(t)

	// Old config: no privacy masking.
	oldPath := writeConfigFile(t, "privacy:\n  mask_pids: false\n")
	oldCfg, err := config.LoadOrDefault(oldPath)
	if err != nil {
		t.Fatalf("LoadOrDefault: %v", err)
	}

	sessions := []*session.SessionState{{ID: "s1", WorkingDir: "/project", PID: 1234}}

	// Before reload: PIDs should be visible.
	before := b.FilterSessions(sessions)
	if len(before) != 1 || before[0].PID != 1234 {
		t.Fatalf("before reload: PID = %d, want 1234", before[0].PID)
	}

	// New config with PID masking enabled.
	newPath := writeConfigFile(t, "privacy:\n  mask_pids: true\n")
	newCfg, err := applyConfigReload(newPath, oldCfg, nil, b)
	if err != nil {
		t.Fatalf("applyConfigReload: %v", err)
	}
	if !newCfg.Privacy.MaskPIDs {
		t.Error("returned config should have MaskPIDs=true")
	}

	// After reload: PIDs should be masked.
	after := b.FilterSessions(sessions)
	if len(after) != 1 || after[0].PID != 0 {
		t.Errorf("after reload: PID = %d, want 0 (masked)", after[0].PID)
	}
}

// TestApplyConfigReload_NoChangesReturnsSameConfig verifies that reloading an
// unchanged config file is a no-op: the function returns the exact same config
// pointer without modifying the broadcaster.
func TestApplyConfigReload_NoChangesReturnsSameConfig(t *testing.T) {
	_, b := newReloadTestBroadcaster(t)

	// Load from an empty file so both old and new configs have identical defaults.
	cfgPath := writeConfigFile(t, "")
	oldCfg, err := config.LoadOrDefault(cfgPath)
	if err != nil {
		t.Fatalf("LoadOrDefault: %v", err)
	}

	// Reload the same file — Diff should find no changes.
	returned, err := applyConfigReload(cfgPath, oldCfg, nil, b)
	if err != nil {
		t.Fatalf("applyConfigReload: %v", err)
	}
	if returned != oldCfg {
		t.Error("no-op reload should return the original config pointer unchanged")
	}
}

// TestApplyConfigReload_MonitorConfigApplied verifies that when a monitor is
// provided and config changes are detected, monitor.SetConfig is called with
// the new configuration.
func TestApplyConfigReload_MonitorConfigApplied(t *testing.T) {
	store, b := newReloadTestBroadcaster(t)

	// Base config: load from empty file (all defaults, poll_interval = 1s).
	basePath := writeConfigFile(t, "")
	oldCfg, err := config.LoadOrDefault(basePath)
	if err != nil {
		t.Fatalf("LoadOrDefault: %v", err)
	}

	mon := monitor.NewMonitor(oldCfg, store, b, nil)

	// New config: different poll interval.
	newPath := writeConfigFile(t, "monitor:\n  poll_interval: 2s\n")
	newCfg, err := applyConfigReload(newPath, oldCfg, mon, b)
	if err != nil {
		t.Fatalf("applyConfigReload: %v", err)
	}
	if newCfg == oldCfg {
		t.Fatal("config should have changed (poll_interval differs)")
	}

	// The monitor's internal config should reflect the new poll interval.
	applied := mon.Config()
	if applied.Monitor.PollInterval != 2*time.Second {
		t.Errorf("monitor PollInterval = %s, want 2s", applied.Monitor.PollInterval)
	}
}

// TestApplyConfigReload_PrivacyFilterBlocksSession verifies that path-based
// filtering applied via reload correctly excludes sessions whose working
// directory no longer matches the allowed pattern.
func TestApplyConfigReload_PrivacyFilterBlocksSession(t *testing.T) {
	_, b := newReloadTestBroadcaster(t)

	// Old config: no path restrictions.
	oldPath := writeConfigFile(t, "")
	oldCfg, err := config.LoadOrDefault(oldPath)
	if err != nil {
		t.Fatalf("LoadOrDefault: %v", err)
	}

	sessions := []*session.SessionState{
		{ID: "s1", WorkingDir: "/home/user/work"},
		{ID: "s2", WorkingDir: "/home/user/personal"},
	}

	// Before reload: both sessions visible.
	before := b.FilterSessions(sessions)
	if len(before) != 2 {
		t.Fatalf("before reload: got %d sessions, want 2", len(before))
	}

	// New config: only allow /home/user/work/*.
	newPath := writeConfigFile(t, "privacy:\n  allowed_paths:\n    - /home/user/work\n")
	_, err = applyConfigReload(newPath, oldCfg, nil, b)
	if err != nil {
		t.Fatalf("applyConfigReload: %v", err)
	}

	// After reload: only the work session is broadcast.
	after := b.FilterSessions(sessions)
	if len(after) != 1 || after[0].ID != "s1" {
		t.Errorf("after reload: got sessions %v, want [s1]", sessionIDs(after))
	}
}

func sessionIDs(sessions []*session.SessionState) []string {
	ids := make([]string, len(sessions))
	for i, s := range sessions {
		ids[i] = s.ID
	}
	return ids
}
