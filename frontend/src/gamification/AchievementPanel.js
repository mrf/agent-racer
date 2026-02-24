import { authFetch } from '../auth.js';

const CATEGORIES = [
  'Session Milestones',
  'Source Diversity',
  'Model Collection',
  'Performance & Endurance',
  'Spectacle',
  'Streaks',
];

const TIER_COLORS = {
  bronze:   { bg: '#92400e', border: '#d97706', text: '#fbbf24' },
  silver:   { bg: '#374151', border: '#9ca3af', text: '#e5e7eb' },
  gold:     { bg: '#78350f', border: '#f59e0b', text: '#fde68a' },
  platinum: { bg: '#1e3a5f', border: '#67e8f9', text: '#a5f3fc' },
};

const TIER_ICONS = {
  bronze:   '\u{1F949}', // third place medal
  silver:   '\u{1F948}', // second place medal
  gold:     '\u{1F947}', // first place medal
  platinum: '\u2728',    // sparkles
};

const CATEGORY_ICONS = {
  'Session Milestones':     '\u{1F3C1}', // checkered flag
  'Source Diversity':       '\u{1F310}', // globe with meridians
  'Model Collection':       '\u{1F9E0}', // brain
  'Performance & Endurance':'\u26A1',    // lightning bolt
  'Spectacle':              '\u{1F3A8}', // artist palette
  'Streaks':                '\u{1F525}', // fire
};

function buildPanelDOM() {
  const overlay = document.createElement('div');
  overlay.id = 'achievement-panel';
  overlay.className = 'achievement-panel hidden';
  overlay.setAttribute('role', 'dialog');
  overlay.setAttribute('aria-label', 'Achievements');
  overlay.setAttribute('aria-modal', 'true');

  overlay.innerHTML = `
    <div class="ap-inner">
      <div class="ap-header">
        <span class="ap-title">Achievements</span>
        <button class="ap-close" aria-label="Close achievements">\u00D7</button>
      </div>
      <div class="ap-body"></div>
      <div class="ap-footer">
        <span class="ap-counter">Loading\u2026</span>
      </div>
    </div>
    <div class="ap-tooltip hidden" role="tooltip"></div>
  `;

  document.body.appendChild(overlay);
  return overlay;
}

