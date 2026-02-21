// Model color/hex utilities duplicated here to avoid circular import
// (Racer.js imports Hamster.js, so Hamster.js cannot import from Racer.js)
const MODEL_COLORS = {
  'claude-opus-4-5-20251101': { main: '#a855f7', dark: '#7c3aed', light: '#c084fc', name: 'Opus' },
  'claude-sonnet-4-20250514': { main: '#3b82f6', dark: '#2563eb', light: '#60a5fa', name: 'Sonnet' },
  'claude-sonnet-4-5-20250929': { main: '#06b6d4', dark: '#0891b2', light: '#22d3ee', name: 'Sonnet' },
  'claude-haiku-3-5-20241022': { main: '#22c55e', dark: '#16a34a', light: '#4ade80', name: 'Haiku' },
  'claude-haiku-4-5-20251001': { main: '#22c55e', dark: '#16a34a', light: '#4ade80', name: 'Haiku' },
};
const DEFAULT_COLOR = { main: '#6b7280', dark: '#4b5563', light: '#9ca3af', name: '?' };

function getModelColor(model) {
  if (MODEL_COLORS[model]) return MODEL_COLORS[model];
  if (model) {
    const lower = model.toLowerCase();
    if (lower.includes('opus')) return { ...MODEL_COLORS['claude-opus-4-5-20251101'], name: 'Opus' };
    if (lower.includes('sonnet')) return { ...MODEL_COLORS['claude-sonnet-4-5-20250929'], name: 'Sonnet' };
    if (lower.includes('haiku')) return { ...MODEL_COLORS['claude-haiku-4-5-20251001'], name: 'Haiku' };
  }
  return DEFAULT_COLOR;
}

function hexToRgb(hex) {
  const r = parseInt(hex.slice(1, 3), 16);
  const g = parseInt(hex.slice(3, 5), 16);
  const b = parseInt(hex.slice(5, 7), 16);
  return { r, g, b };
}

const HAMSTER_SCALE = 2.3 * 0.4;

const BODY_COLOR = '#D2691E';
const BODY_LIGHT = '#DEB887';
const DECK_COLOR = '#8B4513';
const PINK = '#FFB6C1';

const ACTIVITY_GLOW = {
  thinking: '#a855f7',
  tool_use: '#3b82f6',
  complete: '#ffd700',
  errored: '#e94560',
  waiting: '#ff8800',
};

export class Hamster {
  constructor(state) {
    this.id = state.id;
    this.state = state;
    this.displayX = 0;
    this.targetX = 0;
    this.displayY = 0;
    this.targetY = 0;
    this.opacity = 1.0;
    this.initialized = false;

    // Spring physics
    this.springY = 0;
    this.springVel = 0;
    this.springDamping = 0.90;
    this.springStiffness = 0.12;

    // Wheel spin
    this.wheelAngle = 0;

    // Continuous animation phases
    this.earWigglePhase = Math.random() * Math.PI * 2;
    this.tailWagPhase = Math.random() * Math.PI * 2;
    this.dotPhase = 0;

    // Completion / rope state
    this.ropeSnapped = false;
    this.fadeTimer = 0;
    this.completionBurst = false;
    this.celebrationEmitted = false;
    this.starBurstPhase = 0;
    this.goldFlash = 0;

    // Follow jitter for organic movement
    this.jitterX = 0;
    this.jitterY = 0;
    this.jitterTimer = 0;

    // Activity glow
    this.glowIntensity = 0;
    this.targetGlow = 0;
  }

  update(state) {
    const oldActivity = this.state.activity;
    this.state = state;

    if (state.activity !== oldActivity) {
      this.springVel += 1.5;

      if (state.activity === 'complete') {
        this.ropeSnapped = false;
        this.completionBurst = false;
        this.celebrationEmitted = false;
        this.fadeTimer = 0;
        this.goldFlash = 0;
      }
    }
  }

  setTarget(x, y) {
    this.targetX = x;
    this.targetY = y;
    if (!this.initialized) {
      this.displayX = x;
      this.displayY = y;
      this.initialized = true;
    }
  }

