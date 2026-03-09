// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { TimelineScrubber } from './TimelineScrubber.js';

class FakeReplayPlayer {
  constructor() {
    this.snapshots = [];
    this.isPlaying = false;
    this.onSeek = null;
    this.onPlayStateChange = null;
    this.onLoaded = null;
    this.onWarning = null;
    this.loadReplay = vi.fn();
  }

  stop() {
    this.isPlaying = false;
  }

  pause() {
    this.isPlaying = false;
  }

  play() {
    this.isPlaying = true;
  }

  stepBackward() {}

  stepForward() {}

  seek() {}

  setSpeed() {}
}

describe('TimelineScrubber', () => {
  let player;
  let scrubber;

  beforeEach(() => {
    document.body.innerHTML = '';
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({
      clearRect: vi.fn(),
      drawImage: vi.fn(),
      fillRect: vi.fn(),
    });
    player = new FakeReplayPlayer();
    scrubber = new TimelineScrubber(player, vi.fn());
  });

  afterEach(() => {
    scrubber?.close();
    scrubber = null;
    vi.restoreAllMocks();
  });

  it('shows malformed replay warnings in the playback bar', () => {
    scrubber._buildBar();

    player.onWarning('Skipped 1 malformed replay line at line 2.');

    expect(document.body.textContent).toContain('Skipped 1 malformed replay line at line 2.');
  });

  it('shows replay load failures in the playback bar', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    player.loadReplay.mockRejectedValue(new Error('Replay contains no valid snapshots. Skipped 2 malformed replay lines at lines 1, 2.'));

    await scrubber._selectReplay({ id: 'broken', name: 'broken.ndjson' });

    expect(consoleError).toHaveBeenCalledWith('Failed to load replay:', expect.any(Error));
    expect(document.body.textContent).toContain('broken.ndjson');
    expect(document.body.textContent).toContain('Failed to load replay: Replay contains no valid snapshots. Skipped 2 malformed replay lines at lines 1, 2.');
  });
});
