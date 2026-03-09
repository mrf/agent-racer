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
    this.listReplays = vi.fn().mockResolvedValue([
      {
        id: 'replay-1',
        name: 'race-1.ndjson',
        size: 1024,
        createdAt: '2026-03-09T10:00:00Z',
      },
    ]);
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
  let onClose;

  beforeEach(() => {
    document.body.innerHTML = '';
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({
      clearRect: vi.fn(),
      drawImage: vi.fn(),
      fillRect: vi.fn(),
    });
    player = new FakeReplayPlayer();
    onClose = vi.fn();
    scrubber = new TimelineScrubber(player, onClose);
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

  it('adds dialog semantics to the selector modal', async () => {
    await scrubber.open();

    const selector = scrubber._selectorEl;
    expect(selector.getAttribute('role')).toBe('dialog');
    expect(selector.getAttribute('aria-modal')).toBe('true');
    expect(selector.getAttribute('aria-label')).toBe('Replay selector');
    expect(selector.querySelector('button').getAttribute('aria-label')).toBe('Close replay selector');
  });

  it('focuses the close button on open and restores prior focus on close', async () => {
    const opener = document.createElement('button');
    document.body.appendChild(opener);
    opener.focus();

    await scrubber.open();
    expect(document.activeElement).toBe(scrubber._selectorEl.querySelector('button'));

    scrubber.close();
    expect(document.activeElement).toBe(opener);
  });

  it('traps focus inside the selector on Tab and Shift+Tab', async () => {
    await scrubber.open();

    const focusable = scrubber._getSelectorFocusable();
    expect(focusable.length).toBeGreaterThan(1);

    focusable[focusable.length - 1].focus();
    const tabEvent = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true });
    let prevented = false;
    tabEvent.preventDefault = () => { prevented = true; };
    scrubber._selectorEl.dispatchEvent(tabEvent);

    expect(prevented).toBe(true);
    expect(document.activeElement).toBe(focusable[0]);

    focusable[0].focus();
    const shiftTabEvent = new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true });
    let shiftPrevented = false;
    shiftTabEvent.preventDefault = () => { shiftPrevented = true; };
    scrubber._selectorEl.dispatchEvent(shiftTabEvent);

    expect(shiftPrevented).toBe(true);
    expect(document.activeElement).toBe(focusable[focusable.length - 1]);
  });

  it('closes the selector on Escape', async () => {
    await scrubber.open();

    scrubber._selectorEl.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));

    expect(scrubber._selectorEl).toBeNull();
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('shows replay load failures in the playback bar', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    player.loadReplay.mockRejectedValue(new Error('Replay contains no valid snapshots. Skipped 2 malformed replay lines at lines 1, 2.'));

    await scrubber._selectReplay({ id: 'broken', name: 'broken.ndjson' });

    expect(consoleError).toHaveBeenCalledWith('Failed to load replay:', expect.any(Error));
    expect(document.body.textContent).toContain('broken.ndjson');
    expect(document.body.textContent).toContain('Failed to load replay: Replay contains no valid snapshots. Skipped 2 malformed replay lines at lines 1, 2.');
  });

  it('adds aria-labels to replay playback controls', () => {
    scrubber._buildBar();

    const labels = Array.from(document.querySelectorAll('button'))
      .map((button) => button.getAttribute('aria-label'));

    expect(labels).toContain('Step backward one replay frame');
    expect(labels).toContain('Play replay');
    expect(labels).toContain('Step forward one replay frame');
    expect(labels).toContain('Exit replay mode');
    expect(labels).toContain('Set replay speed to 1x');
    expect(labels).toContain('Set replay speed to 2x');
    expect(labels).toContain('Set replay speed to 4x');
    expect(scrubber._slider.getAttribute('aria-label')).toBe('Replay timeline position');
  });

  it('updates the play button aria-label when playback state changes', () => {
    scrubber._buildBar();

    player.onPlayStateChange(true);
    expect(scrubber._playBtn.getAttribute('aria-label')).toBe('Pause replay');

    player.onPlayStateChange(false);
    expect(scrubber._playBtn.getAttribute('aria-label')).toBe('Play replay');
  });
});
