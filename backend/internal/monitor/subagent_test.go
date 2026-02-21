package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/session"
)

func TestSubagentSingleProgressEntry(t *testing.T) {
	path := writeJSONLLines(t,
		`{"type":"progress","toolUseID":"toolu_abc","parentToolUseID":"toolu_parent","sessionId":"sess-1","slug":"explore-codebase","timestamp":"2026-02-20T12:00:00.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-sonnet-4-6-20250514","role":"assistant","content":[{"type":"text","text":"Let me look at the code."},{"type":"tool_use","name":"Grep","id":"inner-1","input":{}}],"usage":{"input_tokens":200,"cache_creation_input_tokens":400,"cache_read_input_tokens":3000,"output_tokens":120}}}}}`,
	)

	result := parseJSONL(t, path)
	if len(result.Subagents) != 1 {
		t.Fatalf("expected 1 subagent, got %d", len(result.Subagents))
	}

	sub := requireSubagent(t, result, "toolu_abc")

	if sub.ID != "toolu_abc" {
		t.Errorf("ID = %s, want toolu_abc", sub.ID)
	}
	if sub.ParentToolUseID != "toolu_parent" {
		t.Errorf("ParentToolUseID = %s, want toolu_parent", sub.ParentToolUseID)
	}
	if sub.Slug != "explore-codebase" {
		t.Errorf("Slug = %s, want explore-codebase", sub.Slug)
	}
	if sub.Model != "claude-sonnet-4-6-20250514" {
		t.Errorf("Model = %s, want claude-sonnet-4-6-20250514", sub.Model)
	}
	if sub.MessageCount != 1 {
		t.Errorf("MessageCount = %d, want 1", sub.MessageCount)
	}
	if sub.ToolCalls != 1 {
		t.Errorf("ToolCalls = %d, want 1", sub.ToolCalls)
	}
	if sub.LastTool != "Grep" {
		t.Errorf("LastTool = %s, want Grep", sub.LastTool)
	}
	if sub.LastActivity != "tool_use" {
		t.Errorf("LastActivity = %s, want tool_use", sub.LastActivity)
	}
	if sub.LatestUsage == nil {
		t.Fatal("expected non-nil LatestUsage")
	}
	if got, want := sub.LatestUsage.TotalContext(), 200+400+3000; got != want {
		t.Errorf("TotalContext() = %d, want %d", got, want)
	}
	if sub.FirstTime.IsZero() {
		t.Error("expected FirstTime to be set")
	}
	if sub.FirstTime != sub.LastTime {
		t.Error("single entry: FirstTime should equal LastTime")
	}
}

func TestSubagentMultipleEntriesSameAgent(t *testing.T) {
	path := writeJSONLLines(t,
		// assistant with Read tool
		`{"type":"progress","toolUseID":"toolu_multi","parentToolUseID":"toolu_p1","sessionId":"sess-2","slug":"fix-bug","timestamp":"2026-02-20T14:00:00.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"tool_use","name":"Read","id":"r1"}],"usage":{"input_tokens":100,"cache_creation_input_tokens":200,"cache_read_input_tokens":800,"output_tokens":50}}}}}`,
		// user reply (tool result)
		`{"type":"progress","toolUseID":"toolu_multi","parentToolUseID":"toolu_p1","sessionId":"sess-2","slug":"fix-bug","timestamp":"2026-02-20T14:00:01.000Z","data":{"message":{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"r1","content":"file contents"}]}}}}`,
		// assistant with Edit tool, updated model and usage
		`{"type":"progress","toolUseID":"toolu_multi","parentToolUseID":"toolu_p1","sessionId":"sess-2","slug":"fix-bug","timestamp":"2026-02-20T14:00:02.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-6","role":"assistant","content":[{"type":"text","text":"I see the issue."},{"type":"tool_use","name":"Edit","id":"e1"}],"usage":{"input_tokens":300,"cache_creation_input_tokens":400,"cache_read_input_tokens":2000,"output_tokens":100}}}}}`,
		// another user reply
		`{"type":"progress","toolUseID":"toolu_multi","parentToolUseID":"toolu_p1","sessionId":"sess-2","slug":"fix-bug","timestamp":"2026-02-20T14:00:03.000Z","data":{"message":{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"e1","content":"edited"}]}}}}`,
	)

	result := parseJSONL(t, path)
	if len(result.Subagents) != 1 {
		t.Fatalf("expected 1 subagent, got %d", len(result.Subagents))
	}

	sub := requireSubagent(t, result, "toolu_multi")

	if sub.MessageCount != 4 {
		t.Errorf("MessageCount = %d, want 4", sub.MessageCount)
	}
	if sub.ToolCalls != 2 {
		t.Errorf("ToolCalls = %d, want 2 (Read + Edit)", sub.ToolCalls)
	}
	if sub.LastTool != "Edit" {
		t.Errorf("LastTool = %s, want Edit", sub.LastTool)
	}
	if sub.Model != "claude-opus-4-6" {
		t.Errorf("Model = %s, want claude-opus-4-6", sub.Model)
	}
	if sub.LatestUsage == nil {
		t.Fatal("expected non-nil LatestUsage")
	}
	if got, want := sub.LatestUsage.InputTokens, 300; got != want {
		t.Errorf("InputTokens = %d, want %d (latest snapshot)", got, want)
	}
	if sub.LastActivity != "waiting" {
		t.Errorf("LastActivity = %s, want waiting", sub.LastActivity)
	}
	if !sub.FirstTime.Before(sub.LastTime) {
		t.Error("expected FirstTime before LastTime")
	}
}

