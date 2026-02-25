import { authFetch } from '../auth.js';

const TIER_REWARDS = {
  1: [],
  2: ['Bronze Badge'],
  3: ['Spark Trail'],
  4: ['Rev Sound'],
  5: ['Metallic Paint'],
  6: ['Silver Badge'],
  7: ['Flame Trail'],
  8: ['Aero Body'],
  9: ['Dark Theme'],
  10: ['Champion Title', 'Gold Badge'],
};

const MAX_TIERS = 10;
const XP_PER_TIER = 1000;
const MAX_XP_LOG_ENTRIES = 20;

function computeTierProgress(tier, xp) {
  if (tier >= MAX_TIERS) return 1;
  const tierXP = xp - (tier - 1) * XP_PER_TIER;
  return Math.max(0, Math.min(tierXP / XP_PER_TIER, 1));
}

export class BattlePassBar {
  constructor(container) {
    this.container = container;
    this.state = { xp: 0, tier: 1, tierProgress: 0, recentXP: [], rewards: [] };
    this.challenges = [];
    this.xpLog = [];
    this.expanded = false;
    this.toastTimer = null;

    this.buildDOM();
    this.injectStyles();
    this.loadInitialData();
  }

  buildDOM() {
    this.container.innerHTML = '';
    this.container.className = 'bp-bar';

    // Collapsed row
    this.collapsedRow = document.createElement('div');
    this.collapsedRow.className = 'bp-collapsed';
    this.collapsedRow.addEventListener('click', () => this.toggleExpanded());

    this.seasonLabel = document.createElement('span');
    this.seasonLabel.className = 'bp-season';

    this.tierBadge = document.createElement('span');
    this.tierBadge.className = 'bp-tier-badge';

    this.xpBarWrap = document.createElement('div');
    this.xpBarWrap.className = 'bp-xp-bar-wrap';

    this.xpBarFill = document.createElement('div');
    this.xpBarFill.className = 'bp-xp-bar-fill';

    this.xpBarLabel = document.createElement('span');
    this.xpBarLabel.className = 'bp-xp-bar-label';

    this.xpBarWrap.appendChild(this.xpBarFill);
    this.xpBarWrap.appendChild(this.xpBarLabel);

    this.xpToast = document.createElement('span');
    this.xpToast.className = 'bp-xp-toast';

    this.collapsedRow.appendChild(this.seasonLabel);
    this.collapsedRow.appendChild(this.tierBadge);
    this.collapsedRow.appendChild(this.xpBarWrap);
    this.collapsedRow.appendChild(this.xpToast);

    // Expanded panel
    this.expandedPanel = document.createElement('div');
    this.expandedPanel.className = 'bp-expanded hidden';

    this.tierTrack = document.createElement('div');
    this.tierTrack.className = 'bp-tier-track';

    this.challengeSection = document.createElement('div');
    this.challengeSection.className = 'bp-challenges';

    this.xpLogSection = document.createElement('div');
    this.xpLogSection.className = 'bp-xp-log';

    this.expandedPanel.appendChild(this.tierTrack);
    this.expandedPanel.appendChild(this.challengeSection);
    this.expandedPanel.appendChild(this.xpLogSection);

    this.container.appendChild(this.collapsedRow);
    this.container.appendChild(this.expandedPanel);
  }

