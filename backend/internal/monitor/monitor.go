package monitor

import (
	"context"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	awmonitor "github.com/mrf/agentwatch/monitor"
	awsession "github.com/mrf/agentwatch/session"
	awsource "github.com/mrf/agentwatch/source"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/racer"
	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

// tokenSnapshot stores a token count at a point in time for burn rate calculation.
type tokenSnapshot struct {
	tokens    int
	timestamp time.Time
}

const defaultTmuxResolverTTL = 5 * time.Second
const defaultProcessActivityInterval = 5 * time.Second

// SnapshotHook is called after each poll with the current snapshot of all sessions.
// It is called synchronously from the poll goroutine; implementations must not block.
type SnapshotHook func([]*session.SessionState)

// terminalInfo records lifecycle event details for terminal transitions,
// allowing the delta handler to determine the correct local Activity.
type terminalInfo struct {
	reason    string
	eventType awsession.LifecycleEventType
}

// Monitor bridges the agentwatch monitor to the racer-specific local
// session.Store and ws.Broadcaster. It delegates source discovery, cursor
// management, stale detection, and health tracking to agentwatch, then
// converts events into local types and applies racer-specific enrichments
// (process activity, tmux resolution, token strategies, burn rate).
type Monitor struct {
	mu  sync.RWMutex
	cfg *config.Config

	store       *session.Store
	broadcaster *ws.Broadcaster
	bridge      *racer.Bridge
	sources     []awsource.Source
	awMon       *awmonitor.Monitor

	// Racer-specific enrichment state.
	prevCPU                 map[int]cpuSample
	lastProcessPoll         time.Time
	processActivity         map[string]ProcessActivity
	processPollInterval     time.Duration
	discoverProcessActivity func(map[int]cpuSample, time.Duration) ([]ProcessActivity, map[int]cpuSample)
	newTmuxResolver         func() *TmuxResolver
	tmuxResolverTTL         time.Duration
	tmuxResolver            *TmuxResolver
	tmuxResolverNext        time.Time
	tmuxResolverSet         bool
	tokenSnapshots          map[string][]tokenSnapshot

	// Lifecycle tracking: populated by handleLifecycleEvent, consumed by handleDeltaEvent.
	terminalReasons map[string]terminalInfo
	pendingRemovals []string

	// sessionSources maps raw agentwatch session IDs to their source name,
	// enabling localID construction for delta-event removals (which lack
	// source information). Populated in handleDeltaEvent from ev.Updates.
	sessionSources map[string]string

	// lastReconcile tracks when we last ran a store reconciliation pass.
	lastReconcile time.Time

	statsEvents      chan<- session.Event
	statsDropped     int64
	statsLastDropLog time.Time
	snapshotHook     SnapshotHook
	reconfigureCh    chan struct{}

	// externalAWMon: when true, SetConfig skips internal agentwatch rebuilds.
	externalAWMon bool
}

func NewMonitor(cfg *config.Config, store *session.Store, broadcaster *ws.Broadcaster, sources []awsource.Source) *Monitor {
	m := &Monitor{
		cfg:                     cfg,
		store:                   store,
		broadcaster:             broadcaster,
		bridge:                  racer.NewBridge(store, broadcaster),
		sources:                 sources,
		prevCPU:                 make(map[int]cpuSample),
		processActivity:         make(map[string]ProcessActivity),
		discoverProcessActivity: DiscoverProcessActivity,
		processPollInterval:     defaultProcessActivityInterval,
		newTmuxResolver:         NewTmuxResolver,
		tmuxResolverTTL:         defaultTmuxResolverTTL,
		tokenSnapshots:          make(map[string][]tokenSnapshot),
		terminalReasons:         make(map[string]terminalInfo),
		sessionSources:          make(map[string]string),
		reconfigureCh:           make(chan struct{}, 1),
	}
	m.awMon = m.buildAWMonitor(sources)
	broadcaster.SetHealthHook(m.SourceHealthSnapshot)
	return m
}

// buildAWMonitor creates a new agentwatch monitor configured from the current
// config. Returns nil if sources is empty (agentwatch requires at least one).
func (m *Monitor) buildAWMonitor(sources []awsource.Source) *awmonitor.Monitor {
	if len(sources) == 0 {
		return nil
	}

	m.mu.RLock()
	cfg := m.cfg
	m.mu.RUnlock()

	threshold := cfg.Monitor.HealthWarningThreshold
	if threshold <= 0 {
		threshold = 3
	}

	awMon, err := awmonitor.New(
		awmonitor.WithSources(sources...),
		awmonitor.WithPollInterval(cfg.Monitor.PollInterval),
		awmonitor.WithSink(awmonitor.EventSinkFunc(m.handleEvent)),
		awmonitor.WithStaleThreshold(cfg.Monitor.SessionStaleAfter),
		awmonitor.WithCompletionRetention(cfg.Monitor.CompletionRemoveAfter),
		awmonitor.WithHealthThreshold(threshold),
	)
	if err != nil {
		slog.Error("failed to create agentwatch monitor", "component", "monitor", "error", err)
		return nil
	}
	return awMon
}

// SetConfig replaces the monitor's config pointer. Timing changes
// (PollInterval, SessionStaleAfter, CompletionRemoveAfter) trigger
// a rebuild of the agentwatch monitor on the next poll cycle.
// When the agentwatch monitor is externally managed (via SetAWMonitor),
// rebuilds are skipped — the caller must provide a new monitor.
func (m *Monitor) SetConfig(cfg *config.Config) {
	m.mu.Lock()
	oldCfg := m.cfg
	m.cfg = cfg
	external := m.externalAWMon
	m.mu.Unlock()

	// Rebuild agentwatch monitor if its immutable settings changed.
	// Skip when externally managed — caller handles rebuilds.
	if !external && (oldCfg.Monitor.SessionStaleAfter != cfg.Monitor.SessionStaleAfter ||
		oldCfg.Monitor.CompletionRemoveAfter != cfg.Monitor.CompletionRemoveAfter ||
		oldCfg.Monitor.HealthWarningThreshold != cfg.Monitor.HealthWarningThreshold) {
		m.rebuildAWMonitor()
	}

	// Signal Start() to recreate its ticker with the updated poll interval.
	select {
	case m.reconfigureCh <- struct{}{}:
	default:
	}
}

// SetSources replaces the monitor's source list and rebuilds the
// agentwatch monitor. Cursor state from the previous monitor is lost;
// sessions will be re-discovered on the next poll.
func (m *Monitor) SetSources(newSources []awsource.Source) {
	m.mu.Lock()
	m.sources = newSources
	m.mu.Unlock()
	m.rebuildAWMonitor()
}

// SetAWMonitor replaces the agentwatch monitor with an externally-created
// one. The caller is responsible for managing its lifecycle (rebuilding on
// config changes). The monitor uses this for health queries and polling.
func (m *Monitor) SetAWMonitor(mon *awmonitor.Monitor) {
	m.mu.Lock()
	m.awMon = mon
	m.externalAWMon = true
	m.mu.Unlock()
}

// HandleEvent implements awmonitor.EventSink, bridging agentwatch events
// to the local session store and broadcaster. Exported so callers can wire
// it as a sink when creating the agentwatch monitor externally.
func (m *Monitor) HandleEvent(ctx context.Context, ev awmonitor.Event) error {
	return m.handleEvent(ctx, ev)
}

func (m *Monitor) rebuildAWMonitor() {
	m.mu.Lock()
	sources := m.sources
	m.mu.Unlock()

	newMon := m.buildAWMonitor(sources)

	m.mu.Lock()
	m.awMon = newMon
	m.mu.Unlock()
}

// SetStatsEvents configures a channel for session lifecycle events.
func (m *Monitor) SetStatsEvents(ch chan<- session.Event) {
	m.statsEvents = ch
}

// SetSnapshotHook registers a function to be called after each poll with a
// snapshot of all current sessions. Pass nil to disable.
func (m *Monitor) SetSnapshotHook(fn SnapshotHook) {
	m.snapshotHook = fn
}

func (m *Monitor) Start(ctx context.Context) {
	m.mu.RLock()
	pollInterval := m.cfg.Monitor.PollInterval
	sourceNames := make([]string, len(m.sources))
	for i, s := range m.sources {
		sourceNames[i] = s.Name()
	}
	m.mu.RUnlock()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	slog.Info("monitor started", "component", "monitor", "sources", sourceNames)

	// Initial poll.
	m.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("monitor stopped", "component", "monitor")
			return
		case <-m.reconfigureCh:
			ticker.Stop()
			m.mu.RLock()
			newInterval := m.cfg.Monitor.PollInterval
			m.mu.RUnlock()
			ticker = time.NewTicker(newInterval)
			slog.Info("monitor poll interval updated", "component", "monitor", "interval", newInterval)
		case <-ticker.C:
			m.poll(ctx)
		}
	}
}

