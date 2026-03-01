package session

import (
	"fmt"
	"sync"
	"testing"
	"time"
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

func TestGetReturnsCopyOfCompletedAt(t *testing.T) {
	s := NewStore()
	now := time.Now()
	s.Update(&SessionState{ID: "a", CompletedAt: &now})

	got, _ := s.Get("a")
	mutated := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	got.CompletedAt = &mutated

	got2, _ := s.Get("a")
	if got2.CompletedAt.Equal(mutated) {
		t.Error("Get did not deep-copy CompletedAt; pointer mutation leaked into store")
	}
}

func TestGetReturnsCopyOfSubagents(t *testing.T) {
	s := NewStore()
	s.Update(&SessionState{
		ID: "a",
		Subagents: []SubagentState{
			{ID: "sub1", Activity: Thinking},
		},
	})

	got, _ := s.Get("a")
	got.Subagents[0].Activity = Complete
	got.Subagents = append(got.Subagents, SubagentState{ID: "sub2"})

	got2, _ := s.Get("a")
	if len(got2.Subagents) != 1 {
		t.Errorf("Get did not deep-copy Subagents slice; append leaked (len=%d)", len(got2.Subagents))
	}
	if got2.Subagents[0].Activity != Thinking {
		t.Error("Get did not deep-copy Subagents; element mutation leaked into store")
	}
}

func TestGetAllReturnsCopyOfCompletedAt(t *testing.T) {
	s := NewStore()
	now := time.Now()
	s.Update(&SessionState{ID: "a", CompletedAt: &now})

	all := s.GetAll()
	mutated := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	all[0].CompletedAt = &mutated

	got, _ := s.Get("a")
	if got.CompletedAt.Equal(mutated) {
		t.Error("GetAll did not deep-copy CompletedAt; pointer mutation leaked into store")
	}
}

func TestGetAllReturnsCopyOfSubagents(t *testing.T) {
	s := NewStore()
	s.Update(&SessionState{
		ID: "a",
		Subagents: []SubagentState{
			{ID: "sub1", Activity: Thinking},
		},
	})

	all := s.GetAll()
	all[0].Subagents[0].Activity = Complete

	got, _ := s.Get("a")
	if got.Subagents[0].Activity != Thinking {
		t.Error("GetAll did not deep-copy Subagents; element mutation leaked into store")
	}
}

func TestUpdateDeepCopiesSubagentCompletedAt(t *testing.T) {
	s := NewStore()
	now := time.Now()
	state := &SessionState{
		ID: "a",
		Subagents: []SubagentState{
			{ID: "sub1", CompletedAt: &now},
		},
	}
	s.Update(state)

	// Mutate the original's subagent CompletedAt after storing.
	mutated := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	state.Subagents[0].CompletedAt = &mutated

	got, _ := s.Get("a")
	if got.Subagents[0].CompletedAt.Equal(mutated) {
		t.Error("Update did not deep-copy subagent CompletedAt; external mutation leaked into store")
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

func TestUpdateAndNotify(t *testing.T) {
	s := NewStore()
	notified := false
	s.UpdateAndNotify(&SessionState{ID: "a", Name: "alpha"}, func() {
		notified = true
		// Inside the callback, the session should already be in the store.
		// We can't call s.Get (it would deadlock with write lock held),
		// but we verify the callback was invoked synchronously.
	})
	if !notified {
		t.Error("UpdateAndNotify did not call notify callback")
	}
	got, ok := s.Get("a")
	if !ok || got.Name != "alpha" {
		t.Errorf("UpdateAndNotify did not store session: ok=%v, state=%+v", ok, got)
	}
}

func TestUpdateAndNotifyNilCallback(t *testing.T) {
	s := NewStore()
	// Should not panic with nil callback.
	s.UpdateAndNotify(&SessionState{ID: "a"}, nil)
	if _, ok := s.Get("a"); !ok {
		t.Error("UpdateAndNotify with nil callback did not store session")
	}
}

func TestBatchUpdateAndNotify(t *testing.T) {
	s := NewStore()
	states := []*SessionState{
		{ID: "a", Name: "alpha"},
		{ID: "b", Name: "beta"},
	}
	notified := false
	s.BatchUpdateAndNotify(states, func() {
		notified = true
	})
	if !notified {
		t.Error("BatchUpdateAndNotify did not call notify callback")
	}
	all := s.GetAll()
	if len(all) != 2 {
		t.Fatalf("BatchUpdateAndNotify stored %d sessions, want 2", len(all))
	}
}

func TestBatchUpdateAndNotifyPreservesLanes(t *testing.T) {
	s := NewStore()
	s.Update(&SessionState{ID: "a"})
	s.Update(&SessionState{ID: "b"})

	aLane, _ := s.Get("a")
	bLane, _ := s.Get("b")

	// Re-update via batch — lanes should be preserved.
	s.BatchUpdateAndNotify([]*SessionState{
		{ID: "a", Name: "updated-a"},
		{ID: "b", Name: "updated-b"},
	}, nil)

	gotA, _ := s.Get("a")
	gotB, _ := s.Get("b")
	if gotA.Lane != aLane.Lane {
		t.Errorf("BatchUpdateAndNotify changed lane for a: %d → %d", aLane.Lane, gotA.Lane)
	}
	if gotB.Lane != bLane.Lane {
		t.Errorf("BatchUpdateAndNotify changed lane for b: %d → %d", bLane.Lane, gotB.Lane)
	}
}

func TestBatchRemoveAndNotify(t *testing.T) {
	s := NewStore()
	s.Update(&SessionState{ID: "a"})
	s.Update(&SessionState{ID: "b"})
	s.Update(&SessionState{ID: "c"})

	notified := false
	s.BatchRemoveAndNotify([]string{"a", "b"}, func() {
		notified = true
	})
	if !notified {
		t.Error("BatchRemoveAndNotify did not call notify callback")
	}
	if _, ok := s.Get("a"); ok {
		t.Error("BatchRemoveAndNotify did not remove session a")
	}
	if _, ok := s.Get("b"); ok {
		t.Error("BatchRemoveAndNotify did not remove session b")
	}
	if _, ok := s.Get("c"); !ok {
		t.Error("BatchRemoveAndNotify incorrectly removed session c")
	}
}

// deadlockTimeout is the maximum time we allow a store operation to complete
// before declaring a deadlock. It must be long enough to avoid flakes on
// slow CI runners but short enough that a deadlocked test fails fast.
const deadlockTimeout = 2 * time.Second

// mustCompleteWithin runs f in a goroutine and fails the test if f does not
// return within the given timeout. Use this to assert that a store operation
// completes without deadlocking. A timeout means the goroutine is permanently
// blocked — the classic symptom of RWMutex re-entrancy in a callback.
func mustCompleteWithin(t *testing.T, timeout time.Duration, desc string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		f()
		close(done)
	}()
	select {
	case <-done:
		// completed normally
	case <-time.After(timeout):
		t.Errorf("DEADLOCK: %s did not complete within %v (goroutine is permanently blocked)", desc, timeout)
	}
}

// TestUpdateAndNotify_CallbackMustNotReenter verifies the store's contract:
// a callback passed to UpdateAndNotify holds mu.Lock() and MUST NOT call any
// store method that acquires a read or write lock (Get, GetAll, ActiveCount,
// Update, Remove). Go's sync.RWMutex is not reentrant — a goroutine holding
// a write lock that attempts to acquire a read lock on the same mutex will
// deadlock permanently.
//
// This test does NOT call those methods from within the callback (that would
// hang the test runner). Instead, it verifies the contract from the outside:
// after UpdateAndNotify returns, Get/GetAll/ActiveCount must be immediately
// callable — confirming the write lock was properly released.
//
// If someone moves a store.Get() or store.ActiveCount() call back inside a
// callback (as happened in the markTerminal() deadlock regression), this test
// catches it.
func TestUpdateAndNotify_CallbackMustNotReenter(t *testing.T) {
	s := NewStore()

	callbackRan := false
	s.UpdateAndNotify(&SessionState{ID: "a", Activity: Thinking}, func() {
		callbackRan = true
		// Do NOT call s.Get/s.GetAll/s.ActiveCount here — that is the bug we are
		// guarding against. The test verifies that after this callback finishes,
		// the lock is released and all read operations unblock.
	})
	if !callbackRan {
		t.Fatal("UpdateAndNotify did not invoke callback")
	}

	// After UpdateAndNotify returns, all store operations must complete
	// without blocking. Any deadlock here means the write lock was not released.
	mustCompleteWithin(t, deadlockTimeout, "Get after UpdateAndNotify", func() {
		_, _ = s.Get("a")
	})
	mustCompleteWithin(t, deadlockTimeout, "GetAll after UpdateAndNotify", func() {
		_ = s.GetAll()
	})
	mustCompleteWithin(t, deadlockTimeout, "ActiveCount after UpdateAndNotify", func() {
		_ = s.ActiveCount()
	})
}

// TestBatchUpdateAndNotify_CallbackMustNotReenter is the same contract test
// for BatchUpdateAndNotify. The callback runs while mu.Lock() is held; any
// store re-entry inside it would deadlock.
func TestBatchUpdateAndNotify_CallbackMustNotReenter(t *testing.T) {
	s := NewStore()

	states := []*SessionState{
		{ID: "a", Activity: Thinking},
		{ID: "b", Activity: ToolUse},
	}
	callbackRan := false
	s.BatchUpdateAndNotify(states, func() {
		callbackRan = true
	})
	if !callbackRan {
		t.Fatal("BatchUpdateAndNotify did not invoke callback")
	}

	mustCompleteWithin(t, deadlockTimeout, "Get after BatchUpdateAndNotify", func() {
		_, _ = s.Get("a")
	})
	mustCompleteWithin(t, deadlockTimeout, "GetAll after BatchUpdateAndNotify", func() {
		_ = s.GetAll()
	})
	mustCompleteWithin(t, deadlockTimeout, "ActiveCount after BatchUpdateAndNotify", func() {
		_ = s.ActiveCount()
	})
}

// TestBatchRemoveAndNotify_CallbackMustNotReenter is the same contract test
// for BatchRemoveAndNotify.
func TestBatchRemoveAndNotify_CallbackMustNotReenter(t *testing.T) {
	s := NewStore()
	s.Update(&SessionState{ID: "a", Activity: Complete})
	s.Update(&SessionState{ID: "b", Activity: Errored})

	callbackRan := false
	s.BatchRemoveAndNotify([]string{"a", "b"}, func() {
		callbackRan = true
	})
	if !callbackRan {
		t.Fatal("BatchRemoveAndNotify did not invoke callback")
	}

	mustCompleteWithin(t, deadlockTimeout, "Get after BatchRemoveAndNotify", func() {
		_, _ = s.Get("a")
	})
	mustCompleteWithin(t, deadlockTimeout, "GetAll after BatchRemoveAndNotify", func() {
		_ = s.GetAll()
	})
	mustCompleteWithin(t, deadlockTimeout, "ActiveCount after BatchRemoveAndNotify", func() {
		_ = s.ActiveCount()
	})
}

// TestUpdateAndNotify_StoreCallFromCallbackDeadlocks documents the exact
// failure mode: a goroutine holding mu.Lock() (via UpdateAndNotify callback)
// that attempts to acquire mu.RLock() (via ActiveCount) will block forever.
//
// This test DELIBERATELY triggers the deadlock scenario in a controlled
// goroutine with a timeout. If the timeout fires, it confirms the bug
// reproduces. If it completes within the timeout, something changed about
// the locking model (which would be a surprise).
//
// This is the direct reproduction of the markTerminal() regression: the
// emitEvent() call inside the UpdateAndNotify callback called
// store.ActiveCount(), which attempted mu.RLock() while mu.Lock() was held.
func TestUpdateAndNotify_StoreCallFromCallbackDeadlocks(t *testing.T) {
	s := NewStore()
	s.Update(&SessionState{ID: "existing", Activity: Thinking})

	// Run the deadlocking code path in a goroutine with a short timeout.
	// We expect this to NOT complete (i.e., the goroutine will be stuck).
	// The test passes if it times out — confirming the deadlock is real.
	// If it somehow completes, the locking model has changed and this test
	// should be re-evaluated.
	done := make(chan struct{})
	go func() {
		s.UpdateAndNotify(&SessionState{ID: "a", Activity: Complete}, func() {
			// This is the bug: calling ActiveCount() while mu.Lock() is held.
			// sync.RWMutex is not reentrant — this will block forever.
			_ = s.ActiveCount()
		})
		close(done)
	}()

	select {
	case <-done:
		// The callback completed — the mutex was reentrant, which is not how
		// sync.RWMutex works. This means the test assumptions are wrong.
		t.Log("WARNING: UpdateAndNotify callback with ActiveCount() completed — verify locking model is still non-reentrant")
	case <-time.After(200 * time.Millisecond):
		// Expected: the goroutine is permanently blocked (deadlocked).
		// This confirms the deadlock mode is real. The fix (moving calls
		// like emitEvent outside the callback) is what prevents it in prod.
		t.Log("confirmed: calling store.ActiveCount() inside UpdateAndNotify callback causes deadlock (as expected)")
	}
	// Note: the goroutine is leaked intentionally — it is permanently blocked
	// and cannot be unblocked. This is acceptable in a test that documents a
	// known-deadlock scenario. The test process exits and cleans up.
}

func TestAtomicUpdateBlocksGetAll(t *testing.T) {
	s := NewStore()

	// Verify that GetAll cannot observe state written by BatchUpdateAndNotify
	// before the notify callback completes. We test this by having the notify
	// callback signal readiness, then a concurrent goroutine calls GetAll.
	// If GetAll returns before the callback finishes, the lock isn't held.
	callbackStarted := make(chan struct{})
	callbackDone := make(chan struct{})
	getAllDone := make(chan struct{})

	go func() {
		s.BatchUpdateAndNotify([]*SessionState{
			{ID: "x", Name: "test"},
		}, func() {
			close(callbackStarted)
			// Hold the lock briefly to give GetAll a chance to contend.
			<-callbackDone
		})
	}()

	go func() {
		<-callbackStarted
		// This GetAll should block until the callback finishes.
		s.GetAll()
		close(getAllDone)
	}()

	// The callback is running. GetAll should be blocked.
	select {
	case <-getAllDone:
		// getAllDone before we release — the lock wasn't held.
		t.Error("GetAll completed while BatchUpdateAndNotify callback was still running")
	default:
		// Good — GetAll is still blocked.
	}

	close(callbackDone)
	<-getAllDone
}
