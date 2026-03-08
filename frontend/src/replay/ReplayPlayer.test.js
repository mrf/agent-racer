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
});
