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
    this._selectorDialogEl = null;
    this._barEl = null;
    this._heatmapCtx = null;
    this._heatmapOffscreen = null;
    this._snapshots = [];
    this._playBtn = null;
    this._slider = null;
    this._nameEl = null;
    this._timeEl = null;
    this._messageEl = null;
    this._speedBtns = [];
    this._returnFocus = null;
    this._selectorKeyDown = (event) => this._handleSelectorKeyDown(event);

    // Wire player callbacks.
    player.onSeek = (index, total) => this._onSeek(index, total);
    player.onPlayStateChange = (playing) => this._onPlayState(playing);
    player.onLoaded = (id, name, total) => this._onLoaded(id, name, total, player.snapshots);
    player.onWarning = (message) => this._setMessage(message, 'warning');
  }

  /** Show the replay file selector. */
  async open() {
    this._returnFocus = document.activeElement;
    this._buildSelector();
    this._focusSelectorClose();
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
    this._restoreFocus();
    this._onClose && this._onClose();
  }

  // ---- Selector ----

  _buildSelector() {
    this._selectorEl = document.createElement('div');
    this._selectorEl.className = 'ts-overlay';
    this._selectorEl.setAttribute('role', 'dialog');
    this._selectorEl.setAttribute('aria-label', 'Replay selector');
    this._selectorEl.setAttribute('aria-modal', 'true');

    this._selectorDialogEl = document.createElement('div');
    this._selectorDialogEl.className = 'ts-dialog';

    const header = document.createElement('div');
    header.className = 'ts-header';

    const title = document.createElement('span');
    title.className = 'ts-title';
    title.textContent = '\u23FA Replay Mode — Select Recording';

    const closeBtn = this._btn('✕', () => this.close());
    closeBtn.setAttribute('aria-label', 'Close replay selector');
    closeBtn.className = 'ts-close-btn';

    header.appendChild(title);
    header.appendChild(closeBtn);

    this._listEl = document.createElement('div');
    this._listEl.className = 'ts-list';
    this._listEl.textContent = 'Loading replays…';

    this._selectorDialogEl.appendChild(header);
    this._selectorDialogEl.appendChild(this._listEl);
    this._selectorEl.appendChild(this._selectorDialogEl);
    this._selectorEl.addEventListener('click', (event) => {
      if (event.target === this._selectorEl) {
        this.close();
      }
    });
    this._selectorEl.addEventListener('keydown', this._selectorKeyDown);
    document.body.appendChild(this._selectorEl);
  }

  _populateSelector(replays) {
    this._listEl.textContent = '';
    if (!replays || replays.length === 0) {
      const empty = document.createElement('div');
      empty.className = 'ts-empty';
      empty.textContent = 'No replays found. Recordings are saved every session.';
      this._listEl.appendChild(empty);
      return;
    }

    for (const r of replays) {
      const item = document.createElement('button');
      item.className = 'ts-replay-item';

      const nameSpan = document.createElement('span');
      nameSpan.className = 'ts-replay-item-name';
      nameSpan.textContent = r.name;

      const sizeSpan = document.createElement('span');
      sizeSpan.className = 'ts-replay-item-size';
      sizeSpan.textContent = _formatSize(r.size);

      const dateSpan = document.createElement('span');
      dateSpan.className = 'ts-replay-item-date';
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
    err.className = 'ts-error';
    err.textContent = 'Error loading replays: ' + msg;
    this._listEl.appendChild(err);
  }

  async _selectReplay(replay) {
    this._removeSelector();
    this._buildBar();
    this._playBtn?.focus();
    if (this._nameEl) {
      this._nameEl.textContent = replay.name;
    }
    this._setMessage('');

    try {
      await this._player.loadReplay(replay.id);
      this._player.play();
    } catch (err) {
      console.error('Failed to load replay:', err);
      this._setMessage(`Failed to load replay: ${err.message}`, 'error');
    }
  }

  _removeSelector() {
    if (this._selectorEl) {
      this._selectorEl.removeEventListener('keydown', this._selectorKeyDown);
      this._selectorEl.remove();
      this._selectorEl = null;
    }
    this._selectorDialogEl = null;
    this._listEl = null;
  }

  // ---- Playback bar ----

  _buildBar() {
    this._barEl = document.createElement('div');
    this._barEl.className = 'ts-bar';

    // Top row: label, name, time, close
    const topRow = document.createElement('div');
    topRow.className = 'ts-top-row';

    const label = document.createElement('span');
    label.className = 'ts-replay-label';
    label.textContent = '\u23FA REPLAY';

    this._nameEl = document.createElement('span');
    this._nameEl.className = 'ts-replay-name';
    this._nameEl.textContent = 'Loading…';

    this._timeEl = document.createElement('span');
    this._timeEl.className = 'ts-time';

    const exitBtn = this._btn('✕ Exit Replay', () => this.close());
    exitBtn.setAttribute('aria-label', 'Exit replay mode');
    exitBtn.className = 'ts-exit-btn';

    topRow.appendChild(label);
    topRow.appendChild(this._nameEl);
    topRow.appendChild(this._timeEl);
    topRow.appendChild(exitBtn);

    // Activity heatmap
    const heatmapCanvas = document.createElement('canvas');
    heatmapCanvas.height = 10;
    heatmapCanvas.className = 'ts-heatmap';
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
    ctrlRow.className = 'ts-ctrl-row';

    const stepBack = this._btn('⏮', () => this._player.stepBackward());
    stepBack.setAttribute('aria-label', 'Step backward one replay frame');
    this._playBtn = this._btn('▶', () => this._togglePlay());
    this._playBtn.setAttribute('aria-label', 'Play replay');
    this._playBtn.classList.add('ts-play-btn');
    const stepFwd = this._btn('⏭', () => this._player.stepForward());
    stepFwd.setAttribute('aria-label', 'Step forward one replay frame');

    this._slider = document.createElement('input');
    this._slider.type = 'range';
    this._slider.min = '0';
    this._slider.max = '0';
    this._slider.value = '0';
    this._slider.setAttribute('aria-label', 'Replay timeline position');
    this._slider.className = 'ts-slider';
    this._slider.oninput = () => {
      const idx = parseInt(this._slider.value, 10);
      this._player.seek(idx);
    };

    this._speedBtns = [1, 2, 4].map(s => {
      const btn = this._btn(s + 'x', () => {
        this._player.setSpeed(s);
        this._speedBtns.forEach(b => { b.classList.remove('active'); });
        btn.classList.add('active');
      });
      btn.setAttribute('aria-label', `Set replay speed to ${s}x`);
      return btn;
    });
    // Mark 1x active initially.
    this._speedBtns[0].classList.add('active');

    ctrlRow.appendChild(stepBack);
    ctrlRow.appendChild(this._playBtn);
    ctrlRow.appendChild(stepFwd);
    ctrlRow.appendChild(this._slider);
    this._speedBtns.forEach(b => ctrlRow.appendChild(b));

    this._barEl.appendChild(topRow);
    this._barEl.appendChild(heatmapCanvas);
    this._barEl.appendChild(ctrlRow);

    this._messageEl = document.createElement('div');
    this._messageEl.className = 'ts-message';
    this._barEl.appendChild(this._messageEl);

    document.body.appendChild(this._barEl);
  }

  _removeBar() {
    if (this._barEl) {
      this._barEl.remove();
      this._barEl = null;
    }
    this._messageEl = null;
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
    if (!this._playBtn) return;
    this._playBtn.textContent = isPlaying ? '\u23F8' : '\u25B6';
    this._playBtn.setAttribute('aria-label', isPlaying ? 'Pause replay' : 'Play replay');
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

  _getSelectorFocusable() {
    if (!this._selectorEl) return [];
    return Array.from(
      this._selectorEl.querySelectorAll(
        'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex="0"]',
      ),
    );
  }

  _handleSelectorKeyDown(event) {
    if (event.key === 'Escape') {
      event.preventDefault();
      this.close();
      return;
    }
    if (event.key !== 'Tab') return;

    const focusable = this._getSelectorFocusable();
    if (!focusable.length) return;

    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey) {
      if (document.activeElement === first) {
        event.preventDefault();
        last.focus();
      }
      return;
    }
    if (document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }

  _focusSelectorClose() {
    if (!this._selectorEl) return;
    this._selectorEl.querySelector('button')?.focus();
  }

  _restoreFocus() {
    this._returnFocus?.focus();
    this._returnFocus = null;
  }

  _btn(label, onClick) {
    const btn = document.createElement('button');
    btn.textContent = label;
    btn.className = 'ts-btn';
    btn.onclick = onClick;
    return btn;
  }

  _setMessage(message, tone = 'warning') {
    if (!this._messageEl) return;

    if (!message) {
      this._messageEl.textContent = '';
      this._messageEl.classList.remove('visible', 'error');
      return;
    }

    this._messageEl.textContent = message;
    this._messageEl.classList.add('visible');
    this._messageEl.classList.toggle('error', tone === 'error');
  }
}

function _formatSize(bytes) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
}
