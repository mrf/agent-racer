const MODEL_COLORS = {
  'claude-opus-4-5-20251101': { main: '#a855f7', dark: '#7c3aed', light: '#c084fc', name: 'Opus' },
  'claude-sonnet-4-20250514': { main: '#3b82f6', dark: '#2563eb', light: '#60a5fa', name: 'Sonnet' },
  'claude-sonnet-4-5-20250929': { main: '#06b6d4', dark: '#0891b2', light: '#22d3ee', name: 'Sonnet' },
  'claude-haiku-3-5-20241022': { main: '#22c55e', dark: '#16a34a', light: '#4ade80', name: 'Haiku' },
};

const DEFAULT_COLOR = { main: '#6b7280', dark: '#4b5563', light: '#9ca3af', name: '?' };

function getModelColor(model) {
  return MODEL_COLORS[model] || DEFAULT_COLOR;
}

function formatTokens(tokens) {
  if (tokens >= 1000) {
    return `${Math.round(tokens / 1000)}K`;
  }
  return `${tokens}`;
}

export class Racer {
  constructor(state) {
    this.id = state.id;
    this.state = state;
    this.displayX = 0;
    this.targetX = 0;
    this.displayY = 0;
    this.targetY = 0;
    this.wobble = 0;
    this.wobbleSpeed = 0.05 + Math.random() * 0.03;
    this.opacity = 1.0;
    this.hazardPhase = 0;
    this.spinAngle = 0;
    this.confettiEmitted = false;
    this.smokeEmitted = false;
    this.thoughtBubblePhase = 0;
    this.initialized = false;
  }

  update(state) {
    this.state = state;
  }

  setTarget(x, y) {
    this.targetX = x;
    this.targetY = y;
    if (!this.initialized) {
      this.displayX = x;
      this.displayY = y;
      this.initialized = true;
    }
  }

  animate(particles) {
    // Smooth lerp toward target
    const lerpSpeed = 0.08;
    this.displayX += (this.targetX - this.displayX) * lerpSpeed;
    this.displayY += (this.targetY - this.displayY) * lerpSpeed;

    this.wobble += this.wobbleSpeed;
    this.thoughtBubblePhase += 0.06;

    const activity = this.state.activity;

    switch (activity) {
      case 'thinking':
      case 'tool_use':
        // Exhaust particles while moving
        if (particles && Math.random() > 0.5) {
          particles.emit('exhaust', this.displayX - 20, this.displayY + 4, 1);
        }
        if (activity === 'tool_use' && particles && Math.random() > 0.7) {
          particles.emit('sparks', this.displayX + 10, this.displayY + 8, 1);
        }
        break;

      case 'waiting':
        this.hazardPhase += 0.1;
        break;

      case 'complete':
        if (!this.confettiEmitted && particles) {
          particles.emit('confetti', this.displayX, this.displayY, 30);
          this.confettiEmitted = true;
        }
        break;

      case 'errored':
        this.spinAngle += 0.15;
        if (!this.smokeEmitted && particles) {
          particles.emit('smoke', this.displayX, this.displayY, 15);
          this.smokeEmitted = true;
        }
        break;

      case 'lost':
        this.opacity = Math.max(0.2, this.opacity - 0.005);
        break;
    }
  }

  draw(ctx) {
    const x = this.displayX;
    const y = this.displayY;
    const color = getModelColor(this.state.model);
    const activity = this.state.activity;

    ctx.save();
    ctx.globalAlpha = this.opacity;

    // Apply spin for errored
    if (activity === 'errored') {
      ctx.translate(x, y);
      ctx.rotate(this.spinAngle);
      ctx.translate(-x, -y);
    }

    // Car body wobble for movement
    const yOff = (activity === 'thinking' || activity === 'tool_use')
      ? Math.sin(this.wobble) * 1.5 : 0;

    this.drawCar(ctx, x, y + yOff, color, activity);
    this.drawInfo(ctx, x, y, color, activity);

    ctx.restore();
  }

