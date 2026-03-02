package mock

import (
	"context"
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

// newTestGen creates a minimal MockGenerator suitable for unit tests.
func newTestGen() *MockGenerator {
	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, time.Hour, time.Hour, 0)
	return NewGenerator(store, broadcaster)
}

// newTestMS builds a mockSession with the given pattern, tokensPerTick and maxTokens.
// tools is set to three entries so ToolUse paths always have a valid tool to cycle through.
func newTestMS(pattern string, tokensPerTick, maxTokens int) *mockSession {
	return &mockSession{
		state: &session.SessionState{
			ID:               "test-" + pattern,
			MaxContextTokens: 200000,
			Activity:         session.Starting,
		},
		tokensPerTick: tokensPerTick,
		pattern:       pattern,
		maxTokens:     maxTokens,
		tools:         []string{"Read", "Write", "Bash"},
	}
}

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
	gen := newTestGen()

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
	gen := newTestGen()

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
	gen := newTestGen()

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
	gen := newTestGen()
	// Intentionally do NOT call SetStatsEvents -- statsEvents is nil.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Should not panic when no channel is configured.
	gen.Start(ctx)
	time.Sleep(600 * time.Millisecond)
}

// ---------------------------------------------------------------------------
// advanceMock: early ticks and burn-rate calculation
// ---------------------------------------------------------------------------

func TestAdvanceMock_EarlyTicksAreStarting(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("steady", 1000, 100000)

	for _, tick := range []int{1, 2} {
		gen.advanceMock(ms, tick)
		if ms.state.Activity != session.Starting {
			t.Errorf("tick %d: Activity = %v, want Starting", tick, ms.state.Activity)
		}
	}
	if ms.state.TokensUsed != 1000 {
		t.Errorf("after 2 early ticks: TokensUsed = %d, want 1000 (500 per tick)", ms.state.TokensUsed)
	}
}

func TestAdvanceMock_BurnRatePositiveWhenTokensGrow(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("steady", 1000, 100000)

	// Advance past early ticks.
	gen.advanceMock(ms, 1)
	gen.advanceMock(ms, 2)
	gen.advanceMock(ms, 3)

	if ms.state.BurnRatePerMinute <= 0 {
		t.Errorf("BurnRatePerMinute = %f after token growth, want > 0", ms.state.BurnRatePerMinute)
	}
}

func TestAdvanceMock_BurnRateZeroDuringStall(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("stall", 800, 120000)

	// Advance past early ticks into the stall phase (tick%70 >= 40).
	gen.advanceMock(ms, 1)
	gen.advanceMock(ms, 2)
	// tick 40: phase = 40%70 = 40, which is the stall start.
	gen.advanceMock(ms, 40)

	if ms.state.BurnRatePerMinute != 0 {
		t.Errorf("BurnRatePerMinute = %f during stall, want 0", ms.state.BurnRatePerMinute)
	}
}

// ---------------------------------------------------------------------------
// advanceSteady
// ---------------------------------------------------------------------------

func TestAdvanceSteady_TokensGrow(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("steady", 1000, 100000)

	before := ms.state.TokensUsed
	gen.advanceSteady(ms, 5)
	after := ms.state.TokensUsed

	delta := after - before
	// tokensPerTick=1000, jitter in [-200,+200), so delta in [800, 1200).
	if delta < 800 || delta >= 1200 {
		t.Errorf("token delta = %d, want in [800, 1200)", delta)
	}
}

func TestAdvanceSteady_ToolUseEveryThirdTick(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("steady", 1000, 100000)

	gen.advanceSteady(ms, 3)
	if ms.state.Activity != session.ToolUse {
		t.Errorf("tick 3: Activity = %v, want ToolUse", ms.state.Activity)
	}
	if ms.state.CurrentTool == "" {
		t.Error("tick 3: CurrentTool is empty during ToolUse")
	}
	if ms.state.ToolCallCount != 1 {
		t.Errorf("tick 3: ToolCallCount = %d, want 1", ms.state.ToolCallCount)
	}

	gen.advanceSteady(ms, 4)
	if ms.state.Activity != session.Thinking {
		t.Errorf("tick 4: Activity = %v, want Thinking", ms.state.Activity)
	}
	if ms.state.CurrentTool != "" {
		t.Errorf("tick 4: CurrentTool = %q, want empty", ms.state.CurrentTool)
	}
}

