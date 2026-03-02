const SHORTCUTS = [
  { key: 'a', label: 'achievements', id: 'achievements' },
  { key: 'g', label: 'garage', id: 'garage' },
  { key: 'd', label: 'debug', id: 'debug' },
  { key: 'm', label: 'mute', id: 'mute' },
  { key: 'f', label: 'fullscreen', id: 'fullscreen' },
  { key: 'esc', label: 'close', id: 'close' },
  { key: 'click', label: 'details', id: 'details' },
];

const BREAKPOINT_NARROW = 720;

export class ShortcutBar {
  constructor(container) {
    this.container = container;
    this.items = new Map();
    this.narrowEls = [];
    this.render();
    window.addEventListener('resize', () => this.updateLayout());
  }

  render() {
    this.container.innerHTML = '';
    for (const s of SHORTCUTS) {
      const item = document.createElement('span');
      item.className = 'shortcut-item';
      item.dataset.id = s.id;

      const key = document.createElement('span');
      key.className = 'shortcut-key';
      key.textContent = s.key;

      const sep = document.createElement('span');
      sep.className = 'shortcut-sep';
      sep.textContent = ':';

      const label = document.createElement('span');
      label.className = 'shortcut-label';
      label.textContent = s.label;

      item.appendChild(key);
      item.appendChild(sep);
      item.appendChild(label);
      this.container.appendChild(item);
      this.items.set(s.id, item);
      this.narrowEls.push(sep, label);
    }
    this.updateLayout();
  }

  updateLayout() {
    const display = window.innerWidth < BREAKPOINT_NARROW ? 'none' : '';
    for (const el of this.narrowEls) el.style.display = display;
  }

  setActive(id, active) {
    const el = this.items.get(id);
    if (el) el.classList.toggle('active', active);
  }
}
