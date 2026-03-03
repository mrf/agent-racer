const FADE_DURATION = 0.2;

const DISMISS_AFTER = {
  thought: 8,
  speech: 4,
  zzz: 30,
  exclamation: 5,
  complete: 3,
};

const MAX_TEXT = 20;

// Tail tip: this many px above the car's display Y (y - TAIL_OFFSET)
const TAIL_OFFSET = 35;

export class SpeechBubble {
  static enabled = true;

  constructor() {
    this.type = null;
    this.text = '';
    this.alpha = 0;
    this.timer = 0;
    this.dismissAfter = 4;
    this.phase = 0;
    this.dotPhase = 0;
    this._state = 'idle'; // 'idle' | 'fadeIn' | 'visible' | 'fadeOut'
  }

  get isVisible() {
    return this._state !== 'idle' && this.alpha > 0;
  }

  show(type, text) {
    this.type = type;
    this.text = text ? String(text).slice(0, MAX_TEXT) : '';
    this.dismissAfter = DISMISS_AFTER[type] ?? 4;
    this.timer = 0;
    this._state = 'fadeIn';
  }

  hide() {
    if (this._state === 'visible' || this._state === 'fadeIn') {
      this._state = 'fadeOut';
    }
  }

  update(dt) {
    this.phase += dt;
    this.dotPhase += dt * 0.8;

    switch (this._state) {
      case 'fadeIn':
        this.alpha = Math.min(1, this.alpha + dt / FADE_DURATION);
        if (this.alpha >= 1) this._state = 'visible';
        break;

      case 'visible':
        this.timer += dt;
        if (this.timer >= this.dismissAfter - FADE_DURATION) {
          this._state = 'fadeOut';
        }
        break;

      case 'fadeOut':
        this.alpha = Math.max(0, this.alpha - dt / FADE_DURATION);
        if (this.alpha <= 0) {
          this._state = 'idle';
          this.type = null;
        }
        break;
    }
  }

  draw(ctx, x, y) {
    if (!SpeechBubble.enabled || this._state === 'idle' || this.alpha <= 0) return;

    ctx.save();
    ctx.globalAlpha *= this.alpha;

    switch (this.type) {
      case 'thought': this._drawThought(ctx, x, y); break;
      case 'speech': this._drawSpeech(ctx, x, y); break;
      case 'zzz': this._drawZzz(ctx, x, y); break;
      case 'exclamation': this._drawExclamation(ctx, x, y); break;
      case 'complete': this._drawComplete(ctx, x, y); break;
    }

    ctx.restore();
  }

  _tailY(y) {
    return y - TAIL_OFFSET;
  }

  _drawThought(ctx, x, y) {
    const tailY = this._tailY(y);

    // Tail: three diminishing dots leading from bubble down to car
    ctx.fillStyle = 'rgba(255,255,255,0.92)';
    ctx.beginPath();
    ctx.arc(x + 3, tailY + 3, 4, 0, Math.PI * 2);
    ctx.fill();
    ctx.beginPath();
    ctx.arc(x + 5, tailY - 5, 3, 0, Math.PI * 2);
    ctx.fill();
    ctx.beginPath();
    ctx.arc(x + 6, tailY - 12, 2, 0, Math.PI * 2);
    ctx.fill();

    // Cloud body: overlapping circles
    const cy = tailY - 32;
    const blobs = [
      [x - 16, cy + 5, 9],
      [x - 5,  cy - 3, 12],
      [x + 9,  cy - 5, 12],
      [x + 21, cy - 1, 10],
      [x + 29, cy + 5, 8],
      [x - 22, cy + 7, 7],
    ];

    ctx.beginPath();
    for (const [bx, by, r] of blobs) {
      ctx.moveTo(bx + r, by);
      ctx.arc(bx, by, r, 0, Math.PI * 2);
    }
    // Fill bottom gap between cloud circles and flat bottom
    ctx.rect(x - 24, cy, 58, 14);
    ctx.fill();

    // Animated dots inside cloud
    for (let i = 0; i < 3; i++) {
      const dotA = 0.35 + 0.65 * Math.max(0, Math.sin(this.dotPhase * Math.PI * 2 - i * 1.2));
      ctx.fillStyle = `rgba(80,80,130,${dotA})`;
      ctx.beginPath();
      ctx.arc(x - 8 + i * 10, cy + 7, 2.5, 0, Math.PI * 2);
      ctx.fill();
    }
  }

