import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

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

/** Build a mock text-only response (no streaming body). */
function textResponse(lines) {
  return {
    ok: true,
    body: null,
    text: vi.fn(async () => lines.join('\n')),
  };
}

/** Three snapshots spaced 1 s apart — reusable across playback tests. */
const THREE_SNAPSHOT_LINES = [
  '{"t":"2026-03-07T12:00:00Z","s":[{"id":"a"}]}',
  '{"t":"2026-03-07T12:00:01Z","s":[{"id":"b"}]}',
  '{"t":"2026-03-07T12:00:02Z","s":[{"id":"c"}]}',
];

/** Load the standard three-snapshot replay into a fresh player. */
async function loadThreeSnapshots() {
  mocks.authFetch.mockResolvedValue(textResponse(THREE_SNAPSHOT_LINES));
  const player = new ReplayPlayer();
  player.onSnapshot = vi.fn();
  player.onSeek = vi.fn();
  player.onLoaded = vi.fn();
  player.onPlayStateChange = vi.fn();
  player.onWarning = vi.fn();
  await player.loadReplay('test');
  // Clear the calls from loadReplay's initial _emit(0).
  player.onSnapshot.mockClear();
  player.onSeek.mockClear();
  return player;
}

describe('ReplayPlayer', () => {
  beforeEach(() => {
    mocks.authFetch.mockReset();
  });

  // ---------- loading / parsing ----------

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

  it('throws when the HTTP response is not ok', async () => {
    mocks.authFetch.mockResolvedValue({ ok: false, status: 404 });

    const player = new ReplayPlayer();
    await expect(player.loadReplay('missing')).rejects.toThrow('Failed to load replay: 404');
  });

  it('treats missing s field as an empty session list', async () => {
    mocks.authFetch.mockResolvedValue(textResponse([
      '{"t":"2026-03-07T12:00:00Z"}',
    ]));

    const player = new ReplayPlayer();
    player.onSnapshot = vi.fn();
    player.onSeek = vi.fn();
    player.onLoaded = vi.fn();
    await player.loadReplay('no-s');

    expect(player.snapshots[0].s).toEqual([]);
    expect(player.onSnapshot).toHaveBeenCalledWith([]);
  });

  // ---------- listReplays ----------

  it('listReplays returns parsed JSON on success', async () => {
    const replays = [{ id: 'r1' }, { id: 'r2' }];
    mocks.authFetch.mockResolvedValue({
      ok: true,
      json: vi.fn(async () => replays),
    });

    const player = new ReplayPlayer();
    const result = await player.listReplays();

    expect(mocks.authFetch).toHaveBeenCalledWith('/api/replays');
    expect(result).toEqual(replays);
  });

  it('listReplays throws on non-ok response', async () => {
    mocks.authFetch.mockResolvedValue({ ok: false, status: 500 });

    const player = new ReplayPlayer();
    await expect(player.listReplays()).rejects.toThrow('Failed to list replays: 500');
  });

  // ---------- play / pause / stop ----------

  describe('play / pause / stop', () => {
    beforeEach(() => {
      vi.useFakeTimers();
    });
    afterEach(() => {
      vi.useRealTimers();
    });

    it('play sets isPlaying and fires onPlayStateChange', async () => {
      const player = await loadThreeSnapshots();

      player.play();

      expect(player.isPlaying).toBe(true);
      expect(player.onPlayStateChange).toHaveBeenCalledWith(true);
    });

    it('play is a no-op when already playing', async () => {
      const player = await loadThreeSnapshots();

      player.play();
      player.onPlayStateChange.mockClear();
      player.play(); // second call — should be ignored

      expect(player.onPlayStateChange).not.toHaveBeenCalled();
    });

    it('pause stops playback and fires onPlayStateChange(false)', async () => {
      const player = await loadThreeSnapshots();

      player.play();
      player.onPlayStateChange.mockClear();
      player.pause();

      expect(player.isPlaying).toBe(false);
      expect(player.onPlayStateChange).toHaveBeenCalledWith(false);
    });

    it('stop resets currentIndex to 0 and pauses', async () => {
      const player = await loadThreeSnapshots();

      player.play();
      vi.advanceTimersByTime(1000); // advance to index 1
      player.stop();

      expect(player.isPlaying).toBe(false);
      expect(player.currentIndex).toBe(0);
    });

    it('play advances through snapshots with correct timing', async () => {
      const player = await loadThreeSnapshots();

      player.play();

      // Snapshot timestamps are 1 s apart → delay = 1000 ms at 1x speed.
      vi.advanceTimersByTime(1000);
      expect(player.onSnapshot).toHaveBeenCalledWith([{ id: 'b' }]);
      expect(player.onSeek).toHaveBeenCalledWith(1, 3);

      vi.advanceTimersByTime(1000);
      expect(player.onSnapshot).toHaveBeenCalledWith([{ id: 'c' }]);
      expect(player.onSeek).toHaveBeenCalledWith(2, 3);
    });

    it('pauses automatically at the last snapshot', async () => {
      const player = await loadThreeSnapshots();

      player.play();
      vi.advanceTimersByTime(2000); // reach end
      player.onPlayStateChange.mockClear();

      // One more tick should not fire anything — already paused at end.
      vi.advanceTimersByTime(1000);
      expect(player.isPlaying).toBe(false);
      expect(player.onPlayStateChange).not.toHaveBeenCalled();
    });

    it('play wraps to the beginning when called at end of replay', async () => {
      const player = await loadThreeSnapshots();

      player.play();
      vi.advanceTimersByTime(2000); // reach end (index 2)
      expect(player.isPlaying).toBe(false);

      player.onPlayStateChange.mockClear();
      player.play();

      expect(player.currentIndex).toBe(0);
      expect(player.isPlaying).toBe(true);
      expect(player.onPlayStateChange).toHaveBeenCalledWith(true);
    });

    it('loadReplay stops current playback', async () => {
      const player = await loadThreeSnapshots();

      player.play();
      expect(player.isPlaying).toBe(true);

      // Load a new replay — should stop playback first.
      mocks.authFetch.mockResolvedValue(textResponse(THREE_SNAPSHOT_LINES));
      await player.loadReplay('new-replay');

      expect(player.isPlaying).toBe(false);
      expect(player.currentIndex).toBe(0);
    });
  });

  // ---------- seek / step ----------

  describe('seek', () => {
    it('seeks to a valid index and emits the frame', async () => {
      const player = await loadThreeSnapshots();

      player.seek(2);

      expect(player.currentIndex).toBe(2);
      expect(player.onSnapshot).toHaveBeenCalledWith([{ id: 'c' }]);
      expect(player.onSeek).toHaveBeenCalledWith(2, 3);
    });

    it('clamps seek to lower bound', async () => {
      const player = await loadThreeSnapshots();

      player.seek(-5);

      expect(player.currentIndex).toBe(0);
      expect(player.onSnapshot).toHaveBeenCalledWith([{ id: 'a' }]);
    });

    it('clamps seek to upper bound', async () => {
      const player = await loadThreeSnapshots();

      player.seek(999);

      expect(player.currentIndex).toBe(2);
      expect(player.onSnapshot).toHaveBeenCalledWith([{ id: 'c' }]);
    });

    it('stepForward advances by one', async () => {
      const player = await loadThreeSnapshots();
      expect(player.currentIndex).toBe(0);

      player.stepForward();

      expect(player.currentIndex).toBe(1);
      expect(player.onSnapshot).toHaveBeenCalledWith([{ id: 'b' }]);
    });

    it('stepBackward moves back by one', async () => {
      const player = await loadThreeSnapshots();
      player.seek(2);
      player.onSnapshot.mockClear();

      player.stepBackward();

      expect(player.currentIndex).toBe(1);
      expect(player.onSnapshot).toHaveBeenCalledWith([{ id: 'b' }]);
    });

    it('stepForward clamps at end', async () => {
      const player = await loadThreeSnapshots();
      player.seek(2);
      player.onSnapshot.mockClear();

      player.stepForward();

      expect(player.currentIndex).toBe(2); // unchanged
    });

    it('stepBackward clamps at start', async () => {
      const player = await loadThreeSnapshots();

      player.stepBackward();

      expect(player.currentIndex).toBe(0); // unchanged
    });
  });

  // ---------- speed ----------

  describe('speed', () => {
    beforeEach(() => {
      vi.useFakeTimers();
    });
    afterEach(() => {
      vi.useRealTimers();
    });

    it('2x speed halves the delay between snapshots', async () => {
      const player = await loadThreeSnapshots();
      player.speed = 2;

      player.play();

      // 1000 ms real gap / 2x = 500 ms timer delay
      vi.advanceTimersByTime(499);
      expect(player.onSnapshot).not.toHaveBeenCalled();

      vi.advanceTimersByTime(1);
      expect(player.onSnapshot).toHaveBeenCalledWith([{ id: 'b' }]);
    });

    it('4x speed quarters the delay between snapshots', async () => {
      const player = await loadThreeSnapshots();
      player.speed = 4;

      player.play();

      // 1000 ms / 4 = 250 ms
      vi.advanceTimersByTime(250);
      expect(player.onSnapshot).toHaveBeenCalledWith([{ id: 'b' }]);
    });

    it('setSpeed during playback reschedules the timer', async () => {
      const player = await loadThreeSnapshots();

      player.play(); // 1x → 1000 ms delay
      vi.advanceTimersByTime(100); // 100 ms into the 1000 ms wait

      player.setSpeed(4); // reschedule: 1000/4 = 250 ms from now
      player.onSnapshot.mockClear();

      vi.advanceTimersByTime(249);
      expect(player.onSnapshot).not.toHaveBeenCalled();

      vi.advanceTimersByTime(1);
      expect(player.onSnapshot).toHaveBeenCalledWith([{ id: 'b' }]);
    });

    it('enforces a minimum 50 ms delay even at high speed', async () => {
      // Snapshots 10 ms apart — at 4x that would be 2.5 ms, but floor is 50.
      mocks.authFetch.mockResolvedValue(textResponse([
        '{"t":"2026-03-07T12:00:00.000Z","s":[{"id":"x"}]}',
        '{"t":"2026-03-07T12:00:00.010Z","s":[{"id":"y"}]}',
      ]));
      const player = new ReplayPlayer();
      player.onSnapshot = vi.fn();
      player.onSeek = vi.fn();
      player.onLoaded = vi.fn();
      player.onPlayStateChange = vi.fn();
      await player.loadReplay('fast');
      player.onSnapshot.mockClear();

      player.speed = 4;
      player.play();

      vi.advanceTimersByTime(49);
      expect(player.onSnapshot).not.toHaveBeenCalled();

      vi.advanceTimersByTime(1);
      expect(player.onSnapshot).toHaveBeenCalledWith([{ id: 'y' }]);
    });

    it('setSpeed is a no-op when not playing', async () => {
      const player = await loadThreeSnapshots();

      player.setSpeed(4);

      expect(player.speed).toBe(4);
      expect(player.isPlaying).toBe(false);
    });
  });
});