func (m *Monitor) poll(ctx context.Context) {
	m.mu.RLock()
	awMon := m.awMon
	m.mu.RUnlock()

	if awMon == nil {
		return
	}

	if err := awMon.PollOnce(ctx); err != nil {
		slog.Warn("poll error", "component", "monitor", "error", err)
	}
}

// ---------------------------------------------------------------------------
// Event sink: bridges agentwatch events to local store + broadcaster.
// ---------------------------------------------------------------------------

func (m *Monitor) handleEvent(ctx context.Context, ev awmonitor.Event) error {
	switch ev.Type {
	case awmonitor.EventLifecycle:
		m.handleLifecycleEvent(ev)
	case awmonitor.EventDelta:
		m.handleDeltaEvent(ev)
	case awmonitor.EventHealth:
		m.handleHealthEvent(ev)
	}
	return nil
}

// handleLifecycleEvent records terminal reasons and pending removals so the
// delta handler can use them. Lifecycle events are delivered before the delta.
func (m *Monitor) handleLifecycleEvent(ev awmonitor.Event) {
	if ev.Lifecycle == nil {
		return
	}
	lc := ev.Lifecycle
	localID := lc.Source + ":" + lc.SessionID

	switch lc.Type {
	case awsession.EventTerminal, awsession.EventStale:
		m.terminalReasons[localID] = terminalInfo{
			reason:    lc.Reason,
			eventType: lc.Type,
		}
	case awsession.EventRemoved:
		m.pendingRemovals = append(m.pendingRemovals, localID)
	}
}

