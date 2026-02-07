package session

import (
	"fmt"
	"sync"
	"testing"
)

func TestNewStore(t *testing.T) {
	s := NewStore()
	if s == nil {
		t.Fatal("NewStore() returned nil")
	}
	if got := len(s.GetAll()); got != 0 {
		t.Errorf("new store has %d sessions, want 0", got)
	}
	if got := s.ActiveCount(); got != 0 {
		t.Errorf("new store ActiveCount() = %d, want 0", got)
	}
}

func TestGetMissing(t *testing.T) {
	s := NewStore()
	st, ok := s.Get("nonexistent")
	if ok {
		t.Error("Get for missing key returned ok=true")
	}
	if st != nil {
		t.Error("Get for missing key returned non-nil state")
	}
}

func TestUpdateAndGet(t *testing.T) {
	s := NewStore()
	s.Update(&SessionState{ID: "a", Name: "alpha", Activity: Thinking})

	st, ok := s.Get("a")
	if !ok {
		t.Fatal("Get returned ok=false after Update")
	}
	if st.ID != "a" || st.Name != "alpha" || st.Activity != Thinking {
		t.Errorf("Get returned unexpected state: %+v", st)
	}
}

func TestGetReturnsCopy(t *testing.T) {
	s := NewStore()
	s.Update(&SessionState{ID: "a", Name: "original"})

	got, _ := s.Get("a")
	got.Name = "mutated"

	got2, _ := s.Get("a")
	if got2.Name != "original" {
		t.Error("Get did not return a copy; mutation leaked into store")
	}
}

func TestUpdateStoresCopy(t *testing.T) {
	s := NewStore()
	state := &SessionState{ID: "a", Name: "original"}
	s.Update(state)

	state.Name = "mutated"

	got, _ := s.Get("a")
	if got.Name != "original" {
		t.Error("Update did not copy input; external mutation leaked into store")
	}
}

func TestLaneAssignment(t *testing.T) {
	s := NewStore()
	s.Update(&SessionState{ID: "a"})
	s.Update(&SessionState{ID: "b"})
	s.Update(&SessionState{ID: "c"})

	tests := []struct {
		id       string
		wantLane int
	}{
		{"a", 0},
		{"b", 1},
		{"c", 2},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got, _ := s.Get(tt.id)
			if got.Lane != tt.wantLane {
				t.Errorf("session %q lane = %d, want %d", tt.id, got.Lane, tt.wantLane)
			}
		})
	}
}

func TestLanePreservedOnUpdate(t *testing.T) {
	s := NewStore()
	s.Update(&SessionState{ID: "a", Name: "v1"})

	before, _ := s.Get("a")
	originalLane := before.Lane

	s.Update(&SessionState{ID: "a", Name: "v2", Lane: 99})

	after, _ := s.Get("a")
	if after.Lane != originalLane {
		t.Errorf("lane changed from %d to %d on re-update", originalLane, after.Lane)
	}
	if after.Name != "v2" {
		t.Errorf("Name not updated: got %q, want %q", after.Name, "v2")
	}
}

func TestGetAll(t *testing.T) {
	s := NewStore()
	s.Update(&SessionState{ID: "a"})
	s.Update(&SessionState{ID: "b"})

	all := s.GetAll()
	if len(all) != 2 {
		t.Fatalf("GetAll() returned %d items, want 2", len(all))
	}

	ids := map[string]bool{}
	for _, st := range all {
		ids[st.ID] = true
	}
	if !ids["a"] || !ids["b"] {
		t.Errorf("GetAll() missing expected IDs, got %v", ids)
	}
}

func TestGetAllReturnsCopies(t *testing.T) {
	s := NewStore()
	s.Update(&SessionState{ID: "a", Name: "original"})

	all := s.GetAll()
	all[0].Name = "mutated"

	got, _ := s.Get("a")
	if got.Name != "original" {
		t.Error("GetAll did not return copies; mutation leaked into store")
	}
}

func TestRemove(t *testing.T) {
	s := NewStore()
	s.Update(&SessionState{ID: "a"})
	s.Update(&SessionState{ID: "b"})

	s.Remove("a")

	if _, ok := s.Get("a"); ok {
		t.Error("Get returned ok=true after Remove")
	}
	if _, ok := s.Get("b"); !ok {
		t.Error("Remove of 'a' also removed 'b'")
	}
}

func TestRemoveNonexistent(t *testing.T) {
	s := NewStore()
	s.Remove("nonexistent") // should not panic
}

func TestActiveCount(t *testing.T) {
	s := NewStore()

	s.Update(&SessionState{ID: "active1", Activity: Thinking})
	s.Update(&SessionState{ID: "active2", Activity: ToolUse})
	s.Update(&SessionState{ID: "done", Activity: Complete})
	s.Update(&SessionState{ID: "err", Activity: Errored})
	s.Update(&SessionState{ID: "lost", Activity: Lost})

	if got := s.ActiveCount(); got != 2 {
		t.Errorf("ActiveCount() = %d, want 2", got)
	}
}

func TestActiveCountAfterRemove(t *testing.T) {
	s := NewStore()
	s.Update(&SessionState{ID: "a", Activity: Thinking})
	s.Update(&SessionState{ID: "b", Activity: Idle})

	if got := s.ActiveCount(); got != 2 {
		t.Errorf("before remove: ActiveCount() = %d, want 2", got)
	}

	s.Remove("a")
	if got := s.ActiveCount(); got != 1 {
		t.Errorf("after remove: ActiveCount() = %d, want 1", got)
	}
}

func TestActiveCountAfterTransition(t *testing.T) {
	s := NewStore()
	s.Update(&SessionState{ID: "a", Activity: Thinking})

	if got := s.ActiveCount(); got != 1 {
		t.Errorf("before transition: ActiveCount() = %d, want 1", got)
	}

	s.Update(&SessionState{ID: "a", Activity: Complete})
	if got := s.ActiveCount(); got != 0 {
		t.Errorf("after transition to Complete: ActiveCount() = %d, want 0", got)
	}
}

func TestActiveCountAllActivities(t *testing.T) {
	tests := []struct {
		activity Activity
		active   bool
	}{
		{Starting, true},
		{Thinking, true},
		{ToolUse, true},
		{Waiting, true},
		{Idle, true},
		{Complete, false},
		{Errored, false},
		{Lost, false},
	}

	s := NewStore()
	for i, tt := range tests {
		s.Update(&SessionState{ID: fmt.Sprintf("s%d", i), Activity: tt.activity})
	}

	if got, want := s.ActiveCount(), 5; got != want {
		t.Errorf("ActiveCount() = %d, want %d", got, want)
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := NewStore()
	var wg sync.WaitGroup
	const goroutines = 50

	for i := 0; i < goroutines; i++ {
		wg.Add(3)

		go func(id string) {
			defer wg.Done()
			s.Update(&SessionState{ID: id, Activity: Thinking})
			s.Update(&SessionState{ID: id, Activity: Complete})
		}(fmt.Sprintf("s%d", i))

		go func(id string) {
			defer wg.Done()
			s.Get(id)
			s.GetAll()
			s.ActiveCount()
		}(fmt.Sprintf("s%d", i))

		go func(id string) {
			defer wg.Done()
			s.Remove(id)
		}(fmt.Sprintf("s%d", i))
	}

	wg.Wait()
}