func TestSubagentMultipleParallelSubagents(t *testing.T) {
	path := writeJSONLLines(t,
		// Subagent A: explore agent
		`{"type":"progress","toolUseID":"toolu_A","parentToolUseID":"toolu_pA","sessionId":"sess-3","slug":"explore-api","timestamp":"2026-02-20T15:00:00.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-haiku-4-5-20251001","role":"assistant","content":[{"type":"tool_use","name":"Glob","id":"g1"}],"usage":{"input_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":200,"output_tokens":30}}}}}`,
		// Subagent B: test runner
		`{"type":"progress","toolUseID":"toolu_B","parentToolUseID":"toolu_pB","sessionId":"sess-3","slug":"run-tests","timestamp":"2026-02-20T15:00:00.500Z","data":{"message":{"type":"assistant","message":{"model":"claude-sonnet-4-6-20250514","role":"assistant","content":[{"type":"tool_use","name":"Bash","id":"b1"}],"usage":{"input_tokens":80,"cache_creation_input_tokens":100,"cache_read_input_tokens":500,"output_tokens":40}}}}}`,
		// Subagent A: user reply
		`{"type":"progress","toolUseID":"toolu_A","parentToolUseID":"toolu_pA","sessionId":"sess-3","slug":"explore-api","timestamp":"2026-02-20T15:00:01.000Z","data":{"message":{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"g1","content":"files found"}]}}}}`,
		// Subagent B: user reply
		`{"type":"progress","toolUseID":"toolu_B","parentToolUseID":"toolu_pB","sessionId":"sess-3","slug":"run-tests","timestamp":"2026-02-20T15:00:01.500Z","data":{"message":{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"b1","content":"tests passed"}]}}}}`,
	)

	result := parseJSONL(t, path)
	if len(result.Subagents) != 2 {
		t.Fatalf("expected 2 subagents, got %d", len(result.Subagents))
	}

	subA := requireSubagent(t, result, "toolu_A")
	subB := requireSubagent(t, result, "toolu_B")

	if subA.Slug != "explore-api" {
		t.Errorf("A.Slug = %s, want explore-api", subA.Slug)
	}
	if subA.Model != "claude-haiku-4-5-20251001" {
		t.Errorf("A.Model = %s, want claude-haiku-4-5-20251001", subA.Model)
	}
	if subA.MessageCount != 2 {
		t.Errorf("A.MessageCount = %d, want 2", subA.MessageCount)
	}
	if subA.ToolCalls != 1 {
		t.Errorf("A.ToolCalls = %d, want 1", subA.ToolCalls)
	}
	if subA.LastTool != "Glob" {
		t.Errorf("A.LastTool = %s, want Glob", subA.LastTool)
	}
	if subA.ParentToolUseID != "toolu_pA" {
		t.Errorf("A.ParentToolUseID = %s, want toolu_pA", subA.ParentToolUseID)
	}

	if subB.Slug != "run-tests" {
		t.Errorf("B.Slug = %s, want run-tests", subB.Slug)
	}
	if subB.Model != "claude-sonnet-4-6-20250514" {
		t.Errorf("B.Model = %s, want claude-sonnet-4-6-20250514", subB.Model)
	}
	if subB.MessageCount != 2 {
		t.Errorf("B.MessageCount = %d, want 2", subB.MessageCount)
	}
	if subB.ToolCalls != 1 {
		t.Errorf("B.ToolCalls = %d, want 1", subB.ToolCalls)
	}
	if subB.LastTool != "Bash" {
		t.Errorf("B.LastTool = %s, want Bash", subB.LastTool)
	}
	if subB.ParentToolUseID != "toolu_pB" {
		t.Errorf("B.ParentToolUseID = %s, want toolu_pB", subB.ParentToolUseID)
	}

	if subA.LastActivity != "waiting" {
		t.Errorf("A.LastActivity = %s, want waiting", subA.LastActivity)
	}
	if subB.LastActivity != "waiting" {
		t.Errorf("B.LastActivity = %s, want waiting", subB.LastActivity)
	}
	if subA.FirstTime.Equal(subB.FirstTime) {
		t.Error("expected different FirstTime for parallel subagents")
	}
}

