// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { AchievementPanel } from './AchievementPanel.js';
import { authFetch } from '../auth.js';

const MOCK_ACHIEVEMENTS = [
  { id: 'a1', name: 'First Race', category: 'Session Milestones', tier: 'bronze', unlocked: true, description: 'Start your first session' },
];

vi.mock('../auth.js', () => ({
  authFetch: vi.fn(() => Promise.resolve({
    ok: true,
    json: () => Promise.resolve(MOCK_ACHIEVEMENTS),
  })),
}));

function mousemoveCalls(spy) {
  return spy.mock.calls.filter(([type]) => type === 'mousemove');
}

function mockAchievements(achievements) {
  authFetch.mockResolvedValueOnce({
    ok: true,
    json: () => Promise.resolve(achievements),
  });
}

let panel;

beforeEach(() => {
  document.body.innerHTML = '';
});

afterEach(() => {
  panel?.destroy();
  panel = null;
  vi.restoreAllMocks();
});

describe('AchievementPanel', () => {
  describe('DOM construction', () => {
    it('creates overlay element on body', () => {
      panel = new AchievementPanel();
      expect(document.getElementById('achievement-panel')).toBeTruthy();
    });

    it('starts hidden', () => {
      panel = new AchievementPanel();
      expect(panel.isVisible).toBe(false);
      expect(document.getElementById('achievement-panel').classList.contains('hidden')).toBe(true);
    });

    it('has dialog role and aria attributes', () => {
      panel = new AchievementPanel();
      const overlay = document.getElementById('achievement-panel');
      expect(overlay.getAttribute('role')).toBe('dialog');
      expect(overlay.getAttribute('aria-label')).toBe('Achievements');
      expect(overlay.getAttribute('aria-modal')).toBe('true');
    });

    it('contains header, body, footer, and tooltip', () => {
      panel = new AchievementPanel();
      expect(document.querySelector('.ap-header')).toBeTruthy();
      expect(document.querySelector('.ap-body')).toBeTruthy();
      expect(document.querySelector('.ap-footer')).toBeTruthy();
      expect(document.querySelector('.ap-tooltip')).toBeTruthy();
    });
  });

  describe('show/hide/toggle', () => {
    it('show() removes hidden class and sets visible', () => {
      panel = new AchievementPanel();
      panel.show();
      expect(panel.isVisible).toBe(true);
      expect(document.getElementById('achievement-panel').classList.contains('hidden')).toBe(false);
    });

    it('hide() adds hidden class and hides tooltip', () => {
      panel = new AchievementPanel();
      panel.show();
      panel.hide();
      expect(panel.isVisible).toBe(false);
      expect(document.querySelector('.ap-tooltip').classList.contains('hidden')).toBe(true);
    });

    it('toggle() switches visibility', () => {
      panel = new AchievementPanel();
      panel.toggle();
      expect(panel.isVisible).toBe(true);
      panel.toggle();
      expect(panel.isVisible).toBe(false);
    });
  });

  describe('close button', () => {
    it('clicking close button hides panel', () => {
      panel = new AchievementPanel();
      panel.show();
      document.querySelector('.ap-close').click();
      expect(panel.isVisible).toBe(false);
    });
  });

  describe('backdrop click', () => {
    it('clicking overlay backdrop hides panel', () => {
      panel = new AchievementPanel();
      panel.show();
      const overlay = document.getElementById('achievement-panel');
      overlay.dispatchEvent(new MouseEvent('click', { bubbles: true }));
      expect(panel.isVisible).toBe(false);
    });
  });

  describe('hydrate', () => {
    it('fetches achievements from /api/achievements', async () => {
      mockAchievements([]);
      panel = new AchievementPanel();
      await panel.hydrate();
      expect(authFetch).toHaveBeenCalledWith('/api/achievements');
    });

    it('renders empty message when no achievements', async () => {
      mockAchievements([]);
      panel = new AchievementPanel();
      await panel.hydrate();
      expect(document.querySelector('.ap-empty-message').textContent).toBe('No achievements found.');
      expect(document.querySelector('.ap-counter').textContent).toBe('0 / 0 unlocked');
    });

    it('renders unlocked count correctly', async () => {
      mockAchievements([
        { id: 'a1', name: 'First', category: 'Session Milestones', tier: 'bronze', unlocked: true },
        { id: 'a2', name: 'Second', category: 'Session Milestones', tier: 'silver', unlocked: false },
        { id: 'a3', name: 'Third', category: 'Streaks', tier: 'gold', unlocked: true },
      ]);
      panel = new AchievementPanel();
      await panel.hydrate();
      expect(document.querySelector('.ap-counter').textContent).toBe('2 / 3 unlocked');
    });

    it('renders achievement tiles with locked/unlocked classes', async () => {
      mockAchievements([
        { id: 'a1', name: 'Unlocked One', category: 'Streaks', tier: 'bronze', unlocked: true },
        { id: 'a2', name: 'Locked One', category: 'Streaks', tier: 'silver', unlocked: false },
      ]);
      panel = new AchievementPanel();
      await panel.hydrate();

      expect(document.querySelector('.ap-tile.unlocked')).toBeTruthy();
      expect(document.querySelector('.ap-tile.locked')).toBeTruthy();
    });

    it('groups achievements by category', async () => {
      mockAchievements([
        { id: 'a1', name: 'Milestone', category: 'Session Milestones', tier: 'bronze', unlocked: true },
        { id: 'a2', name: 'Streak', category: 'Streaks', tier: 'gold', unlocked: false },
      ]);
      panel = new AchievementPanel();
      await panel.hydrate();

      const headers = document.querySelectorAll('.ap-category-header');
      expect(headers.length).toBe(2);
    });

    it('shows padlock icon on locked achievements', async () => {
      mockAchievements([
        { id: 'a1', name: 'Locked', category: 'Streaks', tier: 'bronze', unlocked: false },
      ]);
      panel = new AchievementPanel();
      await panel.hydrate();

      expect(document.querySelector('.ap-tile-padlock')).toBeTruthy();
    });

    it('does not show padlock on unlocked achievements', async () => {
      mockAchievements([
        { id: 'a1', name: 'Open', category: 'Streaks', tier: 'bronze', unlocked: true },
      ]);
      panel = new AchievementPanel();
      await panel.hydrate();

      expect(document.querySelector('.ap-tile-padlock')).toBeNull();
    });

    it('displays tier badge with correct class', async () => {
      mockAchievements([
        { id: 'a1', name: 'Gold Thing', category: 'Spectacle', tier: 'gold', unlocked: true },
      ]);
      panel = new AchievementPanel();
      await panel.hydrate();

      expect(document.querySelector('.ap-tile-tier-gold')).toBeTruthy();
    });

    it('falls back to bronze class for unknown tier', async () => {
      mockAchievements([
        { id: 'a1', name: 'Mystery', category: 'Spectacle', tier: 'diamond', unlocked: true },
      ]);
      panel = new AchievementPanel();
      await panel.hydrate();

      expect(document.querySelector('.ap-tile-tier-bronze')).toBeTruthy();
    });

    it('shows error message on fetch failure', async () => {
      authFetch.mockRejectedValueOnce(new Error('Network error'));
      panel = new AchievementPanel();
      await panel.hydrate();

      expect(document.querySelector('.ap-error-message').textContent).toContain('Failed to load achievements');
      expect(document.querySelector('.ap-error-message').textContent).toContain('Network error');
    });

    it('shows error on non-OK response', async () => {
      authFetch.mockResolvedValueOnce({ ok: false, status: 403 });
      panel = new AchievementPanel();
      await panel.hydrate();

      expect(document.querySelector('.ap-error-message').textContent).toContain('HTTP 403');
    });
  });

  describe('destroy', () => {
    it('removes overlay from DOM', () => {
      panel = new AchievementPanel();
      expect(document.getElementById('achievement-panel')).toBeTruthy();
      panel.destroy();
      expect(document.getElementById('achievement-panel')).toBeNull();
      panel = null;
    });

    it('removes mousemove listener', () => {
      panel = new AchievementPanel();
      panel.show();
      const removeSpy = vi.spyOn(document, 'removeEventListener');
      panel.destroy();
      expect(mousemoveCalls(removeSpy)).toHaveLength(1);
      panel = null;
    });
  });

  describe('focus management', () => {
    it('focuses close button on show()', () => {
      panel = new AchievementPanel();
      panel.show();
      expect(document.activeElement).toBe(document.querySelector('.ap-close'));
    });

    it('returns focus to previous element on hide()', () => {
      const btn = document.createElement('button');
      document.body.appendChild(btn);
      btn.focus();

      panel = new AchievementPanel();
      panel.show();
      panel.hide();

      expect(document.activeElement).toBe(btn);
    });

    it('focus trap: Tab from last focusable wraps to first', async () => {
      mockAchievements([
        { id: 'a1', name: 'First', category: 'Session Milestones', tier: 'bronze', unlocked: true },
      ]);
      panel = new AchievementPanel();
      panel.show();
      await panel.hydrate();

      const focusable = panel._getFocusable();
      expect(focusable.length).toBeGreaterThan(0);

      // Focus last element
      const last = focusable[focusable.length - 1];
      last.focus();

      const tabEvent = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true });
      let defaultPrevented = false;
      tabEvent.preventDefault = () => { defaultPrevented = true; };
      panel._overlay.dispatchEvent(tabEvent);

      expect(defaultPrevented).toBe(true);
      expect(document.activeElement).toBe(focusable[0]);
    });

    it('focus trap: Shift+Tab from first focusable wraps to last', async () => {
      mockAchievements([
        { id: 'a1', name: 'First', category: 'Session Milestones', tier: 'bronze', unlocked: true },
      ]);
      panel = new AchievementPanel();
      panel.show();
      await panel.hydrate();

      const focusable = panel._getFocusable();
      expect(focusable.length).toBeGreaterThan(0);

      // Focus first element
      focusable[0].focus();

      const tabEvent = new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true });
      let defaultPrevented = false;
      tabEvent.preventDefault = () => { defaultPrevented = true; };
      panel._overlay.dispatchEvent(tabEvent);

      expect(defaultPrevented).toBe(true);
      expect(document.activeElement).toBe(focusable[focusable.length - 1]);
    });

    it('non-Tab keydown does not prevent default', () => {
      panel = new AchievementPanel();
      panel.show();

      const escEvent = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true });
      let defaultPrevented = false;
      escEvent.preventDefault = () => { defaultPrevented = true; };
      panel._overlay.dispatchEvent(escEvent);

      expect(defaultPrevented).toBe(false);
    });
  });

  describe('tile accessibility', () => {
    it('tiles have role="button" and tabindex="0"', async () => {
      mockAchievements([
        { id: 'a1', name: 'Unlocked', category: 'Streaks', tier: 'bronze', unlocked: true },
        { id: 'a2', name: 'Locked',   category: 'Streaks', tier: 'silver', unlocked: false },
      ]);
      panel = new AchievementPanel();
      await panel.hydrate();

      const tiles = document.querySelectorAll('.ap-tile');
      for (const tile of tiles) {
        expect(tile.getAttribute('role')).toBe('button');
        expect(tile.getAttribute('tabindex')).toBe('0');
      }
    });

    it('unlocked tile aria-label includes name, tier, and "Unlocked"', async () => {
      mockAchievements([
        { id: 'a1', name: 'Speed King', category: 'Streaks', tier: 'gold', unlocked: true },
      ]);
      panel = new AchievementPanel();
      await panel.hydrate();

      const tile = document.querySelector('.ap-tile.unlocked');
      const label = tile.getAttribute('aria-label');
      expect(label).toContain('Speed King');
      expect(label).toContain('gold');
      expect(label).toContain('Unlocked');
    });

    it('locked tile aria-label includes "Locked"', async () => {
      mockAchievements([
        { id: 'a1', name: 'Mystery', category: 'Streaks', tier: 'platinum', unlocked: false },
      ]);
      panel = new AchievementPanel();
      await panel.hydrate();

      const tile = document.querySelector('.ap-tile.locked');
      const label = tile.getAttribute('aria-label');
      expect(label).toContain('Mystery');
      expect(label).toContain('Locked');
    });

    it('tiles appear in _getFocusable() list when rendered', async () => {
      mockAchievements([
        { id: 'a1', name: 'First', category: 'Session Milestones', tier: 'bronze', unlocked: true },
      ]);
      panel = new AchievementPanel();
      panel.show();
      await panel.hydrate();

      const focusable = panel._getFocusable();
      const tiles = focusable.filter(el => el.classList.contains('ap-tile'));
      expect(tiles.length).toBeGreaterThan(0);
    });
  });

  describe('mousemove listener lifecycle', () => {
    it('does not attach mousemove listener on construction', () => {
      const addSpy = vi.spyOn(document, 'addEventListener');
      panel = new AchievementPanel();
      expect(mousemoveCalls(addSpy)).toHaveLength(0);
    });

    it('attaches mousemove listener when panel is shown', () => {
      panel = new AchievementPanel();
      const addSpy = vi.spyOn(document, 'addEventListener');
      panel.show();
      expect(mousemoveCalls(addSpy)).toHaveLength(1);
    });

    it('removes mousemove listener when panel is hidden', () => {
      panel = new AchievementPanel();
      panel.show();
      const removeSpy = vi.spyOn(document, 'removeEventListener');
      panel.hide();
      expect(mousemoveCalls(removeSpy)).toHaveLength(1);
    });

    it('does not add duplicate listeners on repeated show() calls', () => {
      panel = new AchievementPanel();
      const addSpy = vi.spyOn(document, 'addEventListener');
      panel.show();
      panel.show();
      expect(mousemoveCalls(addSpy)).toHaveLength(1);
    });

    it('does not remove listener if hide() called while already hidden', () => {
      panel = new AchievementPanel();
      const removeSpy = vi.spyOn(document, 'removeEventListener');
      panel.hide();
      expect(mousemoveCalls(removeSpy)).toHaveLength(0);
    });

    it('listener is active only while panel is open (add on show, remove on hide)', () => {
      panel = new AchievementPanel();
      const events = [];
      const origAdd = document.addEventListener.bind(document);
      const origRemove = document.removeEventListener.bind(document);
      vi.spyOn(document, 'addEventListener').mockImplementation((type, fn, ...rest) => {
        if (type === 'mousemove') events.push('add');
        origAdd(type, fn, ...rest);
      });
      vi.spyOn(document, 'removeEventListener').mockImplementation((type, fn, ...rest) => {
        if (type === 'mousemove') events.push('remove');
        origRemove(type, fn, ...rest);
      });

      panel.show();
      panel.hide();
      panel.show();
      panel.hide();

      expect(events).toEqual(['add', 'remove', 'add', 'remove']);
    });
  });
});