func (m *Monitor) handleDeltaEvent(ev awmonitor.Event) {
	now := time.Now()

	m.mu.RLock()
	cfg := m.cfg
	m.mu.RUnlock()

	activityByDir := m.refreshProcessActivity(now)

	var updates []*session.SessionState

	for i := 0; i < len(ev.Updates); i++ {
		awState := &ev.Updates[i]
		localID := awState.Source + ":" + awState.ID

		// Record the source for this raw session ID so delta-event removals
		// (which lack source info) can construct the correct localID.
		m.sessionSources[awState.ID] = awState.Source

		existing, existed := m.store.Get(localID)
		local := m.convertSession(cfg, awState, localID, existing, existed, activityByDir, now)
		updates = append(updates, local)

		// Stats events.
		if !existed {
			m.emitEvent(session.EventNew, local)
		} else if local.IsTerminal() && !existing.IsTerminal() {
			m.emitEvent(session.EventTerminal, local)
		} else {
			m.emitEvent(session.EventUpdate, local)
		}
	}

	// Resolve tmux targets for sessions with PIDs.
	m.resolveTmux(updates, now)

	// Broadcast completion events for newly terminal sessions.
	for i := 0; i < len(updates); i++ {
		if !updates[i].IsTerminal() {
			continue
		}
		existing, existed := m.store.Get(updates[i].ID)
		if existed && !existing.IsTerminal() {
			slog.Info("session terminal", "component", "monitor", "session", updates[i].ID, "name", updates[i].Name, "activity", updates[i].Activity)
			m.bridge.QueueCompletion(updates[i].ID, updates[i].Activity, updates[i].Name)
		} else if !existed {
			// New session discovered already terminal — still broadcast.
			m.bridge.QueueCompletion(updates[i].ID, updates[i].Activity, updates[i].Name)
		}
	}

	// Enrich with positions, write to store, and queue broadcast via Bridge.
	if len(updates) > 0 {
		m.bridge.PushUpdate(updates)
	}

	// Warn if the local store is accumulating too many sessions — this is
	// the leading indicator of the mass-spawn bug where removals fail to
	// keep pace with discovery.
	if count := m.store.Count(); count > 100 {
		slog.Warn("session count unexpectedly high",
			"component", "monitor", "count", count)
	}

	// Handle removals: merge lifecycle-event pendingRemovals with the delta
	// event's Removed list. The lifecycle path is the primary removal signal,
	// but processing ev.Removed as well provides defense-in-depth against
	// sessions lingering in the local store if a lifecycle event is lost.
	removals := m.pendingRemovals
	m.pendingRemovals = nil
	if len(ev.Removed) > 0 {
		seen := make(map[string]struct{}, len(removals))
		for _, id := range removals {
			seen[id] = struct{}{}
		}
		for _, rawID := range ev.Removed {
			src := m.sessionSources[rawID]
			if src == "" {
				continue // unknown source, can't construct localID
			}
			localID := src + ":" + rawID
			if _, ok := seen[localID]; !ok {
				removals = append(removals, localID)
			}
			delete(m.sessionSources, rawID)
		}
	}
	if len(removals) > 0 {
		m.bridge.PushRemoval(removals)
		for _, id := range removals {
			delete(m.tokenSnapshots, id)
		}
	}

	// Clear terminal reasons consumed this cycle.
	for k := range m.terminalReasons {
		delete(m.terminalReasons, k)
	}

	// Periodic reconciliation: prune local store sessions that agentwatch
	// no longer reports. This catches orphaned sessions that slipped through
	// the normal removal path (e.g., after an agentwatch monitor rebuild).
	m.reconcileStore(now, cfg)

	if m.snapshotHook != nil {
		m.snapshotHook(m.store.GetAll())
	}
}