func TestSubagentCompletion(t *testing.T) {
	tests := []struct {
		name      string
		toolUseID string
		parentID  string
		resultID  string
		wantDone  bool
	}{
		{
			name:      "matching tool_result marks subagent completed",
			toolUseID: "toolu_done",
			parentID:  "toolu_task_invoke",
			resultID:  "toolu_task_invoke",
			wantDone:  true,
		},
		{
			name:      "non-matching tool_result leaves subagent incomplete",
			toolUseID: "toolu_still_running",
			parentID:  "toolu_my_task",
			resultID:  "toolu_unrelated",
			wantDone:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeJSONLLines(t,
				fmt.Sprintf(`{"type":"progress","toolUseID":"%s","parentToolUseID":"%s","sessionId":"sess-comp","slug":"task","timestamp":"2026-02-20T16:00:00.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"text","text":"working"}]}}}}`, tt.toolUseID, tt.parentID),
				fmt.Sprintf(`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"%s","content":"result"}]},"sessionId":"sess-comp","timestamp":"2026-02-20T16:00:05.000Z"}`, tt.resultID),
			)

			sub := requireSubagent(t, parseJSONL(t, path), tt.toolUseID)

			if sub.Completed != tt.wantDone {
				t.Errorf("Completed = %v, want %v", sub.Completed, tt.wantDone)
			}
		})
	}
}

func TestSubagentCompletionSelectiveMatch(t *testing.T) {
	path := writeJSONLLines(t,
		`{"type":"progress","toolUseID":"toolu_X","parentToolUseID":"toolu_taskX","sessionId":"sess-6","slug":"task-x","timestamp":"2026-02-20T17:00:00.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"text","text":"x"}]}}}}`,
		`{"type":"progress","toolUseID":"toolu_Y","parentToolUseID":"toolu_taskY","sessionId":"sess-6","slug":"task-y","timestamp":"2026-02-20T17:00:01.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"text","text":"y"}]}}}}`,
		`{"type":"progress","toolUseID":"toolu_Z","parentToolUseID":"toolu_taskZ","sessionId":"sess-6","slug":"task-z","timestamp":"2026-02-20T17:00:02.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"text","text":"z"}]}}}}`,
		// Only subagent Y's parent gets a tool_result
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_taskY","content":"y done"}]},"sessionId":"sess-6","timestamp":"2026-02-20T17:00:10.000Z"}`,
	)

	result := parseJSONL(t, path)
	subX := requireSubagent(t, result, "toolu_X")
	subY := requireSubagent(t, result, "toolu_Y")
	subZ := requireSubagent(t, result, "toolu_Z")

	if subX.Completed {
		t.Error("subagent X should NOT be completed")
	}
	if !subY.Completed {
		t.Error("subagent Y should be completed")
	}
	if subZ.Completed {
		t.Error("subagent Z should NOT be completed")
	}
}

