package ws

import (
	"time"

	"github.com/agent-racer/backend/internal/gamification"
	"github.com/agent-racer/backend/internal/session"
)

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

type WSMessage struct {
	Type    MessageType `json:"type"`
	Seq     uint64      `json:"seq"`
	Payload interface{} `json:"payload"`
}

type SourceHealthStatus string

const (
	StatusHealthy  SourceHealthStatus = "healthy"
	StatusDegraded SourceHealthStatus = "degraded"
	StatusFailed   SourceHealthStatus = "failed"
)

type SourceHealthPayload struct {
	Source           string             `json:"source"`
	Status           SourceHealthStatus `json:"status"`
	DiscoverFailures int                `json:"discoverFailures"`
	ParseFailures    int                `json:"parseFailures"`
	LastError        string             `json:"lastError,omitempty"`
	Timestamp        time.Time          `json:"timestamp"`
}

type SnapshotPayload struct {
	Sessions     []*session.SessionState `json:"sessions"`
	SourceHealth []SourceHealthPayload   `json:"sourceHealth,omitempty"`
}

type DeltaPayload struct {
	Updates []*session.SessionState `json:"updates"`
	Removed []string               `json:"removed,omitempty"`
}

type CompletionPayload struct {
	SessionID string           `json:"sessionId"`
	Activity  session.Activity `json:"activity"`
	Name      string           `json:"name"`
}

type EquippedPayload struct {
	Loadout gamification.Equipped `json:"loadout"`
}

type BattlePassProgressPayload struct {
	XP           int                    `json:"xp"`
	Tier         int                    `json:"tier"`
	TierProgress float64                `json:"tierProgress"`
	RecentXP     []gamification.XPEntry `json:"recentXP"`
	Rewards      []string               `json:"rewards,omitempty"`
}

type AchievementRewardPayload struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type AchievementUnlockedPayload struct {
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Tier        string                    `json:"tier"`
	Reward      *AchievementRewardPayload `json:"reward,omitempty"`
}
