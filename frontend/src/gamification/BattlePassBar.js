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
const NEAR_TIER_UP_THRESHOLD = 0.9;

const CONFETTI_COUNT = 30;
const CONFETTI_CANVAS_HEIGHT = 120;
const CONFETTI_GRAVITY = 0.08;
const CONFETTI_COLORS = ['#f59e0b', '#fbbf24', '#e94560', '#f06292', '#4ade80', '#fff'];
const TIER_UP_DURATION_MS = 2000;

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
    this.tierUpTimer = null;

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

    this.toastContainer = document.createElement('div');
    this.toastContainer.className = 'bp-toast-container';

    this.collapsedRow.appendChild(this.seasonLabel);
    this.collapsedRow.appendChild(this.tierBadge);
    this.collapsedRow.appendChild(this.xpBarWrap);
    this.collapsedRow.appendChild(this.toastContainer);

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
    const prevTier = this.state.tier;

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

    if (payload.tier > prevTier) {
      this.playTierUpCelebration();
    }
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
    const toast = document.createElement('span');
    toast.className = 'bp-xp-toast';
    toast.textContent = `+${total} XP`;

    restartAnimation(this.xpBarWrap, 'bp-xp-flash');

    clearTimeout(this.toastTimer);
    this.toastTimer = setTimeout(() => {
      this.xpToast.classList.remove('visible');
    }, 3000);

    this.toastContainer.appendChild(toast);
    toast.addEventListener('animationend', () => toast.remove());
  }

  playTierUpCelebration() {
    clearTimeout(this.tierUpTimer);

    // Gold flash on collapsed bar
    this.collapsedRow.classList.add('bp-tier-up-flash');

    // Briefly expand to show new tier
    const wasExpanded = this.expanded;
    if (!wasExpanded) {
      this.expanded = true;
      this.expandedPanel.classList.remove('hidden');
      this.renderExpanded();
    }

    // Confetti burst from tier badge
    this.spawnConfetti();

    this.tierUpTimer = setTimeout(() => {
      this.collapsedRow.classList.remove('bp-tier-up-flash');
      if (!wasExpanded) {
        this.expanded = false;
        this.expandedPanel.classList.add('hidden');
      }
    }, TIER_UP_DURATION_MS);
  }

  spawnConfetti() {
    const badgeRect = this.tierBadge.getBoundingClientRect();
    const containerRect = this.container.getBoundingClientRect();

    const canvas = document.createElement('canvas');
    canvas.className = 'bp-confetti-canvas';
    canvas.width = containerRect.width;
    canvas.height = CONFETTI_CANVAS_HEIGHT;
    canvas.style.left = '0';
    canvas.style.top = `${badgeRect.bottom - containerRect.top}px`;
    this.container.appendChild(canvas);

    const ctx = canvas.getContext('2d');
    const originX = badgeRect.left - containerRect.left + badgeRect.width / 2;
    const particles = Array.from({ length: CONFETTI_COUNT }, () => {
      const angle = Math.random() * Math.PI * 0.8 + Math.PI * 0.1;
      const speed = 1.5 + Math.random() * 3;
      const direction = Math.random() > 0.5 ? 1 : -1;
      return {
        x: originX,
        y: 0,
        vx: Math.cos(angle) * speed * direction,
        vy: Math.sin(angle) * speed,
        size: 2 + Math.random() * 3,
        color: CONFETTI_COLORS[Math.floor(Math.random() * CONFETTI_COLORS.length)],
        life: 1,
        decay: 0.015 + Math.random() * 0.01,
        rotation: Math.random() * Math.PI * 2,
        rotSpeed: (Math.random() - 0.5) * 0.2,
      };
    });

    const animate = () => {
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      let alive = false;

      for (const p of particles) {
        if (p.life <= 0) continue;
        alive = true;

        p.x += p.vx;
        p.y += p.vy;
        p.vy += CONFETTI_GRAVITY;
        p.life -= p.decay;
        p.rotation += p.rotSpeed;

        ctx.save();
        ctx.translate(p.x, p.y);
        ctx.rotate(p.rotation);
        ctx.globalAlpha = Math.max(0, p.life);
        ctx.fillStyle = p.color;
        ctx.fillRect(-p.size / 2, -p.size / 2, p.size, p.size * 0.6);
        ctx.restore();
      }

      if (alive) {
        requestAnimationFrame(animate);
      } else {
        canvas.remove();
      }
    };
    requestAnimationFrame(animate);
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

    const nearTierUp = !isMaxTier && tierProgress > NEAR_TIER_UP_THRESHOLD;
    this.xpBarWrap.classList.toggle('bp-near-tier-up', nearTierUp);

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

/** Remove and re-add a CSS class, forcing a reflow to restart the animation. */
function restartAnimation(el, className) {
  el.classList.remove(className);
  void el.offsetWidth;
  el.classList.add(className);
}
