import { TilePalette } from './TilePalette.js';
import { validateTrack } from './TrackValidator.js';
import { authFetch } from '../auth.js';

const CELL_SIZE = 32;
const DEFAULT_WIDTH = 32;
const DEFAULT_HEIGHT = 16;

export class TrackEditor {
  constructor(canvas) {
    this.canvas = canvas;
    this.ctx = canvas.getContext('2d');
    this.active = false;
    this.palette = new TilePalette();

    this.width = DEFAULT_WIDTH;
    this.height = DEFAULT_HEIGHT;
    this.tiles = this._emptyGrid(this.width, this.height);

    this._history = [];
    this._future = [];
    this._painting = false;
    this._paintMode = null;

    this._toolbar = null;
    this._validationEl = null;
    this._trackListEl = null;
    this._currentTrackId = null;
    this._lastValidation = null;

    this._mouseDownHandler = (e) => this._onMouseDown(e);
    this._mouseMoveHandler = (e) => this._onMouseMove(e);
    this._mouseUpHandler = () => this._onMouseUp();
    this._keyHandler = (e) => this._onKey(e);
    this._contextMenuHandler = (e) => { if (this.active) e.preventDefault(); };
  }

  _emptyGrid(w, h) {
    const g = [];
    for (let r = 0; r < h; r++) {
      g.push(new Array(w).fill(''));
    }
    return g;
  }

  toggle() {
    if (this.active) this.deactivate(); else this.activate();
  }

  activate() {
    this.active = true;
    this.palette.mount(document.body);
    this._buildToolbar();
    this._buildValidationEl();
    this.canvas.addEventListener('mousedown', this._mouseDownHandler);
    this.canvas.addEventListener('mousemove', this._mouseMoveHandler);
    window.addEventListener('mouseup', this._mouseUpHandler);
    window.addEventListener('keydown', this._keyHandler);
    this.canvas.addEventListener('contextmenu', this._contextMenuHandler);
    this._validate();
  }

  deactivate() {
    this.active = false;
    this.palette.unmount();
    if (this._toolbar) { this._toolbar.remove(); this._toolbar = null; }
    if (this._validationEl) { this._validationEl.remove(); this._validationEl = null; }
    if (this._trackListEl) { this._trackListEl.remove(); this._trackListEl = null; }
    this.canvas.removeEventListener('mousedown', this._mouseDownHandler);
    this.canvas.removeEventListener('mousemove', this._mouseMoveHandler);
    window.removeEventListener('mouseup', this._mouseUpHandler);
    window.removeEventListener('keydown', this._keyHandler);
    this.canvas.removeEventListener('contextmenu', this._contextMenuHandler);
  }

  _buildToolbar() {
    this._toolbar = document.createElement('div');
    this._toolbar.id = 'track-editor-toolbar';
    Object.assign(this._toolbar.style, {
      position: 'fixed', top: '10px', left: '50%', transform: 'translateX(-50%)',
      background: 'rgba(20,20,30,0.95)', border: '1px solid #666',
      borderRadius: '8px', padding: '8px 12px', display: 'flex', gap: '6px',
      alignItems: 'center', zIndex: '1001', font: '12px Courier New',
    });

    const label = document.createElement('span');
    label.textContent = 'TRACK EDITOR';
    Object.assign(label.style, { color: '#a0a0ff', fontWeight: 'bold' });
    this._toolbar.appendChild(label);

    const addBtn = (text, title, onClick) => {
      const b = document.createElement('button');
      b.textContent = text;
      b.title = title;
      Object.assign(b.style, {
        background: '#2a2a3a', border: '1px solid #555', color: '#ccc',
        padding: '3px 8px', cursor: 'pointer', borderRadius: '3px',
        font: '11px Courier New',
      });
      b.addEventListener('click', onClick);
      this._toolbar.appendChild(b);
      return b;
    };

    addBtn('Undo', 'Ctrl+Z', () => this.undo());
    addBtn('Redo', 'Ctrl+Shift+Z', () => this.redo());
    addBtn('Clear', 'Clear all tiles', () => this._clearAll());
    addBtn('Save', 'Save track to server', () => this._saveTrack());
    addBtn('Tracks', 'Load a track', () => this._showTrackList());
    addBtn('Export', 'Download as JSON', () => this._exportJSON());
    addBtn('Import', 'Upload JSON file', () => this._importJSON());
    addBtn('Close [E]', 'Exit editor', () => this.deactivate());

    document.body.appendChild(this._toolbar);
  }