const reconcileInterval = 30 * time.Second

// reconcileStore periodically scans the local store for sessions that
// agentwatch no longer tracks. Terminal sessions whose last data is older
// than the stale threshold plus completion retention are pruned. This
// defends against session accumulation after monitor rebuilds or lost events.
func (m *Monitor) reconcileStore(now time.Time, cfg *config.Config) {
	if now.Sub(m.lastReconcile) < reconcileInterval {
		return
	}
	m.lastReconcile = now

	stale := cfg.Monitor.SessionStaleAfter
	retention := cfg.Monitor.CompletionRemoveAfter
	if stale <= 0 && retention <= 0 {
		return
	}
	// A session is considered orphaned if it is terminal and its last data
	// arrived longer ago than stale + retention + a 30s safety margin.
	maxAge := stale + retention + 30*time.Second
	cutoff := now.Add(-maxAge)

	allSessions := m.store.GetAll()
	var orphaned []string
	for _, s := range allSessions {
		if !s.IsTerminal() {
			continue
		}
		if s.LastDataReceivedAt.IsZero() || s.LastDataReceivedAt.Before(cutoff) {
			orphaned = append(orphaned, s.ID)
		}
	}
	if len(orphaned) > 0 {
		slog.Warn("reconciliation: pruning orphaned terminal sessions",
			"component", "monitor", "count", len(orphaned))
		m.bridge.PushRemoval(orphaned)
		for _, id := range orphaned {
			delete(m.tokenSnapshots, id)
		}
	}
}

func (m *Monitor) handleHealthEvent(ev awmonitor.Event) {
	if ev.Health == nil {
		return
	}
	h := ev.Health
	status := convertHealthStatus(h.Status)
	msg, err := ws.NewSourceHealthMessage(ws.SourceHealthPayload{
		Source:           h.Source,
		Status:           status,
		DiscoverFailures: h.DiscoverFailures,
		ParseFailures:    h.ParseFailures,
		LastError:        sanitizeHealthError(h.LastError),
		Timestamp:        h.UpdatedAt,
	})
	if err != nil {
		slog.Error("source health marshal failed", "component", "monitor", "source", h.Source, "error", err)
		return
	}
	m.broadcaster.BroadcastMessage(msg)
	slog.Info("health status changed", "component", "monitor", "source", h.Source, "status", status)
}

