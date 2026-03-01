package client

import (
	"encoding/json"
	"testing"
	"time"
)

func TestWSMessageDeserialization(t *testing.T) {
	raw := `{"type":"snapshot","seq":42,"payload":{"sessions":[]}}`
	var msg WSMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if msg.Type != MsgSnapshot {
		t.Errorf("Type = %q, want %q", msg.Type, MsgSnapshot)
	}
	if msg.Seq != 42 {
		t.Errorf("Seq = %d, want 42", msg.Seq)
	}
	if len(msg.Payload) == 0 {
		t.Error("Payload should not be empty")
	}
}

func TestMessageTypeConstants(t *testing.T) {
	types := map[MessageType]string{
		MsgSnapshot:            "snapshot",
		MsgDelta:               "delta",
		MsgCompletion:          "completion",
		MsgEquipped:            "equipped",
		MsgError:               "error",
		MsgAchievementUnlocked: "achievement_unlocked",
		MsgSourceHealth:        "source_health",
		MsgBattlePassProgress:  "battlepass_progress",
	}
	for got, want := range types {
		if string(got) != want {
			t.Errorf("MessageType %q != %q", got, want)
		}
	}
}

func TestSnapshotPayloadDeserialization(t *testing.T) {
	raw := `{
		"sessions": [
			{
				"id": "abc-123",
				"name": "my-project",
				"activity": "thinking",
				"tokensUsed": 50000,
				"maxContextTokens": 200000,
				"contextUtilization": 0.25,
				"model": "claude-sonnet-4-6",
				"workingDir": "/home/user/project",
				"lane": 2
			}
		]
	}`
	var p SnapshotPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(p.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(p.Sessions))
	}
	s := p.Sessions[0]
	if s.ID != "abc-123" {
		t.Errorf("ID = %q, want abc-123", s.ID)
	}
	if s.Activity != ActivityThinking {
		t.Errorf("Activity = %q, want thinking", s.Activity)
	}
	if s.TokensUsed != 50000 {
		t.Errorf("TokensUsed = %d, want 50000", s.TokensUsed)
	}
	if s.ContextUtilization != 0.25 {
		t.Errorf("ContextUtilization = %f, want 0.25", s.ContextUtilization)
	}
	if s.Lane != 2 {
		t.Errorf("Lane = %d, want 2", s.Lane)
	}
}

func TestDeltaPayloadDeserialization(t *testing.T) {
	raw := `{"updates":[{"id":"s1","activity":"idle","model":"m","workingDir":"/"}],"removed":["s2","s3"]}`
	var p DeltaPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(p.Updates) != 1 || p.Updates[0].ID != "s1" {
		t.Errorf("Updates[0].ID = %q, want s1", p.Updates[0].ID)
	}
	if len(p.Removed) != 2 || p.Removed[0] != "s2" || p.Removed[1] != "s3" {
		t.Errorf("Removed = %v, want [s2 s3]", p.Removed)
	}
}

func TestSessionStateOptionalFields(t *testing.T) {
	// CompletedAt is optional (*time.Time).
	raw := `{"id":"x","activity":"complete","model":"m","workingDir":"/","completedAt":"2026-01-01T00:00:00Z"}`
	var s SessionState
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if s.CompletedAt == nil {
		t.Fatal("CompletedAt should not be nil")
	}
	want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !s.CompletedAt.Equal(want) {
		t.Errorf("CompletedAt = %v, want %v", *s.CompletedAt, want)
	}

	// Without completedAt field → nil pointer.
	raw2 := `{"id":"y","activity":"thinking","model":"m","workingDir":"/"}`
	var s2 SessionState
	if err := json.Unmarshal([]byte(raw2), &s2); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if s2.CompletedAt != nil {
		t.Errorf("CompletedAt should be nil when field is absent")
	}
}

func TestActivityConstants(t *testing.T) {
	activities := map[Activity]string{
		ActivityStarting: "starting",
		ActivityThinking: "thinking",
		ActivityToolUse:  "tool_use",
		ActivityWaiting:  "waiting",
		ActivityIdle:     "idle",
		ActivityComplete: "complete",
		ActivityErrored:  "errored",
		ActivityLost:     "lost",
	}
	for got, want := range activities {
		if string(got) != want {
			t.Errorf("Activity %q != %q", got, want)
		}
	}
}

func TestSourceHealthPayloadDeserialization(t *testing.T) {
	raw := `{
		"source":"claude",
		"status":"healthy",
		"discoverFailures":0,
		"parseFailures":2,
		"lastError":"",
		"timestamp":"2026-01-01T12:00:00Z"
	}`
	var p SourceHealthPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if p.Source != "claude" {
		t.Errorf("Source = %q, want claude", p.Source)
	}
	if p.Status != StatusHealthy {
		t.Errorf("Status = %q, want healthy", p.Status)
	}
	if p.ParseFailures != 2 {
		t.Errorf("ParseFailures = %d, want 2", p.ParseFailures)
	}
}
