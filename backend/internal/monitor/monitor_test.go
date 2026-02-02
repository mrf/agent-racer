package monitor

import (
	"testing"
	"time"
)

func TestTrackingKey(t *testing.T) {
	key := trackingKey("claude", "abc-123")
	if key != "claude:abc-123" {
		t.Errorf("trackingKey() = %q, want %q", key, "claude:abc-123")
	}
}

func TestClassifyActivityFromUpdate(t *testing.T) {
	tests := []struct {
		name     string
		update   SourceUpdate
		wantName string
	}{
		{"tool_use", SourceUpdate{Activity: "tool_use"}, "tool_use"},
		{"thinking", SourceUpdate{Activity: "thinking"}, "thinking"},
		{"waiting", SourceUpdate{Activity: "waiting"}, "waiting"},
		{"idle_no_data", SourceUpdate{}, "idle"},
		{"thinking_from_messages", SourceUpdate{MessageCount: 2}, "thinking"},
		{"idle_only_tokens", SourceUpdate{TokensIn: 100}, "idle"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			activity := classifyActivityFromUpdate(tt.update)
			if activity.String() != tt.wantName {
				t.Errorf("classifyActivityFromUpdate() = %q, want %q", activity.String(), tt.wantName)
			}
		})
	}
}

func TestNameFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/home/user/Projects/myapp", "myapp"},
		{"/tmp/test", "test"},
		{"", "unknown"},
		{"/", "unknown"},
		{"/single", "single"},
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
		want int // number of parts
	}{
		{"/home/user/project", 3},
		{"/tmp", 1},
		{"", 0},
		{"/", 0},
	}

	for _, tt := range tests {
		parts := splitPath(tt.path)
		if len(parts) != tt.want {
			t.Errorf("splitPath(%q) returned %d parts, want %d", tt.path, len(parts), tt.want)
		}
	}
}

func TestSourceUpdateHasData(t *testing.T) {
	tests := []struct {
		name   string
		update SourceUpdate
		want   bool
	}{
		{"empty", SourceUpdate{}, false},
		{"session_id", SourceUpdate{SessionID: "x"}, true},
		{"model", SourceUpdate{Model: "x"}, true},
		{"tokens_in", SourceUpdate{TokensIn: 1}, true},
		{"tokens_out", SourceUpdate{TokensOut: 1}, true},
		{"messages", SourceUpdate{MessageCount: 1}, true},
		{"tools", SourceUpdate{ToolCalls: 1}, true},
		{"last_tool", SourceUpdate{LastTool: "x"}, true},
		{"activity", SourceUpdate{Activity: "x"}, true},
		{"last_time", SourceUpdate{LastTime: time.Now()}, true},
		{"working_dir", SourceUpdate{WorkingDir: "x"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.update.HasData() != tt.want {
				t.Errorf("HasData() = %v, want %v", tt.update.HasData(), tt.want)
			}
		})
	}
}
