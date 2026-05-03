// Package racer contains the racing-specific logic layered on top of
// the agentwatch session monitoring library.
package racer

import (
	"time"

	"github.com/mrf/agentwatch/session"
)

// RacerState wraps an agentwatch SessionState with racing-specific fields
// that the library deliberately excludes (lane, position, overtake tracking).
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
}

// IsTerminal returns true if the session's activity is terminal.
func (rs *RacerState) IsTerminal() bool {
	return rs.Activity == session.ActivityTerminal
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
