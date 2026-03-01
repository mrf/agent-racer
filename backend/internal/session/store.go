package session

import (
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
	return result
}

func (s *Store) Update(state *SessionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateLocked(state)
}

// UpdateAndNotify atomically updates a session and calls notify before
// releasing the write lock. This ensures that GetAll (which takes a read
// lock) cannot observe the updated state until the notification — typically
// a broadcaster queue — has been issued.
func (s *Store) UpdateAndNotify(state *SessionState, notify func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateLocked(state)
	if notify != nil {
		notify()
	}
}

// BatchUpdateAndNotify atomically updates multiple sessions and calls notify
// before releasing the write lock.
func (s *Store) BatchUpdateAndNotify(states []*SessionState, notify func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, state := range states {
		s.updateLocked(state)
	}
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

// BatchRemoveAndNotify atomically removes multiple sessions and calls notify
// before releasing the write lock.
func (s *Store) BatchRemoveAndNotify(ids []string, notify func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		delete(s.sessions, id)
	}
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
