package ws

import (
	"encoding/json"
	"time"

	"github.com/agent-racer/backend/internal/gamification"
	"github.com/agent-racer/backend/internal/session"
	agwmonitor "github.com/mrf/agentwatch/monitor"
)

// ─── Wire protocol envelope ─────────────────────────────────────────────────

type MessageType string

const (
	// Generic (agentwatch-compatible) message types.
	MsgSnapshot    MessageType = "snapshot"
	MsgDelta       MessageType = "delta"
	MsgSourceHealth MessageType = "source_health"

	// Racer-specific message types.
	MsgCompletion          MessageType = "completion"
	MsgEquipped            MessageType = "equipped"
	MsgError               MessageType = "error"
	MsgAchievementUnlocked MessageType = "achievement_unlocked"
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

// ─── Generic (agentwatch-compatible) message types ──────────────────────────
//
// These message types map to agentwatch monitor.Event semantics: snapshot
// delivers full state, delta delivers incremental updates and removals,
// source_health delivers per-source operational status.

func NewSnapshotMessage(payload SnapshotPayload) (WSMessage, error) {
	return newMessage(MsgSnapshot, payload)
}

func NewDeltaMessage(payload DeltaPayload) (WSMessage, error) {
	return newMessage(MsgDelta, payload)
}

func NewSourceHealthMessage(payload SourceHealthPayload) (WSMessage, error) {
	return newMessage(MsgSourceHealth, payload)
}

// SourceHealthStatus represents the operational state of a monitored source.
// Values are wire-compatible with agentwatch monitor.HealthStatus.
type SourceHealthStatus string

const (
	StatusHealthy  SourceHealthStatus = "healthy"
	StatusDegraded SourceHealthStatus = "degraded"
	StatusFailed   SourceHealthStatus = "failed"
)

// SourceHealthPayload carries per-source health information.
// Field layout matches agentwatch monitor.Health for interoperability.
type SourceHealthPayload struct {
	Source           string             `json:"source"`
	Status           SourceHealthStatus `json:"status"`
	DiscoverFailures int                `json:"discoverFailures"`
	ParseFailures    int                `json:"parseFailures"`
	LastError        string             `json:"lastError,omitempty"`
	Timestamp        time.Time          `json:"timestamp"`
}

// SourceHealthFromMonitor converts an agentwatch monitor.Health value to the
// ws package's SourceHealthPayload. This is the bridge between agentwatch's
// health reporting and the racer WebSocket protocol.
func SourceHealthFromMonitor(h agwmonitor.Health) SourceHealthPayload {
	return SourceHealthPayload{
		Source:           h.Source,
		Status:           SourceHealthStatus(h.Status),
		DiscoverFailures: h.DiscoverFailures,
		ParseFailures:    h.ParseFailures,
		LastError:        h.LastError,
		Timestamp:        h.UpdatedAt,
	}
}

// SnapshotPayload carries the full state of all tracked sessions.
type SnapshotPayload struct {
	Sessions      []*session.SessionState `json:"sessions"`
	Teams         []session.TeamInfo      `json:"teams,omitempty"`
	SourceHealth  []SourceHealthPayload   `json:"sourceHealth,omitempty"`
	ActiveTrackID string                  `json:"activeTrackId,omitempty"`
}

// DeltaPayload carries incremental session updates and removals.
// Semantics match agentwatch monitor.EventDelta: Updates contains full
// snapshots of changed sessions, Removed contains IDs of sessions that
// were evicted by the retention policy.
type DeltaPayload struct {
	Updates []*session.SessionState `json:"updates"`
	Removed []string                `json:"removed,omitempty"`
	Teams   []session.TeamInfo      `json:"teams,omitempty"`
}

// ─── Racer-specific message types ───────────────────────────────────────────
//
// These message types extend the generic agentwatch protocol with racing
// gamification: completion celebrations, cosmetic equip/unequip, achievements,
// battle pass progression, and position overtakes.

func NewCompletionMessage(payload CompletionPayload) (WSMessage, error) {
	return newMessage(MsgCompletion, payload)
}

func NewEquippedMessage(payload EquippedPayload) (WSMessage, error) {
	return newMessage(MsgEquipped, payload)
}

func NewAchievementUnlockedMessage(payload AchievementUnlockedPayload) (WSMessage, error) {
	return newMessage(MsgAchievementUnlocked, payload)
}

func NewBattlePassProgressMessage(payload BattlePassProgressPayload) (WSMessage, error) {
	return newMessage(MsgBattlePassProgress, payload)
}

func NewOvertakeMessage(payload OvertakePayload) (WSMessage, error) {
	return newMessage(MsgOvertake, payload)
}

// CompletionPayload announces that a session reached a terminal state.
// This is a racer-specific lifecycle notification; agentwatch uses
// monitor.EventLifecycle for the equivalent signal.
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
