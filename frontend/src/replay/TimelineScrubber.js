import { ReplayPlayer } from './ReplayPlayer.js';

const TERMINAL = new Set(['complete', 'errored', 'lost']);

/**
 * TimelineScrubber combines a replay-file selector with a playback control bar.
 *
 * States:
 *  - "select": shows a floating list of available replay files.
 *  - "playing": hides the selector and shows a bottom bar with scrubber controls.
 *
 * Usage:
 *   const scrubber = new TimelineScrubber(player, onClose);
 *   scrubber.open();   // shows selector
 *   scrubber.close();  // cleans up DOM and stops player
 */
export class TimelineScrubber {
  /**
   * @param {ReplayPlayer} player
   * @param {function(): void} onClose  called when the user exits replay mode
   */
  constructor(player, onClose) {
    this._player = player;
    this._onClose = onClose;

    this._selectorEl = null;
    this._barEl = null;
    this._heatmapCtx = null;
    this._heatmapOffscreen = null;
    this._snapshots = [];
    this._playBtn = null;
    this._slider = null;
    this._nameEl = null;
    this._timeEl = null;
    this._speedBtns = [];

    // Wire player callbacks.
    player.onSeek = (index, total) => this._onSeek(index, total);
    player.onPlayStateChange = (playing) => this._onPlayState(playing);
    player.onLoaded = (id, name, total) => this._onLoaded(id, name, total, player.snapshots);
  }

  /** Show the replay file selector. */
  async open() {
    this._buildSelector();
    try {
      const replays = await this._player.listReplays();
      this._populateSelector(replays);
    } catch (err) {
      this._populateSelectorError(err.message);
    }
  }

  /** Destroy all DOM elements and stop playback. */
  close() {
    this._player.stop();
    this._removeSelector();
    this._removeBar();
    this._onClose && this._onClose();
  }

  // ---- Selector ----

  _buildSelector() {
    this._selectorEl = document.createElement('div');
    this._selectorEl.style.cssText = [
      'position:fixed',
      'top:50%',
      'left:50%',
      'transform:translate(-50%,-50%)',
      'background:rgba(10,10,20,0.95)',
      'border:1px solid rgba(100,160,255,0.4)',
      'border-radius:8px',
      'padding:20px',
      'z-index:9100',
      'min-width:380px',
      'max-width:560px',
      'max-height:70vh',
      'display:flex',
      'flex-direction:column',
      'gap:12px',
      'box-shadow:0 8px 32px rgba(0,0,0,0.6)',
      'color:#e8eaf0',
      'font-family:monospace',
    ].join(';');

    const header = document.createElement('div');
    header.style.cssText = 'display:flex;align-items:center;gap:8px;';

    const title = document.createElement('span');
    title.style.cssText = 'font-size:14px;font-weight:bold;color:#8cf;flex:1;';
    title.textContent = '\u23FA Replay Mode — Select Recording';

    const closeBtn = this._btn('✕', () => this.close());
    closeBtn.style.cssText = 'background:rgba(255,80,80,0.2);border:1px solid rgba(255,80,80,0.5);color:#faa;padding:2px 8px;cursor:pointer;font-size:12px;border-radius:3px;';

    header.appendChild(title);
    header.appendChild(closeBtn);

    this._listEl = document.createElement('div');
    this._listEl.style.cssText = 'overflow-y:auto;display:flex;flex-direction:column;gap:6px;';
    this._listEl.textContent = 'Loading replays…';

    this._selectorEl.appendChild(header);
    this._selectorEl.appendChild(this._listEl);
    document.body.appendChild(this._selectorEl);
  }

  _populateSelector(replays) {
    this._listEl.textContent = '';
    if (!replays || replays.length === 0) {
      const empty = document.createElement('div');
      empty.style.cssText = 'color:#888;font-size:12px;padding:8px;';
      empty.textContent = 'No replays found. Recordings are saved every session.';
      this._listEl.appendChild(empty);
      return;
    }

    for (const r of replays) {
      const item = document.createElement('button');
      item.style.cssText = [
        'display:flex',
        'align-items:center',
        'gap:10px',
        'background:rgba(255,255,255,0.05)',
        'border:1px solid rgba(255,255,255,0.1)',
        'border-radius:4px',
        'padding:8px 12px',
        'cursor:pointer',
        'color:#ccd',
        'font-family:monospace',
        'font-size:12px',
        'text-align:left',
        'transition:background 0.15s',
      ].join(';');

      item.onmouseenter = () => { item.style.background = 'rgba(68,170,255,0.15)'; };
      item.onmouseleave = () => { item.style.background = 'rgba(255,255,255,0.05)'; };

      const nameSpan = document.createElement('span');
      nameSpan.style.flex = '1';
      nameSpan.textContent = r.name;

      const sizeSpan = document.createElement('span');
      sizeSpan.style.color = '#888';
      sizeSpan.textContent = _formatSize(r.size);

      const dateSpan = document.createElement('span');
      dateSpan.style.color = '#aaa';
      dateSpan.textContent = new Date(r.createdAt).toLocaleString();

      item.appendChild(nameSpan);
      item.appendChild(sizeSpan);
      item.appendChild(dateSpan);
      item.onclick = () => this._selectReplay(r);

      this._listEl.appendChild(item);
    }
  }