  drawCar(ctx, x, y, color, activity) {
    // Car body
    ctx.fillStyle = color.main;
    ctx.beginPath();
    ctx.moveTo(x + 22, y);
    ctx.lineTo(x + 16, y - 10);
    ctx.lineTo(x - 12, y - 10);
    ctx.lineTo(x - 18, y - 4);
    ctx.lineTo(x - 18, y + 4);
    ctx.lineTo(x - 12, y + 10);
    ctx.lineTo(x + 16, y + 10);
    ctx.closePath();
    ctx.fill();

    // Windshield
    ctx.fillStyle = 'rgba(150,200,255,0.3)';
    ctx.beginPath();
    ctx.moveTo(x + 8, y - 8);
    ctx.lineTo(x + 14, y - 4);
    ctx.lineTo(x + 14, y + 4);
    ctx.lineTo(x + 8, y + 8);
    ctx.closePath();
    ctx.fill();

    // Wheels
    ctx.fillStyle = '#222';
    ctx.beginPath();
    ctx.arc(x - 10, y + 10, 4, 0, Math.PI * 2);
    ctx.fill();
    ctx.beginPath();
    ctx.arc(x + 10, y + 10, 4, 0, Math.PI * 2);
    ctx.fill();
    ctx.beginPath();
    ctx.arc(x - 10, y - 10, 4, 0, Math.PI * 2);
    ctx.fill();
    ctx.beginPath();
    ctx.arc(x + 10, y - 10, 4, 0, Math.PI * 2);
    ctx.fill();

    // Headlight glow for active sessions
    if (activity === 'thinking' || activity === 'tool_use') {
      ctx.fillStyle = 'rgba(255,255,200,0.6)';
      ctx.beginPath();
      ctx.arc(x + 22, y, 3, 0, Math.PI * 2);
      ctx.fill();

      // Glow
      const glow = ctx.createRadialGradient(x + 22, y, 0, x + 22, y, 15);
      glow.addColorStop(0, 'rgba(255,255,200,0.3)');
      glow.addColorStop(1, 'rgba(255,255,200,0)');
      ctx.fillStyle = glow;
      ctx.beginPath();
      ctx.arc(x + 22, y, 15, 0, Math.PI * 2);
      ctx.fill();
    }

    // Hazard lights for waiting
    if (activity === 'waiting') {
      const flash = Math.sin(this.hazardPhase) > 0;
      if (flash) {
        ctx.fillStyle = '#ff8800';
        ctx.beginPath();
        ctx.arc(x + 18, y - 6, 3, 0, Math.PI * 2);
        ctx.fill();
        ctx.beginPath();
        ctx.arc(x + 18, y + 6, 3, 0, Math.PI * 2);
        ctx.fill();
        ctx.beginPath();
        ctx.arc(x - 16, y - 6, 3, 0, Math.PI * 2);
        ctx.fill();
        ctx.beginPath();
        ctx.arc(x - 16, y + 6, 3, 0, Math.PI * 2);
        ctx.fill();
      }
    }

    // Trophy for complete
    if (activity === 'complete') {
      ctx.font = '16px serif';
      ctx.textAlign = 'center';
      ctx.fillText('🏆', x, y - 20);
    }
  }

  drawInfo(ctx, x, y, color, activity) {
    // Session name above
    ctx.fillStyle = '#ddd';
    ctx.font = 'bold 11px Courier New';
    ctx.textAlign = 'center';
    ctx.fillText(this.state.name, x, y - 22);

    // Model badge
    ctx.fillStyle = color.dark;
    const badgeText = color.name;
    const badgeWidth = ctx.measureText(badgeText).width + 8;
    ctx.fillRect(x - badgeWidth / 2, y - 38, badgeWidth, 14);
    ctx.fillStyle = '#fff';
    ctx.font = '9px Courier New';
    ctx.fillText(badgeText, x, y - 27);

    // Token count below
    const tokenText = `${formatTokens(this.state.tokensUsed)}/${formatTokens(this.state.maxContextTokens)}`;
    ctx.fillStyle = '#999';
    ctx.font = '10px Courier New';
    ctx.textAlign = 'center';
    ctx.fillText(tokenText, x, y + 24);

    // Current tool below token count
    if (this.state.currentTool && activity === 'tool_use') {
      ctx.fillStyle = color.light;
      ctx.font = '9px Courier New';
      ctx.fillText(this.state.currentTool, x, y + 34);
    }

    // Thinking bubble
    if (activity === 'thinking') {
      const bubbleAlpha = 0.4 + 0.3 * Math.sin(this.thoughtBubblePhase);
      ctx.globalAlpha = bubbleAlpha;
      ctx.fillStyle = '#fff';
      ctx.beginPath();
      ctx.arc(x + 30, y - 12, 3, 0, Math.PI * 2);
      ctx.fill();
      ctx.beginPath();
      ctx.arc(x + 36, y - 18, 5, 0, Math.PI * 2);
      ctx.fill();
      ctx.globalAlpha = this.opacity;
    }
  }
}