// ---------------------------------------------------------------------------
// Session conversion: agentwatch SessionState → local SessionState.
// ---------------------------------------------------------------------------

func (m *Monitor) convertSession(
	cfg *config.Config,
	awState *awsession.SessionState,
	localID string,
	existing *session.SessionState,
	existed bool,
	activityByDir map[string]ProcessActivity,
	now time.Time,
) *session.SessionState {
	activity := m.mapActivity(awState, localID)

	local := &session.SessionState{
		ID:                 localID,
		Name:               nameFromPath(awState.WorkingDir),
		Slug:               awState.Slug,
		Source:             awState.Source,
		Activity:           activity,
		Lifecycle:          awState.Lifecycle,
		ContextTokens:      awState.ContextTokens,
		OutputTokens:       awState.OutputTokens,
		TokenEstimated:     awState.TokenEstimated,
		MaxContextTokens:   awState.MaxContextTokens,
		ContextUtilization: awState.ContextUtilization,
		CurrentTool:        awState.CurrentTool,
		Model:              awState.Model,
		WorkingDir:         awState.WorkingDir,
		Branch:             awState.Branch,
		StartedAt:          awState.StartedAt,
		LastActivityAt:     awState.LastActivityAt,
		LastDataReceivedAt: awState.LastDataReceivedAt,
		CompletedAt:        awState.CompletedAt,
		MessageCount:       awState.MessageCount,
		ToolCallCount:      awState.ToolCallCount,
		Subagents:          convertSubagents(awState.Subagents, localID),
	}

	// Carry forward local-only fields from existing state.
	if existed {
		local.PID = existing.PID
		local.TmuxTarget = existing.TmuxTarget
		local.CompactionCount = existing.CompactionCount
		local.LastAssistantText = existing.LastAssistantText
		local.LogPath = existing.LogPath
	}

	// Detect branch if not provided by the source.
	if local.Branch == "" && local.WorkingDir != "" {
		local.Branch = detectBranch(local.WorkingDir)
	}

	// Apply token normalization strategy.
	m.resolveTokens(cfg, local, existing, existed)

	// Burn rate.
	local.BurnRatePerMinute = m.calculateBurnRate(localID, local.ContextTokens, now)

	// Process activity enrichment (churning, PID).
	if !local.IsTerminal() && local.Activity != session.Waiting {
		if pa, ok := activityByDir[local.WorkingDir]; ok {
			local.IsChurning = pa.IsChurning(
				cfg.Monitor.ChurningCPUThreshold,
				cfg.Monitor.ChurningRequiresNetwork,
			)
			if pa.PID > 0 && local.PID == 0 {
				local.PID = pa.PID
			}
		}
	}

	return local
}

// mapActivity converts an agentwatch Activity to the local session.Activity.
// For terminal sessions, it uses lifecycle event reasons to distinguish
// Complete, Errored, and Lost.
func (m *Monitor) mapActivity(awState *awsession.SessionState, localID string) session.Activity {
	// Check lifecycle first: a terminal lifecycle overrides the activity field,
	// which may still reflect the pre-terminal state if the source didn't
	// explicitly set Activity to ActivityTerminal.
	if awState.Lifecycle == awsession.LifecycleTerminal {
		return m.mapTerminalActivity(localID)
	}

	switch awState.Activity {
	case awsession.ActivityWorking:
		if awState.CurrentTool != "" {
			return session.ToolUse
		}
		return session.Thinking
	case awsession.ActivityWaiting:
		return session.Waiting
	case awsession.ActivityTerminal:
		return m.mapTerminalActivity(localID)
	default: // ActivityIdle or empty
		return session.Idle
	}
}

