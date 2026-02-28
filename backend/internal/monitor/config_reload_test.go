package monitor

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

// pollCountSource counts Discover calls so tests can measure poll frequency.
type pollCountSource struct {
	count atomic.Int64
}

func (s *pollCountSource) Name() string { return "poll-counter" }

func (s *pollCountSource) Discover() ([]SessionHandle, error) {
	s.count.Add(1)
	return nil, nil
}

func (s *pollCountSource) Parse(_ SessionHandle, offset int64) (SourceUpdate, int64, error) {
	return SourceUpdate{}, offset, nil
}

// TestSetConfigRecreatesPollTicker verifies that calling SetConfig with a new
// PollInterval causes Start() to recreate its ticker so the new interval takes
// effect without restarting the server.
func TestSetConfigRecreatesPollTicker(t *testing.T) {
	src := &pollCountSource{}

	// Start with a very long poll interval (won't fire naturally in the test).
	cfg := defaultTestConfig()
	cfg.Monitor.PollInterval = 10 * time.Second

	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, 50*time.Millisecond, 10*time.Second, 0)
	m := NewMonitor(cfg, store, broadcaster, []Source{src})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go m.Start(ctx)

	// The initial poll fires immediately on Start(). Wait for it.
	deadline := time.Now().Add(500 * time.Millisecond)
	for src.count.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if src.count.Load() < 1 {
		t.Fatal("initial poll did not fire within 500ms")
	}

	// With a 10-second interval, no more polls should happen in the next 200ms.
	beforeReset := src.count.Load()
	time.Sleep(200 * time.Millisecond)
	if src.count.Load() != beforeReset {
		t.Errorf("unexpected poll during 10s interval window (before SetConfig)")
	}

	// SetConfig with a short poll interval — should trigger ticker recreation.
	shortCfg := defaultTestConfig()
	shortCfg.Monitor.PollInterval = 30 * time.Millisecond
	m.SetConfig(shortCfg)

	// With 30ms polls, expect at least 3 more polls within 500ms.
	// Conservative threshold to stay reliable under -race.
	afterReset := src.count.Load()
	deadline = time.Now().Add(500 * time.Millisecond)
	for src.count.Load() < afterReset+3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if src.count.Load() < afterReset+3 {
		t.Errorf("poll interval did not update after SetConfig: got %d polls in 500ms, want >= 3",
			src.count.Load()-afterReset)
	}
}

// TestSetConfigSignalsReconfigureCh verifies that SetConfig sends a signal on
// reconfigureCh so Start() can recreate its ticker immediately.
func TestSetConfigSignalsReconfigureCh(t *testing.T) {
	m, _, _ := newPollTestMonitor(&testSource{}, defaultTestConfig())

	// Channel should be empty before SetConfig.
	select {
	case <-m.reconfigureCh:
		t.Fatal("reconfigureCh should be empty before SetConfig")
	default:
	}

	newCfg := defaultTestConfig()
	newCfg.Monitor.PollInterval = 5 * time.Second
	m.SetConfig(newCfg)

	// Channel should have received a signal.
	select {
	case <-m.reconfigureCh:
		// OK
	default:
		t.Error("SetConfig did not signal reconfigureCh")
	}
}

// TestSetConfigMultipleCallsOneSignal verifies that multiple rapid SetConfig
// calls result in at most one pending signal (channel is buffered(1)).
func TestSetConfigMultipleCallsOneSignal(t *testing.T) {
	m, _, _ := newPollTestMonitor(&testSource{}, defaultTestConfig())

	for i := 0; i < 5; i++ {
		m.SetConfig(defaultTestConfig())
	}

	// Drain the channel — should find exactly one signal.
	count := 0
	for {
		select {
		case <-m.reconfigureCh:
			count++
		default:
			goto done
		}
	}
done:
	if count != 1 {
		t.Errorf("expected 1 pending signal after 5 SetConfig calls, got %d", count)
	}
}