function injectStyles() {
  if (document.getElementById('achievement-panel-styles')) return;
  const style = document.createElement('style');
  style.id = 'achievement-panel-styles';
  style.textContent = `
    .achievement-panel {
      position: fixed;
      inset: 0;
      background: rgba(0, 0, 0, 0.75);
      z-index: 200;
      display: flex;
      align-items: center;
      justify-content: center;
      font-family: 'Courier New', monospace;
    }
    .achievement-panel.hidden { display: none; }

    .ap-inner {
      background: rgba(16, 16, 48, 0.97);
      border: 1px solid #2d2d6e;
      border-radius: 10px;
      box-shadow: 0 16px 64px rgba(0, 0, 0, 0.8);
      width: min(900px, 92vw);
      max-height: 88vh;
      display: flex;
      flex-direction: column;
    }

    .ap-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      padding: 14px 20px;
      border-bottom: 1px solid #2d2d6e;
      flex-shrink: 0;
    }

    .ap-title {
      font-size: 15px;
      font-weight: bold;
      color: #e0e0e0;
      letter-spacing: 2px;
      text-transform: uppercase;
    }

    .ap-close {
      background: none;
      border: none;
      color: #888;
      cursor: pointer;
      font-size: 22px;
      line-height: 1;
      padding: 0;
      width: 28px;
      height: 28px;
      display: flex;
      align-items: center;
      justify-content: center;
      border-radius: 4px;
      transition: background 0.15s, color 0.15s;
    }
    .ap-close:hover { background: rgba(255,255,255,0.1); color: #fff; }

    .ap-body {
      flex: 1;
      overflow-y: auto;
      padding: 20px;
    }
    .ap-body::-webkit-scrollbar { width: 6px; }
    .ap-body::-webkit-scrollbar-track { background: rgba(0,0,0,0.2); }
    .ap-body::-webkit-scrollbar-thumb { background: rgba(255,255,255,0.15); border-radius: 3px; }

    .ap-footer {
      padding: 10px 20px;
      border-top: 1px solid #2d2d6e;
      text-align: right;
      flex-shrink: 0;
    }

    .ap-counter {
      font-size: 12px;
      color: #666;
    }

    .ap-category {
      margin-bottom: 24px;
    }

    .ap-category-header {
      font-size: 11px;
      font-weight: bold;
      color: #666;
      text-transform: uppercase;
      letter-spacing: 2px;
      margin-bottom: 10px;
      display: flex;
      align-items: center;
      gap: 8px;
    }

    .ap-category-header::after {
      content: '';
      flex: 1;
      height: 1px;
      background: #222;
    }

    .ap-grid {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
      gap: 10px;
    }

    .ap-tile {
      background: rgba(255,255,255,0.03);
      border: 1px solid #2a2a4e;
      border-radius: 8px;
      padding: 12px 10px 10px;
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 6px;
      text-align: center;
      cursor: default;
      transition: background 0.15s, border-color 0.15s;
      position: relative;
    }

    .ap-tile.unlocked {
      background: rgba(255,255,255,0.06);
      border-color: #3d3d7e;
    }
    .ap-tile.unlocked:hover {
      background: rgba(255,255,255,0.1);
      border-color: #5555aa;
    }
    .ap-tile.locked {
      opacity: 0.45;
    }
    .ap-tile.locked:hover {
      opacity: 0.65;
      background: rgba(255,255,255,0.04);
    }

    .ap-tile-icon {
      font-size: 24px;
      line-height: 1;
    }

    .ap-tile-name {
      font-size: 11px;
      color: #bbb;
      font-weight: bold;
      line-height: 1.3;
    }
    .ap-tile.locked .ap-tile-name { color: #666; }

    .ap-tile-tier {
      font-size: 10px;
      font-weight: bold;
      padding: 2px 7px;
      border-radius: 3px;
      text-transform: uppercase;
      letter-spacing: 0.5px;
    }

    .ap-tile-padlock {
      position: absolute;
      top: 7px;
      right: 7px;
      font-size: 11px;
      opacity: 0.6;
    }

    .ap-tooltip {
      position: fixed;
      background: rgba(10, 10, 30, 0.98);
      border: 1px solid #444;
      border-radius: 6px;
      padding: 10px 13px;
      font-size: 12px;
      color: #ccc;
      max-width: 260px;
      pointer-events: none;
      z-index: 201;
      box-shadow: 0 4px 16px rgba(0,0,0,0.6);
      line-height: 1.5;
    }
    .ap-tooltip.hidden { display: none; }
    .ap-tooltip-name {
      font-weight: bold;
      color: #e0e0e0;
      margin-bottom: 4px;
    }
    .ap-tooltip-desc { color: #aaa; margin-bottom: 6px; }
    .ap-tooltip-unlocked { color: #22c55e; font-size: 11px; }
    .ap-tooltip-locked { color: #666; font-size: 11px; }
  `;
  document.head.appendChild(style);
}

export class AchievementPanel {
  constructor() {
    injectStyles();
    this._overlay = buildPanelDOM();
    this._body = this._overlay.querySelector('.ap-body');
    this._counter = this._overlay.querySelector('.ap-counter');
    this._tooltip = this._overlay.querySelector('.ap-tooltip');
    this._visible = false;
    this._achievements = [];

    this._overlay.querySelector('.ap-close').addEventListener('click', () => this.hide());
    // Click on backdrop (outside ap-inner) closes panel
    this._overlay.addEventListener('click', (e) => {
      if (e.target === this._overlay) this.hide();
    });

    this._onMouseMove = (e) => this._repositionTooltip(e.clientX, e.clientY);
    document.addEventListener('mousemove', this._onMouseMove);
  }

  async hydrate() {
    try {
      const resp = await authFetch('/api/achievements');
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      this._achievements = await resp.json();
      this._render();
    } catch (err) {
      this._body.innerHTML = `<p style="color:#e94560;font-size:12px">Failed to load achievements: ${err.message}</p>`;
    }
  }

  show() {
    if (this._visible) return;
    this._visible = true;
    this._overlay.classList.remove('hidden');
    this.hydrate();
    this._overlay.querySelector('.ap-close').focus();
  }

  hide() {
    if (!this._visible) return;
    this._visible = false;
    this._overlay.classList.add('hidden');
    this._tooltip.classList.add('hidden');
  }

  toggle() {
    if (this._visible) {
      this.hide();
    } else {
      this.show();
    }
  }

  get isVisible() {
    return this._visible;
  }

