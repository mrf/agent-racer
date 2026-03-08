// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ShortcutBar } from './ShortcutBar.js';

let container;
let shortcutBar;

beforeEach(() => {
  document.body.innerHTML = '<div id="shortcut-bar"></div>';
  container = document.getElementById('shortcut-bar');
  window.innerWidth = 1024;
});

afterEach(() => {
  shortcutBar?.destroy();
  shortcutBar = null;
  vi.restoreAllMocks();
});

describe('ShortcutBar', () => {
  it('removes its resize listener on destroy()', () => {
    const addSpy = vi.spyOn(window, 'addEventListener');
    const removeSpy = vi.spyOn(window, 'removeEventListener');

    shortcutBar = new ShortcutBar(container);

    const resizeCall = addSpy.mock.calls.find(([type]) => type === 'resize');
    expect(resizeCall).toBeTruthy();

    const resizeHandler = resizeCall[1];
    expect(typeof resizeHandler).toBe('function');

    shortcutBar.destroy();
    shortcutBar = null;

    expect(removeSpy).toHaveBeenCalledWith('resize', resizeHandler);
  });

  it('stops reacting to resize events after destroy()', () => {
    shortcutBar = new ShortcutBar(container);

    const label = container.querySelector('.shortcut-label');
    expect(label.style.display).toBe('');

    window.innerWidth = 640;
    window.dispatchEvent(new Event('resize'));
    expect(label.style.display).toBe('none');

    shortcutBar.destroy();
    shortcutBar = null;

    window.innerWidth = 1024;
    window.dispatchEvent(new Event('resize'));
    expect(label.style.display).toBe('none');
  });
});