func TestSubagentIncrementalParsingAccumulates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-session.jsonl")

	chunk1 :=
		`{"type":"progress","toolUseID":"toolu_inc","parentToolUseID":"toolu_pinc","sessionId":"sess-7","slug":"incremental","timestamp":"2026-02-20T18:00:00.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"tool_use","name":"Read","id":"r1"}],"usage":{"input_tokens":100,"cache_creation_input_tokens":50,"cache_read_input_tokens":400,"output_tokens":30}}}}}` + "\n" +
		`{"type":"progress","toolUseID":"toolu_inc","parentToolUseID":"toolu_pinc","sessionId":"sess-7","slug":"incremental","timestamp":"2026-02-20T18:00:01.000Z","data":{"message":{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"r1","content":"data"}]}}}}` + "\n"

	if err := os.WriteFile(path, []byte(chunk1), 0644); err != nil {
		t.Fatal(err)
	}

	result1, offset1, err := ParseSessionJSONL(path, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	sub1 := requireSubagent(t, result1, "toolu_inc")
	if sub1.MessageCount != 2 {
		t.Errorf("chunk1: MessageCount = %d, want 2", sub1.MessageCount)
	}
	if sub1.ToolCalls != 1 {
		t.Errorf("chunk1: ToolCalls = %d, want 1", sub1.ToolCalls)
	}
	if sub1.LastTool != "Read" {
		t.Errorf("chunk1: LastTool = %s, want Read", sub1.LastTool)
	}

	chunk2 :=
		`{"type":"progress","toolUseID":"toolu_inc","parentToolUseID":"toolu_pinc","sessionId":"sess-7","slug":"incremental","timestamp":"2026-02-20T18:00:02.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","role":"assistant","content":[{"type":"tool_use","name":"Write","id":"w1"},{"type":"tool_use","name":"Bash","id":"b1"}],"usage":{"input_tokens":250,"cache_creation_input_tokens":100,"cache_read_input_tokens":1200,"output_tokens":80}}}}}` + "\n" +
		`{"type":"progress","toolUseID":"toolu_inc","parentToolUseID":"toolu_pinc","sessionId":"sess-7","slug":"incremental","timestamp":"2026-02-20T18:00:03.000Z","data":{"message":{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"w1","content":"written"}]}}}}` + "\n"

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(chunk2); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	result2, offset2, err := ParseSessionJSONL(path, offset1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if offset2 <= offset1 {
		t.Errorf("expected offset to advance: %d -> %d", offset1, offset2)
	}

	sub2 := requireSubagent(t, result2, "toolu_inc")

	if sub2.MessageCount != 2 {
		t.Errorf("chunk2: MessageCount = %d, want 2", sub2.MessageCount)
	}
	if sub2.ToolCalls != 2 {
		t.Errorf("chunk2: ToolCalls = %d, want 2 (Write + Bash)", sub2.ToolCalls)
	}
	if sub2.LastTool != "Bash" {
		t.Errorf("chunk2: LastTool = %s, want Bash", sub2.LastTool)
	}
	if sub2.LatestUsage == nil {
		t.Fatal("chunk2: expected non-nil LatestUsage")
	}
	if got, want := sub2.LatestUsage.InputTokens, 250; got != want {
		t.Errorf("chunk2: InputTokens = %d, want %d", got, want)
	}
}

func TestMergeSubagentsAppendsNew(t *testing.T) {
	state := &session.SessionState{ID: "sess-merge-1"}
	ts := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)

	parsed := map[string]*SubagentParseResult{
		"toolu_new": {
			ID:              "toolu_new",
			ParentToolUseID: "toolu_pnew",
			Slug:            "new-task",
			Model:           "claude-opus-4-5-20251101",
			LatestUsage: &TokenUsage{
				InputTokens:              100,
				CacheCreationInputTokens: 50,
				CacheReadInputTokens:     400,
				OutputTokens:             30,
			},
			MessageCount: 3,
			ToolCalls:    2,
			LastTool:     "Edit",
			LastActivity: "tool_use",
			FirstTime:    ts,
			LastTime:     ts.Add(5 * time.Second),
		},
	}

	mergeSubagents(state, parsed)

	if len(state.Subagents) != 1 {
		t.Fatalf("expected 1 subagent, got %d", len(state.Subagents))
	}

	sub := state.Subagents[0]
	if sub.ID != "toolu_new" {
		t.Errorf("ID = %s, want toolu_new", sub.ID)
	}
	if sub.ParentToolUseID != "toolu_pnew" {
		t.Errorf("ParentToolUseID = %s, want toolu_pnew", sub.ParentToolUseID)
	}
	if sub.SessionID != "sess-merge-1" {
		t.Errorf("SessionID = %s, want sess-merge-1", sub.SessionID)
	}
	if sub.Slug != "new-task" {
		t.Errorf("Slug = %s, want new-task", sub.Slug)
	}
	if sub.Model != "claude-opus-4-5-20251101" {
		t.Errorf("Model = %s, want claude-opus-4-5-20251101", sub.Model)
	}
	if sub.Activity != session.ToolUse {
		t.Errorf("Activity = %v, want ToolUse", sub.Activity)
	}
	if sub.CurrentTool != "Edit" {
		t.Errorf("CurrentTool = %s, want Edit", sub.CurrentTool)
	}
	if got, want := sub.TokensUsed, 100+50+400; got != want {
		t.Errorf("TokensUsed = %d, want %d", got, want)
	}
	if sub.MessageCount != 3 {
		t.Errorf("MessageCount = %d, want 3", sub.MessageCount)
	}
	if sub.ToolCallCount != 2 {
		t.Errorf("ToolCallCount = %d, want 2", sub.ToolCallCount)
	}
	if sub.StartedAt != ts {
		t.Errorf("StartedAt = %v, want %v", sub.StartedAt, ts)
	}
	if sub.LastActivityAt != ts.Add(5*time.Second) {
		t.Errorf("LastActivityAt = %v, want %v", sub.LastActivityAt, ts.Add(5*time.Second))
	}
	if sub.CompletedAt != nil {
		t.Error("expected CompletedAt to be nil for non-completed subagent")
	}
}