func TestAdvanceSteady_CompletesAtMaxTokens(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("steady", 1000, 2000)

	// Push tokens close to max so next advance completes.
	ms.state.TokensUsed = 1900

	gen.advanceSteady(ms, 5)

	if !ms.completed {
		t.Fatal("session not completed when TokensUsed >= maxTokens")
	}
	if ms.state.Activity != session.Complete {
		t.Errorf("Activity = %v, want Complete", ms.state.Activity)
	}
	if ms.state.TokensUsed != ms.maxTokens {
		t.Errorf("TokensUsed = %d, want capped at %d", ms.state.TokensUsed, ms.maxTokens)
	}
	if ms.state.CompletedAt == nil {
		t.Error("CompletedAt is nil after completion")
	}
}

func TestAdvanceSteady_ToolsCycleThrough(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("steady", 100, 100000)

	var tools []string
	// Ticks divisible by 3 trigger ToolUse.
	for _, tick := range []int{3, 6, 9, 12} {
		gen.advanceSteady(ms, tick)
		tools = append(tools, ms.state.CurrentTool)
	}

	// tools list is ["Read","Write","Bash"] so cycle is Read, Write, Bash, Read.
	want := []string{"Read", "Write", "Bash", "Read"}
	for i, got := range tools {
		if got != want[i] {
			t.Errorf("tool cycle[%d] = %q, want %q", i, got, want[i])
		}
	}
}

func TestAdvanceSteady_MessageCountIncrements(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("steady", 100, 100000)

	gen.advanceSteady(ms, 3)
	gen.advanceSteady(ms, 4)
	gen.advanceSteady(ms, 5)

	if ms.state.MessageCount != 3 {
		t.Errorf("MessageCount = %d, want 3", ms.state.MessageCount)
	}
}

// ---------------------------------------------------------------------------
// advanceBurst
// ---------------------------------------------------------------------------

func TestAdvanceBurst_BurstPhasesHaveHigherGrowth(t *testing.T) {
	gen := newTestGen()

	// Burst phase: tick%8 < 3 → multiplier 2.5x.
	msBurst := newTestMS("burst", 1000, 200000)
	gen.advanceBurst(msBurst, 0) // 0%8=0, burst
	burstDelta := msBurst.state.TokensUsed

	// Normal phase: tick%8 >= 3.
	msNormal := newTestMS("burst", 1000, 200000)
	gen.advanceBurst(msNormal, 3) // 3%8=3, normal
	normalDelta := msNormal.state.TokensUsed

	// Burst growth base is 2500 (+jitter 0-499), normal is 1000 (+jitter 0-499).
	// Burst minimum (2500) should exceed normal maximum (1499).
	if burstDelta < normalDelta {
		t.Errorf("burst delta (%d) < normal delta (%d); burst multiplier may not be applied", burstDelta, normalDelta)
	}
}

func TestAdvanceBurst_ToolUseDuringBurst(t *testing.T) {
	gen := newTestGen()

	msBurst := newTestMS("burst", 1000, 200000)
	gen.advanceBurst(msBurst, 0) // 0%8=0, burst phase
	if msBurst.state.Activity != session.ToolUse {
		t.Errorf("burst tick: Activity = %v, want ToolUse", msBurst.state.Activity)
	}

	msNormal := newTestMS("burst", 1000, 200000)
	gen.advanceBurst(msNormal, 5) // 5%8=5, normal phase
	if msNormal.state.Activity != session.Thinking {
		t.Errorf("normal tick: Activity = %v, want Thinking", msNormal.state.Activity)
	}
}

