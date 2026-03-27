package session

import (
	"sort"
	"sync"
)

type Store struct {
	mu       sync.RWMutex
	sessions map[string]*SessionState
	nextLane int
}

func NewStore() *Store {
	return &Store{
		sessions: make(map[string]*SessionState),
	}
}

func (s *Store) Get(id string) (*SessionState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.sessions[id]
	if !ok {
		return nil, false
	}
	return st.Clone(), true
}

func (s *Store) GetAll() []*SessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*SessionState, 0, len(s.sessions))
	for _, st := range s.sessions {
		result = append(result, st.Clone())
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

func (s *Store) Update(state *SessionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateLocked(state)
}

// UpdateAndNotify atomically updates a session and then calls notify after
// releasing the write lock. This prevents lock inversion: the notify callback
// typically queues a broadcast (acquiring broadcaster.flushMu), and the
// broadcaster's flush timer calls store.GetAll (acquiring store.mu.RLock).
// Running notify inside the write lock would create a store.mu → flushMu →
// store.mu cycle.
func (s *Store) UpdateAndNotify(state *SessionState, notify func()) {
	s.mu.Lock()
	s.updateLocked(state)
	s.mu.Unlock()
	if notify != nil {
		notify()
	}
}

// BatchUpdateAndNotify atomically updates multiple sessions and then calls
// notify after releasing the write lock. See UpdateAndNotify for rationale.
func (s *Store) BatchUpdateAndNotify(states []*SessionState, notify func()) {
	s.mu.Lock()
	for _, state := range states {
		s.updateLocked(state)
	}
	s.mu.Unlock()
	if notify != nil {
		notify()
	}
}

func (s *Store) updateLocked(state *SessionState) {
	if existing, ok := s.sessions[state.ID]; ok {
		state.Lane = existing.Lane
	} else {
		state.Lane = s.nextLane
		s.nextLane++
	}
	s.sessions[state.ID] = state.Clone()
}

func (s *Store) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

// BatchRemoveAndNotify atomically removes multiple sessions and then calls
// notify after releasing the write lock. See UpdateAndNotify for rationale.
func (s *Store) BatchRemoveAndNotify(ids []string, notify func()) {
	s.mu.Lock()
	for _, id := range ids {
		delete(s.sessions, id)
	}
	s.mu.Unlock()
	if notify != nil {
		notify()
	}
}

func (s *Store) ActiveCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, st := range s.sessions {
		if !st.IsTerminal() {
			count++
		}
	}
	return count
}
