import { getModelColor } from '../session/colors.js';
import { DEFAULT_CONTEXT_WINDOW } from '../session/constants.js';

const WIDTH = 200;
const HEIGHT = 72;
const PAD = 7;
const LABEL_H = 10;
const DOT_MIN_R = 3;
const DOT_MAX_R = 6;
const HIT_PAD = 4; // extra px beyond dot edge for click detection
const FRAME_MS = 100; // 10 fps

function roundRect(ctx, x, y, w, h, r) {
  if (ctx.roundRect) {
    ctx.roundRect(x, y, w, h, r);
  } else {
    ctx.rect(x, y, w, h);
  }
}

function fillCircle(ctx, x, y, r) {
  ctx.beginPath();
  ctx.arc(x, y, r, 0, Math.PI * 2);
  ctx.fill();
}

export class Minimap {
  constructor() {
    this.visible = true;
    this.zoomed = false;
    this._zoomPinned = false; // keyboard-toggled zoom persists across hover/focus
    this.raceCanvas = null;
    this.onDotClick = null;

    this._lastFrameTime = 0;
    this._prevPos = new Map(); // id -> {mx, my}
    this._hitTargets = [];    // [{state, mx, my, r}]

    const el = document.createElement('canvas');
    el.className = 'minimap-canvas';
    el.width = WIDTH;
    el.height = HEIGHT;
    el.title = 'Radar — click a dot to focus session (N to toggle, Shift+N or Tab to zoom)';
    el.tabIndex = 0;
    el.setAttribute('role', 'img');
    el.setAttribute('aria-label', 'Session radar minimap');
    this._canvas = el;
    this._ctx = el.getContext('2d');

    el.addEventListener('click', (e) => this._handleClick(e));
    el.addEventListener('mouseenter', () => this._hoverZoom(true));
    el.addEventListener('mouseleave', () => this._hoverZoom(false));
    el.addEventListener('focus', () => this._hoverZoom(true));
    el.addEventListener('blur', () => this._hoverZoom(false));

    document.body.appendChild(el);
    this._startLoop();
  }

  _applyZoom(on) {
    this.zoomed = on;
    this._canvas.style.transform = on ? 'scale(2)' : '';
  }

  _hoverZoom(on) {
    if (this._zoomPinned) return;
    this._applyZoom(on);
  }

  toggleZoom() {
    this._zoomPinned = !this._zoomPinned;
    this._applyZoom(this._zoomPinned);
    return this._zoomPinned;
  }

  setVisible(visible) {
    this.visible = visible;
    this._canvas.style.display = visible ? 'block' : 'none';
  }

  toggle() {
    this.setVisible(!this.visible);
    return this.visible;
  }

  _startLoop() {
    const tick = (now) => {
      this._animFrame = requestAnimationFrame(tick);
      if (now - this._lastFrameTime < FRAME_MS) return;
      this._lastFrameTime = now;
      if (this.visible && this.raceCanvas && this._ctx) {
        this._draw(now);
      }
    };
    this._animFrame = requestAnimationFrame(tick);
  }

  _draw(now) {
    const rc = this.raceCanvas;
    const racers = [...rc.racers.values()];
    const ctx = this._ctx;

    ctx.clearRect(0, 0, WIDTH, HEIGHT);
    this._drawChrome(ctx);

    if (racers.length === 0) {
      ctx.fillStyle = 'rgba(255,255,255,0.3)';
      ctx.font = '9px Courier New';
      ctx.textAlign = 'center';
      ctx.fillText('no sessions', WIDTH / 2, HEIGHT / 2 + 4);
      this._hitTargets = [];
      this._prevPos.clear();
      return;
    }

    // Prune trails for removed racers
    for (const id of this._prevPos.keys()) {
      if (!rc.racers.has(id)) this._prevPos.delete(id);
    }

    const { toMX, toMY, drawTop, drawW, drawH } = this._computeLayout(racers);

    this._drawTrackGuide(ctx, drawTop, drawH, drawW);
    this._drawViewport(ctx, toMY, drawTop, drawW);

    const hitTargets = [];
    for (const racer of racers) {
      const mx = toMX(racer.displayX);
      const my = toMY(racer.displayY);
      const color = getModelColor(racer.state.model, racer.state.source);
      const tokenFrac = Math.min(
        (racer.state.tokensUsed || 0) / (racer.state.maxContextTokens || DEFAULT_CONTEXT_WINDOW), 1
      );
      const r = DOT_MIN_R + tokenFrac * (DOT_MAX_R - DOT_MIN_R);

      this._drawTrail(ctx, racer.id, mx, my, color);
      this._drawDot(ctx, mx, my, r, color, racer.state.activity || 'idle', now);

      hitTargets.push({ state: racer.state, mx, my, r });
    }

    this._hitTargets = hitTargets;
  }

  _drawChrome(ctx) {
    ctx.fillStyle = 'rgba(10, 10, 30, 0.88)';
    ctx.beginPath();
    roundRect(ctx, 0, 0, WIDTH, HEIGHT, 6);
    ctx.fill();

    ctx.strokeStyle = 'rgba(255,255,255,0.15)';
    ctx.lineWidth = 1;
    ctx.beginPath();
    roundRect(ctx, 0, 0, WIDTH, HEIGHT, 6);
    ctx.stroke();

    ctx.fillStyle = 'rgba(255,255,255,0.3)';
    ctx.font = '7px Courier New';
    ctx.textAlign = 'left';
    ctx.fillText('RADAR', 5, 9);
  }

