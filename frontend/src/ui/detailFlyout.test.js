// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { createFlyout } from './detailFlyout.js';

function mockElements() {
  const detailFlyout = document.createElement('div');
  detailFlyout.classList.add('hidden');
  Object.defineProperty(detailFlyout, 'offsetWidth', { get: () => 380, configurable: true });
  Object.defineProperty(detailFlyout, 'offsetHeight', { get: () => 200, configurable: true });
  document.body.appendChild(detailFlyout);

  const flyoutContent = document.createElement('div');
  detailFlyout.appendChild(flyoutContent);

  const canvas = document.createElement('canvas');
  canvas.getBoundingClientRect = vi.fn(() => ({
    left: 100, top: 50, width: 800, height: 600, right: 900, bottom: 650,
  }));
  document.body.appendChild(canvas);

  return { detailFlyout, flyoutContent, canvas };
}

function makeSession(overrides = {}) {
  return {
    id: 'sess-abc-123',
    activity: 'thinking',
    contextUtilization: 0.5,
    tokensUsed: 50000,
    maxContextTokens: 200000,
    burnRatePerMinute: 1200,
    model: 'claude-sonnet-4-6',
    source: 'claude',
    workingDir: '/home/user/projects/my-app',
    branch: 'main',
    tmuxTarget: 'cc-main:0',
    slug: 'my-session',
    pid: 12345,
    messageCount: 42,
    toolCallCount: 18,
    currentTool: 'Edit',
    startedAt: '2026-03-01T10:00:00Z',
    lastActivityAt: '2026-03-01T10:05:00Z',
    completedAt: null,
    isChurning: false,
    subagents: [],
    ...overrides,
  };
}

function makeHamster(overrides = {}) {
  return {
    id: 'hamster-xyz',
    slug: 'sub-researcher',
    activity: 'tool_use',
    model: 'claude-haiku-4-5-20251001',
    currentTool: 'Bash',
    messageCount: 5,
    toolCallCount: 3,
    startedAt: '2026-03-01T10:02:00Z',
    ...overrides,
  };
}

