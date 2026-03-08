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
