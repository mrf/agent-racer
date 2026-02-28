package gamification

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/agent-racer/backend/internal/session"
)

const (
	saveInterval           = 30 * time.Second
	defaultEventBufferSize = 256
)

// XPEntry records a single XP award with a human-readable reason.
type XPEntry struct {
	Reason string `json:"reason"`
	Amount int    `json:"amount"`
}

// AchievementCallback is invoked for each newly unlocked achievement.
// It receives the achievement and the associated reward (if any).
type AchievementCallback func(achievement Achievement, reward *Reward)

// BattlePassCallback is invoked after XP is awarded, with the updated progress
// and the list of XP entries that triggered the update.
type BattlePassCallback func(progress BattlePassProgress, recentXP []XPEntry)

// StatsTracker observes session lifecycle events and maintains aggregate stats.
// It receives events from the monitor via a channel and periodically persists
// the accumulated stats to disk.
type StatsTracker struct {
	persist           *Store
	stats             *Stats
	events            chan session.Event
	flushCh           chan chan struct{}
	mu                sync.Mutex
	dirty             bool
	counted           map[string]bool  // session IDs already counted for TotalSessions
	contextMilestones map[string]uint8 // session ID -> bitmask: bit0=50%, bit1=90%
	lastTokens        map[string]int   // session ID -> last seen TokensUsed (for delta tracking)
	highUtilSessions  map[string]bool  // session IDs currently at or above 50% context utilization
	lastCompletionAt  time.Time        // tracks last completion time for photo_finish

	achieveEngine  *AchievementEngine
	rewardRegistry *RewardRegistry
	onAchievement  AchievementCallback
	onBattlePass   BattlePassCallback
}

// SeasonConfig controls which battle pass season is active.
type SeasonConfig struct {
	Enabled bool
	Season  string // e.g. "2025-07"
}

// NewStatsTracker creates a StatsTracker backed by the given persistence store.
// It loads existing stats from disk and returns a send-only channel for the
// monitor to deliver events on. bufferSize controls the channel capacity;
// values <= 0 use defaultEventBufferSize. If sc is non-nil and the configured
// season differs from the persisted season, a season rotation is performed.
// The caller must run Run in a goroutine.
func NewStatsTracker(persist *Store, bufferSize int, sc *SeasonConfig) (*StatsTracker, chan<- session.Event, error) {
	if bufferSize <= 0 {
		bufferSize = defaultEventBufferSize
	}
	stats, err := persist.Load()
	if err != nil {
		return nil, nil, err
	}

	if sc != nil && sc.Enabled {
		if rotateSeason(stats, sc.Season) {
			if err := persist.Save(stats); err != nil {
				return nil, nil, err
			}
		}
	}

	ch := make(chan session.Event, bufferSize)
	t := &StatsTracker{
		persist:           persist,
		stats:             stats,
		events:            ch,
		flushCh:           make(chan chan struct{}),
		counted:           make(map[string]bool),
		contextMilestones: make(map[string]uint8),
		lastTokens:        make(map[string]int),
		highUtilSessions:  make(map[string]bool),
		achieveEngine:     NewAchievementEngine(),
		rewardRegistry:    NewRewardRegistry(),
	}
	return t, ch, nil
}

// rotateSeason checks if the configured season differs from the persisted one.
// When it does, it archives the old season's XP/tier, resets the battle pass
// to tier 0/XP 0 with the new season label, and returns true.
// Achievements and equipped cosmetics are left intact (permanent).
func rotateSeason(stats *Stats, season string) bool {
	if stats.BattlePass.Season == season {
		return false
	}
	// Only archive if there was a previous season with progress.
	if stats.BattlePass.Season != "" && (stats.BattlePass.Tier > 0 || stats.BattlePass.XP > 0) {
		stats.ArchivedSeasons = append(stats.ArchivedSeasons, ArchivedSeason{
			Season:   stats.BattlePass.Season,
			Tier:     stats.BattlePass.Tier,
			XP:       stats.BattlePass.XP,
			Archived: time.Now().UTC().Format(time.RFC3339),
		})
	}
	stats.BattlePass = BattlePass{Season: season}
	return true
}

// OnAchievement registers a callback invoked whenever an achievement unlocks.
// Must be called before Run.
func (t *StatsTracker) OnAchievement(cb AchievementCallback) {
	t.onAchievement = cb
}

