// Canvas-rendered toast notifications for achievement unlocks.
// Slides in from top-center, auto-dismisses after 5s, stacks vertically.

const TOAST_WIDTH = 320;
const TOAST_HEIGHT = 72;
const TOAST_MARGIN = 10;
const TOAST_PADDING = 14;
const SLIDE_DURATION = 0.35; // seconds
const DISPLAY_DURATION = 5000; // ms
const FADE_DURATION = 0.3; // seconds

const TIER_BORDER_COLORS = {
  bronze:   '#cd7f32',
  silver:   '#c0c0c0',
  gold:     '#ffd700',
  platinum: '#e5e4e2',
};

const TIER_ICONS = {
  bronze:   '\u{1F949}',
  silver:   '\u{1F948}',
  gold:     '\u{1F947}',
  platinum: '\u2728',
};

export class UnlockToast {
  constructor(engine) {
    this.engine = engine;
    this.toasts = []; // { name, description, tier, phase, progress, dismissTimer }
  }

  show(payload) {
    const toast = {
      name: payload.name || 'Achievement Unlocked',
      description: payload.description || '',
      tier: payload.tier || 'bronze',
      phase: 'enter',  // enter -> visible -> exit
      progress: 0,     // 0..1 for enter/exit animation
      dismissTimer: null,
    };

    this.toasts.push(toast);

    toast.dismissTimer = setTimeout(() => {
      if (toast.phase === 'visible') {
        toast.phase = 'exit';
        toast.progress = 0;
      }
    }, DISPLAY_DURATION);

    this.engine?.playUnlockChime(toast.tier);
  }

  update(dt) {
    for (let i = this.toasts.length - 1; i >= 0; i--) {
      const toast = this.toasts[i];

      if (toast.phase === 'enter') {
        toast.progress += dt / SLIDE_DURATION;
        if (toast.progress >= 1) {
          toast.progress = 1;
          toast.phase = 'visible';
        }
      } else if (toast.phase === 'exit') {
        toast.progress += dt / FADE_DURATION;
        if (toast.progress >= 1) {
          clearTimeout(toast.dismissTimer);
          this.toasts.splice(i, 1);
        }
      }
    }
  }

  draw(ctx, canvasWidth) {
    if (this.toasts.length === 0) return;

    const startX = (canvasWidth - TOAST_WIDTH) / 2;

    for (let i = 0; i < this.toasts.length; i++) {
      const toast = this.toasts[i];
      const stackY = TOAST_MARGIN + i * (TOAST_HEIGHT + TOAST_MARGIN);

      let y, alpha;
      if (toast.phase === 'enter') {
        const t = easeOutCubic(toast.progress);
        y = -TOAST_HEIGHT + t * (stackY + TOAST_HEIGHT);
        alpha = t;
      } else if (toast.phase === 'exit') {
        y = stackY;
        alpha = 1 - toast.progress;
      } else {
        y = stackY;
        alpha = 1;
      }

      ctx.save();
      ctx.globalAlpha = alpha;
      this._drawToast(ctx, startX, y, toast);
      ctx.restore();
    }
  }

  _drawToast(ctx, x, y, toast) {
    const borderColor = TIER_BORDER_COLORS[toast.tier] || TIER_BORDER_COLORS.bronze;
    const icon = TIER_ICONS[toast.tier] || '';
    const radius = 8;

    // Background
    ctx.fillStyle = 'rgba(16, 16, 48, 0.92)';
    ctx.beginPath();
    ctx.roundRect(x, y, TOAST_WIDTH, TOAST_HEIGHT, radius);
    ctx.fill();

    // Border
    ctx.strokeStyle = borderColor;
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.roundRect(x, y, TOAST_WIDTH, TOAST_HEIGHT, radius);
    ctx.stroke();

    // Tier glow along top edge
    const grad = ctx.createLinearGradient(x, y, x, y + 12);
    grad.addColorStop(0, borderColor + '40');
    grad.addColorStop(1, 'transparent');
    ctx.fillStyle = grad;
    ctx.beginPath();
    ctx.roundRect(x, y, TOAST_WIDTH, 12, [radius, radius, 0, 0]);
    ctx.fill();

    // Icon
    ctx.font = '22px serif';
    ctx.textAlign = 'left';
    ctx.textBaseline = 'middle';
    ctx.fillStyle = '#fff';
    ctx.fillText(icon, x + TOAST_PADDING, y + TOAST_HEIGHT / 2);

    // Name + description
    const textX = x + TOAST_PADDING + 32;
    const textMaxWidth = TOAST_WIDTH - TOAST_PADDING * 2 - 32;

    ctx.font = "bold 13px 'Courier New', monospace";
    ctx.fillStyle = '#e0e0e0';
    ctx.textBaseline = 'top';
    ctx.fillText(truncate(toast.name, ctx, textMaxWidth), textX, y + 16);

    ctx.font = "11px 'Courier New', monospace";
    ctx.fillStyle = '#999';
    ctx.fillText(truncate(toast.description, ctx, textMaxWidth), textX, y + 36);

    // Tier label
    ctx.font = "bold 9px 'Courier New', monospace";
    ctx.fillStyle = borderColor;
    ctx.textBaseline = 'bottom';
    ctx.fillText(toast.tier.toUpperCase(), textX, y + TOAST_HEIGHT - 10);
  }

  destroy() {
    for (const toast of this.toasts) {
      clearTimeout(toast.dismissTimer);
    }
    this.toasts = [];
  }
}

function easeOutCubic(t) {
  return 1 - Math.pow(1 - t, 3);
}

function truncate(text, ctx, maxWidth) {
  if (!text) return '';
  if (ctx.measureText(text).width <= maxWidth) return text;
  let truncated = text;
  while (truncated.length > 0 && ctx.measureText(truncated + '\u2026').width > maxWidth) {
    truncated = truncated.slice(0, -1);
  }
  return truncated + '\u2026';
}
