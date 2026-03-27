package session

import (
	"fmt"
	"testing"
	"time"
)

func makeSessionState(id string, activity Activity) *SessionState {
	now := time.Now()
	return &SessionState{
		ID:                 id,
		Name:               "session-" + id,
		Slug:               "bench-slug-" + id,
		Source:             "claude",
		Activity:           activity,
		TokensUsed:         5000,
		MaxContextTokens:   200000,
		ContextUtilization: 0.025,
		CurrentTool:        "Write",
		Model:              "claude-opus-4-6-20250514",
		WorkingDir:         "/home/user/project",
		StartedAt:          now.Add(-10 * time.Minute),
		LastActivityAt:     now,
		LastDataReceivedAt: now,
		MessageCount:       42,
		ToolCallCount:      15,
		Lane:               0,
		BurnRatePerMinute:  120.5,
		Subagents: []SubagentState{
			{
				ID:             "sub-1",
				SessionID:      id,
				Slug:           "sub-slug",
				Model:          "claude-sonnet-4-6-20250514",
				Activity:       Thinking,
				TokensUsed:     1000,
				MessageCount:   5,
				ToolCallCount:  3,
				StartedAt:      now.Add(-5 * time.Minute),
				LastActivityAt: now,
			},
		},
	}
}

func populateStore(s *Store, n int) {
	for i := 0; i < n; i++ {
		activity := Thinking
		if i%5 == 0 {
			activity = Complete
		}
		s.Update(makeSessionState(fmt.Sprintf("s-%d", i), activity))
	}
}

func BenchmarkStoreUpdate(b *testing.B) {
	s := NewStore()
	state := makeSessionState("bench", Thinking)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Update(state)
	}
}

func BenchmarkStoreGet(b *testing.B) {
	s := NewStore()
	populateStore(s, 50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Get("s-25")
	}
}

func BenchmarkStoreGetAll_10(b *testing.B) {
	s := NewStore()
	populateStore(s, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.GetAll()
	}
}

func BenchmarkStoreGetAll_50(b *testing.B) {
	s := NewStore()
	populateStore(s, 50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.GetAll()
	}
}

func BenchmarkStoreGetAll_200(b *testing.B) {
	s := NewStore()
	populateStore(s, 200)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.GetAll()
	}
}

func BenchmarkStoreBatchUpdate(b *testing.B) {
	s := NewStore()
	states := make([]*SessionState, 10)
	for i := 0; i < 10; i++ {
		states[i] = makeSessionState(fmt.Sprintf("batch-%d", i), Thinking)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.BatchUpdateAndNotify(states, nil)
	}
}

func BenchmarkStoreActiveCount(b *testing.B) {
	s := NewStore()
	populateStore(s, 50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.ActiveCount()
	}
}

func BenchmarkStoreRemove(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		s := NewStore()
		populateStore(s, 50)
		b.StartTimer()
		s.Remove("s-25")
	}
}

func BenchmarkSessionClone(b *testing.B) {
	state := makeSessionState("clone-bench", Thinking)
	state.Subagents = append(state.Subagents,
		SubagentState{ID: "sub-2", Slug: "sub-2", Activity: ToolUse, TokensUsed: 2000},
		SubagentState{ID: "sub-3", Slug: "sub-3", Activity: Waiting, TokensUsed: 500},
	)
	completedAt := time.Now()
	state.CompletedAt = &completedAt

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state.Clone()
	}
}

func BenchmarkComputeTeams(b *testing.B) {
	sessions := make([]*SessionState, 30)
	dirs := []string{"/home/user/project-a", "/home/user/project-b", "/home/user/project-c"}
	for i := 0; i < 30; i++ {
		s := makeSessionState(fmt.Sprintf("t-%d", i), Thinking)
		s.WorkingDir = dirs[i%len(dirs)]
		sessions[i] = s
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ComputeTeams(sessions)
	}
}