  animate(particles, dt) {
    const dtScale = dt ? dt / (1 / 60) : 1;

    // Position lerp with follow delay 0.15 + jitter
    const prevX = this.displayX;

    this.jitterTimer += dt || 1 / 60;
    if (this.jitterTimer > 0.3) {
      this.jitterX = (Math.random() - 0.5) * 2;
      this.jitterY = (Math.random() - 0.5) * 1;
      this.jitterTimer = 0;
    }

    this.displayX += (this.targetX + this.jitterX - this.displayX) * 0.15 * dtScale;
    this.displayY += (this.targetY + this.jitterY - this.displayY) * 0.15 * dtScale;

    const speed = Math.abs(this.displayX - prevX);

    // Wheel spin driven by movement delta
    this.wheelAngle += speed * 0.5 * dtScale;

    // Spring physics
    const springForce = -this.springStiffness * this.springY;
    this.springVel += springForce * dtScale;
    this.springVel *= Math.pow(this.springDamping, dtScale);
    this.springY += this.springVel * dtScale;

    // Continuous animations
    this.earWigglePhase += 0.08 * dtScale;
    this.tailWagPhase += 0.12 * dtScale;
    this.dotPhase += 0.04 * dtScale;

    // Glow interpolation
    this.glowIntensity += (this.targetGlow - this.glowIntensity) * 0.1 * dtScale;

    const activity = this.state.activity;

    switch (activity) {
      case 'thinking':
        this.targetGlow = 0.06;
        if (particles && speed > 0.5 && Math.random() > 0.85) {
          particles.emit('exhaust', this.displayX - 10, this.displayY + 5, 1);
        }
        break;
      case 'tool_use':
        this.targetGlow = 0.10;
        if (particles && speed > 0.5 && Math.random() > 0.85) {
          particles.emit('exhaust', this.displayX - 10, this.displayY + 5, 1);
        }
        break;
      case 'complete':
        this.fadeTimer += dt || 1 / 60;
        // Rope disappears immediately, hamster fades to 0.3 over 5s
        if (!this.ropeSnapped && this.fadeTimer >= 0.3) {
          this.ropeSnapped = true;
          this.completionBurst = true;
        }
        this.opacity = Math.max(0.3, 1.0 - (this.fadeTimer / 5) * 0.7);
        if (!this.celebrationEmitted && particles) {
          particles.emit('celebration', this.displayX, this.displayY, 15);
          this.celebrationEmitted = true;
        }
        this.starBurstPhase += 0.05 * dtScale;
        this.goldFlash = Math.min(1, this.fadeTimer * 2);
        this.targetGlow = 0.12;
        break;
      default:
        this.targetGlow = 0.03;
    }
  }

  draw(ctx) {
    const x = this.displayX;
    const y = this.displayY;
    const color = getModelColor(this.state.model, this.state.source);
    const activity = this.state.activity;

    ctx.save();
    ctx.globalAlpha = this.opacity;

    const yOff = this.springY;

    // Subtle underglow matching activity color
    if (this.glowIntensity > 0.01) {
      const glowHex = ACTIVITY_GLOW[activity] || color.main;
      const glowRgb = hexToRgb(glowHex);
      const glowR = 20 * HAMSTER_SCALE;
      const glow = ctx.createRadialGradient(x, y + yOff, 0, x, y + yOff, glowR);
      glow.addColorStop(0, `rgba(${glowRgb.r},${glowRgb.g},${glowRgb.b},${this.glowIntensity})`);
      glow.addColorStop(1, `rgba(${glowRgb.r},${glowRgb.g},${glowRgb.b},0)`);
      ctx.fillStyle = glow;
      ctx.beginPath();
      ctx.arc(x, y + yOff, glowR, 0, Math.PI * 2);
      ctx.fill();
    }

    this._drawSkateboard(ctx, x, y + yOff);
    this._drawHamsterBody(ctx, x, y + yOff, color, activity);
    this._drawActivityIndicator(ctx, x, y + yOff, color, activity);

    if (activity === 'complete' && this.completionBurst) {
      this._drawStarBurst(ctx, x, y + yOff);
    }

    ctx.restore();
  }