func TestAdvanceBurst_CompletesAtMaxTokens(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("burst", 1000, 2000)
	ms.state.TokensUsed = 1999

	gen.advanceBurst(ms, 5)

	if !ms.completed {
		t.Fatal("burst session not completed at maxTokens")
	}
	if ms.state.TokensUsed != ms.maxTokens {
		t.Errorf("TokensUsed = %d, want %d", ms.state.TokensUsed, ms.maxTokens)
	}
}

// ---------------------------------------------------------------------------
// advanceStall
// ---------------------------------------------------------------------------

func TestAdvanceStall_WorkPhaseGrowsTokens(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("stall", 800, 120000)

	// tick 5 → phase=5%70=5, in work phase (< 40).
	before := ms.state.TokensUsed
	gen.advanceStall(ms, 5)
	if ms.state.TokensUsed <= before {
		t.Error("tokens did not grow during work phase")
	}
	if ms.state.Activity == session.Waiting {
		t.Error("should not be Waiting during work phase")
	}
}

func TestAdvanceStall_StallPhaseIsWaiting(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("stall", 800, 120000)

	// Ticks where phase = tick%70 falls in [40, 70).
	stallTicks := []int{40, 50, 60, 69}
	for _, tick := range stallTicks {
		before := ms.state.TokensUsed
		gen.advanceStall(ms, tick)
		if ms.state.Activity != session.Waiting {
			t.Errorf("tick %d (phase %d): Activity = %v, want Waiting", tick, tick%70, ms.state.Activity)
		}
		if ms.state.TokensUsed != before {
			t.Errorf("tick %d: tokens changed during stall (%d → %d)", tick, before, ms.state.TokensUsed)
		}
	}
}

func TestAdvanceStall_CyclePeriodIs70(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("stall", 800, 120000)

	// tick 70 → phase=0, should be working again (not stalling).
	gen.advanceStall(ms, 70)
	if ms.state.Activity == session.Waiting {
		t.Error("tick 70 (phase 0): should resume work after stall cycle resets")
	}

	// tick 110 → phase=40, should be stalling.
	gen.advanceStall(ms, 110)
	if ms.state.Activity != session.Waiting {
		t.Errorf("tick 110 (phase 40): Activity = %v, want Waiting", ms.state.Activity)
	}
}

func TestAdvanceStall_CompletesAtMaxTokens(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("stall", 800, 2000)
	ms.state.TokensUsed = 1999

	gen.advanceStall(ms, 5) // work phase

	if !ms.completed {
		t.Fatal("stall session not completed at maxTokens")
	}
}

// ---------------------------------------------------------------------------
// advanceError
// ---------------------------------------------------------------------------

func TestAdvanceError_GrowsTokensBeforeThreshold(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("error", 1800, 200000)
	ms.errorAt = 0.6

	before := ms.state.TokensUsed
	gen.advanceError(ms, 5)

	if ms.state.TokensUsed <= before {
		t.Error("tokens did not grow before error threshold")
	}
	if ms.completed {
		t.Error("completed before reaching error threshold")
	}
}

func TestAdvanceError_ErrorsAtThreshold(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("error", 1800, 200000)
	ms.errorAt = 0.6

	// Set utilization just below threshold, then advance to cross it.
	// With maxContext=200000 and errorAt=0.6, threshold is 120000 tokens.
	// tokensPerTick=1800 + jitter up to 400 → max ~2200 per tick.
	ms.state.TokensUsed = 119000
	ms.state.UpdateUtilization()

	gen.advanceError(ms, 5)

	if !ms.completed {
		t.Fatal("session did not error after crossing threshold")
	}
	if ms.state.Activity != session.Errored {
		t.Errorf("Activity = %v, want Errored", ms.state.Activity)
	}
	if ms.state.CompletedAt == nil {
		t.Error("CompletedAt is nil after error")
	}
}

func TestAdvanceError_ToolUseEveryThirdTick(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("error", 100, 200000)
	ms.errorAt = 0.9 // high threshold so it won't error early

	gen.advanceError(ms, 6) // 6%3==0 → ToolUse
	if ms.state.Activity != session.ToolUse {
		t.Errorf("tick 6: Activity = %v, want ToolUse", ms.state.Activity)
	}

	gen.advanceError(ms, 7) // 7%3==1 → Thinking
	if ms.state.Activity != session.Thinking {
		t.Errorf("tick 7: Activity = %v, want Thinking", ms.state.Activity)
	}
}