func TestMergeSubagentsUpdatesExisting(t *testing.T) {
	ts := time.Date(2026, 2, 20, 14, 0, 0, 0, time.UTC)

	state := &session.SessionState{
		ID: "sess-merge-2",
		Subagents: []session.SubagentState{
			{
				ID:              "toolu_exist",
				ParentToolUseID: "toolu_pexist",
				SessionID:       "sess-merge-2",
				Slug:            "initial-slug",
				Model:           "claude-opus-4-5-20251101",
				Activity:        session.Thinking,
				CurrentTool:     "Read",
				TokensUsed:      500,
				MessageCount:    2,
				ToolCallCount:   1,
				StartedAt:       ts,
				LastActivityAt:  ts.Add(1 * time.Second),
			},
		},
	}

	parsed := map[string]*SubagentParseResult{
		"toolu_exist": {
			ID:              "toolu_exist",
			ParentToolUseID: "toolu_pexist",
			Slug:            "updated-slug",
			Model:           "claude-opus-4-6",
			LatestUsage: &TokenUsage{
				InputTokens:              300,
				CacheCreationInputTokens: 100,
				CacheReadInputTokens:     800,
				OutputTokens:             60,
			},
			MessageCount: 4,
			ToolCalls:    3,
			LastTool:     "Bash",
			LastActivity: "tool_use",
			FirstTime:    ts,
			LastTime:     ts.Add(10 * time.Second),
		},
	}

	mergeSubagents(state, parsed)

	if len(state.Subagents) != 1 {
		t.Fatalf("expected 1 subagent (updated in place), got %d", len(state.Subagents))
	}

	sub := state.Subagents[0]
	if sub.Slug != "updated-slug" {
		t.Errorf("Slug = %s, want updated-slug", sub.Slug)
	}
	if sub.Model != "claude-opus-4-6" {
		t.Errorf("Model = %s, want claude-opus-4-6", sub.Model)
	}
	if sub.Activity != session.ToolUse {
		t.Errorf("Activity = %v, want ToolUse", sub.Activity)
	}
	if sub.CurrentTool != "Bash" {
		t.Errorf("CurrentTool = %s, want Bash", sub.CurrentTool)
	}
	if got, want := sub.TokensUsed, 1200; got != want {
		t.Errorf("TokensUsed = %d, want %d (max of old=500 and new=1200)", got, want)
	}
	if got, want := sub.MessageCount, 2+4; got != want {
		t.Errorf("MessageCount = %d, want %d (accumulated)", got, want)
	}
	if got, want := sub.ToolCallCount, 1+3; got != want {
		t.Errorf("ToolCallCount = %d, want %d (accumulated)", got, want)
	}
	if sub.LastActivityAt != ts.Add(10*time.Second) {
		t.Errorf("LastActivityAt = %v, want %v", sub.LastActivityAt, ts.Add(10*time.Second))
	}
}

