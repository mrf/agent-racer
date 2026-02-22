package ws

import (
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
)

type WSMessage struct {
	Type    MessageType `json:"type"`
	Payload interface{} `json:"payload"`
}

type SnapshotPayload struct {
	Sessions []*session.SessionState `json:"sessions"`
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