// ---------------------------------------------------------------------------
// advanceMethodical
// ---------------------------------------------------------------------------

func TestAdvanceMethodical_TokensGrow(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("methodical", 600, 160000)

	before := ms.state.TokensUsed
	gen.advanceMethodical(ms, 5)
	if ms.state.TokensUsed <= before {
		t.Error("tokens did not grow for methodical pattern")
	}
}

func TestAdvanceMethodical_MostlyToolUse(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("methodical", 600, 160000)

	toolUseCount := 0
	thinkingCount := 0
	for tick := 1; tick <= 20; tick++ {
		gen.advanceMethodical(ms, tick)
		switch ms.state.Activity {
		case session.ToolUse:
			toolUseCount++
		case session.Thinking:
			thinkingCount++
		}
	}

	// Thinking only on ticks divisible by 5 → 4 out of 20 (ticks 5,10,15,20).
	// ToolUse on the rest → 16 out of 20.
	if thinkingCount != 4 {
		t.Errorf("Thinking ticks = %d, want 4 (every 5th tick)", thinkingCount)
	}
	if toolUseCount != 16 {
		t.Errorf("ToolUse ticks = %d, want 16", toolUseCount)
	}
}

func TestAdvanceMethodical_SinusoidalPaceVariation(t *testing.T) {
	gen := newTestGen()

	// Measure growth at different ticks to verify pace variation.
	// At tick≈0 sin(0)=0 → pace=0.7; at tick≈16 sin(1.6)≈1 → pace≈1.0.
	msLow := newTestMS("methodical", 1000, 500000)
	gen.advanceMethodical(msLow, 1) // sin(0.1)≈0.1 → pace≈0.73
	lowGrowth := msLow.state.TokensUsed

	msHigh := newTestMS("methodical", 1000, 500000)
	gen.advanceMethodical(msHigh, 16) // sin(1.6)≈0.9996 → pace≈1.0
	highGrowth := msHigh.state.TokensUsed

	if highGrowth <= lowGrowth {
		t.Errorf("high-pace growth (%d) <= low-pace growth (%d); sinusoidal variation not working", highGrowth, lowGrowth)
	}
}

func TestAdvanceMethodical_CompletesAtMaxTokens(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("methodical", 1000, 2000)
	ms.state.TokensUsed = 1500

	gen.advanceMethodical(ms, 16) // pace≈1.0 → growth≈1000

	if !ms.completed {
		t.Fatal("methodical session not completed at maxTokens")
	}
	if ms.state.TokensUsed != ms.maxTokens {
		t.Errorf("TokensUsed = %d, want %d", ms.state.TokensUsed, ms.maxTokens)
	}
}

// ---------------------------------------------------------------------------
// advanceSubagents
// ---------------------------------------------------------------------------

func TestAdvanceSubagents_SpawnsAtCorrectTick(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("steady", 1000, 100000)
	ms.subagentDefs = []mockSubagentDef{
		{id: "agent-test-1", slug: "test-agent", model: "haiku", spawnTick: 5, endTick: 20,
			tools: []string{"Read", "Grep"}},
	}

	// Before spawn tick: no subagents.
	gen.advanceSubagents(ms, 4)
	if len(ms.state.Subagents) != 0 {
		t.Errorf("tick 4: expected 0 subagents, got %d", len(ms.state.Subagents))
	}

	// At spawn tick: subagent appears.
	gen.advanceSubagents(ms, 5)
	if len(ms.state.Subagents) != 1 {
		t.Fatalf("tick 5: expected 1 subagent, got %d", len(ms.state.Subagents))
	}

	sub := ms.state.Subagents[0]
	if sub.ID != "agent-test-1" {
		t.Errorf("subagent ID = %q, want %q", sub.ID, "agent-test-1")
	}
	if sub.Slug != "test-agent" {
		t.Errorf("subagent Slug = %q, want %q", sub.Slug, "test-agent")
	}
	if sub.Activity != session.Thinking {
		t.Errorf("subagent Activity = %v, want Thinking", sub.Activity)
	}
	if sub.SessionID != ms.state.ID {
		t.Errorf("subagent SessionID = %q, want %q", sub.SessionID, ms.state.ID)
	}
}