func TestMergeSubagentsCompletedSetsCompletedAt(t *testing.T) {
	ts := time.Date(2026, 2, 20, 16, 0, 0, 0, time.UTC)

	state := &session.SessionState{
		ID: "sess-merge-3",
		Subagents: []session.SubagentState{
			{
				ID:             "toolu_completing",
				SessionID:      "sess-merge-3",
				Slug:           "finishing-task",
				Activity:       session.Thinking,
				MessageCount:   5,
				ToolCallCount:  3,
				LastActivityAt: ts,
			},
		},
	}

	completionTime := ts.Add(10 * time.Second)
	parsed := map[string]*SubagentParseResult{
		"toolu_completing": {
			ID:           "toolu_completing",
			Slug:         "finishing-task",
			MessageCount: 1,
			ToolCalls:    0,
			LastActivity: "waiting",
			LastTime:     completionTime,
			Completed:    true,
		},
	}

	mergeSubagents(state, parsed)

	sub := state.Subagents[0]
	if sub.Activity != session.Complete {
		t.Errorf("Activity = %v, want Complete", sub.Activity)
	}
	if sub.CompletedAt == nil {
		t.Fatal("expected CompletedAt to be set for completed subagent")
	}
	if !sub.CompletedAt.Equal(completionTime) {
		t.Errorf("CompletedAt = %v, want %v", *sub.CompletedAt, completionTime)
	}
	if got, want := sub.MessageCount, 5+1; got != want {
		t.Errorf("MessageCount = %d, want %d", got, want)
	}
}

func TestMergeSubagentsNilUsageKeepsZeroTokens(t *testing.T) {
	state := &session.SessionState{ID: "sess-merge-4"}

	parsed := map[string]*SubagentParseResult{
		"toolu_nousage": {
			ID:           "toolu_nousage",
			Slug:         "no-usage",
			MessageCount: 1,
			LastActivity: "thinking",
			FirstTime:    time.Now(),
			LastTime:     time.Now(),
		},
	}

	mergeSubagents(state, parsed)

	if len(state.Subagents) != 1 {
		t.Fatalf("expected 1 subagent, got %d", len(state.Subagents))
	}
	if state.Subagents[0].TokensUsed != 0 {
		t.Errorf("TokensUsed = %d, want 0 (nil usage)", state.Subagents[0].TokensUsed)
	}
}

func TestMergeSubagentsPrunesStaleEntries(t *testing.T) {
	ts := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)

	state := &session.SessionState{
		ID: "sess-prune",
		Subagents: []session.SubagentState{
			{
				ID:             "toolu_old_A",
				SessionID:      "sess-prune",
				Slug:           "old-task-a",
				Activity:       session.Thinking,
				MessageCount:   3,
				StartedAt:      ts,
				LastActivityAt: ts.Add(5 * time.Second),
			},
			{
				ID:             "toolu_old_B",
				SessionID:      "sess-prune",
				Slug:           "old-task-b",
				Activity:       session.ToolUse,
				MessageCount:   2,
				StartedAt:      ts,
				LastActivityAt: ts.Add(3 * time.Second),
			},
		},
	}

	// New poll only contains toolu_old_A (B has disappeared from the parsed set).
	parsed := map[string]*SubagentParseResult{
		"toolu_old_A": {
			ID:           "toolu_old_A",
			Slug:         "old-task-a",
			MessageCount: 1,
			LastActivity: "thinking",
			LastTime:     ts.Add(10 * time.Second),
		},
	}

	mergeSubagents(state, parsed)

	// toolu_old_B should have been pruned since it's not in the parsed set.
	if len(state.Subagents) != 1 {
		t.Fatalf("expected 1 subagent after pruning, got %d", len(state.Subagents))
	}
	if state.Subagents[0].ID != "toolu_old_A" {
		t.Errorf("remaining subagent ID = %s, want toolu_old_A", state.Subagents[0].ID)
	}
}

func TestMergeSubagentsRetainsCompletedWhenAbsentFromParsed(t *testing.T) {
	ts := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	completedAt := ts.Add(5 * time.Second)

	state := &session.SessionState{
		ID: "sess-retain",
		Subagents: []session.SubagentState{
			{
				ID:             "toolu_done",
				SessionID:      "sess-retain",
				Slug:           "finished-task",
				Activity:       session.Complete,
				CompletedAt:    &completedAt,
				MessageCount:   10,
				StartedAt:      ts,
				LastActivityAt: completedAt,
			},
		},
	}

	// Empty parsed set -- completed subagents survive pruning so the
	// frontend can display their final state.
	parsed := map[string]*SubagentParseResult{}

	mergeSubagents(state, parsed)

	if len(state.Subagents) != 1 {
		t.Fatalf("expected 1 subagent (completed, retained), got %d", len(state.Subagents))
	}
	if state.Subagents[0].ID != "toolu_done" {
		t.Errorf("retained subagent ID = %s, want toolu_done", state.Subagents[0].ID)
	}
}

