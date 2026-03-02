// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

vi.mock('../auth.js', () => ({
  authFetch: vi.fn(),
}));

import { authFetch } from '../auth.js';
import {
  getEquippedPaint,
  getEquippedTrail,
  getEquippedBody,
  getEquippedBadge,
  getEquippedTitle,
  getEquippedLoadout,
  setEquipped,
  onEquippedChange,
  hydrate,
  isHydrated,
} from './CosmeticRegistry.js';

const DEFAULT_LOADOUT = {
  paint: '',
  trail: '',
  body: '',
  badge: '',
  sound: '',
  theme: '',
  title: '',
};

beforeEach(() => {
  setEquipped({ ...DEFAULT_LOADOUT });
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('CosmeticRegistry', () => {
  describe('getEquippedPaint', () => {
    it('returns null when no paint equipped', () => {
      expect(getEquippedPaint()).toBeNull();
    });

    it('returns paint config for a known paint ID', () => {
      const paint = getEquippedPaint('rookie_paint');
      expect(paint).toEqual({ fill: '#4a4a4a', stroke: '#333333', pattern: null });
    });

    it('returns equipped paint when called without argument', () => {
      setEquipped({ paint: 'gemini_paint' });
      const paint = getEquippedPaint();
      expect(paint).toEqual({ fill: '#00bcd4', stroke: '#008a9e', pattern: null });
    });

    it('returns null for unknown paint ID', () => {
      expect(getEquippedPaint('nonexistent_paint')).toBeNull();
    });
  });

  describe('getEquippedTrail', () => {
    it('returns null when no trail equipped', () => {
      expect(getEquippedTrail()).toBeNull();
    });

    it('returns trail config for a known trail ID', () => {
      const trail = getEquippedTrail('spark_trail');
      expect(trail).toHaveProperty('color');
      expect(trail).toHaveProperty('size');
    });

    it('returns a clone, not the original object', () => {
      const trail1 = getEquippedTrail('spark_trail');
      const trail2 = getEquippedTrail('spark_trail');
      expect(trail1).toEqual(trail2);
      expect(trail1).not.toBe(trail2);
    });

    it('returns preset-based trail', () => {
      const trail = getEquippedTrail('blue_flame_trail');
      expect(trail).toEqual({ preset: 'blueFlame' });
    });

    it('returns equipped trail when called without argument', () => {
      setEquipped({ trail: 'flame_trail' });
      const trail = getEquippedTrail();
      expect(trail).toHaveProperty('color');
    });
  });

  describe('getEquippedBody', () => {
    it('returns null when no body equipped', () => {
      expect(getEquippedBody(0)).toBeNull();
    });

    it('returns vertices for a known body with L=0', () => {
      const body = getEquippedBody('aero_body', 0);
      expect(Array.isArray(body)).toBe(true);
      expect(body.length).toBeGreaterThan(0);
      expect(body[0]).toHaveProperty('x');
      expect(body[0]).toHaveProperty('y');
    });

    it('supports single-argument call (L only) when body is equipped', () => {
      setEquipped({ body: 'triple_body' });
      const body = getEquippedBody(5);
      expect(Array.isArray(body)).toBe(true);
    });

    it('applies LIMO_STRETCH to body vertices', () => {
      const bodyL0 = getEquippedBody('aero_body', 0);
      const bodyL5 = getEquippedBody('aero_body', 5);
      expect(bodyL0[0].x).not.toBe(bodyL5[0].x);
    });

    it('returns null for unknown body', () => {
      expect(getEquippedBody('nonexistent_body', 0)).toBeNull();
    });
  });

  describe('getEquippedBadge', () => {
    it('returns null when no badge equipped', () => {
      expect(getEquippedBadge()).toBeNull();
    });

    it('returns badge info for known ID', () => {
      const badge = getEquippedBadge('gold_badge');
      expect(badge).toEqual({ emoji: '\u{1F947}', label: 'Gold' });
    });

    it('returns equipped badge when called without argument', () => {
      setEquipped({ badge: 'bronze_badge' });
      const badge = getEquippedBadge();
      expect(badge).toEqual({ emoji: '\u{1F949}', label: 'Bronze' });
    });
  });

  describe('getEquippedTitle', () => {
    it('returns empty string when no title equipped', () => {
      expect(getEquippedTitle()).toBe('');
    });

    it('returns equipped title', () => {
      setEquipped({ title: 'Champion' });
      expect(getEquippedTitle()).toBe('Champion');
    });
  });

  describe('getEquippedLoadout', () => {
    it('returns all equipped slots', () => {
      const loadout = getEquippedLoadout();
      expect(loadout).toHaveProperty('paint');
      expect(loadout).toHaveProperty('trail');
      expect(loadout).toHaveProperty('body');
      expect(loadout).toHaveProperty('badge');
      expect(loadout).toHaveProperty('sound');
      expect(loadout).toHaveProperty('theme');
      expect(loadout).toHaveProperty('title');
    });

    it('returns a copy, not the internal object', () => {
      const a = getEquippedLoadout();
      const b = getEquippedLoadout();
      expect(a).toEqual(b);
      expect(a).not.toBe(b);
    });
  });

  describe('setEquipped', () => {
    it('updates a single slot', () => {
      setEquipped({ paint: 'rookie_paint' });
      expect(getEquippedLoadout().paint).toBe('rookie_paint');
    });

    it('updates multiple slots at once', () => {
      setEquipped({ paint: 'gemini_paint', badge: 'gold_badge', title: 'Legend' });
      const loadout = getEquippedLoadout();
      expect(loadout.paint).toBe('gemini_paint');
      expect(loadout.badge).toBe('gold_badge');
      expect(loadout.title).toBe('Legend');
    });

    it('does not affect other slots', () => {
      setEquipped({ trail: 'spark_trail' });
      setEquipped({ badge: 'pit_badge' });
      expect(getEquippedLoadout().trail).toBe('spark_trail');
    });

    it('notifies listeners on change', () => {
      const listener = vi.fn();
      const unsub = onEquippedChange(listener);

      setEquipped({ paint: 'codex_paint' });
      expect(listener).toHaveBeenCalledTimes(1);

      unsub();
    });
  });

  describe('onEquippedChange', () => {
    it('calls listener with current equipped state', () => {
      const listener = vi.fn();
      const unsub = onEquippedChange(listener);

      setEquipped({ title: 'Veteran' });
      expect(listener).toHaveBeenCalledWith(
        expect.objectContaining({ title: 'Veteran' }),
      );

      unsub();
    });

    it('returns unsubscribe function that stops notifications', () => {
      const listener = vi.fn();
      const unsub = onEquippedChange(listener);

      setEquipped({ paint: 'codex_paint' });
      expect(listener).toHaveBeenCalledTimes(1);

      unsub();
      setEquipped({ paint: 'gemini_paint' });
      expect(listener).toHaveBeenCalledTimes(1);
    });

    it('supports multiple listeners', () => {
      const a = vi.fn();
      const b = vi.fn();
      const unsubA = onEquippedChange(a);
      const unsubB = onEquippedChange(b);

      setEquipped({ sound: 'rev_sound' });
      expect(a).toHaveBeenCalledTimes(1);
      expect(b).toHaveBeenCalledTimes(1);

      unsubA();
      unsubB();
    });
  });

  describe('hydrate', () => {
    it('fetches from /api/stats and populates equipped state', async () => {
      authFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({
          equipped: { paint: 'holographic_paint', badge: 'collector_badge' },
        }),
      });

      await hydrate();

      expect(authFetch).toHaveBeenCalledWith('/api/stats');
      const loadout = getEquippedLoadout();
      expect(loadout.paint).toBe('holographic_paint');
      expect(loadout.badge).toBe('collector_badge');
    });

    it('does not overwrite state on failed fetch', async () => {
      setEquipped({ paint: 'rookie_paint' });
      authFetch.mockResolvedValueOnce({ ok: false, status: 500 });

      await hydrate();

      expect(getEquippedLoadout().paint).toBe('rookie_paint');
    });

    it('does not overwrite state on network error', async () => {
      setEquipped({ trail: 'spark_trail' });
      authFetch.mockRejectedValueOnce(new Error('network error'));

      await hydrate();

      expect(getEquippedLoadout().trail).toBe('spark_trail');
    });

    it('is idempotent after first successful call', async () => {
      authFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({
          equipped: { paint: 'codex_paint' },
        }),
      });

      await hydrate();
      expect(isHydrated()).toBe(true);
    });
  });
});