  _drawSkateboard(ctx, x, y) {
    ctx.save();
    ctx.translate(x, y);
    ctx.scale(HAMSTER_SCALE, HAMSTER_SCALE);
    ctx.translate(-x, -y);

    // Deck — rounded rect
    const deckW = 28;
    const deckH = 4;
    const deckX = x - deckW / 2;
    const deckY = y - deckH / 2;
    const deckR = 2;

    // Deck shape (fill + outline reuse the same path)
    ctx.beginPath();
    ctx.moveTo(deckX + deckR, deckY);
    ctx.lineTo(deckX + deckW - deckR, deckY);
    ctx.quadraticCurveTo(deckX + deckW, deckY, deckX + deckW, deckY + deckR);
    ctx.lineTo(deckX + deckW, deckY + deckH - deckR);
    ctx.quadraticCurveTo(deckX + deckW, deckY + deckH, deckX + deckW - deckR, deckY + deckH);
    ctx.lineTo(deckX + deckR, deckY + deckH);
    ctx.quadraticCurveTo(deckX, deckY + deckH, deckX, deckY + deckH - deckR);
    ctx.lineTo(deckX, deckY + deckR);
    ctx.quadraticCurveTo(deckX, deckY, deckX + deckR, deckY);
    ctx.closePath();
    ctx.fillStyle = DECK_COLOR;
    ctx.fill();
    ctx.strokeStyle = '#6B3410';
    ctx.lineWidth = 0.5;
    ctx.stroke();

    // Deck stripe
    ctx.strokeStyle = '#A0522D';
    ctx.lineWidth = 0.5;
    ctx.beginPath();
    ctx.moveTo(deckX + 3, deckY + deckH / 2);
    ctx.lineTo(deckX + deckW - 3, deckY + deckH / 2);
    ctx.stroke();

    // 4 wheels — two axles
    const wheelR = 2.5;
    const wheelY = y + deckH / 2 + wheelR;
    const wheelXs = [x - 10, x - 6, x + 6, x + 10];

    for (const wx of wheelXs) {
      this._drawSkateWheel(ctx, wx, wheelY, wheelR);
    }

    ctx.restore();
  }

  _drawSkateWheel(ctx, cx, cy, r) {
    // Tire
    ctx.fillStyle = '#1a1a1a';
    ctx.beginPath();
    ctx.arc(cx, cy, r, 0, Math.PI * 2);
    ctx.fill();

    // Hub
    ctx.fillStyle = '#555';
    ctx.beginPath();
    ctx.arc(cx, cy, r * 0.4, 0, Math.PI * 2);
    ctx.fill();

    // Spokes (3 for tiny wheels)
    ctx.strokeStyle = '#777';
    ctx.lineWidth = 0.5;
    for (let i = 0; i < 3; i++) {
      const angle = this.wheelAngle + (i * Math.PI * 2) / 3;
      ctx.beginPath();
      ctx.moveTo(cx + Math.cos(angle) * r * 0.3, cy + Math.sin(angle) * r * 0.3);
      ctx.lineTo(cx + Math.cos(angle) * r * 0.8, cy + Math.sin(angle) * r * 0.8);
      ctx.stroke();
    }
  }

