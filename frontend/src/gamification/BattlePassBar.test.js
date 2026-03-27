// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

vi.mock('../auth.js', () => ({
  authFetch: vi.fn(),
}));

import { BattlePassBar } from './BattlePassBar.js';
import { authFetch } from '../auth.js';

let container;

function mockStatsResponse(battlePass = {}) {
  return {
    ok: true,
    json: () => Promise.resolve({ battlePass }),
  };
}

function mockChallengesResponse(challenges = []) {
  return {
    ok: true,
    json: () => Promise.resolve(challenges),
  };
}

/** Route /api/stats and /api/challenges to mock responses. */
function mockEndpoints(stats = {}, challenges = []) {
  authFetch.mockImplementation((url) => {
    if (url === '/api/stats') return Promise.resolve(mockStatsResponse(stats));
    if (url === '/api/challenges') return Promise.resolve(mockChallengesResponse(challenges));
    return Promise.reject(new Error('unexpected URL: ' + url));
  });
}

/** Reject all authFetch calls (used when no network data is needed). */
function rejectAllFetches() {
  authFetch.mockImplementation(() => Promise.reject(new Error('not mocked')));
}

/** Create a BattlePassBar and wait for loadInitialData to settle. */
async function createAndHydrate(stats, challenges) {
  mockEndpoints(stats, challenges);
  const bar = new BattlePassBar(container);
  await vi.runAllTimersAsync();
  await Promise.resolve();
  return bar;
}

