// Package client provides WebSocket and HTTP clients for the Agent Racer backend.
// Types mirror the backend wire protocol without importing backend packages.
package client

import (
	"encoding/json"
	"time"
)

// MessageType identifies the kind of WebSocket message.
type MessageType string

const (
	MsgSnapshot            MessageType = "snapshot"
	MsgDelta               MessageType = "delta"
	MsgCompletion          MessageType = "completion"
	MsgEquipped            MessageType = "equipped"
	MsgError               MessageType = "error"
	MsgAchievementUnlocked MessageType = "achievement_unlocked"
	MsgSourceHealth        MessageType = "source_health"
	MsgBattlePassProgress  MessageType = "battlepass_progress"
)

// WSMessage is the envelope for all WebSocket messages.
type WSMessage struct {
	Type    MessageType     `json:"type"`
	Seq     uint64          `json:"seq"`
	Payload json.RawMessage `json:"payload"`
}

// Activity represents a session's current state.
type Activity string

const (
	ActivityStarting Activity = "starting"
	ActivityThinking Activity = "thinking"
	ActivityToolUse  Activity = "tool_use"
	ActivityWaiting  Activity = "waiting"
	ActivityIdle     Activity = "idle"
	ActivityComplete Activity = "complete"
	ActivityErrored  Activity = "errored"
	ActivityLost     Activity = "lost"
)

// SessionState mirrors backend/internal/session.SessionState.
type SessionState struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Slug               string          `json:"slug,omitempty"`
	Source             string          `json:"source"`
	Activity           Activity        `json:"activity"`
	TokensUsed         int             `json:"tokensUsed"`
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
}

// SubagentState mirrors backend/internal/session.SubagentState.
type SubagentState struct {
	ID              string     `json:"id"`
	ParentToolUseID string     `json:"parentToolUseId"`
	SessionID       string     `json:"sessionId"`
	Slug            string     `json:"slug"`
	Model           string     `json:"model"`
	Activity        Activity   `json:"activity"`
	CurrentTool     string     `json:"currentTool,omitempty"`
	TokensUsed      int        `json:"tokensUsed"`
	MessageCount    int        `json:"messageCount"`
	ToolCallCount   int        `json:"toolCallCount"`
	StartedAt       time.Time  `json:"startedAt"`
	LastActivityAt  time.Time  `json:"lastActivityAt"`
	CompletedAt     *time.Time `json:"completedAt,omitempty"`
}

// --- WebSocket payload types ---

// SnapshotPayload is sent on initial connection.
type SnapshotPayload struct {
	Sessions     []*SessionState       `json:"sessions"`
	SourceHealth []SourceHealthPayload `json:"sourceHealth,omitempty"`
}

// DeltaPayload contains incremental session updates.
type DeltaPayload struct {
	Updates []*SessionState `json:"updates"`
	Removed []string        `json:"removed,omitempty"`
}

// CompletionPayload is sent when a session reaches a terminal state.
type CompletionPayload struct {
	SessionID string   `json:"sessionId"`
	Activity  Activity `json:"activity"`
	Name      string   `json:"name"`
}

// EquippedPayload broadcasts the current cosmetic loadout.
type EquippedPayload struct {
	Loadout Equipped `json:"loadout"`
}

// BattlePassProgressPayload is sent when XP is awarded.
type BattlePassProgressPayload struct {
	XP           int       `json:"xp"`
	Tier         int       `json:"tier"`
	TierProgress float64   `json:"tierProgress"`
	RecentXP     []XPEntry `json:"recentXP"`
	Rewards      []string  `json:"rewards,omitempty"`
}

// AchievementRewardPayload describes a reward tied to an achievement.
type AchievementRewardPayload struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AchievementUnlockedPayload is sent when an achievement unlocks.
type AchievementUnlockedPayload struct {
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Tier        string                    `json:"tier"`
	Reward      *AchievementRewardPayload `json:"reward,omitempty"`
}

// SourceHealthStatus indicates a source's health.
type SourceHealthStatus string

const (
	StatusHealthy  SourceHealthStatus = "healthy"
	StatusDegraded SourceHealthStatus = "degraded"
	StatusFailed   SourceHealthStatus = "failed"
)

// SourceHealthPayload reports the health of a session source.
type SourceHealthPayload struct {
	Source           string             `json:"source"`
	Status           SourceHealthStatus `json:"status"`
	DiscoverFailures int                `json:"discoverFailures"`
	ParseFailures    int                `json:"parseFailures"`
	LastError        string             `json:"lastError,omitempty"`
	Timestamp        time.Time          `json:"timestamp"`
}

// --- HTTP response types ---

// XPEntry records a single XP award.
type XPEntry struct {
	Reason string `json:"reason"`
	Amount int    `json:"amount"`
}

// Equipped tracks the active cosmetic in each slot.
type Equipped struct {
	Paint string `json:"paint,omitempty"`
	Trail string `json:"trail,omitempty"`
	Body  string `json:"body,omitempty"`
	Badge string `json:"badge,omitempty"`
	Sound string `json:"sound,omitempty"`
	Theme string `json:"theme,omitempty"`
	Title string `json:"title,omitempty"`
}

// Stats mirrors the aggregate stats returned by /api/stats.
type Stats struct {
	Version                int              `json:"version"`
	TotalSessions          int              `json:"totalSessions"`
	TotalCompletions       int              `json:"totalCompletions"`
	TotalErrors            int              `json:"totalErrors"`
	ConsecutiveCompletions int              `json:"consecutiveCompletions"`
	SessionsPerSource      map[string]int   `json:"sessionsPerSource"`
	SessionsPerModel       map[string]int   `json:"sessionsPerModel"`
	DistinctModelsUsed     int              `json:"distinctModelsUsed"`
	DistinctSourcesUsed    int              `json:"distinctSourcesUsed"`
	MaxContextUtilization  float64          `json:"maxContextUtilization"`
	MaxBurnRate            float64          `json:"maxBurnRate"`
	MaxConcurrentActive    int              `json:"maxConcurrentActive"`
	MaxToolCalls           int              `json:"maxToolCalls"`
	MaxMessages            int              `json:"maxMessages"`
	MaxSessionDurationSec  float64          `json:"maxSessionDurationSec"`
	AchievementsUnlocked   map[string]string `json:"achievementsUnlocked"` // id -> RFC3339 time
	BattlePass             BattlePass       `json:"battlePass"`
	Equipped               Equipped         `json:"equipped"`
	LastUpdated            time.Time        `json:"lastUpdated"`
}

// BattlePass tracks seasonal progression.
type BattlePass struct {
	Season string `json:"season"`
	Tier   int    `json:"tier"`
	XP     int    `json:"xp"`
}

// AchievementResponse is the shape returned by /api/achievements.
type AchievementResponse struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Tier        string     `json:"tier"`
	Category    string     `json:"category"`
	Unlocked    bool       `json:"unlocked"`
	UnlockedAt  *time.Time `json:"unlockedAt,omitempty"`
}

// ChallengeProgress is returned by /api/challenges.
type ChallengeProgress struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Current     int    `json:"current"`
	Target      int    `json:"target"`
	Complete    bool   `json:"complete"`
}

// SoundConfig is returned by /api/config.
type SoundConfig struct {
	Enabled       bool    `json:"enabled"`
	MasterVolume  float64 `json:"master_volume"`
	AmbientVolume float64 `json:"ambient_volume"`
	SfxVolume     float64 `json:"sfx_volume"`
	EnableAmbient bool    `json:"enable_ambient"`
	EnableSfx     bool    `json:"enable_sfx"`
}
