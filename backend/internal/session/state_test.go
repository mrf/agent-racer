package session

import (
	"encoding/json"
	"testing"
	"time"
)

func TestActivityUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		expectValue Activity
	}{
		{
			name:        "valid starting",
			input:       `"starting"`,
			expectError: false,
			expectValue: Starting,
		},
		{
			name:        "valid thinking",
			input:       `"thinking"`,
			expectError: false,
			expectValue: Thinking,
		},
		{
			name:        "valid tool_use",
			input:       `"tool_use"`,
			expectError: false,
			expectValue: ToolUse,
		},
		{
			name:        "valid waiting",
			input:       `"waiting"`,
			expectError: false,
			expectValue: Waiting,
		},
		{
			name:        "valid idle",
			input:       `"idle"`,
			expectError: false,
			expectValue: Idle,
		},
		{
			name:        "valid complete",
			input:       `"complete"`,
			expectError: false,
			expectValue: Complete,
		},
		{
			name:        "valid errored",
			input:       `"errored"`,
			expectError: false,
			expectValue: Errored,
		},
		{
			name:        "valid lost",
			input:       `"lost"`,
			expectError: false,
			expectValue: Lost,
		},
		{
			name:        "unknown activity",
			input:       `"unknown_activity"`,
			expectError: true,
		},
		{
			name:        "empty string",
			input:       `""`,
			expectError: true,
		},
		{
			name:        "invalid json",
			input:       `not json`,
			expectError: true,
		},
	}

	for i := 0; i < len(tests); i++ {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			var a Activity
			err := a.UnmarshalJSON([]byte(tt.input))

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if a != tt.expectValue {
					t.Errorf("got %v, want %v", a, tt.expectValue)
				}
			}
		})
	}
}

func TestActivityMarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		activity Activity
		expected string
	}{
		{
			name:     "starting",
			activity: Starting,
			expected: `"starting"`,
		},
		{
			name:     "thinking",
			activity: Thinking,
			expected: `"thinking"`,
		},
		{
			name:     "tool_use",
			activity: ToolUse,
			expected: `"tool_use"`,
		},
		{
			name:     "waiting",
			activity: Waiting,
			expected: `"waiting"`,
		},
		{
			name:     "idle",
			activity: Idle,
			expected: `"idle"`,
		},
		{
			name:     "complete",
			activity: Complete,
			expected: `"complete"`,
		},
		{
			name:     "errored",
			activity: Errored,
			expected: `"errored"`,
		},
		{
			name:     "lost",
			activity: Lost,
			expected: `"lost"`,
		},
	}

	for i := 0; i < len(tests); i++ {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.activity.MarshalJSON()
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if string(data) != tt.expected {
				t.Errorf("got %s, want %s", string(data), tt.expected)
			}
		})
	}
}

func TestActivityRoundTrip(t *testing.T) {
	activities := []Activity{
		Starting,
		Thinking,
		ToolUse,
		Waiting,
		Idle,
		Complete,
		Errored,
		Lost,
	}

	for i := 0; i < len(activities); i++ {
		original := activities[i]
		t.Run(original.String(), func(t *testing.T) {
			// Marshal to JSON
			data, err := original.MarshalJSON()
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}

			// Unmarshal back
			var unmarshaled Activity
			if err := unmarshaled.UnmarshalJSON(data); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}

			if unmarshaled != original {
				t.Errorf("round trip failed: got %v, want %v", unmarshaled, original)
			}
		})
	}
}

func TestActivityInSessionState(t *testing.T) {
	// Test that unknown activity in SessionState JSON is properly rejected
	jsonData := []byte(`{
		"id": "test-session",
		"name": "Test Session",
		"source": "test",
		"activity": "future_unknown_activity",
		"tokensUsed": 100,
		"model": "test-model",
		"workingDir": "/tmp",
		"startedAt": "2026-02-28T00:00:00Z",
		"lastActivityAt": "2026-02-28T00:00:00Z",
		"lastDataReceivedAt": "2026-02-28T00:00:00Z",
		"messageCount": 0,
		"toolCallCount": 0,
		"lane": 0
	}`)

	var state SessionState
	err := json.Unmarshal(jsonData, &state)
	if err == nil {
		t.Errorf("expected error unmarshaling unknown activity in SessionState, got nil")
	}
}