beforeEach(() => {
  document.body.innerHTML = '';
  container = document.createElement('div');
  document.body.appendChild(container);
  vi.useFakeTimers();
  rejectAllFetches();
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe('BattlePassBar', () => {
  function createBarWithConfettiMock() {
    const bar = new BattlePassBar(container);
    vi.spyOn(bar, 'spawnConfetti').mockImplementation(() => {});
    return bar;
  }

  describe('DOM construction', () => {
    it('builds collapsed row and expanded panel', () => {
      const bar = new BattlePassBar(container);

      expect(container.querySelector('.bp-collapsed')).toBeTruthy();
      expect(container.querySelector('.bp-expanded')).toBeTruthy();
      expect(container.querySelector('.bp-season')).toBeTruthy();
      expect(container.querySelector('.bp-tier-badge')).toBeTruthy();
      expect(container.querySelector('.bp-xp-bar-wrap')).toBeTruthy();
    });

    it('expanded panel starts hidden', () => {
      const bar = new BattlePassBar(container);
      expect(bar.expandedPanel.classList.contains('hidden')).toBe(true);
    });
  });

  describe('loadInitialData', () => {
    it('populates state from /api/stats response', async () => {
      const bar = await createAndHydrate({ tier: 5, xp: 4500, season: 'Alpha' });

      expect(bar.state.tier).toBe(5);
      expect(bar.state.xp).toBe(4500);
      expect(bar.state.season).toBe('Alpha');
    });

    it('uses defaults when stats fetch fails', async () => {
      authFetch.mockImplementation(() => Promise.resolve({ ok: false, status: 500 }));

      const bar = new BattlePassBar(container);
      await vi.runAllTimersAsync();
      await Promise.resolve();

      expect(bar.state.tier).toBe(1);
      expect(bar.state.xp).toBe(0);
    });

    it('loads challenges from /api/challenges', async () => {
      const challenges = [
        { description: 'Run 5 sessions', current: 2, target: 5, complete: false },
      ];
      const bar = await createAndHydrate({ tier: 1, xp: 0 }, challenges);

      expect(bar.challenges).toHaveLength(1);
      expect(bar.challenges[0].description).toBe('Run 5 sessions');
    });
  });

  describe('render', () => {
    it('displays season label', async () => {
      const bar = await createAndHydrate({ tier: 3, xp: 2500, season: 'Beta' });
      expect(bar.seasonLabel.textContent).toBe('Season Beta');
    });

    it('shows "Battle Pass" when no season set', async () => {
      const bar = await createAndHydrate({ tier: 1, xp: 0 });
      expect(bar.seasonLabel.textContent).toBe('Battle Pass');
    });

    it('displays tier badge text', async () => {
      const bar = await createAndHydrate({ tier: 7, xp: 6500 });
      expect(bar.tierBadge.textContent).toBe('Tier 7');
    });

    it('shows MAX label at tier 10', async () => {
      const bar = await createAndHydrate({ tier: 10, xp: 10000 });

      expect(bar.xpBarLabel.textContent).toContain('MAX');
      expect(bar.tierBadge.classList.contains('tier-max')).toBe(true);
    });

    it('displays XP within tier for non-max tiers', async () => {
      const bar = await createAndHydrate({ tier: 3, xp: 2300 });
      // tier 3, xp 2300 => tierXP = 2300 - (3-1)*1000 = 300
      expect(bar.xpBarLabel.textContent).toBe('300 / 1000 XP');
    });

    it('adds near-tier-up class when progress > 90%', async () => {
      // tier 2, xp=1950 => tierXP = 950, progress = 0.95
      const bar = await createAndHydrate({ tier: 2, xp: 1950 });
      expect(bar.xpBarWrap.classList.contains('bp-near-tier-up')).toBe(true);
    });
  });

  describe('toggleExpanded', () => {
    it('toggles expanded panel visibility', () => {
      const bar = new BattlePassBar(container);

      expect(bar.expanded).toBe(false);
      bar.toggleExpanded();
      expect(bar.expanded).toBe(true);
      expect(bar.expandedPanel.classList.contains('hidden')).toBe(false);

      bar.toggleExpanded();
      expect(bar.expanded).toBe(false);
      expect(bar.expandedPanel.classList.contains('hidden')).toBe(true);
    });
  });

  describe('onProgress', () => {
    it('updates state from progress payload', () => {
      const bar = createBarWithConfettiMock();

      bar.onProgress({
        xp: 3000,
        tier: 4,
        tierProgress: 0.0,
        recentXP: [{ amount: 50, reason: 'session_complete' }],
        rewards: ['Metallic Paint'],
      });

      expect(bar.state.xp).toBe(3000);
      expect(bar.state.tier).toBe(4);
      expect(bar.state.rewards).toEqual(['Metallic Paint']);
    });

    it('prepends recent XP entries to xpLog', () => {
      const bar = createBarWithConfettiMock();

      bar.onProgress({
        xp: 100, tier: 1, tierProgress: 0.1,
        recentXP: [{ amount: 50, reason: 'a' }, { amount: 25, reason: 'b' }],
      });

      expect(bar.xpLog).toHaveLength(2);
      // unshift iterates in order, so last entry ends up at index 0
      expect(bar.xpLog[0].reason).toBe('b');
      expect(bar.xpLog[1].reason).toBe('a');
    });

    it('caps xpLog at 20 entries', () => {
      const bar = createBarWithConfettiMock();

      for (let i = 0; i < 19; i++) {
        bar.xpLog.push({ amount: 10, reason: `entry_${i}` });
      }

      bar.onProgress({
        xp: 200, tier: 1, tierProgress: 0.2,
        recentXP: [{ amount: 10, reason: 'new1' }, { amount: 10, reason: 'new2' }],
      });

      expect(bar.xpLog.length).toBe(20);
    });

    it('triggers tier-up celebration when tier increases', () => {
      const bar = createBarWithConfettiMock();
      bar.state.tier = 3;

      const playSpy = vi.spyOn(bar, 'playTierUpCelebration');
      bar.onProgress({
        xp: 4000, tier: 5, tierProgress: 0.0,
        recentXP: [],
      });

      expect(playSpy).toHaveBeenCalled();
    });

    it('does not trigger tier-up when tier stays the same', () => {
      const bar = createBarWithConfettiMock();
      bar.state.tier = 3;

      const playSpy = vi.spyOn(bar, 'playTierUpCelebration');
      bar.onProgress({
        xp: 2500, tier: 3, tierProgress: 0.5,
        recentXP: [],
      });

      expect(playSpy).not.toHaveBeenCalled();
    });
  });

  describe('showXPToast', () => {
    it('does nothing when entries array is empty', () => {
      const bar = new BattlePassBar(container);
      bar.showXPToast([]);
      expect(bar.toastContainer.children).toHaveLength(0);
      expect(bar.toastTimer).toBeNull();
    });

    it('creates a toast with correct text for a single entry', () => {
      const bar = new BattlePassBar(container);
      bar.showXPToast([{ amount: 50, reason: 'session_complete' }]);
      const toasts = bar.toastContainer.querySelectorAll('.bp-xp-toast');
      expect(toasts).toHaveLength(1);
      expect(toasts[0].textContent).toBe('+50 XP');
    });

    it('sums multiple entry amounts into one toast text', () => {
      const bar = new BattlePassBar(container);
      bar.showXPToast([
        { amount: 50, reason: 'session_complete' },
        { amount: 25, reason: 'tool_use' },
        { amount: 10, reason: 'streak' },
      ]);
      const toast = bar.toastContainer.querySelector('.bp-xp-toast');
      expect(toast.textContent).toBe('+85 XP');
    });

    it('adds bp-xp-flash class to xpBarWrap on each call', () => {
      const bar = new BattlePassBar(container);
      bar.showXPToast([{ amount: 100, reason: 'task_complete' }]);
      expect(bar.xpBarWrap.classList.contains('bp-xp-flash')).toBe(true);
    });

    it('restarts flash animation on successive calls', () => {
      const bar = new BattlePassBar(container);
      bar.showXPToast([{ amount: 50, reason: 'a' }]);
      expect(bar.xpBarWrap.classList.contains('bp-xp-flash')).toBe(true);
      bar.showXPToast([{ amount: 30, reason: 'b' }]);
      expect(bar.xpBarWrap.classList.contains('bp-xp-flash')).toBe(true);
    });

    it('sets toastTimer after a call with entries', () => {
      const bar = new BattlePassBar(container);
      expect(bar.toastTimer).toBeNull();
      bar.showXPToast([{ amount: 50, reason: 'session_complete' }]);
      expect(bar.toastTimer).not.toBeNull();
    });

    it('stacks multiple toast elements in toastContainer', () => {
      const bar = new BattlePassBar(container);
      bar.showXPToast([{ amount: 50, reason: 'a' }]);
      bar.showXPToast([{ amount: 30, reason: 'b' }]);
      bar.showXPToast([{ amount: 20, reason: 'c' }]);
      const toasts = bar.toastContainer.querySelectorAll('.bp-xp-toast');
      expect(toasts).toHaveLength(3);
    });

    it('stacked toasts display their own amounts independently', () => {
      const bar = new BattlePassBar(container);
      bar.showXPToast([{ amount: 50, reason: 'a' }]);
      bar.showXPToast([{ amount: 30, reason: 'b' }]);
      const toasts = bar.toastContainer.querySelectorAll('.bp-xp-toast');
      expect(toasts[0].textContent).toBe('+50 XP');
      expect(toasts[1].textContent).toBe('+30 XP');
    });

    it('cancels previous timer when a new toast arrives', () => {
      const bar = new BattlePassBar(container);
      const clearSpy = vi.spyOn(globalThis, 'clearTimeout');

      bar.showXPToast([{ amount: 50, reason: 'a' }]);
      const firstTimer = bar.toastTimer;

      bar.showXPToast([{ amount: 30, reason: 'b' }]);
      expect(clearSpy).toHaveBeenCalledWith(firstTimer);
      expect(bar.toastTimer).not.toBe(firstTimer);
    });

    it('toast removes itself from DOM when animationend fires', () => {
      const bar = new BattlePassBar(container);
      bar.showXPToast([{ amount: 50, reason: 'session_complete' }]);

      const toast = bar.toastContainer.querySelector('.bp-xp-toast');
      expect(toast).toBeTruthy();

      toast.dispatchEvent(new Event('animationend'));

      expect(bar.toastContainer.querySelector('.bp-xp-toast')).toBeNull();
    });

    it('animationend on one stacked toast only removes that toast', () => {
      const bar = new BattlePassBar(container);
      bar.showXPToast([{ amount: 50, reason: 'a' }]);
      bar.showXPToast([{ amount: 30, reason: 'b' }]);

      const toasts = [...bar.toastContainer.querySelectorAll('.bp-xp-toast')];
      expect(toasts).toHaveLength(2);

      toasts[0].dispatchEvent(new Event('animationend'));

      const remaining = bar.toastContainer.querySelectorAll('.bp-xp-toast');
      expect(remaining).toHaveLength(1);
      expect(remaining[0].textContent).toBe('+30 XP');
    });

    it('each toast independently removes itself on its own animationend', () => {
      const bar = new BattlePassBar(container);
      bar.showXPToast([{ amount: 50, reason: 'a' }]);
      bar.showXPToast([{ amount: 30, reason: 'b' }]);

      const toasts = [...bar.toastContainer.querySelectorAll('.bp-xp-toast')];
      toasts[0].dispatchEvent(new Event('animationend'));
      toasts[1].dispatchEvent(new Event('animationend'));

      expect(bar.toastContainer.querySelectorAll('.bp-xp-toast')).toHaveLength(0);
    });

    it('timer callback does not throw when timeout fires', () => {
      const bar = new BattlePassBar(container);
      bar.showXPToast([{ amount: 50, reason: 'session_complete' }]);
      expect(() => vi.advanceTimersByTime(3000)).not.toThrow();
    });
  });

  describe('playTierUpCelebration', () => {
    it('adds tier-up flash class to collapsed row', () => {
      const bar = createBarWithConfettiMock();
      bar.playTierUpCelebration();
      expect(bar.collapsedRow.classList.contains('bp-tier-up-flash')).toBe(true);
    });

    it('removes flash class after timeout', () => {
      const bar = createBarWithConfettiMock();
      bar.playTierUpCelebration();
      vi.advanceTimersByTime(2000);

      expect(bar.collapsedRow.classList.contains('bp-tier-up-flash')).toBe(false);
    });

    it('temporarily expands panel if collapsed', () => {
      const bar = createBarWithConfettiMock();
      expect(bar.expanded).toBe(false);

      bar.playTierUpCelebration();
      expect(bar.expanded).toBe(true);

      vi.advanceTimersByTime(2000);
      expect(bar.expanded).toBe(false);
    });
  });

  describe('renderChallenges', () => {
    it('shows empty message when no challenges', () => {
      const bar = new BattlePassBar(container);
      bar.challenges = [];
      bar.renderChallenges();

      expect(bar.challengeSection.querySelector('.bp-empty-message').textContent).toBe('No active challenges');
    });

    it('renders challenge rows with progress', () => {
      const bar = new BattlePassBar(container);
      bar.challenges = [
        { description: 'Do thing', current: 3, target: 10, complete: false },
        { description: 'Done thing', current: 5, target: 5, complete: true },
      ];
      bar.renderChallenges();

      const rows = bar.challengeSection.querySelectorAll('.bp-challenge-row');
      expect(rows).toHaveLength(2);

      const descs = bar.challengeSection.querySelectorAll('.bp-challenge-desc');
      expect(descs[0].textContent).toBe('Do thing');
      expect(descs[1].classList.contains('complete')).toBe(true);

      const progress = bar.challengeSection.querySelectorAll('.bp-challenge-progress');
      expect(progress[0].textContent).toBe('3/10');
      expect(progress[1].textContent).toBe('5/5');
    });
  });

  describe('renderXPLog', () => {
    it('shows empty message when no XP entries', () => {
      const bar = new BattlePassBar(container);
      bar.xpLog = [];
      bar.renderXPLog();

      expect(bar.xpLogSection.querySelector('.bp-empty-message').textContent).toBe('No XP awarded yet');
    });

    it('renders XP log entries with formatted reason', () => {
      const bar = new BattlePassBar(container);
      bar.xpLog = [
        { amount: 50, reason: 'session_complete' },
        { amount: 25, reason: 'tool_usage' },
      ];
      bar.renderXPLog();

      const entries = bar.xpLogSection.querySelectorAll('.bp-xp-log-entry');
      expect(entries).toHaveLength(2);

      const amounts = bar.xpLogSection.querySelectorAll('.xp-amount');
      expect(amounts[0].textContent).toBe('+50');
      expect(amounts[1].textContent).toBe('+25');
    });
  });

  describe('destroy', () => {
    it('cancels pending confetti animation frame', () => {
      const cancelSpy = vi.spyOn(globalThis, 'cancelAnimationFrame');
      const bar = new BattlePassBar(container);
      bar._confettiRaf = 42;

      bar.destroy();

      expect(cancelSpy).toHaveBeenCalledWith(42);
      expect(bar._confettiRaf).toBeNull();
    });

    it('does not call cancelAnimationFrame when no confetti is active', () => {
      const cancelSpy = vi.spyOn(globalThis, 'cancelAnimationFrame');
      const bar = new BattlePassBar(container);

      bar.destroy();

      expect(cancelSpy).not.toHaveBeenCalled();
    });

    it('clears toast and tier-up timers', () => {
      const clearSpy = vi.spyOn(globalThis, 'clearTimeout');
      const bar = new BattlePassBar(container);
      bar.showXPToast([{ amount: 50, reason: 'test' }]);

      vi.spyOn(bar, 'spawnConfetti').mockImplementation(() => {});
      bar.playTierUpCelebration();

      const savedToastTimer = bar.toastTimer;
      const savedTierUpTimer = bar.tierUpTimer;

      clearSpy.mockClear();
      bar.destroy();

      expect(clearSpy).toHaveBeenCalledWith(savedToastTimer);
      expect(clearSpy).toHaveBeenCalledWith(savedTierUpTimer);
      expect(bar.toastTimer).toBeNull();
      expect(bar.tierUpTimer).toBeNull();
    });

    it('is safe to call multiple times', () => {
      const bar = new BattlePassBar(container);
      bar._confettiRaf = 42;

      bar.destroy();
      expect(() => bar.destroy()).not.toThrow();
    });
  });

  describe('renderTierTrack', () => {
    it('renders 10 tier nodes', () => {
      const bar = new BattlePassBar(container);
      bar.state = { xp: 2500, tier: 3, tierProgress: 0.5, recentXP: [], rewards: [] };
      bar.renderTierTrack();

      const dots = bar.tierTrack.querySelectorAll('.bp-tier-dot');
      expect(dots).toHaveLength(10);
    });

    it('marks completed tiers', () => {
      const bar = new BattlePassBar(container);
      bar.state = { xp: 4000, tier: 5, tierProgress: 0.0, recentXP: [], rewards: [] };
      bar.renderTierTrack();

      const completedDots = bar.tierTrack.querySelectorAll('.bp-tier-dot.completed');
      // Tiers 1-4 completed
      expect(completedDots).toHaveLength(4);
    });

    it('marks current tier', () => {
      const bar = new BattlePassBar(container);
      bar.state = { xp: 4000, tier: 5, tierProgress: 0.0, recentXP: [], rewards: [] };
      bar.renderTierTrack();

      const currentDot = bar.tierTrack.querySelector('.bp-tier-dot.current');
      expect(currentDot).toBeTruthy();
      expect(currentDot.textContent).toBe('5');
    });
  });
});