func TestAdvanceSubagents_CyclesActivity(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("steady", 1000, 100000)
	ms.subagentDefs = []mockSubagentDef{
		{id: "agent-test-1", slug: "test-agent", model: "haiku", spawnTick: 5, endTick: 100,
			tools: []string{"Read", "Grep", "Glob"}},
	}

	// Spawn the subagent.
	gen.advanceSubagents(ms, 5)

	// age = tick - spawnTick. ToolUse when age%3==0, Thinking otherwise.
	// tick 5: age=0 → just spawned (Thinking from spawn).
	// tick 6: age=1 → Thinking
	// tick 7: age=2 → Thinking
	// tick 8: age=3 → ToolUse (3%3=0)
	gen.advanceSubagents(ms, 6)
	if ms.state.Subagents[0].Activity != session.Thinking {
		t.Errorf("age 1: Activity = %v, want Thinking", ms.state.Subagents[0].Activity)
	}

	gen.advanceSubagents(ms, 8) // age=3, 3%3=0 → ToolUse
	if ms.state.Subagents[0].Activity != session.ToolUse {
		t.Errorf("age 3: Activity = %v, want ToolUse", ms.state.Subagents[0].Activity)
	}
	if ms.state.Subagents[0].CurrentTool == "" {
		t.Error("age 3: CurrentTool is empty during ToolUse")
	}
}

func TestAdvanceSubagents_CompletesAtEndTick(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("steady", 1000, 100000)
	ms.subagentDefs = []mockSubagentDef{
		{id: "agent-test-1", slug: "test-agent", model: "haiku", spawnTick: 5, endTick: 10,
			tools: []string{"Read"}},
	}

	gen.advanceSubagents(ms, 5) // spawn
	gen.advanceSubagents(ms, 9) // still active

	if ms.state.Subagents[0].CompletedAt != nil {
		t.Fatal("subagent completed before endTick")
	}

	gen.advanceSubagents(ms, 10) // endTick → complete

	sub := ms.state.Subagents[0]
	if sub.Activity != session.Complete {
		t.Errorf("at endTick: Activity = %v, want Complete", sub.Activity)
	}
	if sub.CompletedAt == nil {
		t.Error("at endTick: CompletedAt is nil")
	}
}

func TestAdvanceSubagents_ParentCompletionCompletesAll(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("steady", 1000, 100000)
	ms.subagentDefs = []mockSubagentDef{
		{id: "agent-open-ended", slug: "open-agent", model: "sonnet", spawnTick: 3, endTick: 0,
			tools: []string{"Read"}},
		{id: "agent-bounded", slug: "bounded-agent", model: "haiku", spawnTick: 3, endTick: 50,
			tools: []string{"Read"}},
	}

	gen.advanceSubagents(ms, 3)  // spawn both
	gen.advanceSubagents(ms, 10) // both active

	// Simulate parent completion.
	ms.completed = true
	gen.advanceSubagents(ms, 11)

	for i, sub := range ms.state.Subagents {
		if sub.Activity != session.Complete {
			t.Errorf("subagent[%d] (%s): Activity = %v, want Complete after parent completion", i, sub.ID, sub.Activity)
		}
		if sub.CompletedAt == nil {
			t.Errorf("subagent[%d] (%s): CompletedAt nil after parent completion", i, sub.ID)
		}
	}
}