  _buildValidationEl() {
    this._validationEl = document.createElement('div');
    Object.assign(this._validationEl.style, {
      position: 'fixed', bottom: '10px', left: '50%', transform: 'translateX(-50%)',
      background: 'rgba(20,20,30,0.9)', border: '1px solid #444',
      borderRadius: '6px', padding: '5px 12px', zIndex: '1000',
      font: '12px Courier New', pointerEvents: 'none',
    });
    document.body.appendChild(this._validationEl);
  }

  _validate() {
    const result = validateTrack(this.tiles);
    this._lastValidation = result;
    if (!this._validationEl) return result;
    this._validationEl.textContent = (result.valid ? '\u2713 ' : '\u2717 ') + result.message;
    const color = result.valid ? '#4ade80' : '#f87171';
    this._validationEl.style.color = color;
    this._validationEl.style.borderColor = color;
    return result;
  }

  _canvasMetrics() {
    const dpr = window.devicePixelRatio || 1;
    const rect = this.canvas.getBoundingClientRect();
    return {
      rect,
      width: this.canvas.width / dpr,
      height: this.canvas.height / dpr,
    };
  }

  _cellAt(e) {
    const { rect, width, height } = this._canvasMetrics();
    const scaleX = rect.width > 0 ? width / rect.width : 1;
    const scaleY = rect.height > 0 ? height / rect.height : 1;
    const x = (e.clientX - rect.left) * scaleX;
    const y = (e.clientY - rect.top) * scaleY;
    const col = Math.floor(x / CELL_SIZE);
    const row = Math.floor(y / CELL_SIZE);
    if (row < 0 || row >= this.height || col < 0 || col >= this.width) return null;
    return { row, col };
  }

  _onMouseDown(e) {
    if (!this.active) return;
    this._painting = true;
    this._paintMode = e.button === 2 ? 'erase' : 'place';
    this._snapshot();
    this._paint(e);
  }

  _onMouseMove(e) {
    if (!this.active || !this._painting) return;
    this._paint(e);
  }

  _onMouseUp() {
    this._painting = false;
    this._paintMode = null;
    this._validate();
  }

  _paint(e) {
    const cell = this._cellAt(e);
    if (!cell) return;
    if (this._paintMode === 'erase') {
      this.tiles[cell.row][cell.col] = '';
    } else {
      this.tiles[cell.row][cell.col] = this.palette.selectedTile;
    }
  }

  _onKey(e) {
    if (!this.active) return;
    if (e.key === 'z' && e.ctrlKey && !e.shiftKey) { e.preventDefault(); this.undo(); }
    if (e.key === 'z' && e.ctrlKey && e.shiftKey) { e.preventDefault(); this.redo(); }
    if (e.key === 'y' && e.ctrlKey) { e.preventDefault(); this.redo(); }
    if (e.key === 'r' && !e.ctrlKey && !e.altKey) { this._rotateSelected(); }
  }

  _rotateSelected() {
    const rotMap = {
      'curve-ne': 'curve-se', 'curve-se': 'curve-sw',
      'curve-sw': 'curve-nw', 'curve-nw': 'curve-ne',
      'straight-h': 'straight-v', 'straight-v': 'straight-h',
    };
    const next = rotMap[this.palette.selectedTile];
    if (next) this.palette.select(next);
  }

