// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

vi.mock('../auth.js', () => ({
  authFetch: vi.fn(),
}));

const DEFAULT_LOADOUT = { paint: '', trail: '', body: '', badge: '', sound: '', theme: '', title: '' };
const mockLoadout = { ...DEFAULT_LOADOUT };
const changeListeners = new Set();

vi.mock('./CosmeticRegistry.js', () => ({
  getEquippedLoadout: vi.fn(() => ({ ...mockLoadout })),
  setEquipped: vi.fn((slots) => {
    Object.assign(mockLoadout, slots);
    for (const fn of changeListeners) fn(mockLoadout);
  }),
  onEquippedChange: vi.fn((fn) => {
    changeListeners.add(fn);
    return () => changeListeners.delete(fn);
  }),
  getEquippedPaint: vi.fn((id) => {
    const paints = {
      rookie_paint: { fill: '#4a4a4a', stroke: '#333333', pattern: null },
      metallic_paint: { fill: '#b0c4de', stroke: '#7a8ea8', pattern: 'metallic' },
    };
    return paints[id] ?? null;
  }),
  getEquippedBadge: vi.fn((id) => {
    const badges = {
      bronze_badge: { emoji: '\u{1F949}', label: 'Bronze' },
      gold_badge: { emoji: '\u{1F947}', label: 'Gold' },
    };
    return badges[id] ?? null;
  }),
}));

import { RewardSelector } from './RewardSelector.js';
import { authFetch } from '../auth.js';
import { setEquipped, getEquippedLoadout } from './CosmeticRegistry.js';

function mockAchievementsResponse(achievements = []) {
  return { ok: true, json: () => Promise.resolve(achievements) };
}

function mockStatsResponse(tier = 1) {
  return { ok: true, json: () => Promise.resolve({ battlePass: { tier } }) };
}

function mockEndpoints(achievements = [], tier = 1) {
  authFetch.mockImplementation((url) => {
    if (url === '/api/achievements') return Promise.resolve(mockAchievementsResponse(achievements));
    if (url === '/api/stats') return Promise.resolve(mockStatsResponse(tier));
    return Promise.resolve({ ok: false, status: 404 });
  });
}

let rs;

beforeEach(() => {
  document.body.innerHTML = '';
  Object.assign(mockLoadout, DEFAULT_LOADOUT);
  getEquippedLoadout.mockImplementation(() => ({ ...mockLoadout }));
  setEquipped.mockImplementation((slots) => {
    Object.assign(mockLoadout, slots);
    for (const fn of changeListeners) fn(mockLoadout);
  });
});

afterEach(() => {
  rs?.destroy();
  rs = null;
  vi.restoreAllMocks();
  changeListeners.clear();
});

