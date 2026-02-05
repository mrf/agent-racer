package session

import (
	"encoding/json"
	"testing"
)

func TestActivityMarshalJSON(t *testing.T) {
	tests := []struct {
		activity Activity
		expected string
	}{
		{Starting, `"starting"`},
		{Thinking, `"thinking"`},
		{ToolUse, `"tool_use"`},
		{Waiting, `"waiting"`},
		{Idle, `"idle"`},
		{Complete, `"complete"`},
		{Errored, `"errored"`},
		{Lost, `"lost"`},
	}

	for _, tt := range tests {
		data, err := json.Marshal(tt.activity)
		if err != nil {
			t.Errorf("Marshal(%v) error: %v", tt.activity, err)
			continue
		}
		if string(data) != tt.expected {
			t.Errorf("Marshal(%v) = %s, want %s", tt.activity, data, tt.expected)
		}
	}
}

func TestActivityUnmarshalJSON(t *testing.T) {
	tests := []struct {
		input    string
		expected Activity
	}{
		{`"thinking"`, Thinking},
		{`"tool_use"`, ToolUse},
		{`"complete"`, Complete},
	}

	for _, tt := range tests {
		var a Activity
		if err := json.Unmarshal([]byte(tt.input), &a); err != nil {
			t.Errorf("Unmarshal(%s) error: %v", tt.input, err)
			continue
		}
		if a != tt.expected {
			t.Errorf("Unmarshal(%s) = %v, want %v", tt.input, a, tt.expected)
		}
	}
}

func TestUpdateUtilization(t *testing.T) {
	s := &SessionState{
		TokensUsed:       100000,
		MaxContextTokens: 200000,
	}
	s.UpdateUtilization()

	if s.ContextUtilization != 0.5 {
		t.Errorf("ContextUtilization = %f, want 0.5", s.ContextUtilization)
	}

	// Test clamping to 1.0
	s.TokensUsed = 250000
	s.UpdateUtilization()
	if s.ContextUtilization != 1.0 {
		t.Errorf("ContextUtilization = %f, want 1.0", s.ContextUtilization)
	}
}

func TestTokenEstimatedJSON(t *testing.T) {
	s := &SessionState{
		ID:             "test:1",
		TokensUsed:     10000,
		TokenEstimated: true,
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded SessionState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if !decoded.TokenEstimated {
		t.Error("TokenEstimated should be true after round-trip")
	}

	// Verify the JSON field name is correct.
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map error: %v", err)
	}
	if _, ok := raw["tokenEstimated"]; !ok {
		t.Error("JSON should contain 'tokenEstimated' field")
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

	for _, tt := range tests {
		s := &SessionState{Activity: tt.activity}
		if s.IsTerminal() != tt.terminal {
			t.Errorf("IsTerminal() for %v = %v, want %v", tt.activity, s.IsTerminal(), tt.terminal)
		}
	}
}
