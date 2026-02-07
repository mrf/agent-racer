const DASHBOARD_GAP = 30;
const DASHBOARD_PADDING = { top: 20, bottom: 20, left: 20, right: 20 };
const STATS_HEIGHT = 40;
const LEADERBOARD_HEADER_HEIGHT = 28;
const LEADERBOARD_ROW_HEIGHT = 26;
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

const TERMINAL_ACTIVITIES = new Set(['complete', 'errored', 'lost']);

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
  getRequiredHeight(sessionCount) {
    const rows = Math.min(Math.max(sessionCount, 0), MAX_LEADERBOARD_ROWS);
    const leaderboardHeight = rows > 0
      ? LEADERBOARD_HEADER_HEIGHT + rows * LEADERBOARD_ROW_HEIGHT
      : 0;
    return DASHBOARD_GAP + DASHBOARD_PADDING.top + STATS_HEIGHT + SECTION_GAP
      + leaderboardHeight + DASHBOARD_PADDING.bottom;
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

  draw(ctx, bounds, sessions) {
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
    this._drawStats(ctx, x, statsY, width, sessions);

    // Leaderboard
    if (sessions.length > 0) {
      const lbY = statsY + STATS_HEIGHT + SECTION_GAP;
      const lbHeight = height - DASHBOARD_PADDING.top - STATS_HEIGHT - SECTION_GAP - DASHBOARD_PADDING.bottom;
      this._drawLeaderboard(ctx, x, lbY, width, lbHeight, sessions);
    }
  }

  _drawStats(ctx, x, y, width, sessions) {
    const active = sessions.filter(s => s.activity === 'thinking' || s.activity === 'tool_use').length;
    const idle = sessions.filter(s => s.activity === 'idle' || s.activity === 'waiting' || s.activity === 'starting').length;
    const done = sessions.filter(s => TERMINAL_ACTIVITIES.has(s.activity)).length;
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
      name: x + 50,
      model: x + width * 0.35,
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

    // Rows
    const rowsStartY = y + LEADERBOARD_HEADER_HEIGHT;
    for (let i = 0; i < rows.length; i++) {
      const rowY = rowsStartY + i * LEADERBOARD_ROW_HEIGHT + LEADERBOARD_ROW_HEIGHT / 2;
      if (rowY - y > maxHeight) break;
      this._drawLeaderboardRow(ctx, cols, rowY, i + 1, rows[i]);
    }

    ctx.textBaseline = 'alphabetic';
  }

  _drawLeaderboardRow(ctx, cols, cy, rank, session) {
    const util = session.contextUtilization || 0;
    const pct = (util * 100).toFixed(0);
    const isTerminal = TERMINAL_ACTIVITIES.has(session.activity);
    const alpha = isTerminal ? 0.4 : 0.85;

    ctx.globalAlpha = alpha;

    // Rank
    ctx.fillStyle = rank <= 3 ? '#d4a017' : '#555';
    ctx.font = rank <= 3 ? 'bold 12px Courier New' : '12px Courier New';
    ctx.textAlign = 'right';
    ctx.textBaseline = 'middle';
    ctx.fillText(`${rank}`, cols.rank + 16, cy);

    // Activity dot
    const dotColor = ACTIVITY_COLORS[session.activity] || '#4b5563';
    ctx.beginPath();
    ctx.arc(cols.name - 8, cy, 3, 0, Math.PI * 2);
    ctx.fillStyle = dotColor;
    ctx.fill();

    // Session name
    ctx.fillStyle = '#bbb';
    ctx.font = '12px Courier New';
    ctx.textAlign = 'left';
    ctx.fillText(shortName(session.name), cols.name, cy);

    // Model
    ctx.fillStyle = '#666';
    ctx.font = '11px Courier New';
    ctx.fillText(shortModel(session.model), cols.model, cy);

    // Tokens
    ctx.fillStyle = '#888';
    ctx.font = '11px Courier New';
    ctx.fillText(formatTokens(session.tokensUsed || 0), cols.tokens, cy);

    // Utilization bar
    const barH = 10;
    const barY = cy - barH / 2;

    // Bar background
    ctx.fillStyle = 'rgba(255,255,255,0.04)';
    ctx.fillRect(cols.bar, barY, cols.barWidth, barH);

    // Bar fill — red above 80%, amber above 50%, green otherwise
    let barColor = '#22c55e';
    if (util > 0.8) barColor = '#e94560';
    else if (util > 0.5) barColor = '#d97706';
    ctx.fillStyle = barColor;
    ctx.fillRect(cols.bar, barY, cols.barWidth * util, barH);

    // Percentage
    ctx.fillStyle = '#777';
    ctx.font = '11px Courier New';
    ctx.fillText(`${pct}%`, cols.pct, cy);

    // Elapsed time
    ctx.fillStyle = '#555';
    ctx.font = '11px Courier New';
    ctx.textAlign = 'right';
    ctx.fillText(elapsed(session.startedAt), cols.elapsed, cy);

    ctx.textAlign = 'left';
    ctx.globalAlpha = 1.0;
  }
}
