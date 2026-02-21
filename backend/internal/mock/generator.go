package mock

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

type mockSession struct {
	state         *session.SessionState
	tokensPerTick int
	pattern       string
	stageTime     int
	maxTokens     int
	errorAt       float64
	tools         []string
	toolIdx       int
	completed     bool
	prevTokens    int
	subagentDefs  []mockSubagentDef
}

type mockSubagentDef struct {
	id        string
	slug      string
	model     string
	spawnTick int    // tick when subagent appears
	endTick   int    // tick when subagent completes (0 = lives until parent completes)
	tools     []string
}

var commonTools = []string{"Read", "Write", "Edit", "Bash", "Grep", "Glob", "Task", "LSP"}

func NewGenerator(store *session.Store, broadcaster *ws.Broadcaster) *MockGenerator {
	return &MockGenerator{
		store:       store,
		broadcaster: broadcaster,
	}
}

type MockGenerator struct {
	store       *session.Store
	broadcaster *ws.Broadcaster
	sessions    []*mockSession
}

func (g *MockGenerator) Start(ctx context.Context) {
	now := time.Now()

	g.sessions = []*mockSession{
		{
			state: &session.SessionState{
				ID: "mock-opus-refactor", Name: "opus-refactor",
				Source: "claude", Model: "claude-opus-4-5-20251101", WorkingDir: "/home/user/myproject",
				MaxContextTokens: 200000, StartedAt: now, LastActivityAt: now,
				Activity: session.Starting, TmuxTarget: "dev:0.0",
			},
			tokensPerTick: 1200, pattern: "steady", maxTokens: 180000,
			tools: []string{"Read", "Grep", "Edit", "Write", "Bash", "Edit", "Read", "Write"},
			subagentDefs: []mockSubagentDef{
				{id: "agent_mock_haiku_researcher", slug: "swift-curious-hamster", model: "claude-haiku-4-5-20251001",
					spawnTick: 8, endTick: 30, tools: []string{"Read", "Grep", "Glob", "Read"}},
				{id: "agent_mock_sonnet_coder", slug: "eager-coding-gopher", model: "claude-sonnet-4-20250514",
					spawnTick: 12, endTick: 0, tools: []string{"Read", "Edit", "Write", "Bash", "Edit"}},
			},
		},
		{
			state: &session.SessionState{
				ID: "mock-sonnet-tests", Name: "sonnet-tests",
				Source: "claude", Model: "claude-sonnet-4-20250514", WorkingDir: "/home/user/webapp",
				MaxContextTokens: 200000, StartedAt: now, LastActivityAt: now,
				Activity: session.Starting, TmuxTarget: "dev:1.0",
			},
			tokensPerTick: 3500, pattern: "burst", maxTokens: 140000,
			tools: []string{"Read", "Write", "Bash", "Bash", "Write", "Bash"},
		},
		{
			state: &session.SessionState{
				ID: "mock-opus-debug", Name: "opus-debug",
				Source: "claude", Model: "claude-opus-4-5-20251101", WorkingDir: "/home/user/api-server",
				MaxContextTokens: 200000, StartedAt: now, LastActivityAt: now,
				Activity: session.Starting, TmuxTarget: "dev:2.0",
			},
			tokensPerTick: 800, pattern: "stall", maxTokens: 120000,
			tools: []string{"Read", "Grep", "Grep", "Read", "Bash", "LSP"},
		},
		{
			state: &session.SessionState{
				ID: "mock-sonnet-feature", Name: "sonnet-feature",
				Source: "claude", Model: "claude-sonnet-4-5-20250929", WorkingDir: "/home/user/frontend",
				MaxContextTokens: 200000, StartedAt: now, LastActivityAt: now,
				Activity: session.Starting,
			},
			tokensPerTick: 1800, pattern: "error", maxTokens: 200000, errorAt: 0.6,
			tools: []string{"Glob", "Read", "Edit", "Write", "Bash", "Edit"},
		},
		{
			state: &session.SessionState{
				ID: "mock-opus-review", Name: "opus-review",
				Source: "claude", Model: "claude-opus-4-5-20251101", WorkingDir: "/home/user/library",
				MaxContextTokens: 200000, StartedAt: now, LastActivityAt: now,
				Activity: session.Starting, TmuxTarget: "dev:3.0",
			},
			tokensPerTick: 600, pattern: "methodical", maxTokens: 160000,
			tools: []string{"Read", "LSP", "Read", "Grep", "Read", "LSP", "Read", "Task"},
			subagentDefs: []mockSubagentDef{
				{id: "agent_mock_haiku_explorer", slug: "tiny-brave-explorer", model: "claude-haiku-4-5-20251001",
					spawnTick: 6, endTick: 22, tools: []string{"Glob", "Read", "Grep", "Read", "Grep"}},
			},
		},
		{
			state: &session.SessionState{
				ID: "mock-codex-migrate", Name: "codex-migrate",
				Source: "codex", Model: "o3", WorkingDir: "/home/user/database",
				MaxContextTokens: 200000, StartedAt: now, LastActivityAt: now,
				Activity: session.Starting,
			},
			tokensPerTick: 2000, pattern: "burst", maxTokens: 150000,
			tools: []string{"Read", "Write", "Bash", "Read", "Write", "Bash"},
		},
		{
			state: &session.SessionState{
				ID: "mock-gemini-analyze", Name: "gemini-analyze",
				Source: "gemini", Model: "gemini-2.5-pro", WorkingDir: "/home/user/analytics",
				MaxContextTokens: 1048576, StartedAt: now, LastActivityAt: now,
				Activity: session.Starting,
			},
			tokensPerTick: 1500, pattern: "methodical", maxTokens: 800000,
			tools: []string{"Read", "Read", "Bash", "Read", "Read", "Bash", "Read", "Read"},
		},
	}

	for _, ms := range g.sessions {
		g.store.Update(ms.state)
	}

	go g.run(ctx)
}

