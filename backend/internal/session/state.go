package session

import (
	"encoding/json"
	"fmt"
	"time"

	agwsession "github.com/mrf/agentwatch/session"
)

// Activity represents what an agent session is currently doing.
// It is a string type so unknown future values round-trip safely.
type Activity string

const (
	Starting Activity = "starting"
	Thinking Activity = "thinking"
	ToolUse  Activity = "tool_use"
	Waiting  Activity = "waiting"
	Idle     Activity = "idle"
	Complete Activity = "complete"
	Errored  Activity = "errored"
	Lost     Activity = "lost"
)

// knownActivities is the set of activities this package recognises.
// Unknown values are still accepted for forward-compatibility.
var knownActivities = map[Activity]struct{}{
	Starting: {},
	Thinking: {},
	ToolUse:  {},
	Waiting:  {},
	Idle:     {},
	Complete: {},
	Errored:  {},
	Lost:     {},
}

func (a Activity) String() string {
	return string(a)
}

func (a Activity) MarshalJSON() ([]byte, error) {
	// Zero value of the old int enum was Starting (iota = 0). Preserve that
	// behaviour for code that creates SessionState without setting Activity.
	if a == "" {
		return json.Marshal(string(Starting))
	}
	return json.Marshal(string(a))
}

func (a *Activity) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v := Activity(s)
	if _, ok := knownActivities[v]; !ok {
		return fmt.Errorf("unknown Activity %q", s)
	}
	*a = v
	return nil
}

// LifecycleState mirrors agentwatch's LifecycleState for session lifecycle tracking.
type LifecycleState = agwsession.LifecycleState

const (
	LifecycleActive   = agwsession.LifecycleActive
	LifecycleTerminal = agwsession.LifecycleTerminal
)

type SessionState struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Slug               string          `json:"slug,omitempty"` // Internal session name (e.g. "mighty-cuddling-castle")
	Source             string          `json:"source"`
	Activity           Activity        `json:"activity"`
	Lifecycle          LifecycleState  `json:"lifecycle,omitempty"`
	ContextTokens      int             `json:"tokensUsed"` // JSON tag kept for wire-protocol compatibility
	OutputTokens       int             `json:"outputTokens,omitempty"`
	TokenEstimated     bool            `json:"tokenEstimated"`
	MaxContextTokens   int             `json:"maxContextTokens"`
	ContextUtilization float64         `json:"contextUtilization"`
	CurrentTool        string          `json:"currentTool,omitempty"`
	Model              string          `json:"model"`
	WorkingDir         string          `json:"workingDir"`
	Branch             string          `json:"branch,omitempty"`
	StartedAt          time.Time       `json:"startedAt"`
	LastActivityAt     time.Time       `json:"lastActivityAt"`
	LastDataReceivedAt time.Time       `json:"lastDataReceivedAt"`
	CompletedAt        *time.Time      `json:"completedAt,omitempty"`
	MessageCount       int             `json:"messageCount"`
	ToolCallCount      int             `json:"toolCallCount"`
	PID                int             `json:"pid,omitempty"`
	IsChurning         bool            `json:"isChurning,omitempty"`
	TmuxTarget         string          `json:"tmuxTarget,omitempty"`
	Lane               int             `json:"lane"`
	BurnRatePerMinute  float64         `json:"burnRatePerMinute,omitempty"`
	CompactionCount    int             `json:"compactionCount,omitempty"`
	Subagents          []SubagentState `json:"subagents,omitempty"`
	LastAssistantText  string          `json:"lastAssistantText,omitempty"`
	Position           int             `json:"position,omitempty"`      // 1-based rank among non-terminal sessions
	PositionDelta      int             `json:"positionDelta,omitempty"` // positive = moved up, negative = dropped
	LogPath            string          `json:"-"` // internal: path to JSONL file, excluded from wire protocol
}

// SubagentState tracks a single subagent (Task tool invocation) within a
// parent Claude Code session. Subagents share the parent's JSONL file and
// are identified by their stable toolUseID.
type SubagentState struct {
	ID              string     `json:"id"`              // toolUseID — stable across all progress entries
	ParentToolUseID string     `json:"parentToolUseId"` // links to parent's tool_use block
	SessionID       string     `json:"sessionId"`       // parent session ID
	Slug            string     `json:"slug"`            // human-friendly display name
	Model           string     `json:"model"`
	Activity        Activity   `json:"activity"`
	CurrentTool     string     `json:"currentTool,omitempty"`
	ContextTokens   int        `json:"tokensUsed"` // JSON tag kept for wire-protocol compatibility
	MessageCount    int        `json:"messageCount"`
	ToolCallCount   int        `json:"toolCallCount"`
	StartedAt       time.Time  `json:"startedAt"`
	LastActivityAt  time.Time  `json:"lastActivityAt"`
	CompletedAt     *time.Time `json:"completedAt,omitempty"`
}

// clone returns a deep copy of the SubagentState, duplicating pointer fields
// so the copy can be mutated independently of the original.
func (sa SubagentState) clone() SubagentState {
	if sa.CompletedAt != nil {
		t := *sa.CompletedAt
		sa.CompletedAt = &t
	}
	return sa
}

// Clone returns a deep copy of the SessionState, duplicating pointer and
// slice fields so the copy can be mutated independently of the original.
func (s *SessionState) Clone() *SessionState {
	c := *s
	if s.CompletedAt != nil {
		t := *s.CompletedAt
		c.CompletedAt = &t
	}
	if len(s.Subagents) > 0 {
		c.Subagents = make([]SubagentState, len(s.Subagents))
		for i, sa := range s.Subagents {
			c.Subagents[i] = sa.clone()
		}
	}
	return &c
}

func (s *SessionState) UpdateUtilization() {
	if s.MaxContextTokens > 0 {
		s.ContextUtilization = float64(s.ContextTokens) / float64(s.MaxContextTokens)
		if s.ContextUtilization > 1.0 {
			s.ContextUtilization = 1.0
		}
	}
}

func (s *SessionState) IsTerminal() bool {
	if s.Lifecycle == LifecycleTerminal {
		return true
	}
	return s.Activity == Complete || s.Activity == Errored || s.Activity == Lost
}