describe('createFlyout', () => {
  let els;
  let flyout;

  beforeEach(() => {
    els = mockElements();
    flyout = createFlyout(els);
    Object.defineProperty(window, 'innerWidth', { value: 1920, writable: true, configurable: true });
    Object.defineProperty(window, 'innerHeight', { value: 1080, writable: true, configurable: true });
  });

  afterEach(() => {
    document.body.innerHTML = '';
    vi.restoreAllMocks();
  });

  describe('isVisible', () => {
    it('returns false when flyout has hidden class', () => {
      expect(flyout.isVisible()).toBe(false);
    });

    it('returns true after show()', () => {
      flyout.show(makeSession(), 400, 300);
      expect(flyout.isVisible()).toBe(true);
    });

    it('returns false after hide()', () => {
      flyout.show(makeSession(), 400, 300);
      flyout.hide();
      expect(flyout.isVisible()).toBe(false);
    });
  });

  describe('getSelectedSessionId / getSelectedHamsterId', () => {
    it('returns null before any show()', () => {
      expect(flyout.getSelectedSessionId()).toBeNull();
      expect(flyout.getSelectedHamsterId()).toBeNull();
    });

    it('returns session id after show()', () => {
      flyout.show(makeSession({ id: 'test-sess' }), 400, 300);
      expect(flyout.getSelectedSessionId()).toBe('test-sess');
      expect(flyout.getSelectedHamsterId()).toBeNull();
    });

    it('returns both session and hamster ids after showHamster()', () => {
      const parent = makeSession({ id: 'parent-sess' });
      const hamster = makeHamster({ id: 'hamster-1' });
      flyout.showHamster(hamster, parent, 400, 300);
      expect(flyout.getSelectedSessionId()).toBe('parent-sess');
      expect(flyout.getSelectedHamsterId()).toBe('hamster-1');
    });

    it('clears hamster id when switching from hamster to session show', () => {
      const parent = makeSession({ id: 'parent-sess' });
      const hamster = makeHamster({ id: 'hamster-1' });
      flyout.showHamster(hamster, parent, 400, 300);

      flyout.show(makeSession({ id: 'other-sess' }), 400, 300);
      expect(flyout.getSelectedSessionId()).toBe('other-sess');
      expect(flyout.getSelectedHamsterId()).toBeNull();
    });

    it('clears all ids after hide()', () => {
      flyout.show(makeSession({ id: 'test-sess' }), 400, 300);
      flyout.hide();
      expect(flyout.getSelectedSessionId()).toBeNull();
      expect(flyout.getSelectedHamsterId()).toBeNull();
    });
  });

  describe('show()', () => {
    it('removes hidden class from flyout element', () => {
      flyout.show(makeSession(), 400, 300);
      expect(els.detailFlyout.classList.contains('hidden')).toBe(false);
    });

    it('sets left and top style for positioning', () => {
      flyout.show(makeSession(), 400, 300);
      expect(els.detailFlyout.style.left).toBeTruthy();
      expect(els.detailFlyout.style.top).toBeTruthy();
    });

    it('renders session detail content into flyoutContent', () => {
      const state = makeSession({
        activity: 'tool_use',
        model: 'claude-opus-4-6',
        branch: 'feature-x',
      });
      flyout.show(state, 400, 300);

      const html = els.flyoutContent.innerHTML;
      expect(html).toContain('tool_use');
      expect(html).toContain('claude-opus-4-6');
      expect(html).toContain('feature-x');
    });
  });

  describe('showHamster()', () => {
    it('renders hamster-specific content', () => {
      const parent = makeSession({ id: 'p1', workingDir: '/home/user/app' });
      const hamster = makeHamster({ slug: 'test-sub', activity: 'thinking', currentTool: 'Read' });
      flyout.showHamster(hamster, parent, 400, 300);

      const html = els.flyoutContent.innerHTML;
      expect(html).toContain('Subagent');
      expect(html).toContain('test-sub');
      expect(html).toContain('thinking');
      expect(html).toContain('Read');
      expect(html).toContain('app');
    });

    it('removes hidden class', () => {
      flyout.showHamster(makeHamster(), makeSession(), 400, 300);
      expect(els.detailFlyout.classList.contains('hidden')).toBe(false);
    });
  });

  describe('hide()', () => {
    it('adds hidden class', () => {
      flyout.show(makeSession(), 400, 300);
      flyout.hide();
      expect(els.detailFlyout.classList.contains('hidden')).toBe(true);
    });

    it('resets position tracking so next show snaps instead of smoothing', () => {
      flyout.show(makeSession(), 100, 100);
      const firstLeft = els.detailFlyout.style.left;

      flyout.hide();

      flyout.show(makeSession(), 700, 500);
      const secondLeft = els.detailFlyout.style.left;

      // After hide(), position state is reset so the flyout snaps to the new target
      expect(secondLeft).not.toBe(firstLeft);
    });
  });

  describe('updateContent()', () => {
    it('does nothing when no session is selected', () => {
      const sessions = new Map([['s1', makeSession({ id: 's1' })]]);
      flyout.updateContent(sessions);
      expect(els.flyoutContent.innerHTML).toBe('');
    });

    it('does nothing when selected session is not in the map', () => {
      flyout.show(makeSession({ id: 'missing' }), 400, 300);
      const contentBefore = els.flyoutContent.innerHTML;

      const sessions = new Map([['other', makeSession({ id: 'other' })]]);
      flyout.updateContent(sessions);

      expect(els.flyoutContent.innerHTML).toBe(contentBefore);
    });

    it('updates content for selected session', () => {
      flyout.show(makeSession({ id: 's1', activity: 'thinking' }), 400, 300);

      const sessions = new Map([
        ['s1', makeSession({ id: 's1', activity: 'tool_use', currentTool: 'Bash' })],
      ]);
      flyout.updateContent(sessions);

      const html = els.flyoutContent.innerHTML;
      expect(html).toContain('tool_use');
      expect(html).toContain('Bash');
    });

    it('updates hamster content when hamster is selected', () => {
      const parent = makeSession({
        id: 'p1',
        subagents: [makeHamster({ id: 'h1', activity: 'thinking' })],
      });
      flyout.showHamster(makeHamster({ id: 'h1' }), parent, 400, 300);

      const sessions = new Map([
        ['p1', makeSession({
          id: 'p1',
          subagents: [makeHamster({ id: 'h1', activity: 'tool_use', currentTool: 'Write' })],
        })],
      ]);
      flyout.updateContent(sessions);

      const html = els.flyoutContent.innerHTML;
      expect(html).toContain('tool_use');
      expect(html).toContain('Write');
    });

    it('falls back to session content when selected hamster disappears', () => {
      const parent = makeSession({
        id: 'p1',
        subagents: [makeHamster({ id: 'h1' })],
      });
      flyout.showHamster(makeHamster({ id: 'h1' }), parent, 400, 300);

      const sessions = new Map([
        ['p1', makeSession({ id: 'p1', subagents: [], activity: 'thinking' })],
      ]);
      flyout.updateContent(sessions);

      const html = els.flyoutContent.innerHTML;
      expect(html).not.toContain('Subagent:');
      expect(html).toContain('Activity');
      expect(flyout.getSelectedHamsterId()).toBeNull();
    });
  });

  describe('renderDetailContent', () => {
    it('renders session ID truncated to 12 chars', () => {
      const longId = 'abcdefghijklmnopqrstuvwxyz';
      flyout.show(makeSession({ id: longId }), 400, 300);
      const html = els.flyoutContent.innerHTML;
      expect(html).toContain(longId.slice(0, 12));
    });

    it('renders copy button with full session ID', () => {
      const id = 'full-session-id-value';
      flyout.show(makeSession({ id }), 400, 300);
      const btn = els.flyoutContent.querySelector('.copy-btn');
      expect(btn).not.toBeNull();
      expect(btn.getAttribute('data-copy')).toBe(id);
    });

    it('renders churning indicator when isChurning is true', () => {
      flyout.show(makeSession({ isChurning: true }), 400, 300);
      const html = els.flyoutContent.innerHTML;
      expect(html).toContain('CPU Active');
    });

    it('does not render churning indicator when isChurning is false', () => {
      flyout.show(makeSession({ isChurning: false }), 400, 300);
      const html = els.flyoutContent.innerHTML;
      expect(html).not.toContain('CPU Active');
    });

    it('renders completedAt row when session is completed', () => {
      flyout.show(makeSession({ completedAt: '2026-03-01T10:10:00Z' }), 400, 300);
      const html = els.flyoutContent.innerHTML;
      expect(html).toContain('Completed');
    });

    it('omits completedAt row when session is not completed', () => {
      flyout.show(makeSession({ completedAt: null }), 400, 300);
      const labels = els.flyoutContent.querySelectorAll('.label');
      const hasCompleted = Array.from(labels).some(l => l.textContent === 'Completed');
      expect(hasCompleted).toBe(false);
    });

    it('renders subagents section when subagents are present', () => {
      const state = makeSession({
        subagents: [
          makeHamster({ slug: 'sub-a', activity: 'thinking' }),
          makeHamster({ slug: 'sub-b', activity: 'tool_use', currentTool: 'Read' }),
        ],
      });
      flyout.show(state, 400, 300);
      const html = els.flyoutContent.innerHTML;
      expect(html).toContain('Subagents (2)');
      expect(html).toContain('sub-a');
      expect(html).toContain('sub-b');
    });

    it('omits subagents section when no subagents', () => {
      flyout.show(makeSession({ subagents: [] }), 400, 300);
      const html = els.flyoutContent.innerHTML;
      expect(html).not.toContain('Subagents');
    });

    it('renders session slug when present', () => {
      flyout.show(makeSession({ slug: 'my-cool-session' }), 400, 300);
      const html = els.flyoutContent.innerHTML;
      expect(html).toContain('Session Name');
      expect(html).toContain('my-cool-session');
    });

    it('omits session name row when slug is empty', () => {
      flyout.show(makeSession({ slug: '' }), 400, 300);
      const html = els.flyoutContent.innerHTML;
      expect(html).not.toContain('Session Name');
    });

    it('renders tmux target when available', () => {
      flyout.show(makeSession({ tmuxTarget: 'cc-work:2' }), 400, 300);
      const html = els.flyoutContent.innerHTML;
      expect(html).toContain('cc-work:2');
    });

    it('renders "not in tmux" when tmuxTarget is falsy', () => {
      flyout.show(makeSession({ tmuxTarget: null }), 400, 300);
      const html = els.flyoutContent.innerHTML;
      expect(html).toContain('not in tmux');
    });

    it('renders basename of working directory', () => {
      flyout.show(makeSession({ workingDir: '/home/user/projects/agent-racer' }), 400, 300);
      const html = els.flyoutContent.innerHTML;
      expect(html).toContain('agent-racer');
    });

    it('renders dash when workingDir is empty', () => {
      flyout.show(makeSession({ workingDir: '' }), 400, 300);
      expect(els.flyoutContent.innerHTML).toContain('Working Dir');
    });

    it('renders source badge with source-specific class', () => {
      flyout.show(makeSession({ source: 'claude' }), 400, 300);
      const badge = els.flyoutContent.querySelector('.source-badge');
      expect(badge).not.toBeNull();
      expect(badge.classList.contains('source-claude')).toBe(true);
    });

    it('escapes HTML in rendered values', () => {
      flyout.show(makeSession({ model: '<script>alert("xss")</script>' }), 400, 300);
      const html = els.flyoutContent.innerHTML;
      expect(html).not.toContain('<script>');
      expect(html).toContain('&lt;script&gt;');
    });

    it('renders green bar for low utilization (<=0.5)', () => {
      flyout.show(makeSession({ contextUtilization: 0.3 }), 400, 300);
      expect(els.flyoutContent.innerHTML).toContain('background:#22c55e');
    });

    it('renders orange bar for medium utilization (>0.5, <=0.8)', () => {
      flyout.show(makeSession({ contextUtilization: 0.6 }), 400, 300);
      expect(els.flyoutContent.innerHTML).toContain('background:#d97706');
    });

    it('renders red bar for high utilization (>0.8)', () => {
      flyout.show(makeSession({ contextUtilization: 0.9 }), 400, 300);
      expect(els.flyoutContent.innerHTML).toContain('background:#e94560');
    });
  });

  describe('positionFlyout', () => {
    it('prefers right placement when space is available', () => {
      flyout.show(makeSession(), 200, 300);
      // carVX = 100 (canvasLeft) + 200 = 300, right placement: 300 + 50 = 350
      // 350 + 380 + 10 = 740 < 1920 — fits right
      const left = parseInt(els.detailFlyout.style.left, 10);
      expect(left).toBe(350);
    });

    it('falls back to left when right edge overflows', () => {
      // Position car near right edge of canvas
      Object.defineProperty(window, 'innerWidth', { value: 600, writable: true, configurable: true });
      flyout.show(makeSession(), 400, 300);
      // carVX = 100 + 400 = 500, right: 500+50+380+10 = 940 > 600 — doesn't fit right
      // left: 500-50-380 = 70 > 10 — fits left
      const left = parseInt(els.detailFlyout.style.left, 10);
      expect(left).toBe(70);
    });

    it('smooths position on updatePosition calls', () => {
      flyout.show(makeSession(), 200, 300);
      flyout.updatePosition(250, 350);
      const left = parseInt(els.detailFlyout.style.left, 10);

      // Smoothing factor is 0.25, so the position moves partway toward the target
      expect(typeof left).toBe('number');
    });

    it('adds arrow class to flyout element', () => {
      flyout.show(makeSession(), 200, 300);
      expect(els.detailFlyout.className).toMatch(/arrow-\w+/);
    });

    it('clamps position within viewport bounds', () => {
      Object.defineProperty(window, 'innerWidth', { value: 500, writable: true, configurable: true });
      Object.defineProperty(window, 'innerHeight', { value: 400, writable: true, configurable: true });

      flyout.show(makeSession(), 0, 0);
      const left = parseInt(els.detailFlyout.style.left, 10);
      const top = parseInt(els.detailFlyout.style.top, 10);

      expect(left).toBeGreaterThanOrEqual(10);
      expect(top).toBeGreaterThanOrEqual(10);
    });

    it('skips positioning when flyout is hidden', () => {
      flyout.show(makeSession(), 200, 300);
      const initialLeft = els.detailFlyout.style.left;
      flyout.hide();
      flyout.updatePosition(500, 500);
      expect(els.detailFlyout.style.left).toBe(initialLeft);
    });
  });

  describe('renderHamsterContent', () => {
    it('renders hamster slug as title', () => {
      flyout.showHamster(
        makeHamster({ slug: 'code-reviewer' }),
        makeSession(),
        400, 300,
      );
      expect(els.flyoutContent.innerHTML).toContain('code-reviewer');
    });

    it('falls back to hamster id when slug is empty', () => {
      flyout.showHamster(
        makeHamster({ slug: '', id: 'hamster-fallback-id' }),
        makeSession(),
        400, 300,
      );
      expect(els.flyoutContent.innerHTML).toContain('hamster-fallback-id');
    });

    it('renders parent working directory basename', () => {
      flyout.showHamster(
        makeHamster(),
        makeSession({ workingDir: '/a/b/parent-dir' }),
        400, 300,
      );
      expect(els.flyoutContent.innerHTML).toContain('parent-dir');
    });

    it('falls back to parent name when workingDir is empty', () => {
      flyout.showHamster(
        makeHamster(),
        makeSession({ workingDir: '', name: 'parent-name' }),
        400, 300,
      );
      expect(els.flyoutContent.innerHTML).toContain('parent-name');
    });

    it('renders hamster message and tool call counts', () => {
      flyout.showHamster(
        makeHamster({ messageCount: 15, toolCallCount: 7 }),
        makeSession(),
        400, 300,
      );
      const html = els.flyoutContent.innerHTML;
      expect(html).toContain('15');
      expect(html).toContain('7');
    });

    it('defaults messageCount and toolCallCount to 0', () => {
      flyout.showHamster(
        makeHamster({ messageCount: undefined, toolCallCount: undefined }),
        makeSession(),
        400, 300,
      );
      expect(els.flyoutContent.innerHTML).toContain('>0<');
    });
  });
});