  _render() {
    if (!this._achievements.length) {
      this._body.innerHTML = '<p style="color:#666;font-size:12px">No achievements found.</p>';
      this._counter.textContent = '0 / 0 unlocked';
      return;
    }

    const unlocked = this._achievements.filter(a => a.unlocked).length;
    this._counter.textContent = `${unlocked} / ${this._achievements.length} unlocked`;

    // Group by category, preserving canonical order
    const byCategory = new Map(CATEGORIES.map(c => [c, []]));
    for (const a of this._achievements) {
      const list = byCategory.get(a.category);
      if (list) list.push(a);
    }

    const fragment = document.createDocumentFragment();
    for (const [category, items] of byCategory) {
      if (!items.length) continue;

      const section = document.createElement('div');
      section.className = 'ap-category';

      const header = document.createElement('div');
      header.className = 'ap-category-header';
      header.textContent = `${CATEGORY_ICONS[category] ?? ''} ${category}`;
      section.appendChild(header);

      const grid = document.createElement('div');
      grid.className = 'ap-grid';

      for (const a of items) {
        grid.appendChild(this._buildTile(a));
      }

      section.appendChild(grid);
      fragment.appendChild(section);
    }

    this._body.innerHTML = '';
    this._body.appendChild(fragment);
  }

  _buildTile(achievement) {
    const tile = document.createElement('div');
    tile.className = `ap-tile ${achievement.unlocked ? 'unlocked' : 'locked'}`;
    tile.setAttribute('data-id', achievement.id);

    const tierInfo = TIER_COLORS[achievement.tier] ?? TIER_COLORS.bronze;
    const tierIcon = TIER_ICONS[achievement.tier] ?? '';

    if (!achievement.unlocked) {
      const padlock = document.createElement('span');
      padlock.className = 'ap-tile-padlock';
      padlock.textContent = '\u{1F512}';
      padlock.setAttribute('aria-hidden', 'true');
      tile.appendChild(padlock);
    }

    const iconEl = document.createElement('div');
    iconEl.className = 'ap-tile-icon';
    iconEl.textContent = tierIcon;
    iconEl.setAttribute('aria-hidden', 'true');
    tile.appendChild(iconEl);

    const nameEl = document.createElement('div');
    nameEl.className = 'ap-tile-name';
    nameEl.textContent = achievement.name;
    tile.appendChild(nameEl);

    const tierEl = document.createElement('div');
    tierEl.className = 'ap-tile-tier';
    tierEl.textContent = achievement.tier;
    tierEl.style.background = tierInfo.bg;
    tierEl.style.border = `1px solid ${tierInfo.border}`;
    tierEl.style.color = tierInfo.text;
    tile.appendChild(tierEl);

    tile.addEventListener('mouseenter', (e) => this._showTooltip(achievement, e.clientX, e.clientY));
    tile.addEventListener('mouseleave', () => this._tooltip.classList.add('hidden'));

    return tile;
  }

  _showTooltip(achievement, x, y) {
    const tierIcon = TIER_ICONS[achievement.tier] ?? '';
    let statusLine;
    if (achievement.unlocked && achievement.unlockedAt) {
      const date = new Date(achievement.unlockedAt).toLocaleDateString();
      statusLine = `<div class="ap-tooltip-unlocked">\u2713 Unlocked ${date}</div>`;
    } else {
      statusLine = `<div class="ap-tooltip-locked">\u{1F512} ${achievement.description}</div>`;
    }

    this._tooltip.innerHTML = `
      <div class="ap-tooltip-name">${tierIcon} ${achievement.name}</div>
      <div class="ap-tooltip-desc">${achievement.description}</div>
      ${statusLine}
    `;
    this._tooltip.classList.remove('hidden');
    this._repositionTooltip(x, y);
  }

  _repositionTooltip(x, y) {
    if (this._tooltip.classList.contains('hidden')) return;
    const margin = 14;
    const tw = this._tooltip.offsetWidth || 260;
    const th = this._tooltip.offsetHeight || 80;

    let left = x + margin;
    let top = y + margin;

    if (left + tw > window.innerWidth - 8) left = x - tw - margin;
    if (top + th > window.innerHeight - 8) top = y - th - margin;

    this._tooltip.style.left = `${Math.max(8, left)}px`;
    this._tooltip.style.top = `${Math.max(8, top)}px`;
  }

  destroy() {
    document.removeEventListener('mousemove', this._onMouseMove);
    this._overlay.remove();
  }
}
