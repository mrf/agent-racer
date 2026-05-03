// Package racer provides racer-specific session state that wraps the generic
// agentwatch session model with racing display fields.
package racer

import (
	agwsession "github.com/mrf/agentwatch/session"
)

// RacerState wraps an agentwatch SessionState with racer-specific display fields.
// The embedded SessionState holds the generic monitoring data; the additional
// fields are agent-racer–specific concepts (lane assignment, race position, etc.).
type RacerState struct {
	agwsession.SessionState

	// Lane is a stable integer assigned at session discovery time.
	// Used to place the racer on a consistent track row.
	Lane int `json:"lane"`

	// Position is the 1-based rank among non-terminal sessions, sorted by
	// context utilization. 0 means unranked (terminal or not yet ranked).
	Position int `json:"position,omitempty"`

	// PositionDelta is positive when the racer moved up (gained rank) since
	// the last update, negative when it dropped. 0 means no change.
	PositionDelta int `json:"positionDelta,omitempty"`
}