  _snapshot() {
    const copy = this.tiles.map(row => row.slice());
    this._history.push(copy);
    if (this._history.length > 50) this._history.shift();
    this._future = [];
  }

  undo() {
    if (this._history.length === 0) return;
    const cur = this.tiles.map(row => row.slice());
    this._future.push(cur);
    this.tiles = this._history.pop();
    this._validate();
  }

  redo() {
    if (this._future.length === 0) return;
    const cur = this.tiles.map(row => row.slice());
    this._history.push(cur);
    this.tiles = this._future.pop();
    this._validate();
  }

  _clearAll() {
    this._snapshot();
    this.tiles = this._emptyGrid(this.width, this.height);
    this._validate();
  }

  async _saveTrack() {
    const name = window.prompt('Track name:', this._currentTrackId || 'My Track');
    if (!name) return;
    const id = name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '') || 'custom';
    const track = { id, name, width: this.width, height: this.height, tiles: this.tiles };
    try {
      const isUpdate = this._currentTrackId === id;
      const url = isUpdate ? '/api/tracks/' + id : '/api/tracks';
      const method = isUpdate ? 'PUT' : 'POST';
      const resp = await authFetch(url, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(track),
      });
      if (resp.ok) {
        this._currentTrackId = id;
        alert('Saved: ' + name);
      } else {
        alert('Save failed: ' + resp.status);
      }
    } catch (err) {
      alert('Save failed: ' + err.message);
    }
  }

  async _showTrackList() {
    try {
      const resp = await authFetch('/api/tracks');
      if (!resp.ok) { alert('Could not load tracks'); return; }
      const tracks = await resp.json();

      if (this._trackListEl) this._trackListEl.remove();
      this._trackListEl = document.createElement('div');
      Object.assign(this._trackListEl.style, {
        position: 'fixed', top: '60px', left: '50%', transform: 'translateX(-50%)',
        background: 'rgba(20,20,30,0.97)', border: '1px solid #666',
        borderRadius: '8px', padding: '12px', zIndex: '1002',
        maxHeight: '60vh', overflowY: 'auto', minWidth: '300px',
        font: '13px Courier New', color: '#ccc',
      });

      const header = document.createElement('div');
      header.textContent = 'Select Track';
      Object.assign(header.style, { color: '#a0a0ff', fontWeight: 'bold', marginBottom: '8px' });
      this._trackListEl.appendChild(header);

      for (let i = 0; i < tracks.length; i++) {
        const t = tracks[i];
        const row = document.createElement('div');
        Object.assign(row.style, {
          display: 'flex', gap: '8px', alignItems: 'center',
          padding: '4px 0', borderBottom: '1px solid #333',
        });

        const nameEl = document.createElement('span');
        nameEl.textContent = t.name || t.id;
        nameEl.style.flex = '1';
        row.appendChild(nameEl);

        const loadBtn = document.createElement('button');
        loadBtn.textContent = 'Load';
        Object.assign(loadBtn.style, {
          background: '#3a3a5a', border: '1px solid #666', color: '#ccc',
          padding: '2px 8px', cursor: 'pointer', borderRadius: '3px',
          font: '11px Courier New',
        });
        loadBtn.addEventListener('click', () => {
          this._loadTrack(t);
          this._trackListEl.remove();
          this._trackListEl = null;
        });
        row.appendChild(loadBtn);
        this._trackListEl.appendChild(row);
      }

      const closeBtn = document.createElement('button');
      closeBtn.textContent = 'Close';
      Object.assign(closeBtn.style, {
        marginTop: '8px', background: '#2a2a3a', border: '1px solid #555',
        color: '#ccc', padding: '4px 12px', cursor: 'pointer',
        borderRadius: '4px', font: '11px Courier New', width: '100%',
      });
      closeBtn.addEventListener('click', () => { this._trackListEl.remove(); this._trackListEl = null; });
      this._trackListEl.appendChild(closeBtn);

      document.body.appendChild(this._trackListEl);
    } catch (err) {
      alert('Failed to load tracks: ' + err.message);
    }
  }

  _loadTrack(t) {
    this._snapshot();
    this.width = t.width || DEFAULT_WIDTH;
    this.height = t.height || DEFAULT_HEIGHT;
    this.tiles = t.tiles || this._emptyGrid(this.width, this.height);
    this._currentTrackId = t.id || null;
    this._validate();
  }

  _exportJSON() {
    const track = {
      id: this._currentTrackId || 'custom-track',
      name: 'Custom Track',
      width: this.width,
      height: this.height,
      tiles: this.tiles,
    };
    const blob = new Blob([JSON.stringify(track, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = track.id + '.json';
    a.click();
    URL.revokeObjectURL(url);
  }

  _importJSON() {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.json';
    input.addEventListener('change', () => {
      const file = input.files[0];
      if (!file) return;
      const reader = new FileReader();
      reader.onload = (ev) => {
        try {
          const t = JSON.parse(ev.target.result);
          this._loadTrack(t);
        } catch (err) {
          alert('Invalid JSON: ' + err.message);
        }
      };
      reader.readAsText(file);
    });
    input.click();
  }

  draw() {
    if (!this.active) return;
    const ctx = this.ctx;
    const { width, height } = this._canvasMetrics();

    ctx.fillStyle = 'rgba(10,10,20,0.88)';
    ctx.fillRect(0, 0, width, height);

    for (let r = 0; r < this.height; r++) {
      for (let c = 0; c < this.width; c++) {
        const x = c * CELL_SIZE;
        const y = r * CELL_SIZE;
        const tile = this.tiles[r][c];

        ctx.fillStyle = tile ? '#1a2a3a' : '#111122';
        ctx.fillRect(x + 1, y + 1, CELL_SIZE - 2, CELL_SIZE - 2);

        ctx.strokeStyle = '#2a2a44';
        ctx.lineWidth = 0.5;
        ctx.strokeRect(x, y, CELL_SIZE, CELL_SIZE);

        if (tile) this._drawTile(ctx, x, y, CELL_SIZE, tile);
      }
    }

    this._drawValidationOverlay(ctx);
  }

  _drawTile(ctx, x, y, sz, tile) {
    const cx = x + sz / 2;
    const cy = y + sz / 2;
    const r = sz * 0.38;

    ctx.save();
    ctx.lineWidth = 3;
    ctx.strokeStyle = '#66aaff';
    ctx.fillStyle = '#66aaff';
    ctx.lineCap = 'round';

    switch (tile) {
      case 'straight-h':
        ctx.beginPath(); ctx.moveTo(x, cy); ctx.lineTo(x + sz, cy); ctx.stroke();
        break;
      case 'straight-v':
        ctx.beginPath(); ctx.moveTo(cx, y); ctx.lineTo(cx, y + sz); ctx.stroke();
        break;
      case 'curve-ne':
        ctx.beginPath(); ctx.arc(x, y + sz, r, -Math.PI / 2, 0, false); ctx.stroke();
        break;
      case 'curve-nw':
        ctx.beginPath(); ctx.arc(x + sz, y + sz, r, Math.PI, Math.PI * 1.5, false); ctx.stroke();
        break;
      case 'curve-se':
        ctx.beginPath(); ctx.arc(x, y, r, 0, Math.PI / 2, false); ctx.stroke();
        break;
      case 'curve-sw':
        ctx.beginPath(); ctx.arc(x + sz, y, r, Math.PI / 2, Math.PI, false); ctx.stroke();
        break;
      case 'chicane':
        ctx.beginPath();
        ctx.moveTo(x, cy);
        ctx.bezierCurveTo(x + sz * 0.3, cy - sz * 0.3, x + sz * 0.7, cy + sz * 0.3, x + sz, cy);
        ctx.stroke();
        break;
      case 'start-line':
        this._drawCheckerLine(ctx, cx, y, sz, '#fff', '#222', 'S');
        break;
      case 'finish-line':
        this._drawCheckerLine(ctx, cx, y, sz, '#e94560', '#1a1a2e', 'F');
        break;
      case 'pit-entry':
        ctx.strokeStyle = '#ffaa00';
        ctx.beginPath(); ctx.moveTo(x, cy); ctx.lineTo(x + sz, cy); ctx.stroke();
        ctx.fillStyle = '#ffaa00';
        ctx.font = '9px Courier New';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText('P', cx, cy - 7);
        break;
      case 'pit-exit':
        ctx.strokeStyle = '#ffaa00';
        ctx.beginPath(); ctx.moveTo(x, cy); ctx.lineTo(x + sz, cy); ctx.stroke();
        ctx.fillStyle = '#ffaa00';
        ctx.font = '9px Courier New';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText('P', cx, cy + 7);
        break;
      case 'grandstand':
        ctx.fillStyle = '#553388';
        ctx.fillRect(x + 3, y + 3, sz - 6, sz - 6);
        ctx.fillStyle = '#bb88ff';
        ctx.font = `${Math.floor(sz * 0.45)}px sans-serif`;
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText('G', cx, cy);
        break;
      case 'tree':
        ctx.fillStyle = '#228833';
        ctx.beginPath();
        ctx.arc(cx, cy, sz * 0.33, 0, Math.PI * 2);
        ctx.fill();
        ctx.fillStyle = '#44aa55';
        ctx.beginPath();
        ctx.arc(cx - 4, cy - 3, sz * 0.2, 0, Math.PI * 2);
        ctx.fill();
        break;
      case 'barrier':
        ctx.fillStyle = '#cc3333';
        ctx.fillRect(x + 3, cy - 5, sz - 6, 10);
        ctx.fillStyle = '#ff6666';
        ctx.fillRect(x + 3, cy - 5, 6, 10);
        break;
      default:
        break;
    }
    ctx.restore();
  }

  _drawCheckerLine(ctx, cx, y, sz, color1, color2, label) {
    const bw = 10, bh = sz - 6;
    const bx = cx - bw / 2, by = y + 3;
    for (let i = 0; i < 3; i++) {
      for (let j = 0; j < 2; j++) {
        ctx.fillStyle = (i + j) % 2 === 0 ? color1 : color2;
        ctx.fillRect(bx + j * (bw / 2), by + i * (bh / 3), bw / 2, bh / 3);
      }
    }
    ctx.fillStyle = color1;
    ctx.font = 'bold 9px Courier New';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'bottom';
    ctx.fillText(label, cx, y + sz - 1);
  }

  _drawValidationOverlay(ctx) {
    const result = this._lastValidation;
    if (result && !result.valid && result.disconnected.length > 0) {
      ctx.strokeStyle = 'rgba(248,113,113,0.8)';
      ctx.lineWidth = 2;
      for (let i = 0; i < result.disconnected.length; i++) {
        const dc = result.disconnected[i];
        const x = dc.col * CELL_SIZE;
        const y = dc.row * CELL_SIZE;
        ctx.beginPath();
        ctx.moveTo(x + 4, y + 4);
        ctx.lineTo(x + CELL_SIZE - 4, y + CELL_SIZE - 4);
        ctx.stroke();
        ctx.beginPath();
        ctx.moveTo(x + CELL_SIZE - 4, y + 4);
        ctx.lineTo(x + 4, y + CELL_SIZE - 4);
        ctx.stroke();
      }
    }
  }

  getCurrentTrack() {
    return {
      id: this._currentTrackId,
      width: this.width,
      height: this.height,
      tiles: this.tiles,
    };
  }
}