  _drawEar(ctx, ex, ey, rotation, bodyFill) {
    ctx.save();
    ctx.translate(ex, ey);
    ctx.rotate(rotation);
    ctx.fillStyle = bodyFill;
    ctx.beginPath();
    ctx.ellipse(0, 0, 3, 3.5, 0, 0, Math.PI * 2);
    ctx.fill();
    ctx.fillStyle = PINK;
    ctx.beginPath();
    ctx.ellipse(0, 0, 1.8, 2.2, 0, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
  }

  _drawHamsterBody(ctx, x, y, color, activity) {
    ctx.save();
    ctx.translate(x, y);
    ctx.scale(HAMSTER_SCALE, HAMSTER_SCALE);
    ctx.translate(-x, -y);

    // Body color — golden tint on completion
    let bodyFill = BODY_COLOR;
    if (activity === 'complete' && this.goldFlash > 0) {
      const bc = hexToRgb(BODY_COLOR);
      const gold = { r: 255, g: 215, b: 0 };
      const f = this.goldFlash * 0.4;
      bodyFill = `rgb(${Math.round(bc.r + (gold.r - bc.r) * f)},${Math.round(bc.g + (gold.g - bc.g) * f)},${Math.round(bc.b + (gold.b - bc.b) * f)})`;
    }

    // --- Tail with wag (behind body) ---
    const tailWag = Math.sin(this.tailWagPhase) * 0.4;
    ctx.strokeStyle = bodyFill;
    ctx.lineWidth = 1.5;
    ctx.lineCap = 'round';
    ctx.beginPath();
    ctx.moveTo(x - 6, y - 6);
    ctx.quadraticCurveTo(
      x - 12 + tailWag * 3, y - 10 + tailWag * 2,
      x - 10, y - 15 + tailWag * 3,
    );
    ctx.stroke();

    // --- Body: warm brown ellipse ---
    ctx.fillStyle = bodyFill;
    ctx.beginPath();
    ctx.ellipse(x, y - 8, 7, 5, 0, 0, Math.PI * 2);
    ctx.fill();

    // Body highlight (lighter belly)
    ctx.fillStyle = BODY_LIGHT;
    ctx.beginPath();
    ctx.ellipse(x + 1, y - 7, 4, 2.5, 0.1, 0, Math.PI * 2);
    ctx.fill();

    // --- Head ---
    const headX = x + 3;
    const headY = y - 15;

    ctx.fillStyle = bodyFill;
    ctx.beginPath();
    ctx.ellipse(headX, headY, 5.5, 5, 0, 0, Math.PI * 2);
    ctx.fill();

    // Cheeks
    ctx.fillStyle = BODY_LIGHT;
    ctx.beginPath();
    ctx.ellipse(headX + 2, headY + 1, 3.5, 2.5, 0.2, 0, Math.PI * 2);
    ctx.fill();

    // --- Ears with wiggle ---
    const earWiggle = Math.sin(this.earWigglePhase) * 0.15;

    this._drawEar(ctx, headX - 4, headY - 4, -0.3 + earWiggle, bodyFill);
    this._drawEar(ctx, headX + 5, headY - 3.5, 0.3 - earWiggle, bodyFill);

    // --- Helmet (model color) — arc over top of head ---
    const prevAlpha = ctx.globalAlpha;
    ctx.globalAlpha = prevAlpha * 0.85;
    ctx.fillStyle = color.main;
    ctx.beginPath();
    ctx.ellipse(headX, headY - 1.5, 5.5, 3.5, 0, Math.PI, 0);
    ctx.fill();
    ctx.globalAlpha = prevAlpha;

    // Helmet highlight
    ctx.strokeStyle = color.light;
    ctx.lineWidth = 0.8;
    ctx.beginPath();
    ctx.ellipse(headX, headY - 1.5, 5.5, 3.5, 0, Math.PI + 0.3, -0.3);
    ctx.stroke();

    // Harness straps
    ctx.strokeStyle = color.dark;
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(headX - 3, headY + 3);
    ctx.lineTo(x - 4, y - 5);
    ctx.stroke();
    ctx.beginPath();
    ctx.moveTo(headX + 5, headY + 3);
    ctx.lineTo(x + 4, y - 5);
    ctx.stroke();

    // --- Eyes ---
    ctx.fillStyle = '#1a1a2e';
    ctx.beginPath();
    ctx.arc(headX + 1, headY, 1.2, 0, Math.PI * 2);
    ctx.fill();
    ctx.fillStyle = '#fff';
    ctx.beginPath();
    ctx.arc(headX + 1.5, headY - 0.5, 0.5, 0, Math.PI * 2);
    ctx.fill();

    ctx.fillStyle = '#1a1a2e';
    ctx.beginPath();
    ctx.arc(headX + 5, headY + 0.5, 1.2, 0, Math.PI * 2);
    ctx.fill();
    ctx.fillStyle = '#fff';
    ctx.beginPath();
    ctx.arc(headX + 5.5, headY, 0.5, 0, Math.PI * 2);
    ctx.fill();

    // --- Nose ---
    ctx.fillStyle = PINK;
    ctx.beginPath();
    ctx.arc(headX + 7, headY + 2, 0.8, 0, Math.PI * 2);
    ctx.fill();

    // --- Paws gripping deck edges ---
    ctx.fillStyle = bodyFill;
    ctx.beginPath();
    ctx.ellipse(x - 10, y - 2, 2, 1.5, -0.3, 0, Math.PI * 2);
    ctx.fill();
    ctx.beginPath();
    ctx.ellipse(x + 10, y - 2, 2, 1.5, 0.3, 0, Math.PI * 2);
    ctx.fill();

    ctx.restore();
  }

  _drawActivityIndicator(ctx, x, y, color, activity) {
    switch (activity) {
      case 'thinking':
        this._drawThoughtBubble(ctx, x, y);
        break;
      case 'tool_use':
        this._drawWrenchBadge(ctx, x, y, color);
        break;
    }
  }

  _drawThoughtBubble(ctx, x, y) {
    const S = HAMSTER_SCALE;
    const bx = x + 12 * S;
    const by = y - 18 * S;
    const bw = 16;
    const bh = 10;
    const br = 3;

    ctx.fillStyle = 'rgba(255,255,255,0.85)';
    ctx.beginPath();
    ctx.moveTo(bx + br, by);
    ctx.lineTo(bx + bw - br, by);
    ctx.quadraticCurveTo(bx + bw, by, bx + bw, by + br);
    ctx.lineTo(bx + bw, by + bh - br);
    ctx.quadraticCurveTo(bx + bw, by + bh, bx + bw - br, by + bh);
    ctx.lineTo(bx + br, by + bh);
    ctx.quadraticCurveTo(bx, by + bh, bx, by + bh - br);
    ctx.lineTo(bx, by + br);
    ctx.quadraticCurveTo(bx, by, bx + br, by);
    ctx.closePath();
    ctx.fill();

    // Tail circles connecting to hamster
    ctx.beginPath();
    ctx.arc(x + 10 * S, y - 14 * S, 2, 0, Math.PI * 2);
    ctx.fill();
    ctx.beginPath();
    ctx.arc(x + 8 * S, y - 12 * S, 1.5, 0, Math.PI * 2);
    ctx.fill();

    // Animated dots
    for (let i = 0; i < 3; i++) {
      const dotAlpha = 0.3 + 0.7 * Math.max(0, Math.sin(this.dotPhase * Math.PI * 2 - i * 1.2));
      ctx.fillStyle = `rgba(80,80,100,${dotAlpha})`;
      ctx.beginPath();
      ctx.arc(bx + 4 + i * 5, by + bh / 2, 1.5, 0, Math.PI * 2);
      ctx.fill();
    }
  }

  _drawWrenchBadge(ctx, x, y, color) {
    const S = HAMSTER_SCALE;
    const bx = x;
    const by = y + 6 * S;

    ctx.save();
    ctx.translate(bx, by);
    ctx.scale(0.6, 0.6);

    // Wrench handle
    ctx.strokeStyle = color.light;
    ctx.lineWidth = 2;
    ctx.lineCap = 'round';
    ctx.beginPath();
    ctx.moveTo(-4, 4);
    ctx.lineTo(2, -2);
    ctx.stroke();

    // Wrench head (open-end)
    ctx.beginPath();
    ctx.arc(3, -3, 3, -0.5, 1.5);
    ctx.stroke();

    ctx.restore();
  }

  _drawStarBurst(ctx, x, y) {
    const phase = this.starBurstPhase;
    const numRays = 8;
    const maxR = 15 * HAMSTER_SCALE;
    const fadeIn = Math.min(1, phase * 3);

    ctx.save();
    ctx.globalAlpha = this.opacity * fadeIn * Math.max(0.3, 1 - phase * 0.15);

    for (let i = 0; i < numRays; i++) {
      const angle = (i / numRays) * Math.PI * 2 + phase;
      const r = maxR * (0.5 + 0.5 * Math.sin(phase * 3 + i));

      ctx.strokeStyle = `rgba(255,215,0,${0.6 * fadeIn})`;
      ctx.lineWidth = 1.5;
      ctx.beginPath();
      ctx.moveTo(x + Math.cos(angle) * 5, y + Math.sin(angle) * 5);
      ctx.lineTo(x + Math.cos(angle) * r, y + Math.sin(angle) * r);
      ctx.stroke();
    }

    ctx.restore();
  }
}
