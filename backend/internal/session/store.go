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
	copy := *st
	return &copy, true
}

func (s *Store) GetAll() []*SessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*SessionState, 0, len(s.sessions))
	for _, st := range s.sessions {
		copy := *st
		result = append(result, &copy)
	}
	return result
}

func (s *Store) Update(state *SessionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.sessions[state.ID]; ok {
		state.Lane = existing.Lane
	} else {
		state.Lane = s.nextLane
		s.nextLane++
	}
	copy := *state
	s.sessions[state.ID] = &copy
}

func (s *Store) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
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
