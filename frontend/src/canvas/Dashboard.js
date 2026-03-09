import { getEquippedBadge, getEquippedTitle } from '../gamification/CosmeticRegistry.js';
import { TERMINAL_ACTIVITIES } from '../session/constants.js';
import { TeamGarage } from './TeamGarage.js';

const DASHBOARD_GAP = 30;
const DASHBOARD_PADDING = { top: 20, bottom: 20, left: 20, right: 20 };
const STATS_HEIGHT = 40;
const TAB_BAR_HEIGHT = 28;
const LEADERBOARD_HEADER_HEIGHT = 28;
const LEADERBOARD_ROW_HEIGHT = 26;
const LEADERBOARD_BAR_HEIGHT = 12;
const LEADERBOARD_BAR_MIN_FILL_WIDTH = 3;
const SECTION_GAP = 16;
const MAX_LEADERBOARD_ROWS = 12;
const MIN_DASHBOARD_HEIGHT = 160;

const ACTIVITY_COLORS = {
  thinking: '#2563eb',
  tool_use: '#d97706',
  waiting: '#854d0e',
  idle: '#4b5563',
  starting: '#7c3aed',
  complete: '#16a34a',
  errored: '#dc2626',
  lost: '#374151',
};

const COMPLETED_ROW_BG = 'rgba(34,197,94,0.12)';
const COMPLETED_ROW_BORDER = 'rgba(134,239,172,0.32)';

