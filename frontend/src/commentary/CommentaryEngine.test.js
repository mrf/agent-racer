import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { CommentaryEngine } from './CommentaryEngine.js';

/** Helper: create a session map from an array of session objects. */
function sessionsMap(...sessions) {
  return new Map(sessions.map(s => [s.id, s]));
}

/** Helper: create engine with message collector. */
function createEngine() {
  const engine = new CommentaryEngine();
  const messages = [];
  engine.onMessage = (m) => messages.push(m);
  return { engine, messages };
}

describe('CommentaryEngine', () => {
  let baseTime;

  beforeEach(() => {
    vi.useFakeTimers();
    baseTime = new Date('2026-03-07T12:00:00Z').getTime();
    vi.setSystemTime(baseTime);
    vi.spyOn(Math, 'random').mockReturnValue(0);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  // --- session_start ---

  it('emits session_start when a session first appears', () => {
    const { engine, messages } = createEngine();

    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Racer', activity: 'thinking' },
    ));

    expect(messages[0]).toContain('Racer');
    expect(messages.length).toBe(1);
  });

  it('uses session id when name is missing', () => {
    const { engine, messages } = createEngine();

    engine.processUpdate(sessionsMap(
      { id: 'abc-123', activity: 'thinking' },
    ));

    expect(messages[0]).toContain('abc-123');
  });

  it('does not emit session_start for terminal sessions', () => {
    const { engine, messages } = createEngine();

    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Done', activity: 'complete' },
    ));

    expect(messages.length).toBe(0);
  });

  it('does not re-emit session_start for a known session', () => {
    const { engine, messages } = createEngine();

    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'R', activity: 'thinking' },
    ));
    vi.setSystemTime(baseTime + 6000);
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'R', activity: 'thinking' },
    ));

    const starts = messages.filter(m => m.includes('pulls onto the track'));
    expect(starts.length).toBe(1);
  });

  // --- context utilization milestones ---

  it('emits context_50 when utilization crosses 50%', () => {
    const { engine, messages } = createEngine();

    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'X', activity: 'thinking', contextUtilization: 0.3 },
    ));
    vi.setSystemTime(baseTime + 6000);
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'X', activity: 'thinking', contextUtilization: 0.55 },
    ));

    expect(messages.some(m => m.includes('50%') || m.includes('halfway'))).toBe(true);
  });

  it('emits context_50 only once per session', () => {
    const { engine, messages } = createEngine();

    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'X', activity: 'thinking', contextUtilization: 0.55 },
    ));
    vi.setSystemTime(baseTime + 6000);
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'X', activity: 'thinking', contextUtilization: 0.6 },
    ));

    const ctx50 = messages.filter(m => m.includes('50%') || m.includes('halfway') || m.includes('midway'));
    expect(ctx50.length).toBe(1);
  });

  it('emits context_90 when utilization crosses 90%', () => {
    const { engine, messages } = createEngine();

    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Y', activity: 'thinking', contextUtilization: 0.85 },
    ));
    vi.setSystemTime(baseTime + 6000);
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Y', activity: 'thinking', contextUtilization: 0.92 },
    ));

    expect(messages.some(m => m.includes('90%'))).toBe(true);
  });

  it('emits context_90 only once per session', () => {
    const { engine, messages } = createEngine();

    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Y', activity: 'thinking', contextUtilization: 0.95 },
    ));
    vi.setSystemTime(baseTime + 6000);
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Y', activity: 'thinking', contextUtilization: 0.99 },
    ));

    const ctx90 = messages.filter(m => m.includes('90%'));
    expect(ctx90.length).toBe(1);
  });

  // --- high_burn ---

  it('emits high_burn commentary only once while rate stays high', () => {
    const { engine, messages } = createEngine();

    // First update: seed token tracking (emits session_start)
    vi.setSystemTime(baseTime);
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Burner', activity: 'thinking', tokensUsed: 0 },
    ));

    // Second update: high burn rate (6000 tokens in 6s = 1000 t/s > 500)
    // 6s gap ensures cooldown (5s) has passed so high_burn emits
    vi.setSystemTime(baseTime + 6000);
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Burner', activity: 'thinking', tokensUsed: 6000 },
    ));

    const highBurnMessages = messages.filter(m => m.includes('burning tokens'));
    expect(highBurnMessages.length).toBe(1);

    // Third update: still high burn rate — should NOT fire again
    vi.setSystemTime(baseTime + 12000);
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Burner', activity: 'thinking', tokensUsed: 12000 },
    ));

    const highBurnAfter = messages.filter(m => m.includes('burning tokens'));
    expect(highBurnAfter.length).toBe(1);
  });

  it('re-emits high_burn commentary after rate drops and rises again', () => {
    const { engine, messages } = createEngine();

    // Seed (emits session_start)
    vi.setSystemTime(baseTime);
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Burner', activity: 'thinking', tokensUsed: 0 },
    ));

    // High burn (6000 tokens in 6s = 1000 t/s)
    vi.setSystemTime(baseTime + 6000);
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Burner', activity: 'thinking', tokensUsed: 6000 },
    ));

    // Rate drops (10 tokens in 6s = ~1.7 t/s) — clears dedup flag
    vi.setSystemTime(baseTime + 12000);
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Burner', activity: 'thinking', tokensUsed: 6010 },
    ));

    // Rate spikes again (6000 tokens in 6s = 1000 t/s)
    vi.setSystemTime(baseTime + 18000);
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Burner', activity: 'thinking', tokensUsed: 12010 },
    ));

    const highBurnMessages = messages.filter(m => m.includes('burning tokens'));
    expect(highBurnMessages.length).toBe(2);
  });

  // --- idle_long ---

  it('emits idle_long when session is idle beyond threshold', () => {
    const { engine, messages } = createEngine();
    const idleTimestamp = new Date(baseTime - 70000).toISOString(); // 70s ago

    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Sleepy', activity: 'thinking', lastDataReceivedAt: idleTimestamp },
    ));

    // idle_long (priority 1) is queued behind session_start — flush after cooldown
    vi.setSystemTime(baseTime + 6000);
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Sleepy', activity: 'thinking', lastDataReceivedAt: idleTimestamp },
    ));

    expect(messages.some(m => m.includes('Sleepy') && (m.includes('nap') || m.includes('idle') || m.includes('quiet')))).toBe(true);
  });

  it('emits idle_long only once per idle stretch', () => {
    const { engine, messages } = createEngine();
    const idleTimestamp = new Date(baseTime - 70000).toISOString();

    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Sleepy', activity: 'thinking', lastDataReceivedAt: idleTimestamp },
    ));
    vi.setSystemTime(baseTime + 6000);
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Sleepy', activity: 'thinking', lastDataReceivedAt: idleTimestamp },
    ));

    const idleMessages = messages.filter(m =>
      m.includes('nap') || m.includes('idle') || m.includes('quiet') || m.includes('action'));
    expect(idleMessages.length).toBe(1);
  });

  it('resets idle tracking when session becomes active again', () => {
    const { engine, messages } = createEngine();

    // First: idle for 70s
    const idleTimestamp = new Date(baseTime - 70000).toISOString();
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Sleepy', activity: 'thinking', lastDataReceivedAt: idleTimestamp },
    ));

    // Now active (recent data)
    vi.setSystemTime(baseTime + 6000);
    const freshTimestamp = new Date(baseTime + 5000).toISOString();
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Sleepy', activity: 'thinking', lastDataReceivedAt: freshTimestamp },
    ));

    // Goes idle again
    vi.setSystemTime(baseTime + 80000);
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Sleepy', activity: 'thinking', lastDataReceivedAt: freshTimestamp },
    ));

    const idleMessages = messages.filter(m =>
      m.includes('nap') || m.includes('idle') || m.includes('quiet') || m.includes('action'));
    expect(idleMessages.length).toBe(2);
  });

  // --- subagent_spawn ---

  it('emits subagent spawn commentary when subagents appear', () => {
    const { engine, messages } = createEngine();

    engine.processUpdate(sessionsMap(
      { id: 'session-1', name: 'Speedy', activity: 'thinking', subagents: [] },
    ));

    vi.setSystemTime(baseTime + 6000);

    engine.processUpdate(sessionsMap(
      { id: 'session-1', name: 'Speedy', activity: 'thinking', subagents: [{ id: 'subagent-1' }] },
    ));

    expect(messages).toEqual([
      'And Speedy pulls onto the track! A new challenger approaches!',
      'Speedy deploys a sub-agent! Teamwork makes the dream work!',
    ]);
  });

  it('emits subagent spawn for each new subagent added', () => {
    const { engine, messages } = createEngine();

    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'S', activity: 'thinking', subagents: [{ id: 'a' }] },
    ));
    vi.setSystemTime(baseTime + 6000);
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'S', activity: 'thinking', subagents: [{ id: 'a' }, { id: 'b' }] },
    ));

    const spawnMessages = messages.filter(m => m.includes('sub-agent') || m.includes('hamster') || m.includes('helper'));
    expect(spawnMessages.length).toBe(2);
  });

  // --- overtake ---

  it('emits overtake when a session moves up in rank', () => {
    const { engine, messages } = createEngine();

    // Initial positions: A ahead of B
    engine.processUpdate(sessionsMap(
      { id: 'a', name: 'Alpha', activity: 'thinking', contextUtilization: 0.6 },
      { id: 'b', name: 'Beta', activity: 'thinking', contextUtilization: 0.3 },
    ));

    vi.setSystemTime(baseTime + 6000);

    // B overtakes A
    engine.processUpdate(sessionsMap(
      { id: 'a', name: 'Alpha', activity: 'thinking', contextUtilization: 0.6 },
      { id: 'b', name: 'Beta', activity: 'thinking', contextUtilization: 0.8 },
    ));

    expect(messages.some(m => m.includes('Beta') && m.includes('Alpha'))).toBe(true);
  });

  // --- onCompletion ---

  it('emits completion commentary via onCompletion', () => {
    const { engine, messages } = createEngine();

    engine.onCompletion('s1', 'Winner', 'complete');
    engine.processUpdate(new Map()); // flush

    expect(messages.some(m => m.includes('Winner'))).toBe(true);
  });

  it('emits error commentary for non-complete activity', () => {
    const { engine, messages } = createEngine();

    engine.onCompletion('s1', 'Crasher', 'errored');
    engine.processUpdate(new Map()); // flush

    expect(messages.some(m => m.includes('Crasher'))).toBe(true);
  });

  // --- onToolUse ---

  it('emits tool_use commentary via onToolUse', () => {
    const { engine, messages } = createEngine();

    engine.onToolUse('Builder');
    engine.processUpdate(new Map()); // flush

    expect(messages.some(m => m.includes('Builder'))).toBe(true);
  });

  // --- onCompaction ---

  it('emits compaction commentary via onCompaction', () => {
    const { engine, messages } = createEngine();

    engine.onCompaction('Compactor');
    engine.processUpdate(new Map()); // flush

    expect(messages.some(m => m.includes('Compactor'))).toBe(true);
  });

  // --- getCurrentMessage / clearMessage ---

  it('getCurrentMessage returns the last emitted message', () => {
    const { engine } = createEngine();

    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'R', activity: 'thinking' },
    ));

    expect(engine.getCurrentMessage()).toContain('R');
  });

  it('clearMessage resets current message to null', () => {
    const { engine } = createEngine();

    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'R', activity: 'thinking' },
    ));
    engine.clearMessage();

    expect(engine.getCurrentMessage()).toBeNull();
  });

  // --- cooldown ---

  it('enforces cooldown between low-priority events', () => {
    const { engine, messages } = createEngine();

    // First event emits immediately
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'A', activity: 'thinking' },
    ));
    expect(messages.length).toBe(1);

    // Second event within cooldown (5s) — queued but not emitted
    vi.setSystemTime(baseTime + 2000);
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'A', activity: 'thinking' },
      { id: 's2', name: 'B', activity: 'thinking' },
    ));
    expect(messages.length).toBe(1);

    // After cooldown — emits
    vi.setSystemTime(baseTime + 6000);
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'A', activity: 'thinking' },
      { id: 's2', name: 'B', activity: 'thinking' },
    ));
    expect(messages.length).toBe(2);
  });

  it('high-priority events bypass cooldown', () => {
    const { engine, messages } = createEngine();

    // First event
    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'A', activity: 'thinking' },
    ));
    expect(messages.length).toBe(1);

    // Completion (priority 10) within cooldown — should bypass
    vi.setSystemTime(baseTime + 1000);
    engine.onCompletion('s1', 'A', 'complete');
    engine.processUpdate(new Map());
    expect(messages.length).toBe(2);
  });

  // --- queue management ---

  it('caps the queue at 10 items', () => {
    const { engine } = createEngine();
    engine._lastEmitTime = baseTime; // pretend we just emitted

    for (let i = 0; i < 15; i++) {
      engine.onToolUse('Tool' + i);
    }

    expect(engine._queue.length).toBe(10);
  });

  it('sorts queue by priority descending, then time ascending', () => {
    const { engine } = createEngine();
    engine._lastEmitTime = baseTime; // block flush

    engine.onToolUse('LowPrio');        // priority 2
    engine.onCompaction('MedPrio');      // priority 5
    engine.onCompletion('x', 'Hi', 'complete'); // priority 10

    expect(engine._queue[0].priority).toBe(10);
    expect(engine._queue[1].priority).toBe(5);
    expect(engine._queue[2].priority).toBe(2);
  });

  // --- cleanup ---

  it('cleans up tracking state when a session is removed', () => {
    const { engine } = createEngine();

    engine.processUpdate(sessionsMap(
      { id: 's1', name: 'Gone', activity: 'thinking', contextUtilization: 0.55 },
    ));

    // Session disappears from the map
    vi.setSystemTime(baseTime + 6000);
    engine.processUpdate(new Map());

    // Internal tracking should be cleaned up
    expect(engine._knownSessions.has('s1')).toBe(false);
    expect(engine._crossedContext50.has('s1')).toBe(false);
    expect(engine._prevPositions.has('s1')).toBe(false);
    expect(engine._prevTokens.has('s1')).toBe(false);
  });
});