// mapTerminalActivity determines Complete/Errored/Lost from the lifecycle
// event reason recorded by handleLifecycleEvent.
func (m *Monitor) mapTerminalActivity(localID string) session.Activity {
	info, ok := m.terminalReasons[localID]
	if !ok {
		return session.Complete
	}
	if info.eventType == awsession.EventStale {
		return session.Lost
	}
	return determineActivityFromReason(info.reason)
}

// determineActivityFromReason inspects the reason field and returns the
// appropriate terminal activity (Complete, Errored, or Lost).
func determineActivityFromReason(reason string) session.Activity {
	if reason == "" {
		return session.Complete
	}

	lowerReason := strings.ToLower(reason)
	errorIndicators := []string{
		"error", "err", "failed", "failure", "crash", "crashed",
		"panic", "exception", "abort", "aborted", "fatal",
		"interrupted", "killed", "terminated",
	}

	for _, indicator := range errorIndicators {
		if strings.Contains(lowerReason, indicator) {
			return session.Errored
		}
	}

	return session.Complete
}

// convertSubagents maps agentwatch SubagentState to local SubagentState.
func convertSubagents(awSubs []awsession.SubagentState, parentID string) []session.SubagentState {
	if len(awSubs) == 0 {
		return nil
	}
	result := make([]session.SubagentState, len(awSubs))
	for i := 0; i < len(awSubs); i++ {
		sub := &awSubs[i]
		activity := convertSubagentActivity(sub.Activity)
		result[i] = session.SubagentState{
			ID:              sub.ID,
			ParentToolUseID: sub.ParentID,
			SessionID:       parentID,
			Activity:        activity,
			CurrentTool:     sub.CurrentTool,
			StartedAt:       sub.StartedAt,
			LastActivityAt:  sub.LastActivityAt,
		}
		if sub.Activity == awsession.ActivityTerminal {
			completedAt := sub.LastActivityAt
			result[i].CompletedAt = &completedAt
			result[i].Activity = session.Complete
		}
	}
	return result
}

// convertSubagentActivity maps an agentwatch session.Activity to the local
// session.Activity enum used by agent-racer.
func convertSubagentActivity(a awsession.Activity) session.Activity {
	switch a {
	case awsession.ActivityWorking:
		return session.Thinking
	case awsession.ActivityWaiting:
		return session.Waiting
	case awsession.ActivityTerminal:
		return session.Complete
	default:
		return session.Idle
	}
}

// ---------------------------------------------------------------------------
// Token resolution.
// ---------------------------------------------------------------------------

// resolveTokens applies the configured token normalization strategy for the
// session's source. For "usage" it prefers real token data and falls back to
// estimation. For "estimate" and "message_count" it always derives tokens
// from the accumulated message count.
func (m *Monitor) resolveTokens(cfg *config.Config, local *session.SessionState, existing *session.SessionState, existed bool) {
	strategy := cfg.TokenStrategy(local.Source)
	tokensPerMsg := cfg.TokenNorm.TokensPerMessage
	if tokensPerMsg <= 0 {
		tokensPerMsg = 2000
	}

	// Prefer source-reported context ceiling; fall back to config.
	if local.MaxContextTokens == 0 {
		modelForLookup := local.Model
		if modelForLookup == "" {
			modelForLookup = "unknown"
		}
		local.MaxContextTokens = cfg.MaxContextTokens(modelForLookup)
	}

	switch strategy {
	case "usage":
		if local.ContextTokens > 0 {
			// Real token data. When transitioning from estimated to actual,
			// accept the real value even if lower.
			if existed && !existing.TokenEstimated && local.ContextTokens < existing.ContextTokens {
				// Don't go backwards on real token data.
				local.ContextTokens = existing.ContextTokens
			}
			local.TokenEstimated = false
		} else if !existed || existing.TokenEstimated || existing.ContextTokens == 0 {
			// No real data yet — fall back to estimation.
			if local.MessageCount > 0 {
				estimated := local.MessageCount * tokensPerMsg
				if !existed || estimated > existing.ContextTokens {
					local.ContextTokens = estimated
					local.TokenEstimated = true
				} else {
					// Carry forward existing estimation.
					local.ContextTokens = existing.ContextTokens
					local.TokenEstimated = existing.TokenEstimated
				}
			}
		} else {
			// Carry forward existing real token data.
			local.ContextTokens = existing.ContextTokens
			local.TokenEstimated = false
		}

	case "estimate", "message_count":
		if local.MessageCount > 0 {
			estimated := local.MessageCount * tokensPerMsg
			if existed && estimated < existing.ContextTokens {
				local.ContextTokens = existing.ContextTokens
			} else {
				local.ContextTokens = estimated
			}
			local.TokenEstimated = true
		}

	default:
		// Unknown strategy: use real data only.
		if existed && local.ContextTokens < existing.ContextTokens {
			local.ContextTokens = existing.ContextTokens
		}
	}

	local.UpdateUtilization()
}