  injectStyles() {
    if (document.getElementById('bp-bar-styles')) return;
    const style = document.createElement('style');
    style.id = 'bp-bar-styles';
    style.textContent = `
      .bp-bar {
        background: #16213e;
        border-bottom: 1px solid #0f3460;
        font-family: 'Courier New', monospace;
        font-size: 12px;
        color: #e0e0e0;
        user-select: none;
      }

      .bp-collapsed {
        display: flex;
        align-items: center;
        padding: 6px 20px;
        gap: 12px;
        cursor: pointer;
        height: 32px;
      }
      .bp-collapsed:hover {
        background: rgba(255,255,255,0.03);
      }

      .bp-season {
        color: #888;
        font-size: 11px;
        text-transform: uppercase;
        letter-spacing: 1px;
        white-space: nowrap;
      }

      .bp-tier-badge {
        background: #e94560;
        color: #fff;
        font-size: 11px;
        font-weight: bold;
        padding: 1px 8px;
        border-radius: 3px;
        white-space: nowrap;
      }
      .bp-tier-badge.tier-max {
        background: #f59e0b;
        color: #1a1a2e;
      }

      .bp-xp-bar-wrap {
        flex: 1;
        max-width: 300px;
        height: 14px;
        background: #222;
        border-radius: 3px;
        position: relative;
        overflow: hidden;
      }

      .bp-xp-bar-fill {
        height: 100%;
        background: linear-gradient(90deg, #e94560, #f06292);
        border-radius: 3px;
        transition: width 0.4s ease;
        width: 0%;
      }
      .bp-xp-bar-fill.tier-max {
        background: linear-gradient(90deg, #f59e0b, #fbbf24);
      }

      .bp-xp-bar-label {
        position: absolute;
        top: 0;
        left: 6px;
        line-height: 14px;
        font-size: 10px;
        color: #fff;
        text-shadow: 0 0 3px #000;
      }

      .bp-xp-toast {
        font-size: 11px;
        font-weight: bold;
        color: #4ade80;
        opacity: 0;
        transition: opacity 0.3s ease;
        white-space: nowrap;
      }
      .bp-xp-toast.visible {
        opacity: 1;
      }

      .bp-expanded {
        padding: 10px 20px 14px;
        border-top: 1px solid #0f3460;
      }
      .bp-expanded.hidden {
        display: none;
      }

      /* Tier track */
      .bp-tier-track {
        display: flex;
        align-items: flex-start;
        gap: 0;
        margin-bottom: 12px;
        overflow-x: auto;
      }

      .bp-tier-node {
        display: flex;
        flex-direction: column;
        align-items: center;
        min-width: 64px;
        position: relative;
      }

      .bp-tier-connector {
        width: 100%;
        height: 2px;
        position: absolute;
        top: 13px;
        left: -50%;
        z-index: 0;
      }

      .bp-tier-dot {
        width: 28px;
        height: 28px;
        border-radius: 50%;
        border: 2px solid #444;
        background: #1a1a2e;
        display: flex;
        align-items: center;
        justify-content: center;
        font-size: 10px;
        font-weight: bold;
        color: #666;
        z-index: 1;
        position: relative;
      }
      .bp-tier-dot.completed {
        background: #e94560;
        border-color: #e94560;
        color: #fff;
      }
      .bp-tier-dot.current {
        border-color: #e94560;
        color: #e94560;
        animation: bp-pulse 1.5s ease-in-out infinite;
      }
      .bp-tier-dot.tier-max-dot {
        background: #f59e0b;
        border-color: #f59e0b;
        color: #1a1a2e;
      }

      @keyframes bp-pulse {
        0%, 100% { box-shadow: 0 0 0 0 rgba(233,69,96,0.4); }
        50% { box-shadow: 0 0 0 6px rgba(233,69,96,0); }
      }

      .bp-tier-reward {
        font-size: 9px;
        color: #666;
        text-align: center;
        margin-top: 4px;
        max-width: 64px;
        line-height: 1.2;
      }
      .bp-tier-reward.unlocked {
        color: #fbbf24;
      }

      /* Challenges */
      .bp-challenges {
        margin-bottom: 10px;
      }
      .bp-challenges-title {
        font-size: 11px;
        color: #888;
        text-transform: uppercase;
        letter-spacing: 1px;
        margin-bottom: 6px;
      }
      .bp-challenge-row {
        display: flex;
        align-items: center;
        gap: 10px;
        padding: 4px 0;
      }
      .bp-challenge-desc {
        flex: 1;
        font-size: 11px;
        color: #ccc;
      }
      .bp-challenge-desc.complete {
        color: #4ade80;
        text-decoration: line-through;
      }
      .bp-challenge-progress {
        font-size: 10px;
        color: #888;
        white-space: nowrap;
      }
      .bp-challenge-bar-wrap {
        width: 60px;
        height: 6px;
        background: #222;
        border-radius: 3px;
        overflow: hidden;
      }
      .bp-challenge-bar-fill {
        height: 100%;
        background: #4ade80;
        border-radius: 3px;
        transition: width 0.3s ease;
      }

      /* XP log */
      .bp-xp-log {
        max-height: 80px;
        overflow-y: auto;
      }
      .bp-xp-log-title {
        font-size: 11px;
        color: #888;
        text-transform: uppercase;
        letter-spacing: 1px;
        margin-bottom: 4px;
      }
      .bp-xp-log-entry {
        font-size: 10px;
        color: #aaa;
        padding: 2px 0;
        display: flex;
        justify-content: space-between;
      }
      .bp-xp-log-entry .xp-amount {
        color: #4ade80;
        font-weight: bold;
      }

      .bp-xp-log::-webkit-scrollbar { width: 4px; }
      .bp-xp-log::-webkit-scrollbar-track { background: transparent; }
      .bp-xp-log::-webkit-scrollbar-thumb { background: rgba(255,255,255,0.15); border-radius: 2px; }
    `;
    document.head.appendChild(style);
  }