describe('AchievementPanel caching', () => {
  beforeEach(() => {
    authFetch.mockClear();
  });

  it('fetches on first hydrate()', async () => {
    panel = new AchievementPanel();
    await panel.hydrate();
    expect(authFetch).toHaveBeenCalledTimes(1);
  });

  it('does not re-fetch on second hydrate() when not dirty', async () => {
    panel = new AchievementPanel();
    await panel.hydrate();
    await panel.hydrate();
    expect(authFetch).toHaveBeenCalledTimes(1);
  });

  it('re-fetches after markDirty()', async () => {
    panel = new AchievementPanel();
    await panel.hydrate();
    panel.markDirty();
    await panel.hydrate();
    expect(authFetch).toHaveBeenCalledTimes(2);
  });

  it('clears dirty flag after successful fetch', async () => {
    panel = new AchievementPanel();
    await panel.hydrate();
    panel.markDirty();
    await panel.hydrate();
    await panel.hydrate(); // should use cache
    expect(authFetch).toHaveBeenCalledTimes(2);
  });

  it('stays dirty on fetch error so next open retries', async () => {
    authFetch.mockResolvedValueOnce({ ok: false, status: 500 });
    panel = new AchievementPanel();
    await panel.hydrate(); // fails
    expect(authFetch).toHaveBeenCalledTimes(1);
    // still dirty, so next hydrate retries
    authFetch.mockResolvedValueOnce({ ok: true, json: () => Promise.resolve(MOCK_ACHIEVEMENTS) });
    await panel.hydrate();
    expect(authFetch).toHaveBeenCalledTimes(2);
  });
});