// ---------------------------------------------------------------------------
// Burn rate.
// ---------------------------------------------------------------------------

const (
	burnRateWindow    = 60 * time.Second
	maxTokenSnapshots = 120
)

// calculateBurnRate computes the token consumption rate (tokens per minute)
// using a rolling window of recent token snapshots.
func (m *Monitor) calculateBurnRate(sessionID string, currentTokens int, now time.Time) float64 {
	if currentTokens <= 0 {
		return 0
	}

	snapshots := m.tokenSnapshots[sessionID]
	snapshots = append(snapshots, tokenSnapshot{
		tokens:    currentTokens,
		timestamp: now,
	})

	// Trim snapshots older than window.
	cutoff := now.Add(-burnRateWindow)
	startIdx := 0
	for i := 0; i < len(snapshots); i++ {
		if snapshots[i].timestamp.After(cutoff) {
			startIdx = i
			break
		}
		startIdx = i + 1
	}
	if startIdx > 0 {
		snapshots = snapshots[startIdx:]
	}

	// Hard cap.
	if len(snapshots) > maxTokenSnapshots {
		snapshots = snapshots[len(snapshots)-maxTokenSnapshots:]
	}

	m.tokenSnapshots[sessionID] = snapshots

	if len(snapshots) < 2 {
		return 0
	}

	oldest := snapshots[0]
	latest := snapshots[len(snapshots)-1]

	tokenDelta := latest.tokens - oldest.tokens
	timeDelta := latest.timestamp.Sub(oldest.timestamp)

	if timeDelta.Seconds() < 5 {
		return 0
	}

	minutes := timeDelta.Minutes()
	if minutes > 0 && tokenDelta > 0 {
		return float64(tokenDelta) / minutes
	}
	return 0
}

// ---------------------------------------------------------------------------
// Process activity & tmux enrichment.
// ---------------------------------------------------------------------------

func (m *Monitor) refreshProcessActivity(now time.Time) map[string]ProcessActivity {
	if m.discoverProcessActivity == nil {
		return m.processActivity
	}
	if m.processPollInterval > 0 && !m.lastProcessPoll.IsZero() && now.Sub(m.lastProcessPoll) < m.processPollInterval {
		return m.processActivity
	}

	elapsed := time.Duration(0)
	if !m.lastProcessPoll.IsZero() {
		elapsed = now.Sub(m.lastProcessPoll)
	}
	activities, newCPU := m.discoverProcessActivity(m.prevCPU, elapsed)
	m.prevCPU = newCPU
	m.lastProcessPoll = now

	activityByDir := make(map[string]ProcessActivity, len(activities))
	for _, a := range activities {
		if existing, ok := activityByDir[a.WorkingDir]; ok {
			if a.CPU > existing.CPU {
				activityByDir[a.WorkingDir] = a
			}
			continue
		}
		activityByDir[a.WorkingDir] = a
	}
	m.processActivity = activityByDir
	return m.processActivity
}

func (m *Monitor) cachedTmuxResolver(now time.Time) *TmuxResolver {
	if m.newTmuxResolver == nil {
		return nil
	}
	if m.tmuxResolverTTL <= 0 {
		return m.newTmuxResolver()
	}
	if m.tmuxResolverSet && now.Before(m.tmuxResolverNext) {
		return m.tmuxResolver
	}
	m.tmuxResolver = m.newTmuxResolver()
	m.tmuxResolverNext = now.Add(m.tmuxResolverTTL)
	m.tmuxResolverSet = true
	return m.tmuxResolver
}

