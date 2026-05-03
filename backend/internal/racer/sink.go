package racer

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mrf/agentwatch/monitor"
	"github.com/mrf/agentwatch/session"
)

// StatsEvent mirrors the agent-racer session.Event structure so the Sink
// can feed the existing gamification StatsTracker without importing its
// internal types directly. Consumers map this to their own event type.
type StatsEvent struct {
	Type        StatsEventType
	State       *RacerState
	ActiveCount int
}

// StatsEventType classifies lifecycle events emitted to the stats channel.
type StatsEventType int

const (
	StatsEventNew      StatsEventType = iota // session first discovered
	StatsEventUpdate                         // per-poll state update
	StatsEventTerminal                       // session reached terminal state
)

// OvertakeCallback is invoked when one session overtakes another.
type OvertakeCallback func(ev OvertakeEvent)

// Sink implements agentwatch monitor.EventSink and layers racing-specific
// logic on top of the generic session monitoring: lane assignment, position
// computation, overtake detection, and stats fan-out.
type Sink struct {
	mu       sync.Mutex
	sessions map[string]*RacerState // keyed by session ID
	nextLane int

	statsEvents   chan<- StatsEvent // nil disables stats emission
	onOvertake    OvertakeCallback // nil disables overtake callbacks
	statsDropped  int64
	statsLastDrop time.Time
}

// SinkOption configures a Sink.
type SinkOption func(*Sink)

// WithStatsChannel configures a channel for session lifecycle events.
// The Sink uses non-blocking sends; slow consumers cause drops.
func WithStatsChannel(ch chan<- StatsEvent) SinkOption {
	return func(s *Sink) { s.statsEvents = ch }
}

// WithOvertakeCallback registers a function called on every overtake event.
func WithOvertakeCallback(cb OvertakeCallback) SinkOption {
	return func(s *Sink) { s.onOvertake = cb }
}

