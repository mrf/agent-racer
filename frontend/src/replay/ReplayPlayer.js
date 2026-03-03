import { authFetch } from '../auth.js';

/**
 * ReplayPlayer manages loading and playback of a recorded session replay.
 *
 * It fetches replay data from the REST API, parses the NDJSON snapshot stream,
 * and drives playback by calling onSnapshot with each frame's session list at
 * the appropriate time interval.
 */
export class ReplayPlayer {
  constructor() {
    /** @type {Array<{t: Date, s: Array}>} */
    this.snapshots = [];
    this.currentIndex = 0;
    this.isPlaying = false;
    this.speed = 1; // 1, 2, or 4
    this._timer = null;

    // Callbacks — set by consumers before calling loadReplay/play.
    /** @type {function(Array): void} */
    this.onSnapshot = null;
    /** @type {function(number, number): void} called with (index, total) */
    this.onSeek = null;
    /** @type {function(boolean): void} */
    this.onPlayStateChange = null;
    /** @type {function(string, string, number): void} called with (id, name, total) */
    this.onLoaded = null;
  }

  /** List available replay files from the server. */
  async listReplays() {
    const res = await authFetch('/api/replays');
    if (!res.ok) throw new Error(`Failed to list replays: ${res.status}`);
    return res.json();
  }

  /**
   * Load a replay by ID from the server.
   * Stops any current playback, fetches and parses the NDJSON file,
   * then calls onLoaded and emits the first frame.
   */
  async loadReplay(id) {
    this.stop();
    this.snapshots = [];
    this.currentIndex = 0;

    const res = await authFetch(`/api/replays/${encodeURIComponent(id)}`);
    if (!res.ok) throw new Error(`Failed to load replay: ${res.status}`);

    const text = await res.text();
    this.snapshots = text
      .split('\n')
      .filter(line => line.trim())
      .map(line => {
        const obj = JSON.parse(line);
        return { t: new Date(obj.t), s: obj.s || [] };
      });

    this.onLoaded && this.onLoaded(id, id, this.snapshots.length);
    this._emit(0);
  }

  /** Start or resume playback from the current position. */
  play() {
    if (this.isPlaying) return;
    if (this.currentIndex >= this.snapshots.length - 1) {
      this.currentIndex = 0;
    }
    this.isPlaying = true;
    this.onPlayStateChange && this.onPlayStateChange(true);
    this._scheduleNext();
  }

  /** Pause playback at the current position. */
  pause() {
    this.isPlaying = false;
    if (this._timer !== null) {
      clearTimeout(this._timer);
      this._timer = null;
    }
    this.onPlayStateChange && this.onPlayStateChange(false);
  }

  /** Stop playback and reset to the beginning. */
  stop() {
    this.pause();
    this.currentIndex = 0;
  }

  /** Seek to an absolute snapshot index and emit that frame. */
  seek(index) {
    const i = Math.max(0, Math.min(index, this.snapshots.length - 1));
    this.currentIndex = i;
    this._emit(i);
  }

  stepForward() {
    this.seek(this.currentIndex + 1);
  }

  stepBackward() {
    this.seek(this.currentIndex - 1);
  }

  /** Set playback speed. Restarts the internal timer if currently playing. */
  setSpeed(speed) {
    this.speed = speed;
    if (this.isPlaying) {
      if (this._timer !== null) clearTimeout(this._timer);
      this._scheduleNext();
    }
  }

  // ---- private ----

  _scheduleNext() {
    if (!this.isPlaying || this.currentIndex >= this.snapshots.length - 1) {
      this.pause();
      return;
    }

    // Use actual timestamps between snapshots when available; fall back to 1s.
    const curr = this.snapshots[this.currentIndex];
    const next = this.snapshots[this.currentIndex + 1];
    let delay = 1000;
    if (curr && next) {
      delay = next.t - curr.t;
    }
    delay = Math.max(50, delay / this.speed);

    this._timer = setTimeout(() => {
      if (!this.isPlaying) return;
      this.currentIndex++;
      this._emit(this.currentIndex);
      this._scheduleNext();
    }, delay);
  }

  _emit(index) {
    if (index < 0 || index >= this.snapshots.length) return;
    const snap = this.snapshots[index];
    this.onSnapshot && this.onSnapshot(snap.s);
    this.onSeek && this.onSeek(index, this.snapshots.length);
  }
}