  async loadInitialData() {
    try {
      const [statsRes, challengesRes] = await Promise.all([
        authFetch('/api/stats'),
        authFetch('/api/challenges'),
      ]);

      if (statsRes.ok) {
        const stats = await statsRes.json();
        const bp = stats.battlePass || {};
        const tier = Math.max(bp.tier || 1, 1);
        const xp = bp.xp || 0;

        this.state = {
          xp,
          tier,
          tierProgress: computeTierProgress(tier, xp),
          recentXP: [],
          rewards: TIER_REWARDS[tier] || [],
          season: bp.season || '',
        };
      }

      if (challengesRes.ok) {
        this.challenges = await challengesRes.json();
      }
    } catch (err) {
      // Silently fail — bar stays at defaults until first WS message
    }

    this.render();
  }

  onProgress(payload) {
    const recentXP = payload.recentXP || [];

    this.state = {
      ...this.state,
      xp: payload.xp,
      tier: payload.tier,
      tierProgress: payload.tierProgress,
      recentXP,
      rewards: payload.rewards || [],
    };

    for (const entry of recentXP) {
      this.xpLog.unshift(entry);
    }
    if (this.xpLog.length > MAX_XP_LOG_ENTRIES) {
      this.xpLog.length = MAX_XP_LOG_ENTRIES;
    }

    this.render();
    this.showXPToast(recentXP);
    this.refreshChallenges();
  }

  async refreshChallenges() {
    try {
      const res = await authFetch('/api/challenges');
      if (res.ok) {
        this.challenges = await res.json();
        if (this.expanded) {
          this.renderChallenges();
        }
      }
    } catch {
      // ignore
    }
  }

  showXPToast(entries) {
    if (!entries.length) return;

    const total = entries.reduce((sum, e) => sum + e.amount, 0);
    this.xpToast.textContent = `+${total} XP`;
    this.xpToast.classList.add('visible');

    clearTimeout(this.toastTimer);
    this.toastTimer = setTimeout(() => {
      this.xpToast.classList.remove('visible');
    }, 3000);
  }

  toggleExpanded() {
    this.expanded = !this.expanded;
    this.expandedPanel.classList.toggle('hidden', !this.expanded);
    if (this.expanded) {
      this.renderExpanded();
    }
  }

  render() {
    const { tier, tierProgress, xp, season } = this.state;

    this.seasonLabel.textContent = season ? `Season ${season}` : 'Battle Pass';

    this.tierBadge.textContent = `Tier ${tier}`;
    this.tierBadge.classList.toggle('tier-max', tier >= MAX_TIERS);

    const isMaxTier = tier >= MAX_TIERS;
    const pct = isMaxTier ? 100 : Math.round(tierProgress * 100);
    this.xpBarFill.style.width = `${pct}%`;
    this.xpBarFill.classList.toggle('tier-max', isMaxTier);

    if (isMaxTier) {
      this.xpBarLabel.textContent = `${xp} XP — MAX`;
    } else {
      const tierXP = xp - (tier - 1) * XP_PER_TIER;
      this.xpBarLabel.textContent = `${tierXP} / ${XP_PER_TIER} XP`;
    }

    if (this.expanded) {
      this.renderExpanded();
    }
  }

