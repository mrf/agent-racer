package gamification

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/agent-racer/backend/internal/session"
)

const saveInterval = 30 * time.Second

// AchievementCallback is invoked for each newly unlocked achievement.
// It receives the achievement and the associated reward (if any).
type AchievementCallback func(achievement Achievement, reward *Reward)

// StatsTracker observes session lifecycle events and maintains aggregate stats.
// It receives events from the monitor via a channel and periodically persists
// the accumulated stats to disk.
type StatsTracker struct {
	persist           *Store
	stats             *Stats
	events            chan session.Event
	mu                sync.Mutex
	dirty             bool
	counted           map[string]bool  // session IDs already counted for TotalSessions
	contextMilestones map[string]uint8 // session ID -> bitmask: bit0=50%, bit1=90%

	achieveEngine  *AchievementEngine
	rewardRegistry *RewardRegistry
	onAchievement  AchievementCallback
}

// NewStatsTracker creates a StatsTracker backed by the given persistence store.
// It loads existing stats from disk and returns a send-only channel for the
// monitor to deliver events on. The caller must run Run in a goroutine.
func NewStatsTracker(persist *Store) (*StatsTracker, chan<- session.Event, error) {
	stats, err := persist.Load()
	if err != nil {
		return nil, nil, err
	}
	ch := make(chan session.Event, 256)
	t := &StatsTracker{
		persist:           persist,
		stats:             stats,
		events:            ch,
		counted:           make(map[string]bool),
		contextMilestones: make(map[string]uint8),
		achieveEngine:     NewAchievementEngine(),
		rewardRegistry:    NewRewardRegistry(),
	}
	return t, ch, nil
}

// OnAchievement registers a callback invoked whenever an achievement unlocks.
// Must be called before Run.
func (t *StatsTracker) OnAchievement(cb AchievementCallback) {
	t.onAchievement = cb
}

// Run processes events and periodically saves dirty stats to disk.
// It blocks until ctx is cancelled, then performs a final save.
func (t *StatsTracker) Run(ctx context.Context) {
	ticker := time.NewTicker(saveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.save()
			return
		case ev := <-t.events:
			t.processEvent(ev)
		case <-ticker.C:
			if t.dirty {
				t.save()
			}
		}
	}
}

// Stats returns a deep copy of the current aggregate stats.
func (t *StatsTracker) Stats() *Stats {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stats.clone()
}

