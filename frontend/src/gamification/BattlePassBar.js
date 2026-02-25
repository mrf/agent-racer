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
      empty.className = 'bp-empty-message';
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
      empty.className = 'bp-empty-message bp-empty-message--small';
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