func (g *MockGenerator) run(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	tick := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick++
			var updates []*session.SessionState
			for _, ms := range g.sessions {
				if ms.completed {
					continue
				}
				g.advanceMock(ms, tick)
				g.store.Update(ms.state)
				copy := *ms.state
				updates = append(updates, &copy)
			}
			if len(updates) > 0 {
				g.broadcaster.QueueUpdate(updates)
			}
		}
	}
}

func (g *MockGenerator) advanceMock(ms *mockSession, tick int) {
	now := time.Now()
	ms.state.LastActivityAt = now

	prevTokens := ms.state.TokensUsed

	if tick <= 2 {
		ms.state.Activity = session.Starting
		ms.state.TokensUsed += 500
		ms.state.UpdateUtilization()
		ms.prevTokens = prevTokens
		return
	}

	switch ms.pattern {
	case "steady":
		g.advanceSteady(ms, tick)
	case "burst":
		g.advanceBurst(ms, tick)
	case "stall":
		g.advanceStall(ms, tick)
	case "error":
		g.advanceError(ms, tick)
	case "methodical":
		g.advanceMethodical(ms, tick)
	}

	// Calculate burn rate: tokens gained this tick, scaled to realistic per-minute rates
	// Target range: 500-7000 tokens/min for demo
	tokenDelta := ms.state.TokensUsed - prevTokens
	if tokenDelta > 0 {
		ms.state.BurnRatePerMinute = float64(tokenDelta) * 2.5
	} else {
		ms.state.BurnRatePerMinute = 0
	}
	ms.prevTokens = prevTokens

	// Advance subagents: spawn, cycle activity, complete
	g.advanceSubagents(ms, tick)
}