func (t *StatsTracker) processEvent(ev session.Event) {
	t.mu.Lock()

	s := ev.State

	// Ensure weekly challenges are rotated before processing.
	RotateChallengesIfNeeded(&t.stats.WeeklyChallenges, time.Now())

	wc := &t.stats.WeeklyChallenges

	switch ev.Type {
	case session.EventNew:
		if t.counted[s.ID] {
			t.mu.Unlock()
			return
		}
		t.counted[s.ID] = true
		t.stats.TotalSessions++
		t.stats.SessionsPerSource[s.Source]++
		t.stats.DistinctSourcesUsed = len(t.stats.SessionsPerSource)
		if ev.ActiveCount > t.stats.MaxConcurrentActive {
			t.stats.MaxConcurrentActive = ev.ActiveCount
		}
		awardXP(&t.stats.BattlePass, XPSessionObserved)
		if t.stats.SessionsPerSource[s.Source] == 1 {
			awardXP(&t.stats.BattlePass, XPNewSource)
		}

		// Weekly challenge: count new session and source.
		wc.Snapshot.TotalSessions++
		wc.Snapshot.SessionsPerSource[s.Source]++

	case session.EventUpdate:
		if s.ContextUtilization > t.stats.MaxContextUtilization {
			t.stats.MaxContextUtilization = s.ContextUtilization
		}
		if s.BurnRatePerMinute > t.stats.MaxBurnRate {
			t.stats.MaxBurnRate = s.BurnRatePerMinute
		}
		if ev.ActiveCount > t.stats.MaxConcurrentActive {
			t.stats.MaxConcurrentActive = ev.ActiveCount
		}
		mask := t.contextMilestones[s.ID]
		if s.ContextUtilization >= 0.9 && mask&0x02 == 0 {
			awardXP(&t.stats.BattlePass, XPContext90Pct)
			t.contextMilestones[s.ID] = mask | 0x02
			wc.Snapshot.Context90PctCount++
		} else if s.ContextUtilization >= 0.5 && mask&0x01 == 0 {
			awardXP(&t.stats.BattlePass, XPContext50Pct)
			t.contextMilestones[s.ID] = mask | 0x01
		}

		// Weekly challenge: accumulate tokens.
		if s.TokensUsed > 0 {
			wc.Snapshot.TokensBurned += s.TokensUsed
		}

	case session.EventTerminal:
		switch s.Activity {
		case session.Complete:
			t.stats.TotalCompletions++
			t.stats.ConsecutiveCompletions++
			awardXP(&t.stats.BattlePass, XPSessionCompletes)
			wc.Snapshot.TotalCompletions++
		case session.Errored:
			t.stats.TotalErrors++
			t.stats.ConsecutiveCompletions = 0
			wc.Snapshot.TotalErrors++
		case session.Lost:
			t.stats.ConsecutiveCompletions = 0
		}

		if s.Model != "" {
			t.stats.SessionsPerModel[s.Model]++
			t.stats.DistinctModelsUsed = len(t.stats.SessionsPerModel)
			if t.stats.SessionsPerModel[s.Model] == 1 {
				awardXP(&t.stats.BattlePass, XPNewModel)
			}
			wc.Snapshot.SessionsPerModel[s.Model]++
			wc.Snapshot.DistinctModels = len(wc.Snapshot.SessionsPerModel)
		}
		if s.ToolCallCount > t.stats.MaxToolCalls {
			t.stats.MaxToolCalls = s.ToolCallCount
		}
		if s.MessageCount > t.stats.MaxMessages {
			t.stats.MaxMessages = s.MessageCount
		}
		if s.CompletedAt != nil && !s.StartedAt.IsZero() {
			dur := s.CompletedAt.Sub(s.StartedAt).Seconds()
			if dur > t.stats.MaxSessionDurationSec {
				t.stats.MaxSessionDurationSec = dur
			}
		}

		delete(t.counted, s.ID)
		delete(t.contextMilestones, s.ID)
	}

	// Award XP for newly completed weekly challenges.
	for _, cp := range EvaluateChallenges(wc) {
		if cp.Complete && !wc.XPAwarded[cp.ID] {
			wc.XPAwarded[cp.ID] = true
			awardXP(&t.stats.BattlePass, XPWeeklyChallenge)
		}
	}

	t.dirty = true

	// Evaluate achievements while still holding the lock so stats are consistent.
	unlocked := t.achieveEngine.Evaluate(t.stats)
	t.mu.Unlock()

	// Dispatch callbacks outside the lock to avoid holding it during broadcast.
	for _, a := range unlocked {
		if t.onAchievement == nil {
			break
		}
		var rw *Reward
		if found, ok := t.rewardRegistry.RewardForAchievement(a.ID); ok {
			rw = &found
		}
		t.onAchievement(a, rw)
	}
}

// Challenges returns the current weekly challenge progress.
func (t *StatsTracker) Challenges() []ChallengeProgress {
	t.mu.Lock()
	defer t.mu.Unlock()
	RotateChallengesIfNeeded(&t.stats.WeeklyChallenges, time.Now())
	return EvaluateChallenges(&t.stats.WeeklyChallenges)
}

// Equip validates and equips rewardID using the given registry, persists
// the change immediately, and returns the updated loadout. It is safe for
// concurrent use.
func (t *StatsTracker) Equip(reg *RewardRegistry, rewardID string) (Equipped, error) {
	t.mu.Lock()
	if err := reg.Equip(rewardID, t.stats); err != nil {
		t.mu.Unlock()
		return Equipped{}, err
	}
	equipped := t.stats.Equipped
	stats := t.stats.clone()
	t.mu.Unlock()

	if err := t.persist.Save(stats); err != nil {
		log.Printf("Failed to save stats after equip: %v", err)
	}
	return equipped, nil
}

func (t *StatsTracker) save() {
	t.mu.Lock()
	stats := t.stats.clone()
	t.dirty = false
	t.mu.Unlock()

	if err := t.persist.Save(stats); err != nil {
		log.Printf("Failed to save stats: %v", err)
	}
}
