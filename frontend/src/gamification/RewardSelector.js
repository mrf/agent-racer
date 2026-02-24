import { authFetch } from '../auth.js';
import {
  getEquippedLoadout,
  setEquipped,
  onEquippedChange,
  getEquippedPaint,
  getEquippedBadge,
} from './CosmeticRegistry.js';

// ── Slot metadata ────────────────────────────────────────────────────
const SLOTS = [
  { key: 'paint', label: 'Paint',       icon: '\u{1F3A8}' }, // artist palette
  { key: 'trail', label: 'Trail',       icon: '\u2728' },    // sparkles
  { key: 'body',  label: 'Body',        icon: '\u{1F3CE}' }, // racing car
  { key: 'badge', label: 'Badge',       icon: '\u{1F3C5}' }, // medal
  { key: 'sound', label: 'Sound',       icon: '\u{1F50A}' }, // speaker
  { key: 'theme', label: 'Track Theme', icon: '\u{1F305}' }, // sunrise
  { key: 'title', label: 'Title',       icon: '\u{1F3F7}' }, // label
];

// ── Reward registry ──────────────────────────────────────────────────
// Mirrors backend buildRewardList() in rewards.go. Each entry includes the
// achievement ID that unlocks it (empty string = battle pass reward).
const REWARDS = [
  // Battle pass tier rewards
  { id: 'bronze_badge',    type: 'badge', name: 'Bronze Badge',     unlockedBy: '' },
  { id: 'spark_trail',     type: 'trail', name: 'Spark Trail',      unlockedBy: '' },
  { id: 'rev_sound',       type: 'sound', name: 'Rev Sound',        unlockedBy: '' },
  { id: 'metallic_paint',  type: 'paint', name: 'Metallic Paint',   unlockedBy: '' },
  { id: 'silver_badge',    type: 'badge', name: 'Silver Badge',     unlockedBy: '' },
  { id: 'flame_trail',     type: 'trail', name: 'Flame Trail',      unlockedBy: '' },
  { id: 'aero_body',       type: 'body',  name: 'Aero Body',        unlockedBy: '' },
  { id: 'dark_theme',      type: 'theme', name: 'Dark Theme',       unlockedBy: '' },
  { id: 'champion_title',  type: 'title', name: 'Champion',         unlockedBy: '' },
  { id: 'gold_badge',      type: 'badge', name: 'Gold Badge',       unlockedBy: '' },

  // Session Milestones
  { id: 'rookie_paint',    type: 'paint', name: 'Rookie Paint',     unlockedBy: 'first_lap' },
  { id: 'pit_badge',       type: 'badge', name: 'Pit Badge',        unlockedBy: 'pit_crew' },
  { id: 'veteran_title',   type: 'title', name: 'Veteran',          unlockedBy: 'veteran_driver' },
  { id: 'century_paint',   type: 'paint', name: 'Century Paint',    unlockedBy: 'century_club' },
  { id: 'legend_title',    type: 'title', name: 'Legend',           unlockedBy: 'track_legend' },

  // Source Diversity
  { id: 'home_trail',      type: 'trail', name: 'Home Trail',       unlockedBy: 'home_turf' },
  { id: 'gemini_paint',    type: 'paint', name: 'Gemini Paint',     unlockedBy: 'gemini_rising' },
  { id: 'codex_paint',     type: 'paint', name: 'Codex Paint',      unlockedBy: 'codex_curious' },
  { id: 'triple_body',     type: 'body',  name: 'Triple Body',      unlockedBy: 'triple_threat' },
  { id: 'polyglot_theme',  type: 'theme', name: 'Polyglot Theme',   unlockedBy: 'polyglot' },

  // Model Collection
  { id: 'opus_sound',       type: 'sound', name: 'Opus Sound',       unlockedBy: 'opus_enthusiast' },
  { id: 'sonnet_sound',     type: 'sound', name: 'Sonnet Sound',     unlockedBy: 'sonnet_fan' },
  { id: 'haiku_trail',      type: 'trail', name: 'Haiku Trail',      unlockedBy: 'haiku_speedster' },
  { id: 'spectrum_paint',   type: 'paint', name: 'Full Spectrum',    unlockedBy: 'full_spectrum' },
  { id: 'collector_badge',  type: 'badge', name: 'Collector Badge',  unlockedBy: 'model_collector' },
  { id: 'connoisseur_body', type: 'body',  name: 'Connoisseur Body', unlockedBy: 'connoisseur' },

  // Performance & Endurance
  { id: 'redline_trail',      type: 'trail', name: 'Redline Trail',      unlockedBy: 'redline' },
  { id: 'afterburner_sound',  type: 'sound', name: 'Afterburner Sound',  unlockedBy: 'afterburner' },
  { id: 'marathon_title',     type: 'title', name: 'Marathoner',         unlockedBy: 'marathon' },
  { id: 'tool_fiend_body',    type: 'body',  name: 'Tool Fiend Body',    unlockedBy: 'tool_fiend' },
  { id: 'clean_sweep_paint',  type: 'paint', name: 'Clean Sweep Paint',  unlockedBy: 'clean_sweep' },

  // Spectacle
  { id: 'grid_badge',           type: 'badge', name: 'Grid Badge',           unlockedBy: 'grid_start' },
  { id: 'full_grid_theme',      type: 'theme', name: 'Full Grid Theme',      unlockedBy: 'full_grid' },
  { id: 'crash_survivor_trail', type: 'trail', name: 'Survivor Trail',       unlockedBy: 'crash_survivor' },
  { id: 'burning_rubber_sound', type: 'sound', name: 'Burning Rubber Sound', unlockedBy: 'burning_rubber' },

  // Streaks
  { id: 'hat_trick_badge',  type: 'badge', name: 'Hat Trick Badge',  unlockedBy: 'hat_trick' },
  { id: 'on_a_roll_trail',  type: 'trail', name: 'On a Roll Trail',  unlockedBy: 'on_a_roll' },
  { id: 'untouchable_title', type: 'title', name: 'Untouchable',     unlockedBy: 'untouchable' },
];

