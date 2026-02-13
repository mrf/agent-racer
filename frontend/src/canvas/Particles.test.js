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
