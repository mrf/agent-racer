export class ParticleSystem {
  constructor() {
    this.particles = [];
  }

  emit(preset, x, y, count = 5) {
    for (let i = 0; i < count; i++) {
      this.particles.push(this.createParticle(preset, x, y));
    }
  }

  createParticle(preset, x, y) {
    const base = {
      x, y,
      life: 1.0,
      decay: 0.02 + Math.random() * 0.03,
      layer: 'behind',
      colorEnd: null,
      sizeMultiplier: null, // use default curve
      gravity: 0,
      flutter: 0,
      flutterSpeed: 0,
      drawMode: 'circle', // 'circle' | 'rect'
      width: 0,
      height: 0,
    };

    switch (preset) {
      case 'exhaust':
        return {
          ...base,
          vx: -1.2 - Math.random() * 1.5,
          vy: (Math.random() - 0.5) * 2.0,
          size: 4 + Math.random() * 4,
          color: { r: 240, g: 240, b: 255 },
          colorEnd: { r: 100, g: 100, b: 110 },
          decay: 0.015 + Math.random() * 0.01,
          sizeMultiplier: 'bloom',
        };
      case 'sparks':
        return {
          ...base,
          vx: -2 + Math.random() * 5,
          vy: -2 + Math.random() * 3,
          size: 1 + Math.random() * 2,
          color: { r: 255, g: 255, b: 100 },
          colorEnd: { r: 255, g: 120, b: 20 },
          decay: 0.04 + Math.random() * 0.02,
          gravity: 0.05,
          layer: 'front',
        };
      case 'smoke':
        return {
          ...base,
          vx: (Math.random() - 0.5) * 2,
          vy: -0.5 - Math.random() * 1,
          size: 8 + Math.random() * 8,
          color: { r: 60, g: 60, b: 70 },
          colorEnd: { r: 40, g: 40, b: 50 },
          decay: 0.008 + Math.random() * 0.004,
          sizeMultiplier: 'bloom',
        };
      case 'confetti':
        return {
          ...base,
          vx: -3 + Math.random() * 6,
          vy: -4 - Math.random() * 3,
          size: 5 + Math.random() * 5,
          color: this._saturatedColor(),
          decay: 0.005 + Math.random() * 0.003,
          gravity: 0.08,
          rotation: Math.random() * Math.PI * 2,
          rotSpeed: (Math.random() - 0.5) * 0.2,
          flutter: 1.5 + Math.random(),
          flutterSpeed: 3 + Math.random() * 2,
          layer: 'front',
        };
      case 'speedLines':
        return {
          ...base,
          vx: -4 - Math.random() * 4,
          vy: 0,
          size: 0, // unused for rect mode
          width: 10 + Math.random() * 10,
          height: 1 + Math.random(),
          color: { r: 150, g: 150, b: 200 }, // overridden by caller via colorOverride
          decay: 0.06 + Math.random() * 0.04,
          drawMode: 'rect',
          baseAlpha: 0.3,
        };
      case 'celebration':
        return {
          ...base,
          vx: -4 + Math.random() * 8,
          vy: -5 - Math.random() * 5,
          size: 6 + Math.random() * 6,
          color: this._saturatedColor(),
          decay: 0.003 + Math.random() * 0.003,
          gravity: 0.06,
          rotation: Math.random() * Math.PI * 2,
          rotSpeed: (Math.random() - 0.5) * 0.15,
          flutter: 2 + Math.random(),
          flutterSpeed: 2 + Math.random() * 2,
          layer: 'front',
          // Some are streamers (tall thin rects)
          drawMode: Math.random() > 0.5 ? 'streamer' : 'circle',
        };
      case 'skidMarks':
        return {
          ...base,
          vx: -0.5 + Math.random(),
          vy: 0,
          size: 2 + Math.random() * 2,
          color: { r: 30, g: 30, b: 40 },
          decay: 0.002,
          gravity: 0,
        };
      default:
        return base;
    }
  }

  _saturatedColor() {
    const hue = Math.random() * 360;
    const c = this._hslToRgb(hue, 90, 60);
    return { r: c[0], g: c[1], b: c[2] };
  }

  _hslToRgb(h, s, l) {
    s /= 100; l /= 100;
    const k = n => (n + h / 30) % 12;
    const a = s * Math.min(l, 1 - l);
    const f = n => l - a * Math.max(-1, Math.min(k(n) - 3, 9 - k(n), 1));
    return [Math.round(f(0) * 255), Math.round(f(8) * 255), Math.round(f(4) * 255)];
  }

  emitWithColor(preset, x, y, count, colorOverride) {
    for (let i = 0; i < count; i++) {
      const p = this.createParticle(preset, x, y);
      if (colorOverride) {
        p.color = { ...colorOverride };
      }
      this.particles.push(p);
    }
  }

  update(dt) {
    const dtScale = dt ? dt / (1 / 60) : 1; // normalize to 60fps baseline
    for (let i = this.particles.length - 1; i >= 0; i--) {
      const p = this.particles[i];
      p.x += (p.vx || 0) * dtScale;
      p.y += (p.vy || 0) * dtScale;
      p.life -= p.decay * dtScale;

      if (p.gravity) {
        p.vy += p.gravity * dtScale;
      }
      if (p.rotation !== undefined) {
        p.rotation += (p.rotSpeed || 0) * dtScale;
      }
      if (p.flutter) {
        p.vx += Math.sin(p.life * p.flutterSpeed * Math.PI * 2) * p.flutter * 0.02 * dtScale;
      }

      if (p.life <= 0) {
        this.particles.splice(i, 1);
      }
    }
  }

  _getSizeMultiplier(life, curve) {
    if (curve === 'bloom') {
      // Grows to 1.2x at life=0.7, shrinks to 0.3x at life=0.0
      if (life > 0.7) {
        return 1.0 + (1.0 - life) / 0.3 * 0.2; // 1.0 -> 1.2
      }
      return 0.3 + (life / 0.7) * 0.9; // 0.3 -> 1.2
    }
    // Default: gentle bloom then fade
    if (life > 0.7) {
      return 0.8 + (1.0 - life) / 0.3 * 0.4;
    }
    return 0.3 + (life / 0.7) * 0.9;
  }

  _lerpColor(c1, c2, t) {
    // t goes from 1.0 (start) to 0.0 (end), so invert for lerp
    const f = 1 - t; // 0 at start, 1 at end
    return {
      r: Math.round(c1.r + (c2.r - c1.r) * f),
      g: Math.round(c1.g + (c2.g - c1.g) * f),
      b: Math.round(c1.b + (c2.b - c1.b) * f),
    };
  }

  drawBehind(ctx) {
    this._drawFiltered(ctx, 'behind');
  }

  drawFront(ctx) {
    this._drawFiltered(ctx, 'front');
  }

  _drawFiltered(ctx, layer) {
    for (const p of this.particles) {
      if ((p.layer || 'behind') !== layer) continue;
      this._drawParticle(ctx, p);
    }
  }

  // Legacy method for compatibility
  draw(ctx) {
    for (const p of this.particles) {
      this._drawParticle(ctx, p);
    }
  }

  _drawParticle(ctx, p) {
    ctx.save();
    const alpha = Math.max(0, p.life) * (p.baseAlpha || 1);

    // Compute color (with gradient if colorEnd set)
    let color = p.color || { r: 255, g: 255, b: 255 };
    if (p.colorEnd) {
      color = this._lerpColor(p.color, p.colorEnd, p.life);
    }
    const { r, g, b } = color;

    // Compute size with curve
    const sizeMult = p.sizeMultiplier ? this._getSizeMultiplier(p.life, p.sizeMultiplier) : 1;

    if (p.drawMode === 'rect' || p.drawMode === 'streamer') {
      // Rectangular particles (speed lines, streamers)
      ctx.globalAlpha = alpha;
      ctx.fillStyle = `rgb(${r},${g},${b})`;
      if (p.rotation !== undefined) {
        ctx.translate(p.x, p.y);
        ctx.rotate(p.rotation);
        if (p.drawMode === 'streamer') {
          ctx.fillRect(-1.5, -6, 3, 12);
        } else {
          ctx.fillRect(-p.width / 2, -p.height / 2, p.width, p.height);
        }
      } else {
        ctx.fillRect(p.x - (p.width || p.size) / 2, p.y - (p.height || p.size) / 2,
          p.width || p.size, p.height || p.size);
      }
    } else if (p.rotation !== undefined) {
      // Rotated confetti rectangles
      ctx.translate(p.x, p.y);
      ctx.rotate(p.rotation);
      ctx.fillStyle = `rgba(${r},${g},${b},${alpha})`;
      const s = p.size * sizeMult;
      ctx.fillRect(-s / 2, -s / 4, s, s / 2);
    } else {
      // Circle particles
      ctx.globalAlpha = alpha;
      ctx.fillStyle = `rgb(${r},${g},${b})`;
      ctx.beginPath();
      const radius = p.size * sizeMult * Math.max(0.1, p.life);
      ctx.arc(p.x, p.y, Math.max(0.5, radius), 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.restore();
  }

  clear() {
    this.particles = [];
  }
}