func (m *Monitor) resolveTmux(updates []*session.SessionState, now time.Time) {
	needsTmux := false
	for i := 0; i < len(updates); i++ {
		if updates[i].PID > 0 {
			needsTmux = true
			break
		}
	}
	if !needsTmux {
		return
	}

	resolver := m.cachedTmuxResolver(now)
	for i := 0; i < len(updates); i++ {
		if updates[i].PID == 0 {
			continue
		}
		target, ok := resolver.Resolve(updates[i].PID)
		if !ok || updates[i].TmuxTarget == target {
			continue
		}
		updates[i].TmuxTarget = target
	}
}

// ---------------------------------------------------------------------------
// Health snapshot.
// ---------------------------------------------------------------------------

// SourceHealthSnapshot builds SourceHealthPayload entries for all non-healthy
// sources. Used by the broadcaster's health hook and the /healthz endpoint.
func (m *Monitor) SourceHealthSnapshot() []ws.SourceHealthPayload {
	m.mu.RLock()
	awMon := m.awMon
	m.mu.RUnlock()

	if awMon == nil {
		return nil
	}

	healthMap := awMon.Health()
	var result []ws.SourceHealthPayload
	now := time.Now()
	for name, h := range healthMap {
		status := convertHealthStatus(h.Status)
		if status == ws.StatusHealthy {
			continue
		}
		result = append(result, ws.SourceHealthPayload{
			Source:           name,
			Status:           status,
			DiscoverFailures: h.DiscoverFailures,
			ParseFailures:    h.ParseFailures,
			LastError:        sanitizeHealthError(h.LastError),
			Timestamp:        now,
		})
	}
	return result
}

func convertHealthStatus(s awmonitor.HealthStatus) ws.SourceHealthStatus {
	switch s {
	case awmonitor.HealthDegraded:
		return ws.StatusDegraded
	case awmonitor.HealthFailed:
		return ws.StatusFailed
	default:
		return ws.StatusHealthy
	}
}

// ---------------------------------------------------------------------------
// Stats event emission.
// ---------------------------------------------------------------------------

func (m *Monitor) emitEvent(evType session.EventType, state *session.SessionState) {
	if m.statsEvents == nil {
		return
	}
	snap := *state
	select {
	case m.statsEvents <- session.Event{
		Type:        evType,
		State:       &snap,
		ActiveCount: m.store.ActiveCount(),
	}:
	default:
		m.statsDropped++
		now := time.Now()
		if m.statsLastDropLog.IsZero() || now.Sub(m.statsLastDropLog) >= 10*time.Second {
			slog.Warn("stats events dropped", "component", "monitor", "count", m.statsDropped)
			m.statsDropped = 0
			m.statsLastDropLog = now
		}
	}
}

// ---------------------------------------------------------------------------
// Utility functions.
// ---------------------------------------------------------------------------

func nameFromPath(path string) string {
	parts := splitPath(path)
	// If the path is inside a .claude/worktrees/<slug>/ directory,
	// use the worktree slug as the display name.
	for i := 0; i < len(parts)-2; i++ {
		if parts[i] == ".claude" && parts[i+1] == "worktrees" {
			return parts[i+2]
		}
	}
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return "unknown"
}

func splitPath(path string) []string {
	var parts []string
	for path != "" && path != "/" {
		dir, file := filepath.Split(path)
		if file != "" {
			parts = append([]string{file}, parts...)
		}
		path = dir
		if path == "" || path == "/" {
			break
		}
		path = path[:len(path)-1]
	}
	return parts
}

// detectBranch runs git rev-parse in the given directory to determine
// the current branch name. Returns empty string on any error.
func detectBranch(dir string) string {
	if dir == "" {
		return ""
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, gitPath, "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "HEAD" {
		return "" // detached HEAD, not useful
	}
	return branch
}
