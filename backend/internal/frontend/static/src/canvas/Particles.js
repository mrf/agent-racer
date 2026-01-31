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
    };

    switch (preset) {
      case 'exhaust':
        return {
          ...base,
          vx: -1.5 - Math.random() * 2,
          vy: (Math.random() - 0.5) * 1.5,
          size: 3 + Math.random() * 4,
          color: { r: 200, g: 200, b: 200 },
          decay: 0.03 + Math.random() * 0.02,
        };
      case 'sparks':
        return {
          ...base,
          vx: -2 + Math.random() * 4,
          vy: -2 + Math.random() * 4,
          size: 2 + Math.random() * 2,
          color: { r: 255, g: 200 + Math.random() * 55, b: 50 },
          decay: 0.05 + Math.random() * 0.03,
        };
      case 'smoke':
        return {
          ...base,
          vx: (Math.random() - 0.5) * 3,
          vy: -1 - Math.random() * 2,
          size: 6 + Math.random() * 8,
          color: { r: 80, g: 80, b: 80 },
          decay: 0.015 + Math.random() * 0.01,
        };
      case 'confetti':
        return {
          ...base,
          vx: -3 + Math.random() * 6,
          vy: -4 - Math.random() * 3,
          size: 4 + Math.random() * 4,
          color: {
            r: Math.random() * 255,
            g: Math.random() * 255,
            b: Math.random() * 255,
          },
          decay: 0.008 + Math.random() * 0.005,
          gravity: 0.1,
          rotation: Math.random() * Math.PI * 2,
          rotSpeed: (Math.random() - 0.5) * 0.2,
        };
      default:
        return base;
    }
  }

  update() {
    for (let i = this.particles.length - 1; i >= 0; i--) {
      const p = this.particles[i];
      p.x += p.vx || 0;
      p.y += p.vy || 0;
      p.life -= p.decay;

      if (p.gravity) {
        p.vy += p.gravity;
      }
      if (p.rotation !== undefined) {
        p.rotation += p.rotSpeed;
      }

      if (p.life <= 0) {
        this.particles.splice(i, 1);
      }
    }
  }

  draw(ctx) {
    for (const p of this.particles) {
      ctx.save();
      const alpha = Math.max(0, p.life);
      const { r, g, b } = p.color || { r: 255, g: 255, b: 255 };

      if (p.rotation !== undefined) {
        ctx.translate(p.x, p.y);
        ctx.rotate(p.rotation);
        ctx.fillStyle = `rgba(${r},${g},${b},${alpha})`;
        ctx.fillRect(-p.size / 2, -p.size / 4, p.size, p.size / 2);
      } else {
        ctx.globalAlpha = alpha;
        ctx.fillStyle = `rgb(${r},${g},${b})`;
        ctx.beginPath();
        ctx.arc(p.x, p.y, p.size * alpha, 0, Math.PI * 2);
        ctx.fill();
      }
      ctx.restore();
    }
  }

  clear() {
    this.particles = [];
  }
}