func TestMergeSubagentsAccumulationBug(t *testing.T) {
	// Reproduces the hamster overflow bug: over many polls, each poll
	// sees a different set of subagents (or none). Without pruning,
	// state.Subagents grows without bound.
	state := &session.SessionState{ID: "sess-overflow"}
	ts := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)

	// Simulate 50 polls, each introducing a new subagent while the
	// previous ones are no longer in the parsed set.
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("toolu_%d", i)
		parsed := map[string]*SubagentParseResult{
			id: {
				ID:           id,
				Slug:         fmt.Sprintf("task-%d", i),
				MessageCount: 1,
				LastActivity: "thinking",
				FirstTime:    ts.Add(time.Duration(i) * time.Second),
				LastTime:     ts.Add(time.Duration(i) * time.Second),
			},
		}
		mergeSubagents(state, parsed)
	}

	// Without pruning, state.Subagents would have 50 entries.
	// Each poll replaces the previous non-completed subagent, so only
	// the most recent one should survive.
	if len(state.Subagents) != 1 {
		t.Errorf("subagent accumulation bug: expected 1 subagent, got %d", len(state.Subagents))
	}
}

func TestMergeSubagentsEmptyParsedPrunesNonCompleted(t *testing.T) {
	ts := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	completedAt := ts.Add(5 * time.Second)

	state := &session.SessionState{
		ID: "sess-empty-prune",
		Subagents: []session.SubagentState{
			{
				ID:             "toolu_active",
				SessionID:      "sess-empty-prune",
				Slug:           "active-task",
				Activity:       session.Thinking,
				MessageCount:   3,
				StartedAt:      ts,
				LastActivityAt: ts.Add(3 * time.Second),
			},
			{
				ID:             "toolu_completed",
				SessionID:      "sess-empty-prune",
				Slug:           "done-task",
				Activity:       session.Complete,
				CompletedAt:    &completedAt,
				MessageCount:   5,
				StartedAt:      ts,
				LastActivityAt: completedAt,
			},
		},
	}

	// Empty parsed set — no subagent data in this poll chunk.
	parsed := map[string]*SubagentParseResult{}

	mergeSubagents(state, parsed)

	// Non-completed subagent should be pruned; completed one retained.
	if len(state.Subagents) != 1 {
		t.Fatalf("expected 1 subagent (completed only), got %d", len(state.Subagents))
	}
	if state.Subagents[0].ID != "toolu_completed" {
		t.Errorf("retained subagent ID = %s, want toolu_completed", state.Subagents[0].ID)
	}
}

