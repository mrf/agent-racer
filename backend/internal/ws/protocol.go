package ws

import (
	"encoding/json"
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
	MsgOvertake            MessageType = "overtake"
)

type WSMessage struct {
	Type    MessageType     `json:"type"`
	Seq     uint64          `json:"seq"`
	Payload json.RawMessage `json:"payload"`
}

// newMessage is a generic helper that marshals payload into a WSMessage.
func newMessage[T any](msgType MessageType, payload T) (WSMessage, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return WSMessage{}, err
	}
	return WSMessage{Type: msgType, Payload: data}, nil
}

func NewSnapshotMessage(payload SnapshotPayload) (WSMessage, error) {
	return newMessage(MsgSnapshot, payload)
}

func NewDeltaMessage(payload DeltaPayload) (WSMessage, error) {
	return newMessage(MsgDelta, payload)
}

func NewCompletionMessage(payload CompletionPayload) (WSMessage, error) {
	return newMessage(MsgCompletion, payload)
}

func NewEquippedMessage(payload EquippedPayload) (WSMessage, error) {
	return newMessage(MsgEquipped, payload)
}

func NewAchievementUnlockedMessage(payload AchievementUnlockedPayload) (WSMessage, error) {
	return newMessage(MsgAchievementUnlocked, payload)
}

func NewSourceHealthMessage(payload SourceHealthPayload) (WSMessage, error) {
	return newMessage(MsgSourceHealth, payload)
}

func NewBattlePassProgressMessage(payload BattlePassProgressPayload) (WSMessage, error) {
	return newMessage(MsgBattlePassProgress, payload)
}

func NewOvertakeMessage(payload OvertakePayload) (WSMessage, error) {
	return newMessage(MsgOvertake, payload)
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
	Teams        []session.TeamInfo      `json:"teams,omitempty"`
	SourceHealth []SourceHealthPayload   `json:"sourceHealth,omitempty"`
}

type DeltaPayload struct {
	Updates []*session.SessionState `json:"updates"`
	Removed []string                `json:"removed,omitempty"`
	Teams   []session.TeamInfo      `json:"teams,omitempty"`
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

type OvertakePayload struct {
	OvertakerID   string `json:"overtakerId"`
	OvertakerName string `json:"overtakerName"`
	OvertakenID   string `json:"overtakenId"`
	OvertakenName string `json:"overtakenName"`
	NewPosition   int    `json:"newPosition"`
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
