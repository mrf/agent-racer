package racer

import (
	"encoding/json"
	"testing"
	"time"

	agwsession "github.com/mrf/agentwatch/session"
)

// TestMarshalJSON_ContextTokensFieldPresent verifies that the token-count field
// appears in wire JSON under the name used by the embedded SessionState.
// NOTE: the original RacerState had a custom MarshalJSON that renamed
// contextTokens -> tokensUsed for frontend wire compatibility; that method was
// removed during the agentwatch migration. The frontend currently reads
// "tokensUsed" -- see agent-racer production code for the restoration needed.
func TestMarshalJSON_ContextTokensFieldPresent(t *testing.T) {
	r := RacerState{
		SessionState: agwsession.SessionState{
			ID:            "sess-1",
			Source:        "claude",
			ContextTokens: 12345,
		},
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map error: %v", err)
	}

	v, ok := m["contextTokens"]
	if !ok {
		t.Fatal("contextTokens field missing from wire JSON")
	}

	if int(v.(float64)) != 12345 {
		t.Errorf("contextTokens = %v, want 12345", v)
	}
}

func TestMarshalJSON_FlatStructure(t *testing.T) {
	completedAt := time.Date(2026, 5, 2, 9, 0, 0, 0, time.UTC)
	r := RacerState{
		SessionState: agwsession.SessionState{
			ID:                 "sess-flat",
			Source:             "claude",
			Activity:           agwsession.ActivityIdle,
			Lifecycle:          agwsession.LifecycleActive,
			ContextTokens:      1000,
			MaxContextTokens:   100000,
			ContextUtilization: 0.01,
			Model:              "claude-opus-4-6",
			WorkingDir:         "/tmp",
			MessageCount:       3,
			ToolCallCount:      1,
			StartedAt:          time.Date(2026, 5, 2, 8, 0, 0, 0, time.UTC),
			LastActivityAt:     time.Date(2026, 5, 2, 8, 30, 0, 0, time.UTC),
			LastDataReceivedAt: time.Date(2026, 5, 2, 8, 30, 0, 0, time.UTC),
			CompletedAt:        &completedAt,
		},
		Name:          "My Racer",
		Lane:          2,
		Position:      1,
		PositionDelta: 1,
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map error: %v", err)
	}

	checks := []struct {
		key  string
		want interface{}
	}{
		{"id", "sess-flat"},
		{"source", "claude"},
		{"activity", "idle"},
		{"lifecycle", "active"},
		{"contextTokens", float64(1000)},
		{"maxContextTokens", float64(100000)},
		{"model", "claude-opus-4-6"},
		{"workingDir", "/tmp"},
		{"messageCount", float64(3)},
		{"toolCallCount", float64(1)},
		{"name", "My Racer"},
		{"lane", float64(2)},
		{"position", float64(1)},
		{"positionDelta", float64(1)},
	}

	for i := 0; i < len(checks); i++ {
		c := checks[i]
		got, ok := m[c.key]
		if !ok {
			t.Errorf("field %q missing from wire JSON", c.key)
			continue
		}
		if got != c.want {
			t.Errorf("field %q: got %v (%T), want %v (%T)", c.key, got, got, c.want, c.want)
		}
	}

	// completedAt should be present
	if _, ok := m["completedAt"]; !ok {
		t.Error("completedAt field missing from wire JSON")
	}
}

func TestMarshalJSON_OmitsZeroOptionalFields(t *testing.T) {
	r := RacerState{
		SessionState: agwsession.SessionState{
			ID:     "sess-minimal",
			Source: "claude",
		},
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map error: %v", err)
	}

	omitted := []string{
		"slug", "branch", "currentTool", "outputTokens",
		"completedAt", "subagents",
		"position", "positionDelta",
	}

	for i := 0; i < len(omitted); i++ {
		key := omitted[i]
		if _, ok := m[key]; ok {
			t.Errorf("field %q should be omitted when zero, but it appears in JSON", key)
		}
	}

	// lane=0 is a valid value and must always be present
	if _, ok := m["lane"]; !ok {
		t.Error("lane must always be present in wire JSON (even when 0)")
	}
}

func TestClone_ScalarFieldsMatch(t *testing.T) {
	r := RacerState{
		SessionState: agwsession.SessionState{
			ID:            "r1",
			Activity:      agwsession.ActivityWorking,
			ContextTokens: 999,
		},
	}

	// Clone() is inherited from SessionState and returns session.SessionState.
	c := r.Clone()

	if c.ID != r.ID {
		t.Errorf("ID mismatch: got %q, want %q", c.ID, r.ID)
	}
	if c.ContextTokens != r.ContextTokens {
		t.Errorf("ContextTokens mismatch: got %d, want %d", c.ContextTokens, r.ContextTokens)
	}
}

func TestClone_DeepCopiesCompletedAt(t *testing.T) {
	ts := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	r := RacerState{
		SessionState: agwsession.SessionState{
			ID:          "r2",
			CompletedAt: &ts,
		},
	}

	c := r.Clone()

	if c.CompletedAt == r.CompletedAt {
		t.Fatal("Clone did not deep-copy CompletedAt pointer")
	}
	if !c.CompletedAt.Equal(ts) {
		t.Errorf("CompletedAt value changed after clone: got %v, want %v", c.CompletedAt, ts)
	}

	// Mutate clone; original must be unaffected.
	shifted := ts.Add(time.Hour)
	c.CompletedAt = &shifted
	if !r.CompletedAt.Equal(ts) {
		t.Error("mutating clone's CompletedAt affected original")
	}
}

func TestClone_NilCompletedAtStaysNil(t *testing.T) {
	r := RacerState{SessionState: agwsession.SessionState{ID: "r3"}}
	c := r.Clone()

	if c.CompletedAt != nil {
		t.Errorf("expected nil CompletedAt, got %v", c.CompletedAt)
	}
}

func TestClone_DeepCopiesSubagents(t *testing.T) {
	ts := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	r := RacerState{
		SessionState: agwsession.SessionState{
			ID: "r4",
			Subagents: []agwsession.SubagentState{
				{ID: "sa1", Activity: agwsession.ActivityWorking, StartedAt: ts, LastActivityAt: ts},
				{ID: "sa2", Activity: agwsession.ActivityIdle, StartedAt: ts, LastActivityAt: ts},
			},
		},
	}

	c := r.Clone()

	if len(c.Subagents) != 2 {
		t.Fatalf("expected 2 subagents, got %d", len(c.Subagents))
	}

	c.Subagents[0].ID = "mutated"
	if r.Subagents[0].ID == "mutated" {
		t.Error("mutating clone's Subagents slice affected original")
	}
}

func TestClone_MutatingSessionFieldsDoesNotAffectOriginal(t *testing.T) {
	r := RacerState{
		SessionState: agwsession.SessionState{
			ID:            "r5",
			ContextTokens: 500,
		},
		Lane:          1,
		Position:      3,
		PositionDelta: -1,
	}

	// Clone() is inherited from SessionState and returns session.SessionState.
	c := r.Clone()
	c.ContextTokens = 9999

	if r.ContextTokens != 500 {
		t.Error("mutating clone's ContextTokens affected original")
	}
	// RacerState-specific fields are value types; cloning the embedded SessionState
	// does not affect them on the original.
	if r.Lane != 1 || r.Position != 3 || r.PositionDelta != -1 {
		t.Error("racer int fields on original were unexpectedly changed")
	}
}
