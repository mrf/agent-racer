package session

// EventType classifies session lifecycle events.
type EventType int

const (
	EventNew      EventType = iota // session first discovered
	EventUpdate                    // per-poll state update (new data arrived)
	EventTerminal                  // session reached terminal state
)

// Event carries a session state snapshot to observers.
type Event struct {
	Type        EventType
	State       *SessionState // snapshot (safe to retain)
	ActiveCount int           // non-terminal sessions at event time
}
