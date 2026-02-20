package gamification

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/agent-racer/backend/internal/session"
)

const saveInterval = 30 * time.Second

// StatsTracker observes session lifecycle events and maintains aggregate stats.
// It receives events from the monitor via a channel and periodically persists
// the accumulated stats to disk.
type StatsTracker struct {
	persist *Store
	stats   *Stats
	events  chan session.Event
	mu      sync.Mutex
	dirty   bool
	counted map[string]bool // session IDs already counted for TotalSessions
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
		persist: persist,
		stats:   stats,
		events:  ch,
		counted: make(map[string]bool),
	}
	return t, ch, nil
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
	defer t.mu.Unlock()

	s := ev.State

	switch ev.Type {
	case session.EventNew:
		if t.counted[s.ID] {
			return
		}
		t.counted[s.ID] = true
		t.stats.TotalSessions++
		t.stats.SessionsPerSource[s.Source]++
		t.stats.DistinctSourcesUsed = len(t.stats.SessionsPerSource)
		if ev.ActiveCount > t.stats.MaxConcurrentActive {
			t.stats.MaxConcurrentActive = ev.ActiveCount
		}

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

	case session.EventTerminal:
		switch s.Activity {
		case session.Complete:
			t.stats.TotalCompletions++
			t.stats.ConsecutiveCompletions++
		case session.Errored:
			t.stats.TotalErrors++
			t.stats.ConsecutiveCompletions = 0
		case session.Lost:
			t.stats.ConsecutiveCompletions = 0
		}

		if s.Model != "" {
			t.stats.SessionsPerModel[s.Model]++
			t.stats.DistinctModelsUsed = len(t.stats.SessionsPerModel)
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
	}

	t.dirty = true
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
