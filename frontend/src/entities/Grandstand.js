const SHIRT_COLORS = [
  '#e94560', '#3b82f6', '#22c55e', '#a855f7', '#f59e0b',
  '#06b6d4', '#ec4899', '#f97316', '#84cc16', '#eee',
];
const SKIN_TONES = ['#f5d0a9', '#d4a574', '#c68642', '#8d5524', '#e0ac69', '#f1c27d'];

const SPEC_SPACING = 13;      // horizontal gap between spectators (px)
const ROW_STEP = 8;           // vertical separation between row baselines (px)
const ROWS_FULL = 3;
const ROWS_COMPACT = 2;
const GRANDSTAND_BOTTOM = 14; // px above trackBounds.y where front-row feet sit

// Wave crest travel speed for 'wave' reaction (px/s)
const WAVE_SPEED = 200;
// Radius in which 'cheer' reaction affects nearby spectators (px)
const CHEER_RADIUS = 90;

const REACTION_DURATIONS = {
  cheer: 1.2,
  wave: 2.0,
  ovation: 3.0,
  gasp: 1.5,
  mexican: 5.0,
};

export class Grandstand {
  constructor() {
    this._spectators = [];
    this._reactions = [];
    this._time = 0;
    this._mexicanPhase = 0;
    this._lastKey = '';
  }

  _buildSpectators(trackWidth, rowCount) {
    this._spectators = [];
    const count = Math.floor((trackWidth + 20) / SPEC_SPACING);
    for (let row = 0; row < rowCount; row++) {
      const xShift = row % 2 === 0 ? 0 : SPEC_SPACING / 2;
      for (let i = 0; i < count; i++) {
        this._spectators.push({
          // x relative to (trackBounds.x - 10)
          x: i * SPEC_SPACING + SPEC_SPACING / 2 + xShift,
          row,
          skinColor: SKIN_TONES[Math.floor(Math.random() * SKIN_TONES.length)],
          shirtColor: SHIRT_COLORS[Math.floor(Math.random() * SHIRT_COLORS.length)],
          phase: Math.random() * Math.PI * 2,
          cheerThreshold: 0.3 + Math.random() * 0.65,
        });
      }
    }
  }

  /**
   * Trigger a crowd reaction event.
   * @param {'cheer'|'wave'|'ovation'|'gasp'|'mexican'} type
   * @param {number} normalizedX - 0-1 position along track width (used by cheer/wave)
   */
  trigger(type, normalizedX = 0.5) {
    if (this._reactions.some(r => r.type === type && r.t < 0.3)) return;
    this._reactions.push({
      type,
      x: normalizedX,
      t: 0,
      duration: REACTION_DURATIONS[type] ?? 1.0,
    });
  }

  update(dt) {
    this._time += dt;
    for (const r of this._reactions) r.t += dt;
    this._reactions = this._reactions.filter(r => r.t < r.duration);
    if (this._reactions.some(r => r.type === 'mexican')) {
      this._mexicanPhase = (this._mexicanPhase + dt * 0.5) % 1.0;
    }
  }

  draw(ctx, trackBounds, crowdMode, excitement) {
    const rowCount = crowdMode === 'compact' ? ROWS_COMPACT : ROWS_FULL;
    const standH = rowCount * ROW_STEP + 6;
    const baseX = trackBounds.x - 10;
    const standW = trackBounds.width + 20;
    const standBottom = trackBounds.y - GRANDSTAND_BOTTOM;
    const standTop = standBottom - standH;

    const key = `${trackBounds.width}|${rowCount}`;
    if (key !== this._lastKey) {
      this._lastKey = key;
      this._buildSpectators(trackBounds.width, rowCount);
    }

    this._drawBackdrop(ctx, baseX, standTop, standW, standH + 4);
    this._drawAllSpectators(ctx, baseX, standBottom, trackBounds.width, excitement);
  }

  _drawBackdrop(ctx, x, y, w, h) {
    const grad = ctx.createLinearGradient(x, y, x, y + h);
    grad.addColorStop(0, '#1a1a30');
    grad.addColorStop(1, '#252545');
    ctx.fillStyle = grad;
    ctx.fillRect(x, y, w, h);
    ctx.fillStyle = 'rgba(0,0,0,0.35)';
    ctx.fillRect(x, y, w, 3);
  }