func (g *MockGenerator) advanceSubagents(ms *mockSession, tick int) {
	if len(ms.subagentDefs) == 0 {
		return
	}

	now := time.Now()

	// Build index of existing subagents by ID
	existing := make(map[string]int, len(ms.state.Subagents))
	for i, sub := range ms.state.Subagents {
		existing[sub.ID] = i
	}

	for _, def := range ms.subagentDefs {
		if tick < def.spawnTick {
			continue
		}

		idx, exists := existing[def.id]
		if !exists {
			// Spawn new subagent
			ms.state.Subagents = append(ms.state.Subagents, session.SubagentState{
				ID:              def.id,
				ParentToolUseID: "toolu_mock_" + def.id,
				SessionID:       ms.state.ID,
				Slug:            def.slug,
				Model:           def.model,
				Activity:        session.Thinking,
				StartedAt:       now,
				LastActivityAt:  now,
			})
			continue
		}

		sub := &ms.state.Subagents[idx]

		// Already completed
		if sub.CompletedAt != nil {
			continue
		}

		// Check if subagent should complete this tick
		if def.endTick > 0 && tick >= def.endTick {
			sub.Activity = session.Complete
			sub.CompletedAt = &now
			sub.LastActivityAt = now
			continue
		}

		// Cycle activity: thinking <-> tool_use
		age := tick - def.spawnTick
		sub.LastActivityAt = now
		sub.MessageCount++

		if age%3 == 0 {
			sub.Activity = session.ToolUse
			sub.CurrentTool = def.tools[age/3%len(def.tools)]
			sub.ToolCallCount++
		} else {
			sub.Activity = session.Thinking
			sub.CurrentTool = ""
		}

		sub.TokensUsed += 200 + rand.Intn(300)
	}

	// When parent completes, complete all remaining active subagents
	if ms.completed {
		for i := range ms.state.Subagents {
			sub := &ms.state.Subagents[i]
			if sub.CompletedAt == nil {
				sub.Activity = session.Complete
				sub.CompletedAt = &now
				sub.LastActivityAt = now
			}
		}
	}
}

func (g *MockGenerator) advanceSteady(ms *mockSession, tick int) {
	jitter := rand.Intn(400) - 200
	ms.state.TokensUsed += ms.tokensPerTick + jitter
	ms.state.MessageCount++

	if tick%3 == 0 {
		ms.state.Activity = session.ToolUse
		ms.state.CurrentTool = ms.tools[ms.toolIdx%len(ms.tools)]
		ms.toolIdx++
		ms.state.ToolCallCount++
	} else {
		ms.state.Activity = session.Thinking
		ms.state.CurrentTool = ""
	}

	ms.state.UpdateUtilization()

	if ms.state.TokensUsed >= ms.maxTokens {
		ms.state.Activity = session.Complete
		ms.state.TokensUsed = ms.maxTokens
		ms.state.UpdateUtilization()
		now := time.Now()
		ms.state.CompletedAt = &now
		ms.completed = true
		g.broadcaster.QueueCompletion(ms.state.ID, session.Complete, ms.state.Name)
	}
}

func (g *MockGenerator) advanceBurst(ms *mockSession, tick int) {
	burstMultiplier := 1.0
	if tick%8 < 3 {
		burstMultiplier = 2.5
	}
	growth := int(float64(ms.tokensPerTick) * burstMultiplier)
	jitter := rand.Intn(500)
	ms.state.TokensUsed += growth + jitter
	ms.state.MessageCount++

	if burstMultiplier > 1 {
		ms.state.Activity = session.ToolUse
		ms.state.CurrentTool = ms.tools[ms.toolIdx%len(ms.tools)]
		ms.toolIdx++
		ms.state.ToolCallCount++
	} else {
		ms.state.Activity = session.Thinking
		ms.state.CurrentTool = ""
	}

	ms.state.UpdateUtilization()

	if ms.state.TokensUsed >= ms.maxTokens {
		ms.state.Activity = session.Complete
		ms.state.TokensUsed = ms.maxTokens
		ms.state.UpdateUtilization()
		now := time.Now()
		ms.state.CompletedAt = &now
		ms.completed = true
		g.broadcaster.QueueCompletion(ms.state.ID, session.Complete, ms.state.Name)
	}
}

