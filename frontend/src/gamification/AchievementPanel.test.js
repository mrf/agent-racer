// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { AchievementPanel } from './AchievementPanel.js';

vi.mock('../auth.js', () => ({
  authFetch: vi.fn(() => Promise.resolve({ ok: false, status: 500 })),
}));

function mousemoveCalls(spy) {
  return spy.mock.calls.filter(([type]) => type === 'mousemove');
}

beforeEach(() => {
  document.body.innerHTML = '';
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('AchievementPanel mousemove listener lifecycle', () => {
  it('does not attach mousemove listener on construction', () => {
    const addSpy = vi.spyOn(document, 'addEventListener');
    new AchievementPanel();
    expect(mousemoveCalls(addSpy)).toHaveLength(0);
  });

  it('attaches mousemove listener when panel is shown', () => {
    const panel = new AchievementPanel();
    const addSpy = vi.spyOn(document, 'addEventListener');
    panel.show();
    expect(mousemoveCalls(addSpy)).toHaveLength(1);
  });

  it('removes mousemove listener when panel is hidden', () => {
    const panel = new AchievementPanel();
    panel.show();
    const removeSpy = vi.spyOn(document, 'removeEventListener');
    panel.hide();
    expect(mousemoveCalls(removeSpy)).toHaveLength(1);
  });

  it('does not add duplicate listeners on repeated show() calls', () => {
    const panel = new AchievementPanel();
    const addSpy = vi.spyOn(document, 'addEventListener');
    panel.show();
    panel.show(); // second call is a no-op
    expect(mousemoveCalls(addSpy)).toHaveLength(1);
  });

  it('does not remove listener if hide() called while already hidden', () => {
    const panel = new AchievementPanel();
    const removeSpy = vi.spyOn(document, 'removeEventListener');
    panel.hide(); // called while not visible -- should be no-op
    expect(mousemoveCalls(removeSpy)).toHaveLength(0);
  });

  it('listener is active only while panel is open (add on show, remove on hide)', () => {
    const panel = new AchievementPanel();
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
