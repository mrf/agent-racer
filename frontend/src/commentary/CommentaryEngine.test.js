import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { CommentaryEngine } from './CommentaryEngine.js';

describe('CommentaryEngine', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-03-07T12:00:00Z'));
    vi.spyOn(Math, 'random').mockReturnValue(0);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  it('emits high_burn commentary only once while rate stays high', () => {
    const engine = new CommentaryEngine();
    const messages = [];
    engine.onMessage = (message) => messages.push(message);

    const baseTime = new Date('2026-03-07T12:00:00Z').getTime();

    // First update: seed token tracking (emits session_start)
    vi.setSystemTime(baseTime);
    engine.processUpdate(new Map([
      ['s1', { id: 's1', name: 'Burner', activity: 'thinking', tokensUsed: 0 }],
    ]));

    // Second update: high burn rate (6000 tokens in 6s = 1000 t/s > 500)
    // 6s gap ensures cooldown (5s) has passed so high_burn emits
    vi.setSystemTime(baseTime + 6000);
    engine.processUpdate(new Map([
      ['s1', { id: 's1', name: 'Burner', activity: 'thinking', tokensUsed: 6000 }],
    ]));

    const highBurnMessages = messages.filter(m => m.includes('burning tokens'));
    expect(highBurnMessages.length).toBe(1);

    // Third update: still high burn rate — should NOT fire again
    vi.setSystemTime(baseTime + 12000);
    engine.processUpdate(new Map([
      ['s1', { id: 's1', name: 'Burner', activity: 'thinking', tokensUsed: 12000 }],
    ]));

    const highBurnAfter = messages.filter(m => m.includes('burning tokens'));
    expect(highBurnAfter.length).toBe(1);
  });

  it('re-emits high_burn commentary after rate drops and rises again', () => {
    const engine = new CommentaryEngine();
    const messages = [];
    engine.onMessage = (message) => messages.push(message);

    const baseTime = new Date('2026-03-07T12:00:00Z').getTime();

    // Seed (emits session_start)
    vi.setSystemTime(baseTime);
    engine.processUpdate(new Map([
      ['s1', { id: 's1', name: 'Burner', activity: 'thinking', tokensUsed: 0 }],
    ]));

    // High burn (6000 tokens in 6s = 1000 t/s)
    vi.setSystemTime(baseTime + 6000);
    engine.processUpdate(new Map([
      ['s1', { id: 's1', name: 'Burner', activity: 'thinking', tokensUsed: 6000 }],
    ]));

    // Rate drops (10 tokens in 6s = ~1.7 t/s) — clears dedup flag
    vi.setSystemTime(baseTime + 12000);
    engine.processUpdate(new Map([
      ['s1', { id: 's1', name: 'Burner', activity: 'thinking', tokensUsed: 6010 }],
    ]));

    // Rate spikes again (6000 tokens in 6s = 1000 t/s)
    vi.setSystemTime(baseTime + 18000);
    engine.processUpdate(new Map([
      ['s1', { id: 's1', name: 'Burner', activity: 'thinking', tokensUsed: 12010 }],
    ]));

    const highBurnMessages = messages.filter(m => m.includes('burning tokens'));
    expect(highBurnMessages.length).toBe(2);
  });

  it('emits subagent spawn commentary when lowercase subagents appear', () => {
    const engine = new CommentaryEngine();
    const messages = [];

    engine.onMessage = (message) => messages.push(message);

    engine.processUpdate(new Map([
      ['session-1', {
        id: 'session-1',
        name: 'Speedy',
        activity: 'thinking',
        subagents: [],
      }],
    ]));

    vi.setSystemTime(new Date('2026-03-07T12:00:06Z'));

    engine.processUpdate(new Map([
      ['session-1', {
        id: 'session-1',
        name: 'Speedy',
        activity: 'thinking',
        subagents: [{ id: 'subagent-1' }],
      }],
    ]));

    expect(messages).toEqual([
      'And Speedy pulls onto the track! A new challenger approaches!',
      'Speedy deploys a sub-agent! Teamwork makes the dream work!',
    ]);
  });
});