  _drawSpeech(ctx, x, y) {
    const tailY = this._tailY(y);
    const text = this.text || '...';

    ctx.font = 'bold 10px Courier New';
    const textW = ctx.measureText(text).width;
    const bw = Math.max(52, textW + 24);
    const bh = 22;
    const br = 6;
    const bx = x - bw / 2;
    const by = tailY - bh - 10;

    // Bubble body with downward tail
    ctx.fillStyle = 'rgba(18,22,48,0.93)';
    ctx.strokeStyle = 'rgba(100,160,255,0.75)';
    ctx.lineWidth = 1.5;

    ctx.beginPath();
    ctx.moveTo(bx + br, by);
    ctx.lineTo(bx + bw - br, by);
    ctx.arcTo(bx + bw, by, bx + bw, by + br, br);
    ctx.lineTo(bx + bw, by + bh - br);
    ctx.arcTo(bx + bw, by + bh, bx + bw - br, by + bh, br);
    ctx.lineTo(x + 7, by + bh);
    ctx.lineTo(x, tailY);      // tail tip
    ctx.lineTo(x - 7, by + bh);
    ctx.lineTo(bx + br, by + bh);
    ctx.arcTo(bx, by + bh, bx, by + bh - br, br);
    ctx.lineTo(bx, by + br);
    ctx.arcTo(bx, by, bx + br, by, br);
    ctx.closePath();
    ctx.fill();
    ctx.stroke();

    // Wrench icon
    ctx.font = '9px sans-serif';
    ctx.textAlign = 'left';
    ctx.textBaseline = 'middle';
    ctx.fillText('\uD83D\uDD27', bx + 5, by + bh / 2); // 🔧

    // Tool name
    ctx.fillStyle = 'rgba(180,210,255,0.95)';
    ctx.font = 'bold 10px Courier New';
    ctx.textAlign = 'center';
    ctx.fillText(text, x + 5, by + bh / 2);
  }

  _drawZzz(ctx, x, y) {
    const baseY = this._tailY(y) - 8;
    const sizes = [13, 10, 8];
    const offsets = [[0, 0], [9, -13], [16, -24]];

    for (let i = 0; i < 3; i++) {
      const [ox, oy] = offsets[i];
      const sz = sizes[i];
      const floatY = Math.sin(this.phase * 1.2 + i * 1.0) * 2;
      const a = 0.35 + 0.5 * Math.max(0, Math.sin(this.phase * 0.7 - i * 0.9));
      ctx.fillStyle = `rgba(140,170,255,${a})`;
      ctx.font = `bold ${sz}px sans-serif`;
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText('Z', x + ox, baseY + oy + floatY);
    }
  }

  _drawExclamation(ctx, x, y) {
    const cy = this._tailY(y) - 22;
    const r = 18;
    const spikes = 8;

    ctx.fillStyle = 'rgba(235,45,45,0.93)';
    ctx.strokeStyle = 'rgba(255,120,0,0.8)';
    ctx.lineWidth = 1.5;

    ctx.beginPath();
    for (let i = 0; i < spikes * 2; i++) {
      const angle = (i / (spikes * 2)) * Math.PI * 2 - Math.PI / 2;
      const jitter = i % 2 !== 0 ? Math.sin(this.phase * 4 + i) * 1.5 : 0;
      const rad = i % 2 === 0 ? r : r * 0.6 + jitter;
      const px = x + Math.cos(angle) * rad;
      const py = cy + Math.sin(angle) * rad;
      if (i === 0) ctx.moveTo(px, py);
      else ctx.lineTo(px, py);
    }
    ctx.closePath();
    ctx.fill();
    ctx.stroke();

    ctx.fillStyle = '#fff';
    ctx.font = 'bold 15px sans-serif';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText('!', x, cy + 1);
  }

  _drawComplete(ctx, x, y) {
    const cy = this._tailY(y) - 22;
    const r = 18;
    const points = 8;

    ctx.fillStyle = 'rgba(255,215,0,0.93)';
    ctx.strokeStyle = 'rgba(200,150,0,0.7)';
    ctx.lineWidth = 1;

    ctx.beginPath();
    for (let i = 0; i < points * 2; i++) {
      const angle = (i / (points * 2)) * Math.PI * 2 - Math.PI / 2;
      const rad = i % 2 === 0 ? r : r * 0.55;
      const px = x + Math.cos(angle) * rad;
      const py = cy + Math.sin(angle) * rad;
      if (i === 0) ctx.moveTo(px, py);
      else ctx.lineTo(px, py);
    }
    ctx.closePath();
    ctx.fill();
    ctx.stroke();

    // Checkmark
    ctx.strokeStyle = '#fff';
    ctx.lineWidth = 2.5;
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    ctx.beginPath();
    ctx.moveTo(x - 5, cy + 1);
    ctx.lineTo(x - 1, cy + 6);
    ctx.lineTo(x + 6, cy - 5);
    ctx.stroke();
  }
}