func TestSessionStateClone(t *testing.T) {
	t.Run("returns a distinct pointer with identical scalar fields", func(t *testing.T) {
		orig := &SessionState{ID: "s1", Activity: Thinking}
		c := orig.Clone()

		if c == orig {
			t.Fatal("Clone returned same pointer")
		}
		if c.ID != orig.ID || c.Activity != orig.Activity {
			t.Errorf("scalar fields differ: clone=%+v, orig=%+v", c, orig)
		}
	})

	t.Run("preserves nil CompletedAt", func(t *testing.T) {
		orig := &SessionState{ID: "s1"}
		c := orig.Clone()

		if c.CompletedAt != nil {
			t.Errorf("expected nil CompletedAt, got %v", c.CompletedAt)
		}
	})

	t.Run("deep-copies CompletedAt", func(t *testing.T) {
		ts := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
		orig := &SessionState{ID: "s2", CompletedAt: &ts}
		c := orig.Clone()

		if c.CompletedAt == orig.CompletedAt {
			t.Fatal("CompletedAt pointer not deep-copied")
		}
		if !c.CompletedAt.Equal(*orig.CompletedAt) {
			t.Errorf("CompletedAt value differs: clone=%v, orig=%v", *c.CompletedAt, *orig.CompletedAt)
		}

		*c.CompletedAt = ts.Add(time.Hour)
		if !orig.CompletedAt.Equal(ts) {
			t.Error("mutating clone's CompletedAt affected the original")
		}
	})

	t.Run("empty subagents slice stays empty", func(t *testing.T) {
		orig := &SessionState{ID: "s3", Subagents: []SubagentState{}}
		c := orig.Clone()

		if len(c.Subagents) != 0 {
			t.Errorf("expected empty subagents, got len %d", len(c.Subagents))
		}
	})

	t.Run("deep-copies subagents slice and pointer fields", func(t *testing.T) {
		completedTime := time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC)
		orig := &SessionState{
			ID: "s4",
			Subagents: []SubagentState{
				{ID: "sa1", Activity: ToolUse},
				{ID: "sa2", Activity: Complete, CompletedAt: &completedTime},
			},
		}
		c := orig.Clone()

		if len(c.Subagents) != 2 {
			t.Fatalf("expected 2 subagents, got %d", len(c.Subagents))
		}

		c.Subagents[0].ID = "mutated"
		if orig.Subagents[0].ID == "mutated" {
			t.Error("mutating clone's Subagents slice affected the original")
		}

		if c.Subagents[1].CompletedAt == orig.Subagents[1].CompletedAt {
			t.Error("subagent CompletedAt pointer not deep-copied")
		}
		*c.Subagents[1].CompletedAt = completedTime.Add(time.Hour)
		if !orig.Subagents[1].CompletedAt.Equal(completedTime) {
			t.Error("mutating clone's subagent CompletedAt affected the original")
		}
	})
}

func TestSubagentStateClone(t *testing.T) {
	t.Run("preserves nil CompletedAt and scalar fields", func(t *testing.T) {
		orig := SubagentState{ID: "sa-nil", Activity: Thinking}
		c := orig.clone()

		if c.CompletedAt != nil {
			t.Errorf("expected nil CompletedAt, got %v", c.CompletedAt)
		}
		if c.ID != orig.ID {
			t.Errorf("ID differs: %s vs %s", c.ID, orig.ID)
		}
	})

	t.Run("deep-copies CompletedAt", func(t *testing.T) {
		ts := time.Date(2026, 3, 2, 8, 0, 0, 0, time.UTC)
		orig := SubagentState{ID: "sa-deep", CompletedAt: &ts}
		c := orig.clone()

		if c.CompletedAt == orig.CompletedAt {
			t.Fatal("CompletedAt pointer not deep-copied")
		}
		*c.CompletedAt = ts.Add(24 * time.Hour)
		if !orig.CompletedAt.Equal(ts) {
			t.Error("mutating clone CompletedAt affected the original")
		}
	})
}

func TestUpdateUtilization(t *testing.T) {
	tests := []struct {
		name     string
		used     int
		max      int
		expected float64
	}{
		{"zero max tokens", 500, 0, 0},
		{"normal ratio", 250, 1000, 0.25},
		{"capped at 1.0", 2000, 1000, 1.0},
	}
	for i := 0; i < len(tests); i++ {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			s := &SessionState{TokensUsed: tt.used, MaxContextTokens: tt.max}
			s.UpdateUtilization()
			if s.ContextUtilization != tt.expected {
				t.Errorf("got %f, want %f", s.ContextUtilization, tt.expected)
			}
		})
	}
}

func TestIsTerminal(t *testing.T) {
	tests := []struct {
		activity Activity
		terminal bool
	}{
		{Starting, false},
		{Thinking, false},
		{ToolUse, false},
		{Waiting, false},
		{Idle, false},
		{Complete, true},
		{Errored, true},
		{Lost, true},
	}
	for i := 0; i < len(tests); i++ {
		tt := tests[i]
		t.Run(tt.activity.String(), func(t *testing.T) {
			s := &SessionState{Activity: tt.activity}
			if got := s.IsTerminal(); got != tt.terminal {
				t.Errorf("IsTerminal() = %v, want %v", got, tt.terminal)
			}
		})
	}
}