  _populateSelectorError(msg) {
    this._listEl.textContent = '';
    const err = document.createElement('div');
    err.style.cssText = 'color:#f88;font-size:12px;padding:8px;';
    err.textContent = 'Error loading replays: ' + msg;
    this._listEl.appendChild(err);
  }

  async _selectReplay(replay) {
    this._removeSelector();
    this._buildBar();

    try {
      await this._player.loadReplay(replay.id);
      this._player.play();
    } catch (err) {
      console.error('Failed to load replay:', err);
    }
  }

  _removeSelector() {
    if (this._selectorEl) {
      this._selectorEl.remove();
      this._selectorEl = null;
    }
  }

  // ---- Playback bar ----

  _buildBar() {
    this._barEl = document.createElement('div');
    this._barEl.style.cssText = [
      'position:fixed',
      'bottom:0',
      'left:0',
      'right:0',
      'background:rgba(5,8,18,0.92)',
      'border-top:1px solid rgba(100,160,255,0.3)',
      'padding:8px 16px 10px',
      'z-index:9000',
      'display:flex',
      'flex-direction:column',
      'gap:6px',
      'backdrop-filter:blur(6px)',
      'font-family:monospace',
    ].join(';');

    // Top row: label, name, time, close
    const topRow = document.createElement('div');
    topRow.style.cssText = 'display:flex;align-items:center;gap:10px;font-size:11px;';

    const label = document.createElement('span');
    label.style.cssText = 'color:#48f;font-size:11px;font-weight:bold;letter-spacing:0.5px;';
    label.textContent = '\u23FA REPLAY';

    this._nameEl = document.createElement('span');
    this._nameEl.style.cssText = 'color:#ccd;flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;';
    this._nameEl.textContent = 'Loading…';

    this._timeEl = document.createElement('span');
    this._timeEl.style.color = '#888';

    const exitBtn = this._btn('✕ Exit Replay', () => this.close());
    exitBtn.style.cssText = 'background:rgba(255,60,60,0.2);border:1px solid rgba(255,60,60,0.5);color:#faa;padding:3px 10px;cursor:pointer;font-size:11px;border-radius:3px;font-family:monospace;';

    topRow.appendChild(label);
    topRow.appendChild(this._nameEl);
    topRow.appendChild(this._timeEl);
    topRow.appendChild(exitBtn);

    // Activity heatmap
    const heatmapCanvas = document.createElement('canvas');
    heatmapCanvas.height = 10;
    heatmapCanvas.style.cssText = 'width:100%;height:10px;cursor:pointer;border-radius:3px;';
    heatmapCanvas.onclick = (e) => {
      const pct = e.offsetX / heatmapCanvas.offsetWidth;
      const idx = Math.floor(pct * Math.max(1, this._player.snapshots.length - 1));
      this._player.seek(idx);
    };
    this._heatmapCanvas = heatmapCanvas;
    this._heatmapCtx = heatmapCanvas.getContext('2d');
    this._heatmapOffscreen = document.createElement('canvas');
    this._heatmapOffscreen.height = 10;

    // Control row: step back | play/pause | step fwd | slider | speeds
    const ctrlRow = document.createElement('div');
    ctrlRow.style.cssText = 'display:flex;align-items:center;gap:6px;';

    const stepBack = this._btn('⏮', () => this._player.stepBackward());
    this._playBtn = this._btn('▶', () => this._togglePlay());
    this._playBtn.style.minWidth = '36px';
    const stepFwd = this._btn('⏭', () => this._player.stepForward());

    this._slider = document.createElement('input');
    this._slider.type = 'range';
    this._slider.min = '0';
    this._slider.max = '0';
    this._slider.value = '0';
    this._slider.style.cssText = 'flex:1;accent-color:#48f;cursor:pointer;';
    this._slider.oninput = () => {
      const idx = parseInt(this._slider.value, 10);
      this._player.seek(idx);
    };

    this._speedBtns = [1, 2, 4].map(s => {
      const btn = this._btn(s + 'x', () => {
        this._player.setSpeed(s);
        this._speedBtns.forEach(b => { b.style.background = 'rgba(255,255,255,0.08)'; });
        btn.style.background = 'rgba(68,136,255,0.35)';
      });
      return btn;
    });
    // Mark 1x active initially.
    this._speedBtns[0].style.background = 'rgba(68,136,255,0.35)';

    ctrlRow.appendChild(stepBack);
    ctrlRow.appendChild(this._playBtn);
    ctrlRow.appendChild(stepFwd);
    ctrlRow.appendChild(this._slider);
    this._speedBtns.forEach(b => ctrlRow.appendChild(b));

    this._barEl.appendChild(topRow);
    this._barEl.appendChild(heatmapCanvas);
    this._barEl.appendChild(ctrlRow);
    document.body.appendChild(this._barEl);
  }

