package monitor

import (
	"testing"
	"time"

	awsession "github.com/mrf/agentwatch/session"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/session"
)

func TestNameFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/home/user/project", "project"},
		{"/home/user/.claude/worktrees/my-feature", "my-feature"},
		{"/home/user/.claude/worktrees/fix-bug/subdir", "fix-bug"},
		{"", "unknown"},
		{"/", "unknown"},
	}
	for _, tt := range tests {
		got := nameFromPath(tt.path)
		if got != tt.want {
			t.Errorf("nameFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestSplitPath(t *testing.T) {
	tests := []struct {
		path string
		want int // expected number of parts
	}{
		{"/home/user/project", 3},
		{"/", 0},
		{"", 0},
	}
	for _, tt := range tests {
		got := splitPath(tt.path)
		if len(got) != tt.want {
			t.Errorf("splitPath(%q) = %d parts, want %d", tt.path, len(got), tt.want)
		}
	}
}

func TestDetermineActivityFromReason(t *testing.T) {
	tests := []struct {
		reason string
		want   session.Activity
	}{
		{"", session.Complete},
		{"user completed task", session.Complete},
		{"error: connection refused", session.Errored},
		{"process crashed unexpectedly", session.Errored},
		{"fatal: out of memory", session.Errored},
		{"interrupted by signal", session.Errored},
		{"SIGKILL terminated the process", session.Errored},
	}
	for _, tt := range tests {
		got := determineActivityFromReason(tt.reason)
		if got != tt.want {
			t.Errorf("determineActivityFromReason(%q) = %q, want %q", tt.reason, got, tt.want)
		}
	}
}

func TestConvertSubagents(t *testing.T) {
	subs := []awsession.SubagentState{
		{
			ID:             "sub-1",
			ParentID:       "parent-1",
			Activity:       awsession.ActivityWorking,
			CurrentTool:    "Bash",
			StartedAt:      time.Now().Add(-1 * time.Minute),
			LastActivityAt: time.Now(),
		},
		{
			ID:             "sub-2",
			ParentID:       "parent-2",
			Activity:       awsession.ActivityTerminal,
			LastActivityAt: time.Now(),
		},
	}

	result := convertSubagents(subs, "test:session-1")

	if len(result) != 2 {
		t.Fatalf("got %d subagents, want 2", len(result))
	}

	// First subagent: working → Thinking.
	if result[0].Activity != session.Thinking {
		t.Errorf("sub[0].Activity = %q, want %q", result[0].Activity, session.Thinking)
	}
	if result[0].SessionID != "test:session-1" {
		t.Errorf("sub[0].SessionID = %q, want %q", result[0].SessionID, "test:session-1")
	}
	if result[0].CurrentTool != "Bash" {
		t.Errorf("sub[0].CurrentTool = %q, want %q", result[0].CurrentTool, "Bash")
	}

	// Second subagent: terminal → Complete with CompletedAt set.
	if result[1].Activity != session.Complete {
		t.Errorf("sub[1].Activity = %q, want %q", result[1].Activity, session.Complete)
	}
	if result[1].CompletedAt == nil {
		t.Error("sub[1].CompletedAt should be non-nil for terminal subagent")
	}
}

func TestConvertSubagents_Nil(t *testing.T) {
	result := convertSubagents(nil, "x")
	if result != nil {
		t.Errorf("got %v, want nil", result)
	}
}

func TestConvertSubagentActivity(t *testing.T) {
	tests := []struct {
		input awsession.Activity
		want  session.Activity
	}{
		{awsession.ActivityWorking, session.Thinking},
		{awsession.ActivityWaiting, session.Waiting},
		{awsession.ActivityTerminal, session.Complete},
		{awsession.ActivityIdle, session.Idle},
		{"", session.Idle},
	}
	for _, tt := range tests {
		got := convertSubagentActivity(tt.input)
		if got != tt.want {
			t.Errorf("convertSubagentActivity(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- Token resolution tests ---

func newTestMonitor(tokenNorm config.TokenNormConfig) *Monitor {
	return &Monitor{
		cfg: &config.Config{
			TokenNorm: tokenNorm,
		},
		tokenSnapshots: make(map[string][]tokenSnapshot),
	}
}

func TestResolveTokens_Usage_RealData(t *testing.T) {
	m := newTestMonitor(config.TokenNormConfig{
		Strategies:       map[string]string{"default": "usage"},
		TokensPerMessage: 2000,
	})
	local := &session.SessionState{
		Source:           "claude",
		ContextTokens:    5000,
		MaxContextTokens: 200000,
	}
	m.resolveTokens(m.cfg, local, nil, false)
	if local.ContextTokens != 5000 {
		t.Errorf("ContextTokens = %d, want 5000", local.ContextTokens)
	}
	if local.TokenEstimated {
		t.Error("should not be estimated when real data")
	}
}

func TestResolveTokens_Usage_FallbackToEstimation(t *testing.T) {
	m := newTestMonitor(config.TokenNormConfig{
		Strategies:       map[string]string{"default": "usage"},
		TokensPerMessage: 2000,
	})
	local := &session.SessionState{
		Source:       "claude",
		MessageCount: 5,
	}
	m.resolveTokens(m.cfg, local, nil, false)
	if local.ContextTokens != 10000 {
		t.Errorf("ContextTokens = %d, want 10000", local.ContextTokens)
	}
	if !local.TokenEstimated {
		t.Error("should be estimated when falling back")
	}
}

func TestResolveTokens_Usage_CarryForwardRealData(t *testing.T) {
	m := newTestMonitor(config.TokenNormConfig{
		Strategies:       map[string]string{"default": "usage"},
		TokensPerMessage: 2000,
	})
	existing := &session.SessionState{
		ContextTokens:  8000,
		TokenEstimated: false,
	}
	local := &session.SessionState{
		Source:        "claude",
		ContextTokens: 0, // no data this poll
	}
	m.resolveTokens(m.cfg, local, existing, true)
	if local.ContextTokens != 8000 {
		t.Errorf("ContextTokens = %d, want 8000 (carried forward)", local.ContextTokens)
	}
	if local.TokenEstimated {
		t.Error("should not be estimated when carrying forward real data")
	}
}

func TestResolveTokens_Estimate(t *testing.T) {
	m := newTestMonitor(config.TokenNormConfig{
		Strategies:       map[string]string{"default": "estimate"},
		TokensPerMessage: 1500,
	})
	local := &session.SessionState{
		Source:        "codex",
		MessageCount:  10,
		ContextTokens: 5000, // real data from source, but strategy overrides
	}
	m.resolveTokens(m.cfg, local, nil, false)
	if local.ContextTokens != 15000 {
		t.Errorf("ContextTokens = %d, want 15000", local.ContextTokens)
	}
	if !local.TokenEstimated {
		t.Error("should be estimated with estimate strategy")
	}
}

func TestResolveTokens_MaxContextFromConfig(t *testing.T) {
	m := newTestMonitor(config.TokenNormConfig{
		Strategies: map[string]string{"default": "usage"},
	})
	m.cfg.Models = map[string]int{
		"claude-3.5-sonnet": 150000,
	}
	local := &session.SessionState{
		Source: "claude",
		Model:  "claude-3.5-sonnet",
	}
	m.resolveTokens(m.cfg, local, nil, false)
	if local.MaxContextTokens != 150000 {
		t.Errorf("MaxContextTokens = %d, want 150000", local.MaxContextTokens)
	}
}

// --- Burn rate tests ---

func TestCalculateBurnRate_Basic(t *testing.T) {
	m := newTestMonitor(config.TokenNormConfig{})
	now := time.Now()

	// First snapshot — no rate yet.
	rate := m.calculateBurnRate("sess1", 1000, now)
	if rate != 0 {
		t.Errorf("first snapshot should return 0, got %f", rate)
	}

	// Second snapshot 30s later with more tokens.
	rate = m.calculateBurnRate("sess1", 2000, now.Add(30*time.Second))
	if rate <= 0 {
		t.Errorf("should have positive rate after 30s, got %f", rate)
	}
	// 1000 tokens in 30s = 2000/min
	expectedRate := 2000.0
	if rate < expectedRate*0.9 || rate > expectedRate*1.1 {
		t.Errorf("rate = %f, expected ~%f", rate, expectedRate)
	}
}

func TestCalculateBurnRate_ZeroTokens(t *testing.T) {
	m := newTestMonitor(config.TokenNormConfig{})
	rate := m.calculateBurnRate("sess1", 0, time.Now())
	if rate != 0 {
		t.Errorf("zero tokens should return 0, got %f", rate)
	}
}

func TestCalculateBurnRate_WindowTrimming(t *testing.T) {
	m := newTestMonitor(config.TokenNormConfig{})
	base := time.Now()

	// Add many snapshots spanning > 60 seconds.
	for i := 0; i < 100; i++ {
		m.calculateBurnRate("sess1", 100*(i+1), base.Add(time.Duration(i)*time.Second))
	}

	// Snapshots older than burnRateWindow (60s) should be trimmed.
	snaps := m.tokenSnapshots["sess1"]
	if len(snaps) > maxTokenSnapshots {
		t.Errorf("snapshots = %d, should be <= %d", len(snaps), maxTokenSnapshots)
	}
}

// --- Activity mapping tests ---

func TestMapActivity(t *testing.T) {
	m := newTestMonitor(config.TokenNormConfig{})
	m.terminalReasons = make(map[string]terminalInfo)

	tests := []struct {
		name    string
		awState *awsession.SessionState
		want    session.Activity
	}{
		{
			"working with tool",
			&awsession.SessionState{Activity: awsession.ActivityWorking, CurrentTool: "Bash"},
			session.ToolUse,
		},
		{
			"working without tool",
			&awsession.SessionState{Activity: awsession.ActivityWorking},
			session.Thinking,
		},
		{
			"waiting",
			&awsession.SessionState{Activity: awsession.ActivityWaiting},
			session.Waiting,
		},
		{
			"idle",
			&awsession.SessionState{Activity: awsession.ActivityIdle},
			session.Idle,
		},
		{
			"terminal no reason",
			&awsession.SessionState{Activity: awsession.ActivityTerminal},
			session.Complete,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.mapActivity(tt.awState, "test:id")
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMapActivity_TerminalWithReason(t *testing.T) {
	m := newTestMonitor(config.TokenNormConfig{})
	m.terminalReasons = map[string]terminalInfo{
		"test:error-session": {reason: "error: connection failed", eventType: awsession.EventTerminal},
		"test:stale-session": {reason: "no data", eventType: awsession.EventStale},
	}

	// Error reason → Errored.
	got := m.mapActivity(&awsession.SessionState{Activity: awsession.ActivityTerminal}, "test:error-session")
	if got != session.Errored {
		t.Errorf("error reason: got %q, want %q", got, session.Errored)
	}

	// Stale → Lost.
	got = m.mapActivity(&awsession.SessionState{Activity: awsession.ActivityTerminal}, "test:stale-session")
	if got != session.Lost {
		t.Errorf("stale: got %q, want %q", got, session.Lost)
	}
}
