import { describe, it, expect } from 'vitest';
import { ParticleSystem } from './Particles.js';

const FRAME = 1 / 60;

function makeParticle(overrides = {}) {
  return {
    x: 0, y: 0, vx: 0, vy: 0,
    life: 1.0, decay: 0.01, gravity: 0,
    ...overrides,
  };
}

function systemWith(overrides) {
  const sys = new ParticleSystem();
  sys.particles.push(makeParticle(overrides));
  return sys;
}

describe('ParticleSystem', () => {
  describe('_hslToRgb', () => {
    const sys = new ParticleSystem();

    it('converts pure red (h=0, s=100, l=50)', () => {
      expect(sys._hslToRgb(0, 100, 50)).toEqual([255, 0, 0]);
    });

    it('converts pure green (h=120, s=100, l=50)', () => {
      expect(sys._hslToRgb(120, 100, 50)).toEqual([0, 255, 0]);
    });

    it('converts pure blue (h=240, s=100, l=50)', () => {
      expect(sys._hslToRgb(240, 100, 50)).toEqual([0, 0, 255]);
    });

    it('converts white (s=0, l=100)', () => {
      expect(sys._hslToRgb(0, 0, 100)).toEqual([255, 255, 255]);
    });

    it('converts black (s=0, l=0)', () => {
      expect(sys._hslToRgb(0, 0, 0)).toEqual([0, 0, 0]);
    });

    it('converts mid-gray (s=0, l=50)', () => {
      expect(sys._hslToRgb(0, 0, 50)).toEqual([128, 128, 128]);
    });

    it('converts yellow (h=60, s=100, l=50)', () => {
      expect(sys._hslToRgb(60, 100, 50)).toEqual([255, 255, 0]);
    });

    it('converts cyan (h=180, s=100, l=50)', () => {
      expect(sys._hslToRgb(180, 100, 50)).toEqual([0, 255, 255]);
    });

    it('handles partial saturation', () => {
      const [r, g, b] = sys._hslToRgb(0, 50, 50);
      expect(r).toBeGreaterThan(g);
      expect(r).toBeGreaterThan(b);
      expect(g).toBe(b);
    });
  });

  describe('_lerpColor', () => {
    const sys = new ParticleSystem();
    const red = { r: 255, g: 0, b: 0 };
    const blue = { r: 0, g: 0, b: 255 };

    it('returns start color at t=1 (start of life)', () => {
      expect(sys._lerpColor(red, blue, 1)).toEqual({ r: 255, g: 0, b: 0 });
    });

    it('returns end color at t=0 (end of life)', () => {
      expect(sys._lerpColor(red, blue, 0)).toEqual({ r: 0, g: 0, b: 255 });
    });

    it('returns midpoint at t=0.5', () => {
      expect(sys._lerpColor(red, blue, 0.5)).toEqual({ r: 128, g: 0, b: 128 });
    });

    it('works with identical colors', () => {
      const gray = { r: 100, g: 100, b: 100 };
      expect(sys._lerpColor(gray, gray, 0.3)).toEqual({ r: 100, g: 100, b: 100 });
    });
  });

  describe('lifespan decay', () => {
    it('decreases life by decay each frame at 60fps baseline', () => {
      const sys = systemWith({ decay: 0.1 });
      sys.update(FRAME);
      expect(sys.particles[0].life).toBeCloseTo(0.9, 5);
    });

    it('removes particle when life reaches 0', () => {
      const sys = systemWith({ life: 0.05, decay: 0.1 });
      sys.update(FRAME);
      expect(sys.particles).toHaveLength(0);
    });

    it('scales decay with dt', () => {
      const sys = systemWith({ decay: 0.1 });
      sys.update(2 * FRAME);
      expect(sys.particles[0].life).toBeCloseTo(0.8, 5);
    });

    it('defaults dtScale to 1 when dt is undefined', () => {
      const sys = systemWith({ decay: 0.25 });
      sys.update();
      expect(sys.particles[0].life).toBeCloseTo(0.75, 5);
    });
  });

  describe('_getSizeMultiplier', () => {
    const sys = new ParticleSystem();

    describe('bloom curve', () => {
      it('returns 1.0 at life=1.0', () => {
        expect(sys._getSizeMultiplier(1.0, 'bloom')).toBeCloseTo(1.0, 5);
      });

      it('peaks at 1.2 at life=0.7', () => {
        expect(sys._getSizeMultiplier(0.7, 'bloom')).toBeCloseTo(1.2, 5);
      });

      it('returns 0.3 at life=0.0', () => {
        expect(sys._getSizeMultiplier(0.0, 'bloom')).toBeCloseTo(0.3, 5);
      });

      it('is between 0.3 and 1.2 at life=0.35', () => {
        expect(sys._getSizeMultiplier(0.35, 'bloom')).toBeCloseTo(0.75, 5);
      });

      it('increases from 1.0 toward 1.2 as life drops from 1.0 to 0.7', () => {
        const at1 = sys._getSizeMultiplier(1.0, 'bloom');
        const at085 = sys._getSizeMultiplier(0.85, 'bloom');
        const at07 = sys._getSizeMultiplier(0.7, 'bloom');
        expect(at085).toBeGreaterThan(at1);
        expect(at07).toBeGreaterThan(at085);
      });
    });

    describe('default curve', () => {
      it('returns 0.8 at life=1.0', () => {
        expect(sys._getSizeMultiplier(1.0, null)).toBeCloseTo(0.8, 5);
      });

      it('peaks at 1.2 at life=0.7', () => {
        expect(sys._getSizeMultiplier(0.7, null)).toBeCloseTo(1.2, 5);
      });

      it('returns 0.3 at life=0.0', () => {
        expect(sys._getSizeMultiplier(0.0, null)).toBeCloseTo(0.3, 5);
      });

      it('increases from 0.8 toward 1.2 as life drops from 1.0 to 0.7', () => {
        const at1 = sys._getSizeMultiplier(1.0, null);
        const at07 = sys._getSizeMultiplier(0.7, null);
        expect(at07).toBeGreaterThan(at1);
      });
    });
  });

  describe('emit', () => {
    it('creates particles with the given preset', () => {
      const sys = new ParticleSystem();
      sys.emit('exhaust', 10, 20, 3);
      expect(sys.particles).toHaveLength(3);
      for (const p of sys.particles) {
        expect(p.x).toBe(10);
        expect(p.y).toBe(20);
        expect(p.sizeMultiplier).toBe('bloom');
      }
    });

    it('defaults to 5 particles', () => {
      const sys = new ParticleSystem();
      sys.emit('sparks', 0, 0);
      expect(sys.particles).toHaveLength(5);
    });

    it('accumulates particles across calls', () => {
      const sys = new ParticleSystem();
      sys.emit('smoke', 0, 0, 2);
      sys.emit('smoke', 0, 0, 3);
      expect(sys.particles).toHaveLength(5);
    });
  });

  describe('emitWithColor', () => {
    it('overrides particle color with provided color', () => {
      const sys = new ParticleSystem();
      const override = { r: 10, g: 20, b: 30 };
      sys.emitWithColor('exhaust', 5, 10, 2, override);
      expect(sys.particles).toHaveLength(2);
      for (const p of sys.particles) {
        expect(p.color).toEqual(override);
        expect(p.x).toBe(5);
        expect(p.y).toBe(10);
      }
    });

    it('does not override color when colorOverride is null', () => {
      const sys = new ParticleSystem();
      sys.emitWithColor('exhaust', 0, 0, 1, null);
      // exhaust default color
      expect(sys.particles[0].color.r).toBe(240);
    });

    it('creates a copy of the override so mutations are isolated', () => {
      const sys = new ParticleSystem();
      const override = { r: 100, g: 100, b: 100 };
      sys.emitWithColor('sparks', 0, 0, 1, override);
      override.r = 0;
      expect(sys.particles[0].color.r).toBe(100);
    });
  });

  describe('createParticle presets', () => {
    const sys = new ParticleSystem();

    it('exhaust has bloom sizeMultiplier and behind layer', () => {
      const p = sys.createParticle('exhaust', 0, 0);
      expect(p.sizeMultiplier).toBe('bloom');
      expect(p.layer).toBe('behind');
      expect(p.colorEnd).toBeTruthy();
    });

    it('sparks has gravity and front layer', () => {
      const p = sys.createParticle('sparks', 0, 0);
      expect(p.gravity).toBe(0.05);
      expect(p.layer).toBe('front');
    });

    it('confetti has rotation and flutter', () => {
      const p = sys.createParticle('confetti', 0, 0);
      expect(p.rotation).toBeDefined();
      expect(p.flutter).toBeGreaterThan(0);
      expect(p.gravity).toBe(0.08);
      expect(p.layer).toBe('front');
    });

    it('speedLines uses rect drawMode', () => {
      const p = sys.createParticle('speedLines', 0, 0);
      expect(p.drawMode).toBe('rect');
      expect(p.width).toBeGreaterThan(0);
    });

    it('celebration produces both streamer and circle drawModes', () => {
      const modes = new Set();
      for (let i = 0; i < 100; i++) {
        modes.add(sys.createParticle('celebration', 0, 0).drawMode);
      }
      expect(modes.has('streamer')).toBe(true);
      expect(modes.has('circle')).toBe(true);
    });

    it('skidMarks has very low decay', () => {
      const p = sys.createParticle('skidMarks', 0, 0);
      expect(p.decay).toBe(0.002);
      expect(p.gravity).toBe(0);
    });

    it('smoke has bloom sizeMultiplier', () => {
      const p = sys.createParticle('smoke', 0, 0);
      expect(p.sizeMultiplier).toBe('bloom');
    });

    it('flame has bloom sizeMultiplier and color gradient', () => {
      const p = sys.createParticle('flame', 0, 0);
      expect(p.sizeMultiplier).toBe('bloom');
      expect(p.color.r).toBe(255);
      expect(p.colorEnd.r).toBe(255);
    });

    it('blueFlame has blue color range', () => {
      const p = sys.createParticle('blueFlame', 0, 0);
      expect(p.color.b).toBe(255);
      expect(p.colorEnd.b).toBe(200);
    });

    it('redSparks has front layer and gravity', () => {
      const p = sys.createParticle('redSparks', 0, 0);
      expect(p.layer).toBe('front');
      expect(p.gravity).toBe(0.06);
    });

    it('afterburn has bloom sizeMultiplier', () => {
      const p = sys.createParticle('afterburn', 0, 0);
      expect(p.sizeMultiplier).toBe('bloom');
    });

    it('prismatic picks a color band and has bloom', () => {
      const p = sys.createParticle('prismatic', 0, 0);
      expect(p.sizeMultiplier).toBe('bloom');
      expect(p.baseAlpha).toBe(0.7);
    });

    it('confettiBurst uses rect drawMode and front layer', () => {
      const p = sys.createParticle('confettiBurst', 0, 0);
      expect(p.drawMode).toBe('rect');
      expect(p.layer).toBe('front');
      expect(p.gravity).toBe(0.07);
    });

    it('tireSmoke has bloom sizeMultiplier', () => {
      const p = sys.createParticle('tireSmoke', 0, 0);
      expect(p.sizeMultiplier).toBe('bloom');
    });

    it('snowfall has flutter and gravity', () => {
      const p = sys.createParticle('snowfall', 0, 0);
      expect(p.flutter).toBeGreaterThan(0);
      expect(p.gravity).toBe(0.01);
    });

    it('sakura has rotation and flutter', () => {
      const p = sys.createParticle('sakura', 0, 0);
      expect(p.rotation).toBeDefined();
      expect(p.flutter).toBeGreaterThan(0);
      expect(p.gravity).toBe(0.015);
    });

    it('autumn has rotation and flutter', () => {
      const p = sys.createParticle('autumn', 0, 0);
      expect(p.rotation).toBeDefined();
      expect(p.flutter).toBeGreaterThan(0);
      expect(p.gravity).toBe(0.02);
    });

    it('unknown preset returns base particle', () => {
      const p = sys.createParticle('nonexistent', 100, 200);
      expect(p.x).toBe(100);
      expect(p.y).toBe(200);
      expect(p.life).toBe(1.0);
      expect(p.layer).toBe('behind');
      expect(p.drawMode).toBe('circle');
    });
  });

  describe('clear', () => {
    it('removes all particles', () => {
      const sys = new ParticleSystem();
      sys.emit('exhaust', 0, 0, 10);
      expect(sys.particles.length).toBe(10);
      sys.clear();
      expect(sys.particles).toHaveLength(0);
    });
  });

  describe('update position and rotation', () => {
    it('moves particle by vx and vy', () => {
      const sys = systemWith({ x: 10, y: 20, vx: 3, vy: -2 });
      sys.update(FRAME);
      expect(sys.particles[0].x).toBeCloseTo(13, 5);
      expect(sys.particles[0].y).toBeCloseTo(18, 5);
    });

    it('advances rotation by rotSpeed', () => {
      const sys = new ParticleSystem();
      sys.particles.push(makeParticle({ rotation: 0, rotSpeed: 0.5 }));
      sys.update(FRAME);
      expect(sys.particles[0].rotation).toBeCloseTo(0.5, 5);
    });

    it('applies flutter to vx based on life and flutterSpeed', () => {
      const sys = new ParticleSystem();
      sys.particles.push(makeParticle({
        vx: 0, flutter: 2.0, flutterSpeed: 1, life: 0.5,
      }));
      const vxBefore = sys.particles[0].vx;
      sys.update(FRAME);
      expect(sys.particles[0].vx).not.toBe(vxBefore);
    });
  });

  describe('gravity application', () => {
    it('increases vy by gravity each frame', () => {
      const sys = systemWith({ gravity: 0.1 });
      sys.update(FRAME);
      expect(sys.particles[0].vy).toBeCloseTo(0.1, 5);
    });

    it('accumulates gravity over multiple frames', () => {
      const sys = systemWith({ gravity: 0.05 });
      sys.update(FRAME);
      sys.update(FRAME);
      sys.update(FRAME);
      expect(sys.particles[0].vy).toBeCloseTo(0.15, 5);
    });

    it('does not apply gravity when gravity is 0', () => {
      const sys = systemWith({ vy: 1.0, gravity: 0 });
      sys.update(FRAME);
      expect(sys.particles[0].vy).toBeCloseTo(1.0, 5);
    });

    it('scales gravity with dt', () => {
      const sys = systemWith({ gravity: 0.1 });
      sys.update(2 * FRAME);
      expect(sys.particles[0].vy).toBeCloseTo(0.2, 5);
    });

    it('affects y position through vy', () => {
      const sys = systemWith({ y: 100, gravity: 0.5 });
      sys.update(FRAME);
      expect(sys.particles[0].vy).toBeCloseTo(0.5, 5);
      sys.update(FRAME);
      expect(sys.particles[0].y).toBeCloseTo(100.5, 5);
      expect(sys.particles[0].vy).toBeCloseTo(1.0, 5);
    });
  });
});