  renderExpanded() {
    this.renderTierTrack();
    this.renderChallenges();
    this.renderXPLog();
  }

  renderTierTrack() {
    this.tierTrack.innerHTML = '';
    const { tier } = this.state;

    for (let t = 1; t <= MAX_TIERS; t++) {
      const node = document.createElement('div');
      node.className = 'bp-tier-node';

      const dot = document.createElement('div');
      dot.className = 'bp-tier-dot';
      dot.textContent = t;

      if (t < tier) {
        dot.classList.add('completed');
      } else if (t === tier) {
        dot.classList.add('current');
        if (tier >= MAX_TIERS) dot.classList.add('tier-max-dot');
      }

      const reward = document.createElement('div');
      reward.className = 'bp-tier-reward';
      if (t < tier) reward.classList.add('unlocked');
      const rewards = TIER_REWARDS[t] || [];
      reward.textContent = rewards.length ? rewards.join(', ') : '';

      node.appendChild(dot);
      node.appendChild(reward);
      this.tierTrack.appendChild(node);
    }
  }

  renderChallenges() {
    this.challengeSection.innerHTML = '';

    const title = document.createElement('div');
    title.className = 'bp-challenges-title';
    title.textContent = 'Weekly Challenges';
    this.challengeSection.appendChild(title);

    if (!this.challenges.length) {
      const empty = document.createElement('div');
      empty.style.cssText = 'font-size: 11px; color: #666; padding: 4px 0;';
      empty.textContent = 'No active challenges';
      this.challengeSection.appendChild(empty);
      return;
    }

    for (const c of this.challenges) {
      const row = document.createElement('div');
      row.className = 'bp-challenge-row';

      const desc = document.createElement('span');
      desc.className = 'bp-challenge-desc';
      if (c.complete) desc.classList.add('complete');
      desc.textContent = c.description;

      const progress = document.createElement('span');
      progress.className = 'bp-challenge-progress';
      progress.textContent = `${Math.min(c.current, c.target)}/${c.target}`;

      const barWrap = document.createElement('div');
      barWrap.className = 'bp-challenge-bar-wrap';

      const barFill = document.createElement('div');
      barFill.className = 'bp-challenge-bar-fill';
      const pct = Math.min(100, Math.round((c.current / c.target) * 100));
      barFill.style.width = `${pct}%`;

      barWrap.appendChild(barFill);
      row.appendChild(desc);
      row.appendChild(progress);
      row.appendChild(barWrap);
      this.challengeSection.appendChild(row);
    }
  }

  renderXPLog() {
    this.xpLogSection.innerHTML = '';

    const title = document.createElement('div');
    title.className = 'bp-xp-log-title';
    title.textContent = 'Recent XP';
    this.xpLogSection.appendChild(title);

    if (!this.xpLog.length) {
      const empty = document.createElement('div');
      empty.style.cssText = 'font-size: 10px; color: #666; padding: 2px 0;';
      empty.textContent = 'No XP awarded yet';
      this.xpLogSection.appendChild(empty);
      return;
    }

    for (const entry of this.xpLog) {
      const row = document.createElement('div');
      row.className = 'bp-xp-log-entry';

      const reason = document.createElement('span');
      reason.textContent = formatReason(entry.reason);

      const amount = document.createElement('span');
      amount.className = 'xp-amount';
      amount.textContent = `+${entry.amount}`;

      row.appendChild(reason);
      row.appendChild(amount);
      this.xpLogSection.appendChild(row);
    }
  }
}

function formatReason(reason) {
  return reason.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
}
