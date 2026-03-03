/**
 * TeamGarage renders team garage banners for the Dashboard teams tab.
 * Each banner shows a team's color swatch, name, member count, and token total.
 */

const BANNER_HEIGHT = 36;
const BANNER_GAP = 6;
const BANNER_RADIUS = 4;
const MIN_MEMBERS_FOR_GARAGE = 2;

function formatTokens(tokens) {
  if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(1)}M`;
  if (tokens >= 1000) return `${Math.round(tokens / 1000)}K`;
  return `${tokens}`;
}

function formatBurnRate(rate) {
  if (!rate || rate <= 0) return '';
  if (rate >= 1000) return `${(rate / 1000).toFixed(1)}K/m`;
  return `${Math.round(rate)}/m`;
}

export class TeamGarage {
  /**
   * Returns the height required to display the given teams list.
   * Solo teams (< MIN_MEMBERS_FOR_GARAGE) are counted too since we show them dimmed.
   */
  getRequiredHeight(teamCount) {
    if (teamCount <= 0) return 0;
    return teamCount * BANNER_HEIGHT + Math.max(0, teamCount - 1) * BANNER_GAP;
  }

  /**
   * Draw team banners into the given bounds rectangle.
   * @param {CanvasRenderingContext2D} ctx
   * @param {number} x
   * @param {number} y
   * @param {number} width
   * @param {number} maxHeight
   * @param {Array} teams - array of TeamInfo from backend
   */
  draw(ctx, x, y, width, maxHeight, teams) {
    if (!teams || teams.length === 0) {
      ctx.fillStyle = '#555';
      ctx.font = '12px Courier New';
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText('No teams detected', x + width / 2, y + 20);
      ctx.textBaseline = 'alphabetic';
      return;
    }

    // Header
    ctx.fillStyle = '#444';
    ctx.font = 'bold 10px Courier New';
    ctx.textAlign = 'left';
    ctx.textBaseline = 'middle';
    ctx.fillText('TEAM', x + 48, y + 7);
    ctx.textAlign = 'right';
    ctx.fillText('MEMBERS', x + width * 0.55, y + 7);
    ctx.fillText('TOKENS', x + width * 0.73, y + 7);
    ctx.fillText('BURN/MIN', x + width - 8, y + 7);
    ctx.textBaseline = 'alphabetic';

    const startY = y + 18;

    for (let i = 0; i < teams.length; i++) {
      const team = teams[i];
      const bannerY = startY + i * (BANNER_HEIGHT + BANNER_GAP);
      if (bannerY + BANNER_HEIGHT > y + maxHeight) break;

      const isMulti = team.sessionCount >= MIN_MEMBERS_FOR_GARAGE;
      ctx.globalAlpha = isMulti ? 0.92 : 0.45;

      this._drawBanner(ctx, x, bannerY, width, team);
    }
    ctx.globalAlpha = 1.0;
  }

  _drawBanner(ctx, x, y, width, team) {
    const h = BANNER_HEIGHT;

    // Banner background
    ctx.fillStyle = 'rgba(255,255,255,0.04)';
    ctx.beginPath();
    ctx.roundRect(x, y, width, h, BANNER_RADIUS);
    ctx.fill();

    // Left color swatch
    ctx.fillStyle = team.color;
    ctx.beginPath();
    ctx.roundRect(x, y, 8, h, [BANNER_RADIUS, 0, 0, BANNER_RADIUS]);
    ctx.fill();

    // Team glow dot
    ctx.fillStyle = team.color;
    ctx.beginPath();
    ctx.arc(x + 22, y + h / 2, 5, 0, Math.PI * 2);
    ctx.fill();

    // Activity glow for active teams
    if (team.activeCount > 0) {
      const pulse = 0.5 + 0.5 * Math.sin(performance.now() / 600);
      const savedAlpha = ctx.globalAlpha;
      ctx.fillStyle = team.color;
      ctx.globalAlpha *= (0.15 + 0.1 * pulse);
      ctx.beginPath();
      ctx.arc(x + 22, y + h / 2, 10, 0, Math.PI * 2);
      ctx.fill();
      ctx.globalAlpha = savedAlpha;
    }

    const cy = y + h / 2;

    // Team name
    const nameX = x + 36;
    const nameMaxW = width * 0.42 - 36;
    ctx.fillStyle = '#ddd';
    ctx.font = 'bold 12px Courier New';
    ctx.textAlign = 'left';
    ctx.textBaseline = 'middle';
    const name = team.name;
    const nameW = ctx.measureText(name).width;
    if (nameW <= nameMaxW) {
      ctx.fillText(name, nameX, cy);
    } else {
      // Truncate
      let truncated = name;
      while (ctx.measureText(truncated + '\u2026').width > nameMaxW && truncated.length > 1) {
        truncated = truncated.slice(0, -1);
      }
      ctx.fillText(truncated + '\u2026', nameX, cy);
    }

    // Members count: "N active / M total"
    ctx.fillStyle = team.activeCount > 0 ? '#22c55e' : '#666';
    ctx.font = '11px Courier New';
    ctx.textAlign = 'right';
    const memberLabel = team.activeCount > 0
      ? `${team.activeCount}/${team.sessionCount}`
      : `${team.sessionCount}`;
    ctx.fillText(memberLabel, x + width * 0.55, cy);

    // Tokens
    ctx.fillStyle = '#a855f7';
    ctx.fillText(formatTokens(team.totalTokens), x + width * 0.73, cy);

    // Burn rate
    const burn = formatBurnRate(team.avgBurnRate);
    ctx.fillStyle = burn ? '#d97706' : '#444';
    ctx.fillText(burn || '-', x + width - 8, cy);

    // Completion/error pips below the name (for multi-member teams)
    if (team.sessionCount >= MIN_MEMBERS_FOR_GARAGE) {
      const pipY = y + h - 7;
      const pipX = nameX;
      for (let i = 0; i < team.completionCount; i++) {
        ctx.fillStyle = '#16a34a';
        ctx.beginPath();
        ctx.arc(pipX + i * 8, pipY, 2.5, 0, Math.PI * 2);
        ctx.fill();
      }
      const offset = team.completionCount * 8;
      for (let i = 0; i < team.errorCount; i++) {
        ctx.fillStyle = '#dc2626';
        ctx.beginPath();
        ctx.arc(pipX + offset + i * 8, pipY, 2.5, 0, Math.PI * 2);
        ctx.fill();
      }
    }

    ctx.textBaseline = 'alphabetic';
  }
}
