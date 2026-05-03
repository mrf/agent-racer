// Package racer contains the racing-specific logic layered on top of
// the agentwatch session monitoring library.
package racer

import (
	"encoding/json"
	"time"

	"github.com/mrf/agentwatch/session"
)

// RacerState wraps an agentwatch SessionState with racing-specific fields
// that the library deliberately excludes (lane, position, overtake tracking,
// and agent-racer UI annotations).
type RacerState struct {
	session.SessionState

	// Name is a display-friendly label derived from the session's working directory.
	Name string `json:"name"`

	// Lane is a stable visual lane index assigned on first discovery.
	// Lanes are never reassigned for a given session.
	Lane int `json:"lane"`

	// Position is the 1-based rank among non-terminal sessions, ordered by
	// context utilization descending. Zero means the session is terminal.
	Position int `json:"position,omitempty"`

	// PositionDelta is the change from the previous position.
	// Positive = moved up, negative = dropped back.
	PositionDelta int `json:"positionDelta,omitempty"`

	// IsChurning indicates the session is in a tight CPU-active loop.
	IsChurning bool `json:"isChurning,omitempty"`

	// BurnRatePerMinute is the rolling token-burn rate (tokens/min).
	BurnRatePerMinute float64 `json:"burnRatePerMinute,omitempty"`

	// CompactionCount is how many times this session has been compacted.
	CompactionCount int `json:"compactionCount,omitempty"`

	// PID is the OS process ID of the agent, if known.
	PID int `json:"pid,omitempty"`

	// TmuxTarget is the tmux window target for this session (e.g. "agent-racer:1").
	TmuxTarget string `json:"tmuxTarget,omitempty"`

	// LastAssistantText is the most recent assistant message excerpt.
	LastAssistantText string `json:"lastAssistantText,omitempty"`
}

// IsTerminal returns true if the session's activity is terminal.
func (r *RacerState) IsTerminal() bool {
	return r.Activity == session.ActivityTerminal
}

// Clone returns a deep copy of the RacerState, duplicating pointer and slice
// fields so the copy can be mutated independently of the original.
func (r *RacerState) Clone() *RacerState {
	c := *r
	c.SessionState = r.SessionState.Clone()
	return &c
}

// MarshalJSON serialises RacerState to wire format. It renames the embedded
// SessionState's contextTokens field to tokensUsed for frontend compatibility.
func (r RacerState) MarshalJSON() ([]byte, error) {
	ss := r.SessionState
	type wire struct {
		ID                 string                    `json:"id"`
		Source             string                    `json:"source"`
		Slug               string                    `json:"slug,omitempty"`
		Activity           session.Activity          `json:"activity"`
		Lifecycle          session.LifecycleState    `json:"lifecycle"`
		ContextTokens      int                       `json:"tokensUsed"`
		OutputTokens       int                       `json:"outputTokens,omitempty"`
		TokenEstimated     bool                      `json:"tokenEstimated"`
		MaxContextTokens   int                       `json:"maxContextTokens"`
		ContextUtilization float64                   `json:"contextUtilization"`
		CurrentTool        string                    `json:"currentTool,omitempty"`
		Model              string                    `json:"model"`
		WorkingDir         string                    `json:"workingDir"`
		Branch             string                    `json:"branch,omitempty"`
		MessageCount       int                       `json:"messageCount"`
		ToolCallCount      int                       `json:"toolCallCount"`
		StartedAt          time.Time                 `json:"startedAt"`
		LastActivityAt     time.Time                 `json:"lastActivityAt"`
		LastDataReceivedAt time.Time                 `json:"lastDataReceivedAt"`
		CompletedAt        *time.Time                `json:"completedAt,omitempty"`
		Subagents          []session.SubagentState   `json:"subagents,omitempty"`
		Name               string                    `json:"name"`
		Lane               int                       `json:"lane"`
		Position           int                       `json:"position,omitempty"`
		PositionDelta      int                       `json:"positionDelta,omitempty"`
		IsChurning         bool                      `json:"isChurning,omitempty"`
		BurnRatePerMinute  float64                   `json:"burnRatePerMinute,omitempty"`
		CompactionCount    int                       `json:"compactionCount,omitempty"`
		PID                int                       `json:"pid,omitempty"`
		TmuxTarget         string                    `json:"tmuxTarget,omitempty"`
		LastAssistantText  string                    `json:"lastAssistantText,omitempty"`
	}
	return json.Marshal(wire{
		ID:                 ss.ID,
		Source:             ss.Source,
		Slug:               ss.Slug,
		Activity:           ss.Activity,
		Lifecycle:          ss.Lifecycle,
		ContextTokens:      ss.ContextTokens,
		OutputTokens:       ss.OutputTokens,
		TokenEstimated:     ss.TokenEstimated,
		MaxContextTokens:   ss.MaxContextTokens,
		ContextUtilization: ss.ContextUtilization,
		CurrentTool:        ss.CurrentTool,
		Model:              ss.Model,
		WorkingDir:         ss.WorkingDir,
		Branch:             ss.Branch,
		MessageCount:       ss.MessageCount,
		ToolCallCount:      ss.ToolCallCount,
		StartedAt:          ss.StartedAt,
		LastActivityAt:     ss.LastActivityAt,
		LastDataReceivedAt: ss.LastDataReceivedAt,
		CompletedAt:        ss.CompletedAt,
		Subagents:          ss.Subagents,
		Name:               r.Name,
		Lane:               r.Lane,
		Position:           r.Position,
		PositionDelta:      r.PositionDelta,
		IsChurning:         r.IsChurning,
		BurnRatePerMinute:  r.BurnRatePerMinute,
		CompactionCount:    r.CompactionCount,
		PID:                r.PID,
		TmuxTarget:         r.TmuxTarget,
		LastAssistantText:  r.LastAssistantText,
	})
}

// OvertakeEvent records one session passing another in the rankings.
type OvertakeEvent struct {
	OvertakerID   string `json:"overtakerId"`
	OvertakerName string `json:"overtakerName"`
	OvertakenID   string `json:"overtakenId"`
	OvertakenName string `json:"overtakenName"`
	NewPosition   int    `json:"newPosition"`
	At            time.Time
}