  _removeBar() {
    if (this._barEl) {
      this._barEl.remove();
      this._barEl = null;
    }
  }

  // ---- Player callbacks ----

  _onLoaded(id, name, total, snapshots) {
    this._snapshots = snapshots;
    if (this._nameEl) this._nameEl.textContent = name;
    if (this._slider) {
      this._slider.max = String(Math.max(0, total - 1));
      this._slider.value = '0';
    }
    this._drawHeatmap(snapshots);
  }

  _onSeek(index, total) {
    if (this._slider) this._slider.value = String(index);

    const snap = this._snapshots[index];
    if (this._timeEl && snap) {
      this._timeEl.textContent = snap.t.toLocaleTimeString() + '\u2002' + (index + 1) + '/' + total;
    }

    this._drawHeatmapCursor(index);
  }

  _onPlayState(isPlaying) {
    if (this._playBtn) this._playBtn.textContent = isPlaying ? '\u23F8' : '\u25B6';
  }

  _togglePlay() {
    if (this._player.isPlaying) {
      this._player.pause();
    } else {
      this._player.play();
    }
  }

  // ---- Heatmap ----

  _drawHeatmap(snapshots) {
    const canvas = this._heatmapCanvas;
    const offscreen = this._heatmapOffscreen;
    if (!canvas || !offscreen || !snapshots.length) return;

    const w = canvas.offsetWidth || 600;
    offscreen.width = w;
    const ctx = offscreen.getContext('2d');
    const n = snapshots.length;

    for (let i = 0; i < w; i++) {
      const snapIdx = Math.floor((i / w) * n);
      const snap = snapshots[snapIdx];
      if (!snap) continue;
      const active = snap.s.filter(s => !TERMINAL.has(s.activity)).length;
      const intensity = Math.min(1, active / 5);
      const r = Math.round(20 + intensity * 20);
      const g = Math.round(50 + intensity * 110);
      const b = Math.round(80 + intensity * 160);
      ctx.fillStyle = `rgb(${r},${g},${b})`;
      ctx.fillRect(i, 0, 1, 10);
    }

    canvas.width = w;
    this._heatmapCtx.drawImage(offscreen, 0, 0);
  }

  _drawHeatmapCursor(index) {
    const canvas = this._heatmapCanvas;
    const offscreen = this._heatmapOffscreen;
    if (!canvas || !offscreen || !this._snapshots.length) return;

    const ctx = this._heatmapCtx;
    ctx.clearRect(0, 0, canvas.width, 10);
    ctx.drawImage(offscreen, 0, 0);

    const x = Math.floor((index / Math.max(1, this._snapshots.length - 1)) * canvas.width);
    ctx.fillStyle = 'rgba(255,255,255,0.85)';
    ctx.fillRect(Math.max(0, x - 1), 0, 2, 10);
  }

  // ---- Helpers ----

  _btn(label, onClick) {
    const btn = document.createElement('button');
    btn.textContent = label;
    btn.style.cssText = 'background:rgba(255,255,255,0.08);border:1px solid rgba(255,255,255,0.2);color:#dde;padding:3px 8px;cursor:pointer;font-size:13px;border-radius:3px;font-family:monospace;';
    btn.onclick = onClick;
    return btn;
  }
}

function _formatSize(bytes) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
}