func TestAdvanceSubagents_AlreadyCompletedSubagentIsSkipped(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("steady", 1000, 100000)
	ms.subagentDefs = []mockSubagentDef{
		{id: "agent-test-1", slug: "test-agent", model: "haiku", spawnTick: 5, endTick: 8,
			tools: []string{"Read"}},
	}

	gen.advanceSubagents(ms, 5) // spawn
	gen.advanceSubagents(ms, 8) // complete

	tokensAfterComplete := ms.state.Subagents[0].TokensUsed

	gen.advanceSubagents(ms, 9) // should skip

	if ms.state.Subagents[0].TokensUsed != tokensAfterComplete {
		t.Errorf("tokens changed after completion: %d → %d",
			tokensAfterComplete, ms.state.Subagents[0].TokensUsed)
	}
}

func TestAdvanceSubagents_NoDefsIsNoop(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("steady", 1000, 100000)

	gen.advanceSubagents(ms, 10)
	if len(ms.state.Subagents) != 0 {
		t.Errorf("expected 0 subagents, got %d", len(ms.state.Subagents))
	}
}

func TestAdvanceSubagents_TokensAndCountersIncrement(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("steady", 1000, 100000)
	ms.subagentDefs = []mockSubagentDef{
		{id: "agent-test-1", slug: "test-agent", model: "haiku", spawnTick: 5, endTick: 100,
			tools: []string{"Read"}},
	}

	gen.advanceSubagents(ms, 5) // spawn
	gen.advanceSubagents(ms, 6) // first activity tick

	sub := ms.state.Subagents[0]
	if sub.TokensUsed <= 0 {
		t.Error("TokensUsed should be > 0 after activity tick")
	}
	if sub.MessageCount <= 0 {
		t.Error("MessageCount should be > 0 after activity tick")
	}
}

func TestAdvanceSubagents_ToolsCycleThroughDefs(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("steady", 1000, 100000)
	ms.subagentDefs = []mockSubagentDef{
		{id: "agent-test-1", slug: "test-agent", model: "haiku", spawnTick: 0, endTick: 100,
			tools: []string{"Read", "Grep", "Glob"}},
	}

	gen.advanceSubagents(ms, 0) // spawn

	// Collect tools from ToolUse ticks. ToolUse when age%3==0.
	// age=0 was the spawn tick (returned early with continue), so first activity at age=3,6,9.
	var tools []string
	for _, tick := range []int{3, 6, 9} {
		gen.advanceSubagents(ms, tick)
		if ms.state.Subagents[0].Activity == session.ToolUse {
			tools = append(tools, ms.state.Subagents[0].CurrentTool)
		}
	}

	// age/3 cycles: 3/3=1→Grep, 6/3=2→Glob, 9/3=3→Read (wraps).
	want := []string{"Grep", "Glob", "Read"}
	if len(tools) != len(want) {
		t.Fatalf("collected %d tool entries, want %d", len(tools), len(want))
	}
	for i, got := range tools {
		if got != want[i] {
			t.Errorf("tool[%d] = %q, want %q", i, got, want[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Integration: full advanceMock dispatches correctly
// ---------------------------------------------------------------------------

func TestAdvanceMock_DispatchesCorrectPattern(t *testing.T) {
	gen := newTestGen()

	patterns := []string{"steady", "burst", "stall", "error", "methodical"}
	for _, p := range patterns {
		ms := newTestMS(p, 500, 100000)
		if p == "error" {
			ms.errorAt = 0.9
		}

		// Advance past early ticks (1,2) and into pattern logic (tick 5).
		gen.advanceMock(ms, 1)
		gen.advanceMock(ms, 2)
		gen.advanceMock(ms, 5)

		// All patterns should have grown tokens and moved past Starting.
		if ms.state.TokensUsed <= 1000 {
			t.Errorf("pattern %q: TokensUsed = %d, want > 1000 after tick 5", p, ms.state.TokensUsed)
		}
		if ms.state.Activity == session.Starting {
			t.Errorf("pattern %q: still Starting after tick 5", p)
		}
	}
}

func TestAdvanceMock_UtilizationUpdated(t *testing.T) {
	gen := newTestGen()
	ms := newTestMS("steady", 1000, 100000)

	gen.advanceMock(ms, 1)

	if ms.state.ContextUtilization <= 0 {
		t.Error("ContextUtilization not updated after advance")
	}
}