// Battle pass tier requirements for rewards with unlockedBy === ''
const BATTLE_PASS_TIERS = {
  bronze_badge:   2,
  spark_trail:    3,
  rev_sound:      4,
  metallic_paint: 5,
  silver_badge:   6,
  flame_trail:    7,
  aero_body:      8,
  dark_theme:     9,
  champion_title: 10,
  gold_badge:     10,
};

function escapeHTML(s) {
  if (!s) return '';
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

// ── Styles ───────────────────────────────────────────────────────────

function injectStyles() {
  if (document.getElementById('reward-selector-styles')) return;
  const style = document.createElement('style');
  style.id = 'reward-selector-styles';
  style.textContent = `
    .reward-selector {
      position: fixed;
      inset: 0;
      background: rgba(0, 0, 0, 0.75);
      z-index: 200;
      display: flex;
      align-items: center;
      justify-content: center;
      font-family: 'Courier New', monospace;
    }
    .reward-selector.hidden { display: none; }

    .rs-inner {
      background: rgba(16, 16, 48, 0.97);
      border: 1px solid #2d2d6e;
      border-radius: 10px;
      box-shadow: 0 16px 64px rgba(0, 0, 0, 0.8);
      width: min(1100px, 95vw);
      max-height: 90vh;
      display: flex;
      flex-direction: column;
    }

    .rs-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      padding: 14px 20px;
      border-bottom: 1px solid #2d2d6e;
      flex-shrink: 0;
    }

    .rs-title {
      font-size: 15px;
      font-weight: bold;
      color: #e0e0e0;
      letter-spacing: 2px;
      text-transform: uppercase;
    }

    .rs-close {
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
    .rs-close:hover { background: rgba(255,255,255,0.1); color: #fff; }

    .rs-body {
      flex: 1;
      overflow-y: auto;
      overflow-x: auto;
      padding: 16px 20px;
    }
    .rs-body::-webkit-scrollbar { width: 6px; height: 6px; }
    .rs-body::-webkit-scrollbar-track { background: rgba(0,0,0,0.2); }
    .rs-body::-webkit-scrollbar-thumb { background: rgba(255,255,255,0.15); border-radius: 3px; }

    .rs-columns {
      display: flex;
      gap: 12px;
      min-width: max-content;
    }

    .rs-column {
      min-width: 130px;
      flex: 1;
    }

    .rs-column-header {
      font-size: 11px;
      font-weight: bold;
      color: #999;
      text-transform: uppercase;
      letter-spacing: 1.5px;
      text-align: center;
      margin-bottom: 10px;
      padding-bottom: 8px;
      border-bottom: 1px solid #2a2a4e;
      white-space: nowrap;
    }

    .rs-column-icon {
      margin-right: 4px;
    }

    .rs-tile {
      background: rgba(255,255,255,0.03);
      border: 2px solid #2a2a4e;
      border-radius: 8px;
      padding: 10px 8px;
      margin-bottom: 8px;
      text-align: center;
      transition: background 0.15s, border-color 0.15s, opacity 0.15s;
      position: relative;
    }

    /* Equipped state */
    .rs-tile.equipped {
      border-color: #22c55e;
      background: rgba(34, 197, 94, 0.1);
      box-shadow: 0 0 8px rgba(34, 197, 94, 0.3);
    }

    /* Unlocked + equippable */
    .rs-tile.equippable {
      cursor: pointer;
      border-color: #3d3d7e;
      background: rgba(255,255,255,0.05);
    }
    .rs-tile.equippable:hover {
      border-color: #6666cc;
      background: rgba(255,255,255,0.1);
    }

    /* Locked */
    .rs-tile.locked {
      opacity: 0.4;
      cursor: default;
    }
    .rs-tile.locked:hover {
      opacity: 0.55;
    }

    .rs-tile-preview {
      height: 28px;
      display: flex;
      align-items: center;
      justify-content: center;
      margin-bottom: 6px;
    }

    .rs-paint-swatch {
      width: 36px;
      height: 20px;
      border-radius: 4px;
      border: 1px solid rgba(255,255,255,0.2);
    }

    .rs-tile-name {
      font-size: 10px;
      font-weight: bold;
      color: #ccc;
      line-height: 1.3;
      margin-bottom: 2px;
    }
    .rs-tile.locked .rs-tile-name { color: #666; }

    .rs-tile-status {
      font-size: 9px;
      font-weight: bold;
      text-transform: uppercase;
      letter-spacing: 0.5px;
      padding: 2px 6px;
      border-radius: 3px;
      display: inline-block;
      margin-top: 4px;
    }
    .rs-tile-status.equipped {
      color: #22c55e;
      background: rgba(34, 197, 94, 0.15);
    }
    .rs-tile-status.equip {
      color: #8888dd;
      background: rgba(88, 88, 200, 0.15);
    }
    .rs-tile-status.locked {
      color: #666;
      background: rgba(100, 100, 100, 0.15);
    }

    .rs-tile-lock-text {
      font-size: 9px;
      color: #555;
      margin-top: 3px;
      line-height: 1.3;
    }

    .rs-footer {
      padding: 10px 20px;
      border-top: 1px solid #2d2d6e;
      font-size: 11px;
      color: #555;
      text-align: center;
      flex-shrink: 0;
    }

    .rs-equipping {
      pointer-events: none;
      opacity: 0.7;
    }
  `;
  document.head.appendChild(style);
}

// ── DOM ──────────────────────────────────────────────────────────────

function buildDOM() {
  const overlay = document.createElement('div');
  overlay.id = 'reward-selector';
  overlay.className = 'reward-selector hidden';
  overlay.setAttribute('role', 'dialog');
  overlay.setAttribute('aria-label', 'Garage — Reward Selector');
  overlay.setAttribute('aria-modal', 'true');

  overlay.innerHTML = `
    <div class="rs-inner">
      <div class="rs-header">
        <span class="rs-title">Garage</span>
        <button class="rs-close" aria-label="Close garage">\u00D7</button>
      </div>
      <div class="rs-body">
        <div class="rs-columns"></div>
      </div>
      <div class="rs-footer">Click an unlocked reward to equip it</div>
    </div>
  `;

  document.body.appendChild(overlay);
  return overlay;
}

// ── RewardSelector ───────────────────────────────────────────────────

export class RewardSelector {
  constructor() {
    injectStyles();
    this._overlay = buildDOM();
    this._columns = this._overlay.querySelector('.rs-columns');
    this._visible = false;
    this._achievements = [];      // from /api/achievements
    this._battlePassTier = 0;     // from /api/stats
    this._equipping = false;      // guard against double-click

    this._overlay.querySelector('.rs-close').addEventListener('click', () => this.hide());
    this._overlay.addEventListener('click', (e) => {
      if (e.target === this._overlay) this.hide();
    });

    // Re-render when cosmetic state changes externally (WebSocket broadcast)
    this._unsubscribe = onEquippedChange(() => {
      if (this._visible) this._render();
    });
  }

  async _fetchData() {
    try {
      const [achResp, statsResp] = await Promise.all([
        authFetch('/api/achievements'),
        authFetch('/api/stats'),
      ]);
      if (!achResp.ok) throw new Error(`achievements: HTTP ${achResp.status}`);
      this._achievements = await achResp.json();
      if (statsResp.ok) {
        const stats = await statsResp.json();
        this._battlePassTier = stats.battlePass?.tier ?? 0;
      }
    } catch (err) {
      this._achievements = [];
      this._columns.innerHTML = `<p style="color:#e94560;font-size:12px">Failed to load: ${escapeHTML(err.message)}</p>`;
    }
  }

  show() {
    if (this._visible) return;
    this._visible = true;
    this._overlay.classList.remove('hidden');
    this._fetchData().then(() => this._render());
    this._overlay.querySelector('.rs-close').focus();
  }

  hide() {
    if (!this._visible) return;
    this._visible = false;
    this._overlay.classList.add('hidden');
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

  // ── Rendering ────────────────────────────────────────────────────

  _render() {
    const loadout = getEquippedLoadout();
    const unlockedSet = new Set(
      this._achievements.filter(a => a.unlocked).map(a => a.id),
    );
    // Map achievement ID → name for lock requirement text
    const achievementNames = new Map(
      this._achievements.map(a => [a.id, a.name]),
    );

    const fragment = document.createDocumentFragment();

    for (const slot of SLOTS) {
      const col = document.createElement('div');
      col.className = 'rs-column';

      const header = document.createElement('div');
      header.className = 'rs-column-header';
      header.innerHTML = `<span class="rs-column-icon">${slot.icon}</span>${escapeHTML(slot.label)}`;
      col.appendChild(header);

      // "None" tile — unequip
      col.appendChild(this._buildNoneTile(slot.key, loadout[slot.key]));

      const slotRewards = REWARDS.filter(r => r.type === slot.key);
      for (const reward of slotRewards) {
        const isEquipped = loadout[slot.key] === reward.id;
        let isUnlocked;
        if (reward.unlockedBy === '') {
          // Battle pass reward: check tier
          const requiredTier = BATTLE_PASS_TIERS[reward.id] ?? Infinity;
          isUnlocked = this._battlePassTier >= requiredTier;
        } else {
          isUnlocked = unlockedSet.has(reward.unlockedBy);
        }

        col.appendChild(
          this._buildTile(reward, slot.key, isEquipped, isUnlocked, achievementNames),
        );
      }

      fragment.appendChild(col);
    }

    this._columns.innerHTML = '';
    this._columns.appendChild(fragment);
  }

  _buildNoneTile(slotKey, currentlyEquipped) {
    const tile = document.createElement('div');
    const isActive = !currentlyEquipped;
    tile.className = `rs-tile ${isActive ? 'equipped' : 'equippable'}`;

    const preview = document.createElement('div');
    preview.className = 'rs-tile-preview';
    preview.textContent = '\u2014'; // em dash
    preview.style.color = '#555';
    tile.appendChild(preview);

    const name = document.createElement('div');
    name.className = 'rs-tile-name';
    name.textContent = 'None';
    tile.appendChild(name);

    const status = document.createElement('div');
    status.className = `rs-tile-status ${isActive ? 'equipped' : 'equip'}`;
    status.textContent = isActive ? 'Active' : 'Clear';
    tile.appendChild(status);

    if (!isActive) {
      tile.addEventListener('click', () => this._doUnequip(slotKey));
    }

    return tile;
  }

  _buildTile(reward, slotKey, isEquipped, isUnlocked, achievementNames) {
    const tile = document.createElement('div');

    if (isEquipped) {
      tile.className = 'rs-tile equipped';
    } else if (isUnlocked) {
      tile.className = 'rs-tile equippable';
    } else {
      tile.className = 'rs-tile locked';
    }

    const preview = document.createElement('div');
    preview.className = 'rs-tile-preview';
    this._renderPreview(preview, reward);
    tile.appendChild(preview);

    const nameEl = document.createElement('div');
    nameEl.className = 'rs-tile-name';
    nameEl.textContent = reward.name;
    tile.appendChild(nameEl);

    const status = document.createElement('div');
    if (isEquipped) {
      status.className = 'rs-tile-status equipped';
      status.textContent = 'Equipped';
    } else if (isUnlocked) {
      status.className = 'rs-tile-status equip';
      status.textContent = 'Equip';
    } else {
      status.className = 'rs-tile-status locked';
      status.textContent = '\u{1F512} Locked';
    }
    tile.appendChild(status);

    // Lock requirement text
    if (!isUnlocked) {
      const lockText = document.createElement('div');
      lockText.className = 'rs-tile-lock-text';
      if (reward.unlockedBy) {
        const achName = achievementNames.get(reward.unlockedBy);
        lockText.textContent = `Requires: ${achName || reward.unlockedBy}`;
      } else {
        const tier = BATTLE_PASS_TIERS[reward.id];
        lockText.textContent = tier ? `Battle Pass Tier ${tier}` : 'Battle Pass reward';
      }
      tile.appendChild(lockText);
    }

    if (isUnlocked && !isEquipped) {
      tile.addEventListener('click', () => this._doEquip(reward.id, slotKey));
    }

    return tile;
  }

  _renderPreview(container, reward) {
    const slot = SLOTS.find(s => s.key === reward.type);

    if (reward.type === 'paint') {
      const paint = getEquippedPaint(reward.id);
      if (paint) {
        const swatch = document.createElement('div');
        swatch.className = 'rs-paint-swatch';
        swatch.style.background = paint.fill;
        swatch.style.borderColor = paint.stroke;
        container.appendChild(swatch);
      } else {
        container.textContent = slot?.icon ?? '';
      }
      return;
    }

    if (reward.type === 'badge') {
      const badge = getEquippedBadge(reward.id);
      container.textContent = badge ? badge.emoji : (slot?.icon ?? '');
      container.style.fontSize = '18px';
      return;
    }

    if (reward.type === 'title') {
      container.textContent = `"${reward.name}"`;
      container.style.fontSize = '10px';
      container.style.color = '#aaa';
      container.style.fontStyle = 'italic';
      return;
    }

    // trail, body, sound, theme: use the slot icon at 18px
    container.textContent = slot?.icon ?? '';
    container.style.fontSize = '18px';
  }

  // ── Equip / Unequip ──────────────────────────────────────────────

  async _doEquip(rewardId, slot) {
    if (this._equipping) return;
    this._equipping = true;
    this._columns.classList.add('rs-equipping');

    try {
      const resp = await authFetch('/api/equip', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ rewardId, slot }),
      });
      if (!resp.ok) {
        const text = await resp.text();
        console.error('Equip failed:', text);
        return;
      }
      const loadout = await resp.json();
      setEquipped(loadout);
    } catch (err) {
      console.error('Equip error:', err);
    } finally {
      this._equipping = false;
      this._columns.classList.remove('rs-equipping');
    }
  }

  _doUnequip(slot) {
    if (this._equipping) return;
    // Update locally — the backend persists on the next equip call or page reload
    setEquipped({ [slot]: '' });
  }

  destroy() {
    this._unsubscribe();
    this._overlay.remove();
  }
}
