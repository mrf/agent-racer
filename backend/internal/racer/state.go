// Package racer extends the agentwatch session model with race-specific fields.
package racer

import (
	"encoding/json"
	"time"

	agwsession "github.com/mrf/agentwatch/session"
)

// RacerState extends agentwatch's generic SessionState with fields specific to
// the agent-racer game layer (lane assignment, leaderboard position, churn
// metrics, etc.). The embedded SessionState carries all generic monitoring
// data; RacerState adds only racer-specific concerns.
type RacerState struct {
	agwsession.SessionState

	// Identity / display
	Name string

	// Race position
	Lane          int
	Position      int // 1-based rank among non-terminal sessions
	PositionDelta int // positive = moved up, negative = dropped

	// Churn / compaction metrics
	IsChurning        bool
	BurnRatePerMinute float64
	CompactionCount   int

	// Host-process awareness (racer-specific; generic library omits these)
	PID        int
	TmuxTarget string

	// Preview text shown in race HUD
	LastAssistantText string
}

// MarshalJSON produces a flat JSON object that is backward-compatible with the
// wire format the frontend expects. The key translation is contextTokens
// (agentwatch field name) → tokensUsed (legacy field name).
func (r RacerState) MarshalJSON() ([]byte, error) {
	type wire struct {
		// Base session fields
		ID                 string                     `json:"id"`
		Source             string                     `json:"source"`
		Slug               string                     `json:"slug,omitempty"`
		Activity           agwsession.Activity        `json:"activity"`
		Lifecycle          agwsession.LifecycleState  `json:"lifecycle"`
		TokensUsed         int                        `json:"tokensUsed"` // renamed from contextTokens
		OutputTokens       int                        `json:"outputTokens,omitempty"`
		TokenEstimated     bool                       `json:"tokenEstimated"`
		MaxContextTokens   int                        `json:"maxContextTokens"`
		ContextUtilization float64                    `json:"contextUtilization"`
		Model              string                     `json:"model"`
		WorkingDir         string                     `json:"workingDir"`
		Branch             string                     `json:"branch,omitempty"`
		CurrentTool        string                     `json:"currentTool,omitempty"`
		MessageCount       int                        `json:"messageCount"`
		ToolCallCount      int                        `json:"toolCallCount"`
		StartedAt          time.Time                  `json:"startedAt"`
		LastActivityAt     time.Time                  `json:"lastActivityAt"`
		LastDataReceivedAt time.Time                  `json:"lastDataReceivedAt"`
		CompletedAt        *time.Time                 `json:"completedAt,omitempty"`
		Subagents          []agwsession.SubagentState `json:"subagents,omitempty"`
		// Racer-specific fields
		Name              string  `json:"name"`
		PID               int     `json:"pid,omitempty"`
		IsChurning        bool    `json:"isChurning,omitempty"`
		TmuxTarget        string  `json:"tmuxTarget,omitempty"`
		Lane              int     `json:"lane"`
		BurnRatePerMinute float64 `json:"burnRatePerMinute,omitempty"`
		CompactionCount   int     `json:"compactionCount,omitempty"`
		LastAssistantText string  `json:"lastAssistantText,omitempty"`
		Position          int     `json:"position,omitempty"`
		PositionDelta     int     `json:"positionDelta,omitempty"`
	}

	return json.Marshal(wire{
		ID:                 r.ID,
		Source:             r.Source,
		Slug:               r.Slug,
		Activity:           r.Activity,
		Lifecycle:          r.Lifecycle,
		TokensUsed:         r.ContextTokens,
		OutputTokens:       r.OutputTokens,
		TokenEstimated:     r.TokenEstimated,
		MaxContextTokens:   r.MaxContextTokens,
		ContextUtilization: r.ContextUtilization,
		Model:              r.Model,
		WorkingDir:         r.WorkingDir,
		Branch:             r.Branch,
		CurrentTool:        r.CurrentTool,
		MessageCount:       r.MessageCount,
		ToolCallCount:      r.ToolCallCount,
		StartedAt:          r.StartedAt,
		LastActivityAt:     r.LastActivityAt,
		LastDataReceivedAt: r.LastDataReceivedAt,
		CompletedAt:        r.CompletedAt,
		Subagents:          r.Subagents,
		Name:               r.Name,
		PID:                r.PID,
		IsChurning:         r.IsChurning,
		TmuxTarget:         r.TmuxTarget,
		Lane:               r.Lane,
		BurnRatePerMinute:  r.BurnRatePerMinute,
		CompactionCount:    r.CompactionCount,
		LastAssistantText:  r.LastAssistantText,
		Position:           r.Position,
		PositionDelta:      r.PositionDelta,
	})
}

// Clone returns a deep copy of r. Callers may mutate the copy without
// affecting the original.
func (r RacerState) Clone() RacerState {
	c := r
	c.SessionState = r.SessionState.Clone()
	return c
}
