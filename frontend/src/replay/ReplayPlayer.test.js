import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  authFetch: vi.fn(),
}));

vi.mock('../auth.js', () => ({
  authFetch: mocks.authFetch,
}));

import { ReplayPlayer } from './ReplayPlayer.js';

function createChunkedStream(chunks) {
  const encoder = new TextEncoder();

  return new ReadableStream({
    start(controller) {
      for (let i = 0; i < chunks.length; i++) {
        controller.enqueue(encoder.encode(chunks[i]));
      }
      controller.close();
    },
  });
}

describe('ReplayPlayer', () => {
  beforeEach(() => {
    mocks.authFetch.mockReset();
  });

  it('streams NDJSON snapshots without buffering the whole replay as text', async () => {
    const text = vi.fn(async () => {
      throw new Error('loadReplay should not call response.text when a stream is available');
    });

    mocks.authFetch.mockResolvedValue({
      ok: true,
      body: createChunkedStream([
        '{"t":"2026-03-07T12:00:00Z","s":[{"id":"alpha"}]}\n{"t":"2026-03-07T12:00:01',
        'Z","s":[{"id":"beta"}]}\r\n',
        '\n{"t":"2026-03-07T12:00:02Z","s":[]}',
      ]),
      text,
    });

    const player = new ReplayPlayer();
    player.onLoaded = vi.fn();
    player.onSnapshot = vi.fn();
    player.onSeek = vi.fn();

    await player.loadReplay('replay-123');

    expect(text).not.toHaveBeenCalled();
    expect(player.snapshots).toHaveLength(3);
    expect(player.snapshots[0].t.toISOString()).toBe('2026-03-07T12:00:00.000Z');
    expect(player.snapshots[0].s).toEqual([{ id: 'alpha' }]);
    expect(player.snapshots[1].t.toISOString()).toBe('2026-03-07T12:00:01.000Z');
    expect(player.snapshots[1].s).toEqual([{ id: 'beta' }]);
    expect(player.snapshots[2].s).toEqual([]);
    expect(player.onLoaded).toHaveBeenCalledWith('replay-123', 'replay-123', 3);
    expect(player.onSnapshot).toHaveBeenCalledWith([{ id: 'alpha' }]);
    expect(player.onSeek).toHaveBeenCalledWith(0, 3);
  });

  it('skips malformed replay lines in text responses and reports a warning', async () => {
    mocks.authFetch.mockResolvedValue({
      ok: true,
      body: null,
      text: vi.fn(async () => [
        '{"t":"2026-03-07T12:00:00Z","s":[{"id":"alpha"}]}',
        '{bad json',
        '{"t":"2026-03-07T12:00:02Z","s":[{"id":"gamma"}]}',
      ].join('\n')),
    });

    const player = new ReplayPlayer();
    player.onLoaded = vi.fn();
    player.onSnapshot = vi.fn();
    player.onSeek = vi.fn();
    player.onWarning = vi.fn();

    await player.loadReplay('replay-456');

    expect(player.snapshots).toHaveLength(2);
    expect(player.snapshots[0].s).toEqual([{ id: 'alpha' }]);
    expect(player.snapshots[1].s).toEqual([{ id: 'gamma' }]);
    expect(player.onWarning).toHaveBeenCalledWith('Skipped 1 malformed replay line at line 2.');
    expect(player.onLoaded).toHaveBeenCalledWith('replay-456', 'replay-456', 2);
    expect(player.onSnapshot).toHaveBeenCalledWith([{ id: 'alpha' }]);
    expect(player.onSeek).toHaveBeenCalledWith(0, 2);
  });

  it('throws a replay error when all replay lines are malformed', async () => {
    mocks.authFetch.mockResolvedValue({
      ok: true,
      body: null,
      text: vi.fn(async () => ['{bad json', '{"t":'].join('\n')),
    });

    const player = new ReplayPlayer();
    player.onWarning = vi.fn();

    await expect(player.loadReplay('broken-replay')).rejects.toThrow(
      'Replay contains no valid snapshots. Skipped 2 malformed replay lines at lines 1, 2.'
    );
    expect(player.onWarning).not.toHaveBeenCalled();
  });
});