  _drawAllSpectators(ctx, baseX, baseY, trackWidth, excitement) {
    const clampedExcitement = Math.min(1, Math.max(0, excitement));
    const totalWidth = trackWidth + 20;

    for (const spec of this._spectators) {
      const absX = baseX + spec.x;
      const rowY = baseY - spec.row * ROW_STEP;
      const normX = spec.x / totalWidth;

      let bounce = Math.sin(this._time * 0.5 + spec.phase) * 0.4;
      let armRaise = false;
      let leansForward = false;

      // Excitement-threshold cheering
      if (clampedExcitement > spec.cheerThreshold) {
        bounce += Math.abs(Math.sin(this._time * 5 + spec.phase)) * 3 * clampedExcitement;
        armRaise = true;
      }

      // Apply active reactions
      for (const r of this._reactions) {
        const progress = r.t / r.duration;
        switch (r.type) {
          case 'cheer': {
            const dist = Math.abs(normX - r.x) * trackWidth;
            if (dist < CHEER_RADIUS) {
              const intensity = (1 - dist / CHEER_RADIUS) * Math.max(0, 1 - progress / 0.8);
              bounce += Math.abs(Math.sin(this._time * 8 + spec.phase)) * 4 * intensity;
              if (intensity > 0.2) armRaise = true;
            }
            break;
          }
          case 'wave': {
            const specAbsX = normX * trackWidth;
            const eventAbsX = r.x * trackWidth;
            const dist = Math.abs(specAbsX - eventAbsX);
            const crest = r.t * WAVE_SPEED;
            const waveDist = Math.abs(dist - crest);
            if (waveDist < 25) {
              const intensity = Math.max(0, 1 - waveDist / 25);
              bounce += intensity * 5;
              if (intensity > 0.3) armRaise = true;
            }
            break;
          }
          case 'ovation': {
            const decay = progress < 0.6 ? 1 : Math.max(0, (1 - progress) / 0.4);
            bounce += Math.abs(Math.sin(this._time * 4 + spec.phase * 0.2)) * 6 * decay;
            armRaise = true;
            break;
          }
          case 'gasp': {
            const intensity = progress < 0.5 ? 1 : Math.max(0, (1 - progress) * 2);
            bounce -= 2 * intensity;
            leansForward = intensity > 0.3;
            break;
          }
          case 'mexican': {
            const dist = Math.abs(normX - this._mexicanPhase);
            const wrapDist = Math.min(dist, 1 - dist);
            if (wrapDist < 0.05) {
              const intensity = Math.max(0, 1 - wrapDist / 0.05);
              bounce += intensity * 5;
              if (intensity > 0.5) armRaise = true;
            }
            break;
          }
        }
      }

      this._drawSpec(ctx, absX, rowY, spec, bounce, armRaise, leansForward);
    }
  }

  _drawSpec(ctx, x, y, spec, bounce, armRaise, leansForward) {
    const halfW = 2;
    const bodyH = 5;
    const headR = 2.5;
    const armLen = 4;
    const leanX = leansForward ? 2 : 0;
    const feetY = y - bounce;
    const bodyTop = feetY - bodyH;
    const headCY = bodyTop - headR;

    ctx.fillStyle = spec.shirtColor;
    ctx.fillRect(x - halfW + leanX, bodyTop, halfW * 2, bodyH);

    ctx.fillStyle = spec.skinColor;
    ctx.beginPath();
    ctx.arc(x + leanX, headCY, headR, 0, Math.PI * 2);
    ctx.fill();

    ctx.strokeStyle = spec.skinColor;
    ctx.lineWidth = 1;
    const shoulderY = bodyTop + 1;

    if (armRaise) {
      const wave = Math.sin(this._time * 8 + spec.phase) * 0.5;
      ctx.beginPath();
      ctx.moveTo(x - halfW + leanX, shoulderY);
      ctx.lineTo(x - halfW - armLen + wave + leanX, headCY - armLen * 0.5);
      ctx.stroke();
      ctx.beginPath();
      ctx.moveTo(x + halfW + leanX, shoulderY);
      ctx.lineTo(x + halfW + armLen - wave + leanX, headCY - armLen * 0.5);
      ctx.stroke();
    } else if (leansForward) {
      ctx.beginPath();
      ctx.moveTo(x - halfW + leanX, shoulderY);
      ctx.lineTo(x - halfW - 1 + leanX, shoulderY + 3);
      ctx.stroke();
      ctx.beginPath();
      ctx.moveTo(x + halfW + leanX, shoulderY);
      ctx.lineTo(x + halfW + 1 + leanX, shoulderY + 3);
      ctx.stroke();
    } else {
      ctx.beginPath();
      ctx.moveTo(x - halfW + leanX, shoulderY);
      ctx.lineTo(x - halfW - 1 + leanX, feetY - 1);
      ctx.stroke();
      ctx.beginPath();
      ctx.moveTo(x + halfW + leanX, shoulderY);
      ctx.lineTo(x + halfW + 1 + leanX, feetY - 1);
      ctx.stroke();
    }
  }
}
