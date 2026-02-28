import { authFetch } from '../auth.js';

const CATEGORIES = [
  'Session Milestones',
  'Source Diversity',
  'Model Collection',
  'Performance & Endurance',
  'Spectacle',
  'Streaks',
];

const TIER_CLASSES = new Set(['bronze', 'silver', 'gold', 'platinum']);

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

function escapeHTML(s) {
  if (!s) return '';
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

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

export class AchievementPanel {
  constructor() {
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
      this._body.innerHTML = `<p class="ap-error-message">Failed to load achievements: ${escapeHTML(err.message)}</p>`;
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
      this._body.innerHTML = '<p class="ap-empty-message">No achievements found.</p>';
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
    const tierClass = TIER_CLASSES.has(achievement.tier) ? achievement.tier : 'bronze';
    tierEl.className = `ap-tile-tier ap-tile-tier-${tierClass}`;
    tierEl.textContent = achievement.tier;
    tile.appendChild(tierEl);

    tile.addEventListener('mouseenter', (e) => this._showTooltip(achievement, e.clientX, e.clientY));
    tile.addEventListener('mouseleave', () => this._tooltip.classList.add('hidden'));

    return tile;
  }

  _showTooltip(achievement, x, y) {
    const tierIcon = TIER_ICONS[achievement.tier] ?? '';
    const name = escapeHTML(achievement.name);
    const desc = escapeHTML(achievement.description);
    let statusLine;
    if (achievement.unlocked && achievement.unlockedAt) {
      const date = new Date(achievement.unlockedAt).toLocaleDateString();
      statusLine = `<div class="ap-tooltip-unlocked">\u2713 Unlocked ${date}</div>`;
    } else {
      statusLine = `<div class="ap-tooltip-locked">\u{1F512} ${desc}</div>`;
    }

    this._tooltip.innerHTML = `
      <div class="ap-tooltip-name">${tierIcon} ${name}</div>
      <div class="ap-tooltip-desc">${desc}</div>
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
