package session

import (
	"encoding/json"
	"testing"
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