// OnBattlePassProgress registers a callback invoked whenever XP is awarded.
// Must be called before Run.
func (t *StatsTracker) OnBattlePassProgress(cb BattlePassCallback) {
	t.onBattlePass = cb
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
		case done := <-t.flushCh:
			t.drainEvents()
			close(done)
		case <-ticker.C:
			if t.dirty {
				t.save()
			}
		}
	}
}

// drainEvents processes all events currently buffered in the event channel.
func (t *StatsTracker) drainEvents() {
	for {
		select {
		case ev := <-t.events:
			t.processEvent(ev)
		default:
			return
		}
	}
}

// Flush blocks until all events currently queued in the event channel have
// been processed by the Run goroutine. It is used in tests to replace
// time.Sleep-based synchronization with a deterministic wait.
func (t *StatsTracker) Flush() {
	done := make(chan struct{})
	t.flushCh <- done
	<-done
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
	var xpEntries []XPEntry

	trackXP := func(reason string, amount int) {
		awardXP(&t.stats.BattlePass, amount)
		xpEntries = append(xpEntries, XPEntry{Reason: reason, Amount: amount})
	}

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
		trackXP("session_observed", XPSessionObserved)
		if t.stats.SessionsPerSource[s.Source] == 1 {
			trackXP("new_source", XPNewSource)
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
		// Track sessions simultaneously above 50% context utilization.
		t.highUtilSessions[s.ID] = s.ContextUtilization >= 0.5
		highUtilCount := 0
		for _, above := range t.highUtilSessions {
			if above {
				highUtilCount++
			}
		}
		if highUtilCount > t.stats.MaxHighUtilizationSimultaneous {
			t.stats.MaxHighUtilizationSimultaneous = highUtilCount
		}
		mask := t.contextMilestones[s.ID]
		if s.ContextUtilization >= 0.9 && mask&0x02 == 0 {
			trackXP("context_90pct", XPContext90Pct)
			t.contextMilestones[s.ID] = mask | 0x02
			wc.Snapshot.Context90PctCount++
		} else if s.ContextUtilization >= 0.5 && mask&0x01 == 0 {
			trackXP("context_50pct", XPContext50Pct)
			t.contextMilestones[s.ID] = mask | 0x01
		}

		// Weekly challenge: accumulate token delta (TokensUsed is cumulative).
		if s.TokensUsed > 0 {
			prev := t.lastTokens[s.ID]
			if delta := s.TokensUsed - prev; delta > 0 {
				wc.Snapshot.TokensBurned += delta
			}
			t.lastTokens[s.ID] = s.TokensUsed
		}

	case session.EventTerminal:
		switch s.Activity {
		case session.Complete:
			t.stats.TotalCompletions++
			t.stats.ConsecutiveCompletions++
			trackXP("session_complete", XPSessionCompletes)
			wc.Snapshot.TotalCompletions++

			now := time.Now()
			if !t.lastCompletionAt.IsZero() && now.Sub(t.lastCompletionAt) <= 10*time.Second {
				t.stats.PhotoFinishSeen = true
			}
			t.lastCompletionAt = now
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
				trackXP("new_model", XPNewModel)
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
		delete(t.lastTokens, s.ID)
		delete(t.highUtilSessions, s.ID)
	}

	// Award XP for newly completed weekly challenges.
	for _, cp := range EvaluateChallenges(wc) {
		if cp.Complete && !wc.XPAwarded[cp.ID] {
			wc.XPAwarded[cp.ID] = true
			awardXP(&t.stats.BattlePass, XPWeeklyChallenge)
		}
	}

	t.dirty = true

	// Capture battlepass progress while still under lock.
	var bpProgress BattlePassProgress
	if len(xpEntries) > 0 {
		bpProgress = getProgress(&t.stats.BattlePass)
	}

	// Evaluate achievements while still holding the lock so stats are consistent.
	unlocked := t.achieveEngine.Evaluate(t.stats)
	for _, a := range unlocked {
		awardXP(&t.stats.BattlePass, AchievementXP(a.Tier))
	}
	t.mu.Unlock()

	// Dispatch callbacks outside the lock to avoid holding it during broadcast.
	if len(xpEntries) > 0 && t.onBattlePass != nil {
		t.onBattlePass(bpProgress, xpEntries)
	}

	if t.onAchievement != nil {
		for _, a := range unlocked {
			var rw *Reward
			if found, ok := t.rewardRegistry.RewardForAchievement(a.ID); ok {
				rw = &found
			}
			t.onAchievement(a, rw)
		}
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