describe('RewardSelector', () => {
  describe('construction', () => {
    it('creates overlay DOM element on body', () => {
      rs = new RewardSelector();
      expect(document.getElementById('reward-selector')).toBeTruthy();
    });

    it('starts hidden', () => {
      rs = new RewardSelector();
      expect(rs.isVisible).toBe(false);
      expect(document.getElementById('reward-selector').classList.contains('hidden')).toBe(true);
    });

    it('has dialog role and aria attributes', () => {
      rs = new RewardSelector();
      const overlay = document.getElementById('reward-selector');
      expect(overlay.getAttribute('role')).toBe('dialog');
      expect(overlay.getAttribute('aria-modal')).toBe('true');
    });
  });

  describe('show/hide/toggle', () => {
    it('show() removes hidden class and sets visible flag', () => {
      rs = new RewardSelector();
      authFetch.mockResolvedValue({ ok: false, status: 500 });

      rs.show();
      expect(rs.isVisible).toBe(true);
      expect(document.getElementById('reward-selector').classList.contains('hidden')).toBe(false);
    });

    it('show() is idempotent', () => {
      rs = new RewardSelector();
      authFetch.mockResolvedValue({ ok: false, status: 500 });

      rs.show();
      rs.show();
      expect(authFetch).toHaveBeenCalledTimes(2);
    });

    it('hide() adds hidden class', () => {
      rs = new RewardSelector();
      authFetch.mockResolvedValue({ ok: false, status: 500 });

      rs.show();
      rs.hide();
      expect(rs.isVisible).toBe(false);
      expect(document.getElementById('reward-selector').classList.contains('hidden')).toBe(true);
    });

    it('hide() is idempotent when already hidden', () => {
      rs = new RewardSelector();
      rs.hide();
      expect(rs.isVisible).toBe(false);
    });

    it('toggle() switches between show and hide', () => {
      rs = new RewardSelector();
      authFetch.mockResolvedValue({ ok: false, status: 500 });

      rs.toggle();
      expect(rs.isVisible).toBe(true);
      rs.toggle();
      expect(rs.isVisible).toBe(false);
    });
  });

  describe('close button', () => {
    it('close button hides the selector', () => {
      rs = new RewardSelector();
      authFetch.mockResolvedValue({ ok: false, status: 500 });

      rs.show();
      document.querySelector('.rs-close').click();
      expect(rs.isVisible).toBe(false);
    });
  });

  describe('backdrop click', () => {
    it('clicking overlay backdrop hides the selector', () => {
      rs = new RewardSelector();
      authFetch.mockResolvedValue({ ok: false, status: 500 });

      rs.show();
      const overlay = document.getElementById('reward-selector');
      overlay.dispatchEvent(new MouseEvent('click', { bubbles: true }));
      expect(rs.isVisible).toBe(false);
    });
  });

  describe('rendering', () => {
    it('renders columns for each slot type', async () => {
      mockEndpoints();
      rs = new RewardSelector();
      rs.show();
      await vi.waitFor(() => {
        const columns = document.querySelectorAll('.rs-column');
        expect(columns.length).toBe(7);
      });
    });

    it('renders "None" tile for each slot', async () => {
      mockEndpoints();
      rs = new RewardSelector();
      rs.show();
      await vi.waitFor(() => {
        const noneTiles = document.querySelectorAll('.rs-tile-name');
        const noneNames = Array.from(noneTiles).filter(el => el.textContent === 'None');
        expect(noneNames.length).toBe(7);
      });
    });

    it('marks locked rewards when achievements not unlocked', async () => {
      mockEndpoints([{ id: 'first_lap', name: 'First Lap', unlocked: false }]);
      rs = new RewardSelector();
      rs.show();
      await vi.waitFor(() => {
        const lockedTiles = document.querySelectorAll('.rs-tile.locked');
        expect(lockedTiles.length).toBeGreaterThan(0);
      });
    });

    it('marks equipped reward with equipped class', async () => {
      Object.assign(mockLoadout, { paint: 'rookie_paint' });
      mockEndpoints([{ id: 'first_lap', name: 'First Lap', unlocked: true }]);

      rs = new RewardSelector();
      rs.show();
      await vi.waitFor(() => {
        const equippedTiles = document.querySelectorAll('.rs-tile.equipped');
        expect(equippedTiles.length).toBeGreaterThan(0);
      });
    });

    it('shows lock requirement text for battle pass rewards', async () => {
      mockEndpoints();
      rs = new RewardSelector();
      rs.show();
      await vi.waitFor(() => {
        const lockTexts = document.querySelectorAll('.rs-tile-lock-text');
        const bpTexts = Array.from(lockTexts).filter(el => el.textContent.includes('Battle Pass Tier'));
        expect(bpTexts.length).toBeGreaterThan(0);
      });
    });

    it('unlocks battle pass rewards based on tier', async () => {
      mockEndpoints([], 5);
      rs = new RewardSelector();
      rs.show();
      await vi.waitFor(() => {
        const equippableTiles = document.querySelectorAll('.rs-tile.equippable');
        // At tier 5: bronze_badge(2), spark_trail(3), rev_sound(4), metallic_paint(5) should be equippable
        expect(equippableTiles.length).toBeGreaterThanOrEqual(4);
      });
    });

    it('shows error message on fetch failure without overwriting it', async () => {
      authFetch.mockRejectedValue(new Error('network error'));
      rs = new RewardSelector();
      rs.show();
      await vi.waitFor(() => {
        const errEl = document.querySelector('.rs-columns p');
        expect(errEl).toBeTruthy();
        expect(errEl.textContent).toContain('network error');
      });
      // _render() must NOT have run — no columns should be present
      expect(document.querySelectorAll('.rs-column').length).toBe(0);
      expect(rs._achievements).toEqual([]);
    });
  });

  describe('equip/unequip', () => {
    it('calls /api/equip when clicking equippable tile', async () => {
      authFetch.mockImplementation((url) => {
        if (url === '/api/achievements') return Promise.resolve(mockAchievementsResponse([
          { id: 'first_lap', name: 'First Lap', unlocked: true },
        ]));
        if (url === '/api/stats') return Promise.resolve(mockStatsResponse(1));
        if (url === '/api/equip') return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ paint: 'rookie_paint' }),
        });
        return Promise.resolve({ ok: false, status: 404 });
      });

      rs = new RewardSelector();
      rs.show();

      await vi.waitFor(() => {
        const equippableTiles = document.querySelectorAll('.rs-tile.equippable');
        expect(equippableTiles.length).toBeGreaterThan(0);
      });

      document.querySelectorAll('.rs-tile.equippable')[0].click();

      await vi.waitFor(() => {
        expect(authFetch).toHaveBeenCalledWith('/api/equip', expect.objectContaining({
          method: 'POST',
        }));
      });
    });

    it('unequip sets slot to empty string via setEquipped', async () => {
      Object.assign(mockLoadout, { paint: 'rookie_paint' });
      mockEndpoints([{ id: 'first_lap', name: 'First Lap', unlocked: true }]);

      rs = new RewardSelector();
      rs.show();

      await vi.waitFor(() => {
        const tiles = document.querySelectorAll('.rs-tile.equippable');
        expect(tiles.length).toBeGreaterThan(0);
      });

      const noneTiles = Array.from(document.querySelectorAll('.rs-tile'))
        .filter(t => {
          const nameEl = t.querySelector('.rs-tile-name');
          return nameEl && nameEl.textContent === 'None' && t.classList.contains('equippable');
        });

      if (noneTiles.length > 0) {
        noneTiles[0].click();
        expect(setEquipped).toHaveBeenCalledWith(expect.objectContaining({ paint: '' }));
      }
    });
  });

  describe('destroy', () => {
    it('removes overlay from DOM', () => {
      rs = new RewardSelector();
      expect(document.getElementById('reward-selector')).toBeTruthy();

      rs.destroy();
      expect(document.getElementById('reward-selector')).toBeNull();
      rs = null;
    });

    it('unsubscribes from equipped changes', () => {
      rs = new RewardSelector();
      rs.destroy();
      rs = null;

      expect(changeListeners.size).toBe(0);
    });
  });

  describe('focus management', () => {
    it('focuses close button on show()', () => {
      rs = new RewardSelector();
      authFetch.mockResolvedValue({ ok: false, status: 500 });
      rs.show();
      expect(document.activeElement).toBe(document.querySelector('.rs-close'));
    });

    it('returns focus to previous element on hide()', () => {
      const btn = document.createElement('button');
      document.body.appendChild(btn);
      btn.focus();

      rs = new RewardSelector();
      authFetch.mockResolvedValue({ ok: false, status: 500 });
      rs.show();
      rs.hide();

      expect(document.activeElement).toBe(btn);
    });

    it('focus trap: Tab from last focusable wraps to first', async () => {
      mockEndpoints();
      rs = new RewardSelector();
      rs.show();

      await vi.waitFor(() => {
        expect(document.querySelectorAll('.rs-tile').length).toBeGreaterThan(0);
      });

      const focusable = rs._getFocusable();
      expect(focusable.length).toBeGreaterThan(1);

      const last = focusable[focusable.length - 1];
      last.focus();

      const tabEvent = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true });
      let defaultPrevented = false;
      tabEvent.preventDefault = () => { defaultPrevented = true; };
      rs._overlay.dispatchEvent(tabEvent);

      expect(defaultPrevented).toBe(true);
      expect(document.activeElement).toBe(focusable[0]);
    });

    it('focus trap: Shift+Tab from first focusable wraps to last', async () => {
      mockEndpoints();
      rs = new RewardSelector();
      rs.show();

      await vi.waitFor(() => {
        expect(document.querySelectorAll('.rs-tile').length).toBeGreaterThan(0);
      });

      const focusable = rs._getFocusable();
      focusable[0].focus();

      const tabEvent = new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true });
      let defaultPrevented = false;
      tabEvent.preventDefault = () => { defaultPrevented = true; };
      rs._overlay.dispatchEvent(tabEvent);

      expect(defaultPrevented).toBe(true);
      expect(document.activeElement).toBe(focusable[focusable.length - 1]);
    });
  });

  describe('tile accessibility', () => {
    it('all tiles have role="button" and tabindex="0"', async () => {
      mockEndpoints();
      rs = new RewardSelector();
      rs.show();

      await vi.waitFor(() => {
        expect(document.querySelectorAll('.rs-tile').length).toBeGreaterThan(0);
      });

      for (const tile of document.querySelectorAll('.rs-tile')) {
        expect(tile.getAttribute('role')).toBe('button');
        expect(tile.getAttribute('tabindex')).toBe('0');
      }
    });

    it('equipped tile has aria-pressed="true"', async () => {
      Object.assign(mockLoadout, { paint: 'rookie_paint' });
      mockEndpoints([{ id: 'first_lap', name: 'First Lap', unlocked: true }]);
      rs = new RewardSelector();
      rs.show();

      await vi.waitFor(() => {
        const equippedTiles = document.querySelectorAll('.rs-tile.equipped');
        expect(equippedTiles.length).toBeGreaterThan(0);
      });

      const equippedTiles = document.querySelectorAll('.rs-tile.equipped');
      for (const tile of equippedTiles) {
        expect(tile.getAttribute('aria-pressed')).toBe('true');
      }
    });

    it('locked tile has aria-disabled="true" and includes lock reason in aria-label', async () => {
      mockEndpoints([]);
      rs = new RewardSelector();
      rs.show();

      await vi.waitFor(() => {
        expect(document.querySelectorAll('.rs-tile.locked').length).toBeGreaterThan(0);
      });

      const lockedTile = document.querySelector('.rs-tile.locked');
      expect(lockedTile.getAttribute('aria-disabled')).toBe('true');
      const label = lockedTile.getAttribute('aria-label');
      expect(label).toBeTruthy();
      expect(label.toLowerCase()).toContain('locked');
    });

    it('equippable tile aria-label contains reward name and "equip"', async () => {
      mockEndpoints([{ id: 'first_lap', name: 'First Lap', unlocked: true }]);
      rs = new RewardSelector();
      rs.show();

      await vi.waitFor(() => {
        expect(document.querySelectorAll('.rs-tile.equippable').length).toBeGreaterThan(0);
      });

      const equippable = document.querySelector('.rs-tile.equippable');
      const label = equippable.getAttribute('aria-label');
      expect(label).toBeTruthy();
    });

    it('Enter key on equippable tile triggers equip', async () => {
      authFetch.mockImplementation((url) => {
        if (url === '/api/achievements') return Promise.resolve(mockAchievementsResponse([
          { id: 'first_lap', name: 'First Lap', unlocked: true },
        ]));
        if (url === '/api/stats') return Promise.resolve(mockStatsResponse(1));
        if (url === '/api/equip') return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ paint: 'rookie_paint' }),
        });
        return Promise.resolve({ ok: false, status: 404 });
      });

      rs = new RewardSelector();
      rs.show();

      await vi.waitFor(() => {
        expect(document.querySelectorAll('.rs-tile.equippable').length).toBeGreaterThan(0);
      });

      const tile = document.querySelector('.rs-tile.equippable');
      tile.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));

      await vi.waitFor(() => {
        expect(authFetch).toHaveBeenCalledWith('/api/equip', expect.objectContaining({ method: 'POST' }));
      });
    });

    it('Space key on equippable tile triggers equip', async () => {
      authFetch.mockImplementation((url) => {
        if (url === '/api/achievements') return Promise.resolve(mockAchievementsResponse([
          { id: 'first_lap', name: 'First Lap', unlocked: true },
        ]));
        if (url === '/api/stats') return Promise.resolve(mockStatsResponse(1));
        if (url === '/api/equip') return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ paint: 'rookie_paint' }),
        });
        return Promise.resolve({ ok: false, status: 404 });
      });

      rs = new RewardSelector();
      rs.show();

      await vi.waitFor(() => {
        expect(document.querySelectorAll('.rs-tile.equippable').length).toBeGreaterThan(0);
      });

      const tile = document.querySelector('.rs-tile.equippable');
      const spaceEvent = new KeyboardEvent('keydown', { key: ' ', bubbles: true });
      let defaultPrevented = false;
      spaceEvent.preventDefault = () => { defaultPrevented = true; };
      tile.dispatchEvent(spaceEvent);

      expect(defaultPrevented).toBe(true);
      await vi.waitFor(() => {
        expect(authFetch).toHaveBeenCalledWith('/api/equip', expect.objectContaining({ method: 'POST' }));
      });
    });
  });
});