  _computeLayout(racers) {
    let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
    for (const racer of racers) {
      if (racer.displayX < minX) minX = racer.displayX;
      if (racer.displayX > maxX) maxX = racer.displayX;
      if (racer.displayY < minY) minY = racer.displayY;
      if (racer.displayY > maxY) maxY = racer.displayY;
    }

    const drawTop = LABEL_H + 2;
    const drawW = WIDTH - PAD * 2;
    const drawH = HEIGHT - drawTop - PAD;
    const spanX = Math.max(maxX - minX, 100);
    const spanY = Math.max(maxY - minY, 40);

    const toMX = (wx) => PAD + (wx - minX) / spanX * drawW;
    const toMY = (wy) => drawTop + (wy - minY) / spanY * drawH;

    return { toMX, toMY, drawTop, drawW, drawH };
  }

  _drawTrackGuide(ctx, drawTop, drawH, drawW) {
    const midY = drawTop + drawH / 2;
    ctx.strokeStyle = 'rgba(255,255,255,0.07)';
    ctx.lineWidth = 1;
    ctx.setLineDash([3, 5]);
    ctx.beginPath();
    ctx.moveTo(PAD, midY);
    ctx.lineTo(PAD + drawW, midY);
    ctx.stroke();
    ctx.setLineDash([]);
  }

  _drawViewport(ctx, toMY, drawTop, drawW) {
    const container = document.getElementById('race-container');
    if (!container) return;

    const vy1 = Math.max(drawTop, toMY(container.scrollTop));
    const vy2 = Math.min(HEIGHT - PAD, toMY(container.scrollTop + container.clientHeight));
    if (vy2 <= vy1) return;

    ctx.strokeStyle = 'rgba(255,255,255,0.22)';
    ctx.lineWidth = 1;
    ctx.beginPath();
    roundRect(ctx, PAD, vy1, drawW, vy2 - vy1, 2);
    ctx.stroke();
  }

  _drawTrail(ctx, id, mx, my, color) {
    const prev = this._prevPos.get(id);
    if (prev) {
      const dx = mx - prev.mx;
      const dy = my - prev.my;
      if (dx * dx + dy * dy > 1) {
        ctx.strokeStyle = color.main + '55';
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        ctx.moveTo(prev.mx, prev.my);
        ctx.lineTo(mx, my);
        ctx.stroke();
      }
    }
    this._prevPos.set(id, { mx, my });
  }

  _drawDot(ctx, mx, my, r, color, activity, now) {
    if (activity === 'complete') {
      this._drawStar(ctx, mx, my, r, color.main);
      return;
    }

    if (activity === 'errored') {
      if (Math.sin(now / 250) > 0) {
        ctx.fillStyle = '#e94560';
        fillCircle(ctx, mx, my, r);
      }
      return;
    }

    if (activity === 'thinking' || activity === 'tool_use') {
      ctx.fillStyle = color.main;
      fillCircle(ctx, mx, my, r);
      // Specular highlight
      ctx.fillStyle = 'rgba(255,255,255,0.45)';
      fillCircle(ctx, mx - r * 0.28, my - r * 0.28, r * 0.35);
      return;
    }

    if (activity === 'idle' || activity === 'waiting' || activity === 'starting') {
      const pulse = 0.45 + 0.55 * Math.sin(now / 700);
      ctx.globalAlpha = pulse;
      ctx.fillStyle = color.main;
      fillCircle(ctx, mx, my, r);
      ctx.globalAlpha = 1;
      return;
    }

    // lost, unknown -- dim
    ctx.globalAlpha = 0.35;
    ctx.fillStyle = color.main;
    fillCircle(ctx, mx, my, r);
    ctx.globalAlpha = 1;
  }

  _drawStar(ctx, cx, cy, r, color) {
    ctx.fillStyle = color;
    ctx.beginPath();
    for (let i = 0; i < 10; i++) {
      const angle = (i * Math.PI / 5) - Math.PI / 2;
      const rad = i % 2 === 0 ? r : r * 0.45;
      const x = cx + Math.cos(angle) * rad;
      const y = cy + Math.sin(angle) * rad;
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    }
    ctx.closePath();
    ctx.fill();
  }

  _handleClick(e) {
    if (!this._hitTargets.length || !this.onDotClick) return;

    const rect = this._canvas.getBoundingClientRect();
    const scaleX = WIDTH / rect.width;
    const scaleY = HEIGHT / rect.height;
    const mx = (e.clientX - rect.left) * scaleX;
    const my = (e.clientY - rect.top) * scaleY;

    let bestDist = Infinity;
    let bestTarget = null;
    for (const t of this._hitTargets) {
      const dx = t.mx - mx;
      const dy = t.my - my;
      const dist = Math.sqrt(dx * dx + dy * dy);
      if (dist < bestDist) {
        bestDist = dist;
        bestTarget = t;
      }
    }

    if (bestTarget && bestDist < bestTarget.r + HIT_PAD) {
      this.onDotClick(bestTarget.state);
    }
  }

  destroy() {
    if (this._animFrame) cancelAnimationFrame(this._animFrame);
    this._canvas.remove();
  }
}