function formatTokens(tokens) {
  if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(1)}M`;
  if (tokens >= 1000) return `${Math.round(tokens / 1000)}K`;
  return `${tokens}`;
}

function shortName(name, maxLen = 24) {
  if (!name) return '(unnamed)';
  if (name.length <= maxLen) return name;
  return name.slice(0, maxLen - 1) + '\u2026';
}

function shortModel(model) {
  if (!model) return '?';
  // Extract meaningful part: "claude-opus-4-5-..." -> "opus-4-5"
  const parts = model.split('-');
  if (parts[0] === 'claude' && parts.length >= 3) {
    return parts.slice(1, 3).join('-');
  }
  return model.slice(0, 12);
}

function elapsed(startStr) {
  if (!startStr) return '';
  const ms = Date.now() - new Date(startStr).getTime();
  const mins = Math.floor(ms / 60000);
  if (mins < 60) return `${mins}m`;
  const hrs = Math.floor(mins / 60);
  return `${hrs}h${mins % 60}m`;
}

export class Dashboard {
  constructor() {
    this._activeTab = 'sessions';
    this._teamGarage = new TeamGarage();
    // Stores bounds of tab buttons for click detection: [{label, x, y, w, h}]
    this._tabButtons = [];
  }

  getRequiredHeight(sessionCount) {
    const rows = Math.min(Math.max(sessionCount, 0), MAX_LEADERBOARD_ROWS);
    const leaderboardHeight = rows > 0
      ? LEADERBOARD_HEADER_HEIGHT + rows * LEADERBOARD_ROW_HEIGHT
      : 0;
    return DASHBOARD_GAP + DASHBOARD_PADDING.top + STATS_HEIGHT + SECTION_GAP
      + TAB_BAR_HEIGHT + SECTION_GAP + leaderboardHeight + DASHBOARD_PADDING.bottom;
  }

  getMinHeight() {
    return MIN_DASHBOARD_HEIGHT;
  }

  getBounds(canvasWidth, dashboardTop, availableHeight) {
    const width = canvasWidth - DASHBOARD_PADDING.left - DASHBOARD_PADDING.right;
    return {
      x: DASHBOARD_PADDING.left,
      y: dashboardTop + DASHBOARD_GAP,
      width,
      height: availableHeight - DASHBOARD_GAP,
    };
  }

  /** Returns true if the click was consumed by a tab button. */
  handleClick(mx, my) {
    for (const btn of this._tabButtons) {
      if (mx >= btn.x && mx <= btn.x + btn.w && my >= btn.y && my <= btn.y + btn.h) {
        this._activeTab = btn.label;
        return true;
      }
    }
    return false;
  }

  draw(ctx, bounds, sessions, zoneCounts, teams) {
    if (!bounds || bounds.height < 40) return;

    const { x, y, width, height } = bounds;

    // Subtle separator line
    ctx.strokeStyle = 'rgba(255,255,255,0.06)';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(x, y);
    ctx.lineTo(x + width, y);
    ctx.stroke();

    // Aggregate stats
    const statsY = y + DASHBOARD_PADDING.top;
    this._drawStats(ctx, x, statsY, width, sessions, zoneCounts);

    // Tab bar
    const tabY = statsY + STATS_HEIGHT + SECTION_GAP;
    this._drawTabBar(ctx, x, tabY, width, sessions, teams || []);

    // Content below tabs
    const contentY = tabY + TAB_BAR_HEIGHT + SECTION_GAP;
    const contentHeight = height - DASHBOARD_PADDING.top - STATS_HEIGHT - SECTION_GAP
      - TAB_BAR_HEIGHT - SECTION_GAP - DASHBOARD_PADDING.bottom;

    if (this._activeTab === 'sessions') {
      if (sessions.length > 0) {
        this._drawLeaderboard(ctx, x, contentY, width, contentHeight, sessions);
      }
    } else {
      this._teamGarage.draw(ctx, x, contentY, width, contentHeight, teams || []);
    }
  }

  _drawTabBar(ctx, x, y, width, sessions, teams) {
    const tabs = [
      { label: 'sessions', text: `SESSIONS (${sessions.length})` },
      { label: 'teams', text: `TEAMS (${teams.length})` },
    ];
    const tabW = 120;
    const tabH = TAB_BAR_HEIGHT - 4;
    const tabGap = 6;

    this._tabButtons = [];

    for (let i = 0; i < tabs.length; i++) {
      const tab = tabs[i];
      const tx = x + i * (tabW + tabGap);
      const ty = y + 2;
      const active = this._activeTab === tab.label;

      // Button background
      ctx.fillStyle = active ? 'rgba(255,255,255,0.12)' : 'rgba(255,255,255,0.04)';
      ctx.beginPath();
      ctx.roundRect(tx, ty, tabW, tabH, 3);
      ctx.fill();

      // Active indicator line at bottom
      if (active) {
        ctx.fillStyle = '#3b82f6';
        ctx.fillRect(tx + 4, ty + tabH - 2, tabW - 8, 2);
      }

      // Label
      ctx.fillStyle = active ? '#e0e0e0' : '#555';
      ctx.font = active ? 'bold 10px Courier New' : '10px Courier New';
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText(tab.text, tx + tabW / 2, ty + tabH / 2 - 1);

      this._tabButtons.push({ label: tab.label, x: tx, y: ty, w: tabW, h: tabH });
    }

    ctx.textBaseline = 'alphabetic';
    ctx.textAlign = 'left';
  }

  _drawStats(ctx, x, y, width, sessions, zoneCounts) {
    const active = zoneCounts?.racing ?? 0;
    const idle = zoneCounts?.pit ?? 0;
    const done = zoneCounts?.parked ?? 0;
    const totalTokens = sessions.reduce((sum, s) => sum + (s.tokensUsed || 0), 0);
    const totalTools = sessions.reduce((sum, s) => sum + (s.toolCallCount || 0), 0);
    const totalMessages = sessions.reduce((sum, s) => sum + (s.messageCount || 0), 0);

    const stats = [
      { label: 'RACING', value: `${active}`, color: '#2563eb' },
      { label: 'PIT', value: `${idle}`, color: '#854d0e' },
      { label: 'PARKED', value: `${done}`, color: '#4b5563' },
      { label: 'TOKENS', value: formatTokens(totalTokens), color: '#a855f7' },
      { label: 'TOOLS', value: `${totalTools}`, color: '#d97706' },
      { label: 'MSGS', value: `${totalMessages}`, color: '#3b82f6' },
    ];

    const cellWidth = Math.min(width / stats.length, 140);
    const totalStatsWidth = cellWidth * stats.length;
    const startX = x + (width - totalStatsWidth) / 2;

    for (let i = 0; i < stats.length; i++) {
      const stat = stats[i];
      const cx = startX + i * cellWidth + cellWidth / 2;

      // Value
      ctx.fillStyle = stat.color;
      ctx.font = 'bold 18px Courier New';
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText(stat.value, cx, y + 10);

      // Label
      ctx.fillStyle = '#555';
      ctx.font = '10px Courier New';
      ctx.fillText(stat.label, cx, y + 30);
    }

    ctx.textBaseline = 'alphabetic';
  }

  _drawLeaderboard(ctx, x, y, width, maxHeight, sessions) {
    // Sort by context utilization descending (leaders first)
    const sorted = [...sessions].sort((a, b) => (b.contextUtilization || 0) - (a.contextUtilization || 0));
    const rows = sorted.slice(0, MAX_LEADERBOARD_ROWS);

    // Column layout
    const cols = {
      rank: x + 20,
      badge: x + 46,
      name: x + 68,
      model: x + width * 0.38,
      tokens: x + width * 0.52,
      bar: x + width * 0.64,
      barWidth: width * 0.22,
      pct: x + width * 0.88,
      elapsed: x + width - 20,
    };

    // Header
    ctx.fillStyle = '#444';
    ctx.font = 'bold 10px Courier New';
    ctx.textAlign = 'left';
    ctx.textBaseline = 'middle';
    ctx.fillText('#', cols.rank, y + LEADERBOARD_HEADER_HEIGHT / 2);
    ctx.fillText('SESSION', cols.name, y + LEADERBOARD_HEADER_HEIGHT / 2);
    ctx.fillText('MODEL', cols.model, y + LEADERBOARD_HEADER_HEIGHT / 2);
    // Badge header intentionally blank — emoji column is self-explanatory
    ctx.fillText('TOKENS', cols.tokens, y + LEADERBOARD_HEADER_HEIGHT / 2);
    ctx.fillText('CONTEXT', cols.bar, y + LEADERBOARD_HEADER_HEIGHT / 2);
    ctx.textAlign = 'right';
    ctx.fillText('TIME', cols.elapsed, y + LEADERBOARD_HEADER_HEIGHT / 2);
    ctx.textAlign = 'left';

    // Header underline
    ctx.strokeStyle = 'rgba(255,255,255,0.05)';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(x + 10, y + LEADERBOARD_HEADER_HEIGHT);
    ctx.lineTo(x + width - 10, y + LEADERBOARD_HEADER_HEIGHT);
    ctx.stroke();

    // Resolve cosmetics once (global, not per-session)
    const badge = getEquippedBadge();
    const title = getEquippedTitle();

    // Rows
    const rowsStartY = y + LEADERBOARD_HEADER_HEIGHT;
    for (let i = 0; i < rows.length; i++) {
      const rowY = rowsStartY + i * LEADERBOARD_ROW_HEIGHT + LEADERBOARD_ROW_HEIGHT / 2;
      if (rowY - y > maxHeight) break;
      this._drawLeaderboardRow(ctx, cols, rowY, i + 1, rows[i], badge, title);
    }

    ctx.textBaseline = 'alphabetic';
  }

  _drawLeaderboardRow(ctx, cols, cy, rank, session, badge, title) {
    const util = Math.max(0, Math.min(session.contextUtilization || 0, 1));
    const pct = (util * 100).toFixed(0);
    const isComplete = session.activity === 'complete';
    const isTerminal = TERMINAL_ACTIVITIES.has(session.activity);
    const alpha = isComplete ? 0.95 : isTerminal ? 0.4 : 0.85;

    if (isComplete) {
      const rowLeft = cols.rank - 8;
      const rowTop = Math.round(cy - LEADERBOARD_ROW_HEIGHT / 2 + 2);
      const rowWidth = cols.elapsed - rowLeft + 8;
      const rowHeight = LEADERBOARD_ROW_HEIGHT - 4;
      ctx.fillStyle = COMPLETED_ROW_BG;
      ctx.fillRect(rowLeft, rowTop, rowWidth, rowHeight);
      ctx.strokeStyle = COMPLETED_ROW_BORDER;
      ctx.lineWidth = 1;
      ctx.strokeRect(rowLeft + 0.5, rowTop + 0.5, rowWidth - 1, rowHeight - 1);
    }

    ctx.globalAlpha = alpha;

    // Rank with position delta arrow
    ctx.fillStyle = rank <= 3 ? '#d4a017' : '#555';
    ctx.font = rank <= 3 ? 'bold 12px Courier New' : '12px Courier New';
    ctx.textAlign = 'right';
    ctx.textBaseline = 'middle';
    ctx.fillText(`${rank}`, cols.rank + 16, cy);

    // Position delta indicator (↑ green / ↓ red)
    const delta = session.positionDelta || 0;
    if (delta !== 0 && !isTerminal) {
      ctx.font = '10px sans-serif';
      ctx.fillStyle = delta > 0 ? '#22c55e' : '#e94560';
      ctx.textAlign = 'left';
      ctx.fillText(delta > 0 ? '\u2191' : '\u2193', cols.rank + 18, cy);
    }

    // Badge
    if (badge) {
      ctx.font = '13px sans-serif';
      ctx.textAlign = 'center';
      ctx.fillText(badge.emoji, cols.badge, cy);
    }

    // Activity dot
    if (isComplete) {
      this._drawCompletedFlagIcon(ctx, cols.name - 8, cy);
    } else {
      const dotColor = ACTIVITY_COLORS[session.activity] || '#4b5563';
      ctx.beginPath();
      ctx.arc(cols.name - 8, cy, 3, 0, Math.PI * 2);
      ctx.fillStyle = dotColor;
      ctx.fill();
    }

    // Session name (with optional title prefix)
    ctx.font = '12px Courier New';
    ctx.textAlign = 'left';
    if (title) {
      ctx.fillStyle = '#d4a017';
      ctx.fillText(title, cols.name, cy);
      const titleWidth = ctx.measureText(title + ' ').width;
      ctx.fillStyle = isComplete ? '#dcfce7' : '#bbb';
      ctx.fillText(shortName(session.name, 18), cols.name + titleWidth, cy);
    } else {
      ctx.fillStyle = isComplete ? '#dcfce7' : '#bbb';
      ctx.fillText(shortName(session.name), cols.name, cy);
    }

    // Model
    ctx.fillStyle = isComplete ? '#86efac' : '#666';
    ctx.font = '11px Courier New';
    ctx.fillText(shortModel(session.model), cols.model, cy);

    // Tokens
    ctx.fillStyle = isComplete ? '#bbf7d0' : '#888';
    ctx.font = '11px Courier New';
    ctx.fillText(formatTokens(session.tokensUsed || 0), cols.tokens, cy);

    // Utilization bar
    const barX = Math.round(cols.bar);
    const barY = Math.round(cy - LEADERBOARD_BAR_HEIGHT / 2);
    const barWidth = Math.max(0, Math.round(cols.barWidth));
    const innerX = barX + 1;
    const innerY = barY + 1;
    const innerWidth = Math.max(0, barWidth - 2);
    const innerHeight = Math.max(0, LEADERBOARD_BAR_HEIGHT - 2);

    // Bar background
    ctx.fillStyle = 'rgba(255,255,255,0.06)';
    ctx.fillRect(barX, barY, barWidth, LEADERBOARD_BAR_HEIGHT);
    ctx.strokeStyle = 'rgba(255,255,255,0.18)';
    ctx.lineWidth = 1;
    ctx.strokeRect(barX + 0.5, barY + 0.5, Math.max(0, barWidth - 1), LEADERBOARD_BAR_HEIGHT - 1);

    // Bar fill — red above 80%, amber above 50%, green otherwise
    let barColor = '#22c55e';
    if (isComplete) barColor = '#16a34a';
    else if (util > 0.8) barColor = '#e94560';
    else if (util > 0.5) barColor = '#d97706';
    if (util > 0 && innerWidth > 0 && innerHeight > 0) {
      const fillWidth = Math.min(
        innerWidth,
        Math.max(Math.round(innerWidth * util), LEADERBOARD_BAR_MIN_FILL_WIDTH),
      );
      ctx.fillStyle = barColor;
      ctx.fillRect(innerX, innerY, fillWidth, innerHeight);
    }

    // Percentage
    ctx.fillStyle = isComplete ? '#dcfce7' : '#777';
    ctx.font = '11px Courier New';
    ctx.fillText(isComplete ? 'DONE' : `${pct}%`, cols.pct, cy);

    // Elapsed time
    ctx.fillStyle = isComplete ? '#86efac' : '#555';
    ctx.font = '11px Courier New';
    ctx.textAlign = 'right';
    ctx.fillText(elapsed(session.startedAt), cols.elapsed, cy);

    ctx.textAlign = 'left';
    ctx.globalAlpha = 1.0;
  }

  _drawCompletedFlagIcon(ctx, x, y) {
    const poleX = x - 4;
    const poleTop = y - 6;
    ctx.strokeStyle = '#86efac';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(poleX, y + 5);
    ctx.lineTo(poleX, poleTop);
    ctx.stroke();

    const size = 2.5;
    for (let row = 0; row < 3; row++) {
      for (let col = 0; col < 4; col++) {
        ctx.fillStyle = (row + col) % 2 === 0 ? '#f8fafc' : '#111827';
        ctx.fillRect(poleX + col * size, poleTop + row * size, size, size);
      }
    }
  }
}
