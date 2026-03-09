// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { HelpPopup } from './HelpPopup.js';

let popup;

beforeEach(() => {
  document.body.innerHTML = '';
});

afterEach(() => {
  document.body.innerHTML = '';
  popup = null;
});

describe('HelpPopup', () => {
  it('does not inject DOM during construction', () => {
    popup = new HelpPopup();
    expect(document.getElementById('help-popup')).toBeNull();
    expect(popup.isVisible).toBe(false);
  });

  it('creates and shows the overlay on first show()', () => {
    popup = new HelpPopup();
    popup.show();

    const overlay = document.getElementById('help-popup');
    expect(overlay).toBeTruthy();
    expect(overlay.classList.contains('hidden')).toBe(false);
    expect(popup.isVisible).toBe(true);
  });

  it('wires close interactions after lazy creation', () => {
    popup = new HelpPopup();
    popup.show();

    document.querySelector('.help-close').click();
    expect(popup.isVisible).toBe(false);

    popup.show();
    const overlay = document.getElementById('help-popup');
    overlay.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    expect(popup.isVisible).toBe(false);
  });
});
