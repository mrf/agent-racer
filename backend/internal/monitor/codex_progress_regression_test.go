package monitor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	awsession "github.com/mrf/agentwatch/session"
	awsource "github.com/mrf/agentwatch/source"
	awcodex "github.com/mrf/agentwatch/sources/codex"

	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

// codexFixtureContent is the regression fixture containing a token_count event
// with both total_token_usage (64M cumulative) and last_token_usage (194K
// current context). The parser must use last_token_usage for track progress.
const codexFixtureContent = `{"timestamp":"2026-03-08T00:41:42.190Z","type":"session_meta","payload":{"id":"fixture-last-token-usage","timestamp":"2026-03-08T00:41:42.190Z","cwd":"/workspace/project","originator":"codex_cli_rs","cli_version":"0.111.0","source":"cli","model_provider":"openai","model":"gpt-5.4"}}
{"timestamp":"2026-03-08T00:41:42.190Z","type":"event_msg","payload":{"type":"task_started","turn_id":"turn-1","model_context_window":258400,"collaboration_mode_kind":"default"}}
{"timestamp":"2026-03-08T00:41:42.194Z","type":"turn_context","payload":{"turn_id":"turn-1","cwd":"/workspace/project","current_date":"2026-03-07","timezone":"America/Los_Angeles","approval_policy":"on-request","model":"gpt-5.4","effort":"high"}}
{"timestamp":"2026-03-08T00:41:57.923Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":64346294,"cached_input_tokens":61355392,"output_tokens":161772,"reasoning_output_tokens":59756,"total_tokens":64508066},"last_token_usage":{"input_tokens":194819,"cached_input_tokens":141696,"output_tokens":546,"reasoning_output_tokens":302,"total_tokens":195365},"model_context_window":258400},"rate_limits":{"limit_id":"codex","primary":{"used_percent":19.0,"window_minutes":300},"secondary":{"used_percent":6.0,"window_minutes":10080}}}}
`

// newCodexTestMonitor creates a Monitor configured for Codex token usage tests.
// Process and tmux enrichments are disabled. The caller must defer broadcaster.Stop().
func newCodexTestMonitor(src awsource.Source) (*Monitor, *session.Store, *ws.Broadcaster) {
	cfg := testConfig()
	cfg.Monitor.PollInterval = 100 * time.Millisecond
	cfg.TokenNorm.Strategies["codex"] = "usage"
	return newPollTestMonitor(src, cfg)
}

// TestCodexProgressUsesLastTokenUsage verifies the full pipeline:
// Codex source parser → agentwatch monitor → agent-racer resolveTokens.
// The Codex fixture has total_token_usage.input_tokens = 64,346,294 and
// last_token_usage.input_tokens = 194,819. Track progress MUST use
// last_token_usage (current context) not total_token_usage (cumulative).
//
// Regression for agent-racer-0bdq / agent-racer-6j33.
func TestCodexProgressUsesLastTokenUsage(t *testing.T) {
	// Write fixture to temp dir in the expected Codex layout.
	root := t.TempDir()
	sessDir := filepath.Join(root, "sessions", "2026", "03", "08")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}
	rolloutPath := filepath.Join(sessDir, "rollout-2026-03-08T00-41-42-fixture-last-token-usage.jsonl")
	if err := os.WriteFile(rolloutPath, []byte(codexFixtureContent), 0644); err != nil {
		t.Fatal(err)
	}

	src := awcodex.New(awcodex.WithRoot(root))
	mon, store, broadcaster := newCodexTestMonitor(src)
	defer broadcaster.Stop()

	mon.poll(context.Background())

	// Find the codex session in the store.
	var codexState *session.SessionState
	for _, s := range store.GetAll() {
		if s.Source == "codex" {
			codexState = s
			break
		}
	}
	if codexState == nil {
		t.Fatal("codex session not found in store after poll")
	}

	// Core assertion: ContextTokens must be from last_token_usage (194,819),
	// NOT total_token_usage (64,346,294).
	if codexState.ContextTokens == 64346294 {
		t.Fatalf("ContextTokens = %d (total_token_usage); must use last_token_usage instead",
			codexState.ContextTokens)
	}
	if codexState.ContextTokens != 194819 {
		t.Errorf("ContextTokens = %d, want 194819 (from last_token_usage.input_tokens)",
			codexState.ContextTokens)
	}

	if codexState.MaxContextTokens != 258400 {
		t.Errorf("MaxContextTokens = %d, want 258400", codexState.MaxContextTokens)
	}

	// Utilization must be ~0.754, definitely NOT clamped to 1.0.
	expectedUtil := float64(194819) / float64(258400)
	if codexState.ContextUtilization > 0.99 {
		t.Fatalf("ContextUtilization = %.4f (clamped to 1.0); regression — using total_token_usage",
			codexState.ContextUtilization)
	}
	const tolerance = 0.01
	if codexState.ContextUtilization < expectedUtil-tolerance || codexState.ContextUtilization > expectedUtil+tolerance {
		t.Errorf("ContextUtilization = %.4f, want ~%.4f",
			codexState.ContextUtilization, expectedUtil)
	}

	if codexState.TokenEstimated {
		t.Error("TokenEstimated = true, want false (real token data from source)")
	}
}