// NewSink creates a Sink implementing monitor.EventSink.
func NewSink(opts ...SinkOption) *Sink {
	s := &Sink{
		sessions: make(map[string]*RacerState),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// HandleEvent implements monitor.EventSink. It processes agentwatch events
// and maintains the racing overlay state.
func (s *Sink) HandleEvent(_ context.Context, ev monitor.Event) error {
	switch ev.Type {
	case monitor.EventSnapshot:
		s.handleSnapshot(ev)
	case monitor.EventDelta:
		s.handleDelta(ev)
	case monitor.EventLifecycle:
		s.handleLifecycle(ev)
	case monitor.EventHealth:
		// Health events don't affect racing state; ignore for now.
	}
	return nil
}

// GetAll returns a snapshot of all current racer states.
func (s *Sink) GetAll() []*RacerState {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*RacerState, 0, len(s.sessions))
	for _, rs := range s.sessions {
		cp := *rs
		result = append(result, &cp)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// Get returns the racer state for a single session, or nil if not found.
func (s *Sink) Get(id string) *RacerState {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs, ok := s.sessions[id]
	if !ok {
		return nil
	}
	cp := *rs
	return &cp
}

// handleSnapshot replaces all tracked sessions with the snapshot contents,
// preserving lane assignments for sessions that already exist.
func (s *Sink) handleSnapshot(ev monitor.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Preserve existing lane assignments.
	oldLanes := make(map[string]int, len(s.sessions))
	for id, rs := range s.sessions {
		oldLanes[id] = rs.Lane
	}

	newSessions := make(map[string]*RacerState, len(ev.Sessions))
	for i := 0; i < len(ev.Sessions); i++ {
		ss := &ev.Sessions[i]
		rs := &RacerState{SessionState: ss.Clone()}
		rs.Name = nameFromSession(ss)

		if lane, ok := oldLanes[ss.ID]; ok {
			rs.Lane = lane
		} else {
			rs.Lane = s.nextLane
			s.nextLane++
		}
		newSessions[ss.ID] = rs
	}
	s.sessions = newSessions

	s.computePositionsLocked()
}

// handleDelta processes incremental session updates: new sessions get lanes,
// existing sessions update, removed sessions are deleted.
func (s *Sink) handleDelta(ev monitor.Event) {
	s.mu.Lock()

	// Capture previous positions for overtake detection.
	prevPos := make(map[string]int, len(s.sessions))
	prevNames := make(map[string]string, len(s.sessions))
	for id, rs := range s.sessions {
		prevPos[id] = rs.Position
		prevNames[id] = rs.Name
	}

	// Apply updates.
	for i := 0; i < len(ev.Updates); i++ {
		ss := &ev.Updates[i]
		existing, ok := s.sessions[ss.ID]
		if ok {
			// Preserve lane, update session state.
			lane := existing.Lane
			existing.SessionState = ss.Clone()
			existing.Name = nameFromSession(ss)
			existing.Lane = lane

			s.emitStatsTypeLocked(existing, StatsEventUpdate)
		} else {
			// New session: assign lane.
			rs := &RacerState{SessionState: ss.Clone()}
			rs.Name = nameFromSession(ss)
			rs.Lane = s.nextLane
			s.nextLane++
			s.sessions[ss.ID] = rs

			s.emitStatsTypeLocked(rs, StatsEventNew)
		}
	}

	// Remove sessions.
	for i := 0; i < len(ev.Removed); i++ {
		delete(s.sessions, ev.Removed[i])
	}

	s.computePositionsLocked()

	// Collect overtake events while still holding the lock.
	var overtakes []OvertakeEvent
	if s.onOvertake != nil {
		overtakes = s.detectOvertakesLocked(prevPos, prevNames)
	}

	s.mu.Unlock()

	// Dispatch overtake callbacks outside the lock.
	for i := 0; i < len(overtakes); i++ {
		s.onOvertake(overtakes[i])
	}
}

// handleLifecycle processes lifecycle events (terminal transitions, etc.).
func (s *Sink) handleLifecycle(ev monitor.Event) {
	if ev.Lifecycle == nil {
		return
	}
	lc := ev.Lifecycle

	s.mu.Lock()
	defer s.mu.Unlock()

	rs, ok := s.sessions[lc.SessionID]
	if !ok {
		return
	}

	if lc.Type == session.EventTerminal {
		rs.Activity = session.ActivityTerminal
		if rs.CompletedAt == nil {
			t := lc.At
			rs.CompletedAt = &t
		}
		s.emitStatsTypeLocked(rs, StatsEventTerminal)
	}

	if lc.Type == session.EventRemoved {
		delete(s.sessions, lc.SessionID)
	}

	s.computePositionsLocked()
}

// computePositionsLocked sorts non-terminal sessions by context utilization
// descending and assigns 1-based positions. Caller must hold s.mu.
func (s *Sink) computePositionsLocked() {
	type entry struct {
		id   string
		util float64
	}

	racing := make([]entry, 0, len(s.sessions))
	for id, rs := range s.sessions {
		if !rs.IsTerminal() {
			racing = append(racing, entry{id, rs.ContextUtilization})
		}
	}

	sort.Slice(racing, func(i, j int) bool {
		return racing[i].util > racing[j].util
	})

	// Build position map.
	posMap := make(map[string]int, len(racing))
	for i := 0; i < len(racing); i++ {
		posMap[racing[i].id] = i + 1
	}

	// Apply positions to all sessions.
	for id, rs := range s.sessions {
		if pos, ok := posMap[id]; ok {
			oldPos := rs.Position
			rs.Position = pos
			if oldPos > 0 {
				rs.PositionDelta = oldPos - pos // positive = moved up
			} else {
				rs.PositionDelta = 0
			}
		} else {
			// Terminal session — clear racing position.
			rs.Position = 0
			rs.PositionDelta = 0
		}
	}
}

// detectOvertakesLocked compares current positions against previous ones
// and returns overtake events. Caller must hold s.mu.
func (s *Sink) detectOvertakesLocked(prevPos map[string]int, prevNames map[string]string) []OvertakeEvent {
	// Build reverse map: previous position -> ID for non-terminal sessions.
	prevOrder := make(map[int]string, len(prevPos))
	for id, pos := range prevPos {
		if pos > 0 {
			prevOrder[pos] = id
		}
	}

	var overtakes []OvertakeEvent
	for id, rs := range s.sessions {
		if rs.Position == 0 || rs.PositionDelta <= 0 {
			continue
		}
		// This session moved up. Check who was previously at its new position.
		overtakenID, ok := prevOrder[rs.Position]
		if !ok || overtakenID == id {
			continue
		}
		overtakenName := prevNames[overtakenID]
		if overtakenName == "" {
			overtakenName = overtakenID
		}
		overtakes = append(overtakes, OvertakeEvent{
			OvertakerID:   id,
			OvertakerName: rs.Name,
			OvertakenID:   overtakenID,
			OvertakenName: overtakenName,
			NewPosition:   rs.Position,
			At:            time.Now(),
		})
	}
	return overtakes
}

// emitStatsTypeLocked sends a stats event of the given type.
// Caller must hold s.mu.
func (s *Sink) emitStatsTypeLocked(rs *RacerState, evType StatsEventType) {
	if s.statsEvents == nil {
		return
	}

	activeCount := 0
	for _, r := range s.sessions {
		if !r.IsTerminal() {
			activeCount++
		}
	}

	cp := *rs
	s.emitStats(StatsEvent{
		Type:        evType,
		State:       &cp,
		ActiveCount: activeCount,
	})
}

// emitStats performs a non-blocking send on the stats channel.
func (s *Sink) emitStats(ev StatsEvent) {
	select {
	case s.statsEvents <- ev:
	default:
		s.statsDropped++
		now := time.Now()
		if s.statsLastDrop.IsZero() || now.Sub(s.statsLastDrop) >= 10*time.Second {
			slog.Warn("racer sink: stats events dropped", "count", s.statsDropped)
			s.statsDropped = 0
			s.statsLastDrop = now
		}
	}
}

// nameFromSession derives a display name from a session state.
// If the working dir is inside a .claude/worktrees/<slug>/ directory,
// the slug is used. Otherwise the last path component is used.
func nameFromSession(ss *session.SessionState) string {
	if ss.Slug != "" {
		return ss.Slug
	}
	return nameFromPath(ss.WorkingDir)
}

// nameFromPath extracts a display name from a filesystem path.
func nameFromPath(path string) string {
	// Split into components, filtering empty strings from leading/trailing slashes.
	parts := strings.Split(path, "/")
	filtered := parts[:0]
	for i := 0; i < len(parts); i++ {
		if parts[i] != "" {
			filtered = append(filtered, parts[i])
		}
	}

	// Check for .claude/worktrees/<slug> pattern.
	for i := 0; i < len(filtered)-2; i++ {
		if filtered[i] == ".claude" && filtered[i+1] == "worktrees" {
			return filtered[i+2]
		}
	}
	if len(filtered) > 0 {
		return filtered[len(filtered)-1]
	}
	return "unknown"
}
