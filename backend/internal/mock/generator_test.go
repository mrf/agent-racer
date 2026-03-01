package mock

import (
	"context"
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

// drainEvents collects all events currently in ch without blocking.
func drainEvents(ch <-chan session.Event) []session.Event {
	var events []session.Event
	for {
		select {
		case ev := <-ch:
			events = append(events, ev)
		default:
			return events
		}
	}
}

func TestMockGenerator_EmitsEventNewOnStart(t *testing.T) {
	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, time.Hour, time.Hour, 0)
	gen := NewGenerator(store, broadcaster)

	// Buffer large enough to hold all EventNew events without blocking.
	ch := make(chan session.Event, 32)
	gen.SetStatsEvents(ch)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gen.Start(ctx)

	// Start() emits EventNew synchronously before launching the run goroutine,
	// so all events should be in the channel immediately.
	events := drainEvents(ch)

	wantCount := len(gen.sessions)
	newCount := 0
	for _, ev := range events {
		if ev.Type == session.EventNew {
			newCount++
		}
	}

	if newCount == 0 {
		t.Fatal("Start() emitted no EventNew events; gamification system would receive no XP or achievement triggers for mock sessions")
	}
	if newCount != wantCount {
		t.Errorf("Start() emitted %d EventNew events, want %d (one per mock session)", newCount, wantCount)
	}
}

func TestMockGenerator_EventNewHasValidState(t *testing.T) {
	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, time.Hour, time.Hour, 0)
	gen := NewGenerator(store, broadcaster)

	ch := make(chan session.Event, 32)
	gen.SetStatsEvents(ch)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gen.Start(ctx)

	events := drainEvents(ch)
	for _, ev := range events {
		if ev.Type != session.EventNew {
			continue
		}
		if ev.State == nil {
			t.Error("EventNew has nil State")
			continue
		}
		if ev.State.ID == "" {
			t.Errorf("EventNew State has empty ID")
		}
		if ev.State.Source == "" {
			t.Errorf("EventNew State %q has empty Source", ev.State.ID)
		}
	}
}

func TestMockGenerator_EmitsEventUpdateOnTick(t *testing.T) {
	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, time.Hour, time.Hour, 0)
	gen := NewGenerator(store, broadcaster)

	ch := make(chan session.Event, 256)
	gen.SetStatsEvents(ch)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gen.Start(ctx)

	// Drain the initial EventNew events.
	drainEvents(ch)

	// Wait for at least one ticker cycle (500ms) plus a margin.
	time.Sleep(700 * time.Millisecond)

	events := drainEvents(ch)
	updateCount := 0
	for _, ev := range events {
		if ev.Type == session.EventUpdate || ev.Type == session.EventTerminal {
			updateCount++
		}
	}

	if updateCount == 0 {
		t.Error("run() emitted no EventUpdate or EventTerminal events after one tick; gamification stats will not be updated for mock sessions")
	}
}

func TestMockGenerator_NoStatsEvents_DoesNotPanic(t *testing.T) {
	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, time.Hour, time.Hour, 0)
	gen := NewGenerator(store, broadcaster)
	// Intentionally do NOT call SetStatsEvents — statsEvents is nil.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Should not panic when no channel is configured.
	gen.Start(ctx)
	time.Sleep(600 * time.Millisecond)
}