func TestClassifySubagentActivity(t *testing.T) {
	tests := []struct {
		name     string
		pr       *SubagentParseResult
		expected session.Activity
	}{
		{
			name:     "tool_use maps to ToolUse",
			pr:       &SubagentParseResult{LastActivity: "tool_use", MessageCount: 1},
			expected: session.ToolUse,
		},
		{
			name:     "thinking maps to Thinking",
			pr:       &SubagentParseResult{LastActivity: "thinking", MessageCount: 1},
			expected: session.Thinking,
		},
		{
			name:     "waiting maps to Waiting",
			pr:       &SubagentParseResult{LastActivity: "waiting", MessageCount: 1},
			expected: session.Waiting,
		},
		{
			name:     "empty activity with messages defaults to Thinking",
			pr:       &SubagentParseResult{LastActivity: "", MessageCount: 1},
			expected: session.Thinking,
		},
		{
			name:     "empty activity with no messages defaults to Idle",
			pr:       &SubagentParseResult{LastActivity: "", MessageCount: 0},
			expected: session.Idle,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifySubagentActivity(tt.pr)
			if got != tt.expected {
				t.Errorf("classifySubagentActivity() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestPhantomProgressEntriesFiltered verifies that progress entries where
// toolUseID == parentToolUseID are skipped. These are phantom entries
// emitted for tool calls within subagent sessions, not real subagents.
func TestPhantomProgressEntriesFiltered(t *testing.T) {
	path := writeJSONLLines(t,
		// Real subagent: toolUseID != parentToolUseID
		`{"type":"progress","toolUseID":"agent_abc","parentToolUseID":"toolu_parent","sessionId":"sess-phantom","slug":"real-agent","timestamp":"2026-02-20T12:00:00.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-6","role":"assistant","content":[{"type":"text","text":"working"}]}}}}`,
		// Phantom: toolUseID == parentToolUseID (tool call within subagent)
		`{"type":"progress","toolUseID":"toolu_phantom","parentToolUseID":"toolu_phantom","sessionId":"sess-phantom","slug":"phantom-agent","timestamp":"2026-02-20T12:00:01.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-6","role":"assistant","content":[{"type":"tool_use","name":"Read","id":"r1"}]}}}}`,
	)

	result := parseJSONL(t, path)

	if len(result.Subagents) != 1 {
		t.Fatalf("expected 1 subagent (phantom filtered), got %d", len(result.Subagents))
	}

	sub := requireSubagent(t, result, "agent_abc")
	if sub.Slug != "real-agent" {
		t.Errorf("Slug = %s, want real-agent", sub.Slug)
	}

	// Ensure the phantom is not present
	if _, exists := result.Subagents["toolu_phantom"]; exists {
		t.Error("phantom entry (toolUseID == parentToolUseID) should be filtered out")
	}
}

// TestCrossBatchCompletionDetection verifies that a tool_result arriving
// in a batch with no new progress entries still marks the subagent as
// completed, using the knownParents map from prior batches.
func TestCrossBatchCompletionDetection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-session.jsonl")

	// Batch 1: subagent appears via progress entries.
	chunk1 :=
		`{"type":"progress","toolUseID":"agent_1","parentToolUseID":"toolu_task1","sessionId":"sess-xbatch","slug":"my-task","timestamp":"2026-02-20T12:00:00.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-6","role":"assistant","content":[{"type":"tool_use","name":"Bash","id":"b1"}]}}}}` + "\n" +
		`{"type":"progress","toolUseID":"agent_1","parentToolUseID":"toolu_task1","sessionId":"sess-xbatch","slug":"my-task","timestamp":"2026-02-20T12:00:01.000Z","data":{"message":{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"b1","content":"ok"}]}}}}` + "\n"

	if err := os.WriteFile(path, []byte(chunk1), 0644); err != nil {
		t.Fatal(err)
	}

	result1, offset1, err := ParseSessionJSONL(path, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	sub1 := requireSubagent(t, result1, "agent_1")
	if sub1.Completed {
		t.Error("batch 1: subagent should NOT be completed yet")
	}

	// Batch 2: tool_result arrives with no new progress entries.
	// The knownParents map tells the parser about "agent_1".
	chunk2 :=
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_task1","content":"task done"}]},"sessionId":"sess-xbatch","timestamp":"2026-02-20T12:00:15.000Z"}` + "\n"

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(chunk2)
	f.Close()

	// Pass knownParents from batch 1's results.
	knownParents := map[string]string{
		"toolu_task1": "agent_1", // parentToolUseID → toolUseID
	}

	result2, offset2, err := ParseSessionJSONL(path, offset1, knownParents)
	if err != nil {
		t.Fatal(err)
	}
	if offset2 <= offset1 {
		t.Errorf("expected offset to advance: %d -> %d", offset1, offset2)
	}

	// The subagent should be created (via cross-batch) and marked completed.
	sub2 := requireSubagent(t, result2, "agent_1")
	if !sub2.Completed {
		t.Error("batch 2: subagent should be completed via cross-batch detection")
	}
}

// TestCrossBatchCompletionDoesNotOverrideCurrentBatch verifies that when
// both a progress entry and a tool_result for the same subagent appear in
// the same batch, the current batch's SubagentParseResult takes precedence.
func TestCrossBatchCompletionDoesNotOverrideCurrentBatch(t *testing.T) {
	path := writeJSONLLines(t,
		// Progress and completion in the same batch
		`{"type":"progress","toolUseID":"agent_2","parentToolUseID":"toolu_task2","sessionId":"sess-same","slug":"same-batch","timestamp":"2026-02-20T12:00:00.000Z","data":{"message":{"type":"assistant","message":{"model":"claude-opus-4-6","role":"assistant","content":[{"type":"text","text":"done"}]}}}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_task2","content":"result"}]},"sessionId":"sess-same","timestamp":"2026-02-20T12:00:01.000Z"}`,
	)

	// Even with knownParents, the current batch's entry should be used
	knownParents := map[string]string{
		"toolu_task2": "agent_2",
	}

	result, _, err := ParseSessionJSONL(path, 0, knownParents)
	if err != nil {
		t.Fatal(err)
	}

	sub := requireSubagent(t, result, "agent_2")
	if !sub.Completed {
		t.Error("subagent should be completed")
	}
	// Current batch entry should preserve the full data (not just minimal)
	if sub.Slug != "same-batch" {
		t.Errorf("Slug = %s, want same-batch (current batch entry should be preserved)", sub.Slug)
	}
}
