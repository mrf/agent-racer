package session

import (
	"encoding/json"
	"time"
)

type Activity int

const (
	Starting Activity = iota
	Thinking
	ToolUse
	Waiting
	Idle
	Complete
	Errored
	Lost
)

var activityNames = map[Activity]string{
	Starting: "starting",
	Thinking: "thinking",
	ToolUse:  "tool_use",
	Waiting:  "waiting",
	Idle:     "idle",
	Complete: "complete",
	Errored:  "errored",
	Lost:     "lost",
}

var activityFromName = map[string]Activity{
	"starting": Starting,
	"thinking": Thinking,
	"tool_use": ToolUse,
	"waiting":  Waiting,
	"idle":     Idle,
	"complete": Complete,
	"errored":  Errored,
	"lost":     Lost,
}

func (a Activity) String() string {
	if s, ok := activityNames[a]; ok {
		return s
	}
	return "unknown"
}

func (a Activity) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

func (a *Activity) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if v, ok := activityFromName[s]; ok {
		*a = v
	}
	return nil
}

type SessionState struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	Source             string     `json:"source"`
	Activity           Activity   `json:"activity"`
	TokensUsed         int        `json:"tokensUsed"`
	TokenEstimated     bool       `json:"tokenEstimated"`
	MaxContextTokens   int        `json:"maxContextTokens"`
	ContextUtilization float64    `json:"contextUtilization"`
	CurrentTool        string     `json:"currentTool,omitempty"`
	Model              string     `json:"model"`
	WorkingDir         string     `json:"workingDir"`
	StartedAt          time.Time  `json:"startedAt"`
	LastActivityAt     time.Time  `json:"lastActivityAt"`
	LastDataReceivedAt time.Time  `json:"lastDataReceivedAt"`
	CompletedAt        *time.Time `json:"completedAt,omitempty"`
	MessageCount       int        `json:"messageCount"`
	ToolCallCount      int        `json:"toolCallCount"`
	PID                int        `json:"pid,omitempty"`
	IsChurning         bool       `json:"isChurning,omitempty"`
	TmuxTarget         string     `json:"tmuxTarget,omitempty"`
	Lane               int        `json:"lane"`
	BurnRatePerMinute  float64    `json:"burnRatePerMinute,omitempty"`
}

func (s *SessionState) UpdateUtilization() {
	if s.MaxContextTokens > 0 {
		s.ContextUtilization = float64(s.TokensUsed) / float64(s.MaxContextTokens)
		if s.ContextUtilization > 1.0 {
			s.ContextUtilization = 1.0
		}
	}
}

func (s *SessionState) IsTerminal() bool {
	return s.Activity == Complete || s.Activity == Errored || s.Activity == Lost
}