// TestCodexProgressTotalTokenUsageWouldCauseRegression demonstrates that
// if total_token_usage were used as the numerator, the session would
// immediately hit 100% utilization.
func TestCodexProgressTotalTokenUsageWouldCauseRegression(t *testing.T) {
	src := newStubSource("codex")
	mon, store, broadcaster := newCodexTestMonitor(src)
	defer broadcaster.Stop()

	now := time.Now()
	src.setHandles([]awsource.SessionHandle{
		{ID: "regress-1", Source: "codex", StartedAt: now},
	})
	src.setUpdate("regress-1", awsource.SourceUpdate{
		SessionID:        "regress-1",
		Activity:         awsession.ActivityWorking,
		ContextTokens:    64346294, // total_token_usage — WRONG
		MaxContextTokens: 258400,
		LastActivityAt:   now,
		WorkingDir:       "/workspace/project",
		Model:            "gpt-5.4",
	})

	mon.poll(context.Background())

	state, ok := store.Get("codex:regress-1")
	if !ok {
		t.Fatal("session not found")
	}

	// total_token_usage vastly exceeds the context window, causing
	// utilization to be clamped to 1.0 immediately.
	if state.ContextUtilization != 1.0 {
		t.Errorf("expected 1.0 utilization from total_token_usage regression; got %.4f",
			state.ContextUtilization)
	}
	if state.ContextTokens != 64346294 {
		t.Errorf("ContextTokens = %d, want 64346294 for regression scenario", state.ContextTokens)
	}
}

// TestCodexProgressDoesNotGoBackwards verifies that resolveTokens preserves
// higher real token values across polls, preventing progress bar jitter.
func TestCodexProgressDoesNotGoBackwards(t *testing.T) {
	src := newStubSource("codex")
	mon, store, broadcaster := newCodexTestMonitor(src)
	defer broadcaster.Stop()

	now := time.Now()
	src.setHandles([]awsource.SessionHandle{
		{ID: "mono-1", Source: "codex", StartedAt: now},
	})

	// First poll: 194,819 tokens.
	src.setUpdate("mono-1", awsource.SourceUpdate{
		SessionID:        "mono-1",
		Activity:         awsession.ActivityWorking,
		ContextTokens:    194819,
		MaxContextTokens: 258400,
		LastActivityAt:   now,
		WorkingDir:       "/workspace/project",
	})
	mon.poll(context.Background())

	state, ok := store.Get("codex:mono-1")
	if !ok {
		t.Fatal("session not found after first poll")
	}
	if state.ContextTokens != 194819 {
		t.Fatalf("first poll: ContextTokens = %d, want 194819", state.ContextTokens)
	}

	// Second poll: lower token count (e.g. new turn with smaller context).
	// The monotonic logic should preserve the higher value.
	src.setUpdate("mono-1", awsource.SourceUpdate{
		SessionID:        "mono-1",
		Activity:         awsession.ActivityWorking,
		ContextTokens:    100000,
		MaxContextTokens: 258400,
		LastActivityAt:   now.Add(5 * time.Second),
		WorkingDir:       "/workspace/project",
	})
	mon.poll(context.Background())

	state, ok = store.Get("codex:mono-1")
	if !ok {
		t.Fatal("session not found after second poll")
	}
	if state.ContextTokens != 194819 {
		t.Errorf("second poll: ContextTokens = %d, want 194819 (should not go backwards)",
			state.ContextTokens)
	}
}