func (g *MockGenerator) advanceStall(ms *mockSession, tick int) {
	// Repeating cycle: work for 40 ticks, stall (waiting) for 30 ticks.
	// This ensures the waiting window is always reachable regardless of
	// when an e2e test starts observing (the connection-status test
	// restarts the server mid-run).
	const cyclePeriod = 70
	phase := tick % cyclePeriod
	stallStart := 40

	if phase >= stallStart {
		ms.state.Activity = session.Waiting
		ms.state.CurrentTool = ""
		return
	}

	jitter := rand.Intn(200)
	ms.state.TokensUsed += ms.tokensPerTick + jitter
	ms.state.MessageCount++

	if tick%4 == 0 {
		ms.state.Activity = session.ToolUse
		ms.state.CurrentTool = ms.tools[ms.toolIdx%len(ms.tools)]
		ms.toolIdx++
		ms.state.ToolCallCount++
	} else {
		ms.state.Activity = session.Thinking
		ms.state.CurrentTool = ""
	}

	ms.state.UpdateUtilization()

	if ms.state.TokensUsed >= ms.maxTokens {
		ms.state.Activity = session.Complete
		ms.state.TokensUsed = ms.maxTokens
		ms.state.UpdateUtilization()
		now := time.Now()
		ms.state.CompletedAt = &now
		ms.completed = true
		g.broadcaster.QueueCompletion(ms.state.ID, session.Complete, ms.state.Name)
	}
}

func (g *MockGenerator) advanceError(ms *mockSession, tick int) {
	jitter := rand.Intn(400)
	ms.state.TokensUsed += ms.tokensPerTick + jitter
	ms.state.MessageCount++

	if tick%3 == 0 {
		ms.state.Activity = session.ToolUse
		ms.state.CurrentTool = ms.tools[ms.toolIdx%len(ms.tools)]
		ms.toolIdx++
		ms.state.ToolCallCount++
	} else {
		ms.state.Activity = session.Thinking
		ms.state.CurrentTool = ""
	}

	ms.state.UpdateUtilization()

	if ms.state.ContextUtilization >= ms.errorAt {
		ms.state.Activity = session.Errored
		ms.state.CurrentTool = ""
		now := time.Now()
		ms.state.CompletedAt = &now
		ms.completed = true
		g.broadcaster.QueueCompletion(ms.state.ID, session.Errored, ms.state.Name)
	}
}

func (g *MockGenerator) advanceMethodical(ms *mockSession, tick int) {
	// Slow, steady with lots of reading/LSP — sinusoidal pace variation
	pace := 0.7 + 0.3*math.Sin(float64(tick)/10.0)
	growth := int(float64(ms.tokensPerTick) * pace)
	ms.state.TokensUsed += growth
	ms.state.MessageCount++

	// Mostly tool use (reading/analyzing)
	if tick%5 == 0 {
		ms.state.Activity = session.Thinking
		ms.state.CurrentTool = ""
	} else {
		ms.state.Activity = session.ToolUse
		ms.state.CurrentTool = ms.tools[ms.toolIdx%len(ms.tools)]
		ms.toolIdx++
		ms.state.ToolCallCount++
	}

	ms.state.UpdateUtilization()

	if ms.state.TokensUsed >= ms.maxTokens {
		ms.state.Activity = session.Complete
		ms.state.TokensUsed = ms.maxTokens
		ms.state.UpdateUtilization()
		now := time.Now()
		ms.state.CompletedAt = &now
		ms.completed = true
		g.broadcaster.QueueCompletion(ms.state.ID, session.Complete, ms.state.Name)
	}
}
