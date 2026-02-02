const MODEL_COLORS = {
  'claude-opus-4-5-20251101': { main: '#a855f7', dark: '#7c3aed', light: '#c084fc', name: 'Opus' },
  'claude-sonnet-4-20250514': { main: '#3b82f6', dark: '#2563eb', light: '#60a5fa', name: 'Sonnet' },
  'claude-sonnet-4-5-20250929': { main: '#06b6d4', dark: '#0891b2', light: '#22d3ee', name: 'Sonnet' },
  'claude-haiku-3-5-20241022': { main: '#22c55e', dark: '#16a34a', light: '#4ade80', name: 'Haiku' },
};

const DEFAULT_COLOR = { main: '#6b7280', dark: '#4b5563', light: '#9ca3af', name: '?' };

function shortModelName(model) {
  if (!model) return '?';
  const parts = model.split(/[-_]/).filter(Boolean);
  if (parts.length === 0) return model.slice(0, 6).toUpperCase();
  if (parts[0] === 'gemini') {
    const version = parts[1] ? parts[1].replace(/[^0-9.]/g, '') : '';
    const tier = parts[2] ? parts[2][0].toUpperCase() : '';
    return `G${version}${tier}` || 'G';
  }
  if (parts[0].startsWith('o')) {
    return parts[0].toUpperCase();
  }
  if (parts[0] === 'gpt') {
    return `${parts[0].toUpperCase()}${parts[1] ? parts[1] : ''}`.slice(0, 6);
  }
  return parts[0].slice(0, 6).toUpperCase();
}

function getModelColor(model, source) {
  if (MODEL_COLORS[model]) {
    return MODEL_COLORS[model];
  }
  if (model) {
    return { ...DEFAULT_COLOR, name: shortModelName(model) };
  }
  if (source) {
    return { ...DEFAULT_COLOR, name: source.toUpperCase() };
  }
  return DEFAULT_COLOR;
}

function formatTokens(tokens) {
  if (tokens >= 1000) {
    return `${Math.round(tokens / 1000)}K`;
  }
  return `${tokens}`;
}

function hexToRgb(hex) {
  const r = parseInt(hex.slice(1, 3), 16);
  const g = parseInt(hex.slice(3, 5), 16);
  const b = parseInt(hex.slice(5, 7), 16);
  return { r, g, b };
}

function lightenHex(hex, amount) {
  const { r, g, b } = hexToRgb(hex);
  return `rgb(${Math.min(255, r + amount)},${Math.min(255, g + amount)},${Math.min(255, b + amount)})`;
}

export class Racer {
  constructor(state) {
    this.id = state.id;
    this.state = state;
    this.displayX = 0;
    this.targetX = 0;
    this.displayY = 0;
    this.targetY = 0;
    this.opacity = 1.0;
    this.hazardPhase = 0;
    this.spinAngle = 0;
    this.confettiEmitted = false;
    this.smokeEmitted = false;
    this.thoughtBubblePhase = 0;
    this.initialized = false;

    // New: wheel rotation
    this.wheelAngle = 0;

    // New: spring-based suspension
    this.springY = 0;
    this.springVel = 0;
    this.springDamping = 0.92;
    this.springStiffness = 0.15;

    // New: activity transitions
    this.prevActivity = state.activity;
    this.transitionTimer = 0;
    this.glowIntensity = 0;
    this.targetGlow = 0;
    this.colorBrightness = 0;

    // New: error multi-stage
    this.errorStage = 0; // 0=skid, 1=spin accelerate, 2=smoke, 3=darken
    this.errorTimer = 0;
    this.skidEmitted = false;

    // New: completion effects
    this.completionTimer = 0;
    this.goldFlash = 0;

    // New: ghost trail for 'lost'
    this.posHistory = []; // ring buffer of {x,y}

    // New: dot animation for thought bubble
    this.dotPhase = 0;
  }

  update(state) {
    const oldActivity = this.state.activity;
    this.state = state;

    // Detect activity transition
    if (state.activity !== oldActivity) {
      this.prevActivity = oldActivity;
      this.transitionTimer = 0.3; // 0.3s transition

      // Add spring energy on transition
      this.springVel += 2.5;

      // Reset stage-specific flags on new activity
      if (state.activity === 'errored') {
        this.errorStage = 0;
        this.errorTimer = 0;
        this.skidEmitted = false;
        this.smokeEmitted = false;
        this.spinAngle = 0;
      }
      if (state.activity === 'complete') {
        this.confettiEmitted = false;
        this.completionTimer = 0;
        this.goldFlash = 0;
      }
    }
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

  animate(particles, dt) {
    const dtScale = dt ? dt / (1 / 60) : 1;

    // Smooth lerp toward target
    const lerpSpeed = 0.08;
    const prevX = this.displayX;
    this.displayX += (this.targetX - this.displayX) * lerpSpeed * dtScale;
    this.displayY += (this.targetY - this.displayY) * lerpSpeed * dtScale;

    // Speed for wheel rotation and effects
    const speed = Math.abs(this.displayX - prevX);

    // Wheel rotation
    this.wheelAngle += speed * 0.3 * dtScale;

    // Spring-based suspension
    const springForce = -this.springStiffness * this.springY;
    this.springVel += springForce * dtScale;
    this.springVel *= Math.pow(this.springDamping, dtScale);
    this.springY += this.springVel * dtScale;

    // Transition timer
    if (this.transitionTimer > 0) {
      this.transitionTimer = Math.max(0, this.transitionTimer - (dt || 1 / 60));
    }

    // Glow interpolation
    this.glowIntensity += (this.targetGlow - this.glowIntensity) * 0.1 * dtScale;

    this.thoughtBubblePhase += 0.06 * dtScale;
    this.dotPhase += 0.04 * dtScale;

    // Store position history for ghost trail
    this.posHistory.push({ x: this.displayX, y: this.displayY });
    if (this.posHistory.length > 4) this.posHistory.shift();

    const activity = this.state.activity;

    switch (activity) {
      case 'thinking':
        this.targetGlow = 0.08;
        if (particles && speed > 0.5 && Math.random() > 0.5) {
          particles.emit('exhaust', this.displayX - 19, this.displayY + 2, 1);
        }
        break;

      case 'tool_use':
        this.targetGlow = 0.12;
        this.colorBrightness = Math.min(40, this.colorBrightness + 2 * dtScale);
        if (particles && Math.random() > 0.5) {
          particles.emit('exhaust', this.displayX - 19, this.displayY + 2, 1);
        }
        if (particles && Math.random() > 0.6) {
          particles.emit('sparks', this.displayX + 10, this.displayY + 8, 1);
        }
        // Speed lines for fast movement
        if (particles && speed > 1.5) {
          const color = getModelColor(this.state.model, this.state.source);
          const rgb = hexToRgb(color.main);
          particles.emitWithColor('speedLines', this.displayX - 25, this.displayY, 1, rgb);
        }
        break;

      case 'waiting':
        this.hazardPhase += 0.1 * dtScale;
        this.targetGlow = 0.05;
        this.colorBrightness = Math.max(0, this.colorBrightness - 1 * dtScale);
        break;

      case 'complete':
        this.completionTimer += dt || 1 / 60;
        this.goldFlash = this.completionTimer < 2 ?
          0.5 + 0.5 * Math.sin(this.completionTimer * 8) : 1.0;
        if (!this.confettiEmitted && particles) {
          particles.emit('celebration', this.displayX, this.displayY, 60);
          this.confettiEmitted = true;
        }
        this.targetGlow = 0.15;
        break;

      case 'errored':
        this.errorTimer += dt || 1 / 60;
        if (this.errorTimer < 0.5) {
          // Stage 0: skid marks
          this.errorStage = 0;
          if (!this.skidEmitted && particles) {
            particles.emit('skidMarks', this.displayX - 11, this.displayY + 10, 8);
            particles.emit('skidMarks', this.displayX + 12, this.displayY + 10, 8);
            this.skidEmitted = true;
          }
          this.spinAngle += 0.05 * dtScale;
        } else if (this.errorTimer < 1.0) {
          // Stage 1: spin accelerates
          this.errorStage = 1;
          this.spinAngle += 0.2 * dtScale;
        } else if (this.errorTimer < 1.5) {
          // Stage 2: large smoke cloud
          this.errorStage = 2;
          if (!this.smokeEmitted && particles) {
            particles.emit('smoke', this.displayX, this.displayY, 30);
            this.smokeEmitted = true;
          }
          this.spinAngle += 0.1 * dtScale;
        } else {
          // Stage 3: darken to grayscale
          this.errorStage = 3;
          this.spinAngle += 0.02 * dtScale;
        }
        break;

      case 'lost':
        this.opacity = Math.max(0.2, this.opacity - 0.005 * dtScale);
        this.targetGlow = 0;
        break;

      default:
        this.targetGlow = 0;
        this.colorBrightness = Math.max(0, this.colorBrightness - 1 * dtScale);
    }
  }

  draw(ctx) {
    const x = this.displayX;
    const y = this.displayY;
    const color = getModelColor(this.state.model, this.state.source);
    const activity = this.state.activity;

    ctx.save();
    ctx.globalAlpha = this.opacity;

    // Apply spin for errored
    if (activity === 'errored') {
      ctx.translate(x, y);
      ctx.rotate(this.spinAngle);
      ctx.translate(-x, -y);
    }

    // Suspension bounce
    const yOff = this.springY;

    // Ghost trail for 'lost'
    if (activity === 'lost' && this.posHistory.length > 1) {
      for (let i = 0; i < this.posHistory.length - 1; i++) {
        const pos = this.posHistory[i];
        const ghostAlpha = (i / this.posHistory.length) * 0.15;
        ctx.save();
        ctx.globalAlpha = ghostAlpha;
        this.drawCar(ctx, pos.x, pos.y, color, activity);
        ctx.restore();
      }
    }

    // Car shadow
    ctx.fillStyle = 'rgba(0,0,0,0.2)';
    ctx.beginPath();
    ctx.ellipse(x + 1, y + 12, 18, 3, 0, 0, Math.PI * 2);
    ctx.fill();

    // Glow aura
    if (this.glowIntensity > 0.01) {
      const glowColor = hexToRgb(color.main);
      const glow = ctx.createRadialGradient(x, y + yOff, 0, x, y + yOff, 35);
      glow.addColorStop(0, `rgba(${glowColor.r},${glowColor.g},${glowColor.b},${this.glowIntensity})`);
      glow.addColorStop(1, `rgba(${glowColor.r},${glowColor.g},${glowColor.b},0)`);
      ctx.fillStyle = glow;
      ctx.beginPath();
      ctx.arc(x, y + yOff, 35, 0, Math.PI * 2);
      ctx.fill();
    }

    this.drawCar(ctx, x, y + yOff, color, activity);
    this.drawActivityEffects(ctx, x, y + yOff, color, activity);
    this.drawInfo(ctx, x, y, color, activity);

    ctx.restore();
  }

  drawCar(ctx, x, y, color, activity) {
    // Determine car color (grayscale for errored stage 3, gold tint for complete)
    let bodyColor = color.main;
    if (activity === 'errored' && this.errorStage >= 3) {
      bodyColor = '#555';
    } else if (activity === 'complete' && this.goldFlash > 0) {
      const mc = hexToRgb(color.main);
      const gold = { r: 255, g: 215, b: 0 };
      const f = this.goldFlash;
      bodyColor = `rgb(${Math.round(mc.r + (gold.r - mc.r) * f)},${Math.round(mc.g + (gold.g - mc.g) * f)},${Math.round(mc.b + (gold.b - mc.b) * f)})`;
    } else if (this.colorBrightness > 0) {
      bodyColor = lightenHex(color.main, this.colorBrightness);
    }

    // --- Wheels (drawn first, behind body) ---
    const rearWheelX = x - 11;
    const frontWheelX = x + 12;
    const wheelY = y + 5;
    const wheelR = 5;
    this._drawWheel(ctx, rearWheelX, wheelY, wheelR);
    this._drawWheel(ctx, frontWheelX, wheelY, wheelR);

    // --- Car body - side profile racing car facing right ---
    ctx.fillStyle = bodyColor;
    ctx.beginPath();
    ctx.moveTo(x - 17, y + 2);        // rear bottom
    ctx.lineTo(x - 17, y - 3);        // rear face
    ctx.lineTo(x - 13, y - 7);        // rear roofline
    ctx.lineTo(x - 4, y - 9);         // roof
    ctx.lineTo(x + 3, y - 9);         // roof front
    ctx.lineTo(x + 9, y - 5);         // windshield slope
    ctx.lineTo(x + 15, y - 3);        // hood
    ctx.lineTo(x + 21, y - 1);        // nose top
    ctx.quadraticCurveTo(x + 23, y, x + 21, y + 1); // nose tip curve
    ctx.lineTo(x + 18, y + 2);        // front bottom
    ctx.closePath();                    // flat underbody
    ctx.fill();

    // Body outline
    ctx.strokeStyle = color.dark;
    ctx.lineWidth = 1;
    ctx.stroke();

    // Lower panel / side skirt (darker shade for depth)
    ctx.fillStyle = color.dark;
    ctx.beginPath();
    ctx.moveTo(x - 17, y + 2);
    ctx.lineTo(x - 17, y);
    ctx.lineTo(x + 19, y);
    ctx.lineTo(x + 21, y + 1);
    ctx.lineTo(x + 18, y + 2);
    ctx.closePath();
    ctx.fill();

    // Top edge highlight
    ctx.strokeStyle = lightenHex(color.main, 35);
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(x - 12, y - 7);
    ctx.lineTo(x - 4, y - 9);
    ctx.lineTo(x + 3, y - 9);
    ctx.stroke();

    // Racing stripe
    ctx.strokeStyle = lightenHex(color.main, 60);
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.moveTo(x - 15, y - 2);
    ctx.lineTo(x + 19, y - 2);
    ctx.stroke();

    // Windshield (side view)
    ctx.fillStyle = 'rgba(100,180,255,0.3)';
    ctx.beginPath();
    ctx.moveTo(x + 3, y - 9);
    ctx.lineTo(x + 9, y - 5);
    ctx.lineTo(x + 9, y);
    ctx.lineTo(x + 3, y);
    ctx.closePath();
    ctx.fill();
    ctx.strokeStyle = 'rgba(180,210,255,0.3)';
    ctx.lineWidth = 0.5;
    ctx.stroke();

    // Rear spoiler
    ctx.fillStyle = color.dark;
    ctx.fillRect(x - 16, y - 12, 2, 5);
    ctx.beginPath();
    ctx.moveTo(x - 19, y - 13);
    ctx.lineTo(x - 11, y - 13);
    ctx.lineTo(x - 12, y - 11);
    ctx.lineTo(x - 18, y - 11);
    ctx.closePath();
    ctx.fill();

    // Headlight
    ctx.fillStyle = 'rgba(255,255,220,0.7)';
    ctx.beginPath();
    ctx.arc(x + 20, y, 2, 0, Math.PI * 2);
    ctx.fill();

    // Taillight
    ctx.fillStyle = 'rgba(255,30,30,0.7)';
    ctx.beginPath();
    ctx.arc(x - 17, y - 1, 2, 0, Math.PI * 2);
    ctx.fill();

    // Exhaust pipe
    ctx.fillStyle = '#333';
    ctx.beginPath();
    ctx.arc(x - 17, y + 1, 1.5, 0, Math.PI * 2);
    ctx.fill();

    // X overlay for errored stage 3
    if (activity === 'errored' && this.errorStage >= 3) {
      ctx.strokeStyle = '#e94560';
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.moveTo(x - 14, y - 8);
      ctx.lineTo(x + 16, y + 4);
      ctx.stroke();
      ctx.beginPath();
      ctx.moveTo(x + 16, y - 8);
      ctx.lineTo(x - 14, y + 4);
      ctx.stroke();
    }
  }

  _drawWheel(ctx, cx, cy, r) {
    // Tire
    ctx.fillStyle = '#1a1a1a';
    ctx.beginPath();
    ctx.arc(cx, cy, r, 0, Math.PI * 2);
    ctx.fill();

    // Hub
    ctx.fillStyle = '#444';
    ctx.beginPath();
    ctx.arc(cx, cy, r * 0.4, 0, Math.PI * 2);
    ctx.fill();

    // Spokes (4 lines through center, rotated by wheelAngle)
    ctx.strokeStyle = '#666';
    ctx.lineWidth = 0.8;
    for (let i = 0; i < 4; i++) {
      const angle = this.wheelAngle + (i * Math.PI / 4);
      ctx.beginPath();
      ctx.moveTo(cx + Math.cos(angle) * r * 0.3, cy + Math.sin(angle) * r * 0.3);
      ctx.lineTo(cx + Math.cos(angle) * r * 0.9, cy + Math.sin(angle) * r * 0.9);
      ctx.stroke();
    }
  }

  drawActivityEffects(ctx, x, y, color, activity) {
    switch (activity) {
      case 'thinking':
        this._drawThoughtBubble(ctx, x, y);
        break;

      case 'tool_use':
        this._drawHeadlight(ctx, x, y);
        this._drawToolBadge(ctx, x, y, color);
        break;

      case 'waiting':
        this._drawHazardLights(ctx, x, y);
        this._drawWarningTriangle(ctx, x, y);
        break;

      case 'complete':
        this._drawCheckerFlag(ctx, x, y);
        break;
    }
  }

  _drawThoughtBubble(ctx, x, y) {
    // Rounded rect thought bubble with animated dots
    const bx = x + 28;
    const by = y - 16;
    const bw = 24;
    const bh = 14;
    const br = 4;

    ctx.fillStyle = 'rgba(255,255,255,0.85)';
    ctx.beginPath();
    ctx.moveTo(bx + br, by);
    ctx.lineTo(bx + bw - br, by);
    ctx.quadraticCurveTo(bx + bw, by, bx + bw, by + br);
    ctx.lineTo(bx + bw, by + bh - br);
    ctx.quadraticCurveTo(bx + bw, by + bh, bx + bw - br, by + bh);
    ctx.lineTo(bx + br, by + bh);
    ctx.quadraticCurveTo(bx, by + bh, bx, by + bh - br);
    ctx.lineTo(bx, by + br);
    ctx.quadraticCurveTo(bx, by, bx + br, by);
    ctx.closePath();
    ctx.fill();

    // Tail circles
    ctx.beginPath();
    ctx.arc(x + 25, y - 6, 3, 0, Math.PI * 2);
    ctx.fill();
    ctx.beginPath();
    ctx.arc(x + 23, y - 2, 2, 0, Math.PI * 2);
    ctx.fill();

    // Animated dots "..."
    for (let i = 0; i < 3; i++) {
      const dotAlpha = 0.3 + 0.7 * Math.max(0, Math.sin(this.dotPhase * Math.PI * 2 - i * 1.2));
      ctx.fillStyle = `rgba(80,80,100,${dotAlpha})`;
      ctx.beginPath();
      ctx.arc(bx + 6 + i * 7, by + bh / 2, 2, 0, Math.PI * 2);
      ctx.fill();
    }
  }

  _drawHeadlight(ctx, x, y) {
    // Headlight point
    ctx.fillStyle = 'rgba(255,255,200,0.8)';
    ctx.beginPath();
    ctx.arc(x + 21, y, 3, 0, Math.PI * 2);
    ctx.fill();

    // Headlight beam cone (40px ahead)
    ctx.save();
    const beam = ctx.createRadialGradient(x + 21, y, 2, x + 21, y, 40);
    beam.addColorStop(0, 'rgba(255,255,200,0.2)');
    beam.addColorStop(1, 'rgba(255,255,200,0)');
    ctx.fillStyle = beam;
    ctx.beginPath();
    ctx.moveTo(x + 21, y - 2);
    ctx.lineTo(x + 60, y - 10);
    ctx.lineTo(x + 60, y + 10);
    ctx.lineTo(x + 21, y + 2);
    ctx.closePath();
    ctx.fill();
    ctx.restore();

    // Small glow
    const glow = ctx.createRadialGradient(x + 21, y, 0, x + 21, y, 15);
    glow.addColorStop(0, 'rgba(255,255,200,0.3)');
    glow.addColorStop(1, 'rgba(255,255,200,0)');
    ctx.fillStyle = glow;
    ctx.beginPath();
    ctx.arc(x + 21, y, 15, 0, Math.PI * 2);
    ctx.fill();
  }

  _drawToolBadge(ctx, x, y, color) {
    if (!this.state.currentTool) return;

    const text = this.state.currentTool;
    ctx.font = '9px Courier New';
    const textW = ctx.measureText(text).width;
    const bx = x - textW / 2 - 5;
    const by = y + 24;
    const bw = textW + 10;
    const bh = 14;

    // Pill background
    ctx.fillStyle = 'rgba(20,20,35,0.85)';
    ctx.beginPath();
    ctx.moveTo(bx + 7, by);
    ctx.lineTo(bx + bw - 7, by);
    ctx.quadraticCurveTo(bx + bw, by, bx + bw, by + 7);
    ctx.quadraticCurveTo(bx + bw, by + bh, bx + bw - 7, by + bh);
    ctx.lineTo(bx + 7, by + bh);
    ctx.quadraticCurveTo(bx, by + bh, bx, by + 7);
    ctx.quadraticCurveTo(bx, by, bx + 7, by);
    ctx.closePath();
    ctx.fill();

    // Text
    ctx.fillStyle = color.light;
    ctx.textAlign = 'center';
    ctx.fillText(text, x, by + 10);
  }

  _drawHazardLights(ctx, x, y) {
    const flash = Math.sin(this.hazardPhase) > 0;
    if (!flash) return;

    // Hazard lights at front and rear (side view)
    const positions = [
      [x + 20, y],        // front
      [x - 17, y - 1],    // rear
    ];
    for (const [hx, hy] of positions) {
      // Glow halo
      const glow = ctx.createRadialGradient(hx, hy, 0, hx, hy, 10);
      glow.addColorStop(0, 'rgba(255,136,0,0.5)');
      glow.addColorStop(1, 'rgba(255,136,0,0)');
      ctx.fillStyle = glow;
      ctx.beginPath();
      ctx.arc(hx, hy, 10, 0, Math.PI * 2);
      ctx.fill();

      // Light
      ctx.fillStyle = '#ff8800';
      ctx.beginPath();
      ctx.arc(hx, hy, 3, 0, Math.PI * 2);
      ctx.fill();
    }

    // Pulsing amber glow around car
    const pulseAlpha = Math.abs(Math.sin(this.hazardPhase)) * 0.15;
    const carGlow = ctx.createRadialGradient(x, y, 5, x, y, 30);
    carGlow.addColorStop(0, `rgba(255,170,0,${pulseAlpha})`);
    carGlow.addColorStop(1, 'rgba(255,170,0,0)');
    ctx.fillStyle = carGlow;
    ctx.beginPath();
    ctx.arc(x, y, 30, 0, Math.PI * 2);
    ctx.fill();
  }

  _drawWarningTriangle(ctx, x, y) {
    // Small warning triangle above car
    const tx = x;
    const ty = y - 22;

    // Triangle
    ctx.fillStyle = '#ffaa00';
    ctx.beginPath();
    ctx.moveTo(tx, ty - 6);
    ctx.lineTo(tx - 5, ty + 3);
    ctx.lineTo(tx + 5, ty + 3);
    ctx.closePath();
    ctx.fill();

    // Exclamation mark
    ctx.fillStyle = '#1a1a2e';
    ctx.font = 'bold 7px sans-serif';
    ctx.textAlign = 'center';
    ctx.fillText('!', tx, ty + 2);
  }

  _drawCheckerFlag(ctx, x, y) {
    // Checkered flag waving above car
    const flagX = x - 3;
    const flagY = y - 24;

    // Pole
    ctx.strokeStyle = '#888';
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.moveTo(flagX, flagY + 12);
    ctx.lineTo(flagX, flagY - 4);
    ctx.stroke();

    // Flag with slight wave
    const wave = Math.sin(this.completionTimer * 4) * 1.5;
    const s = 3;
    for (let r = 0; r < 3; r++) {
      for (let c = 0; c < 4; c++) {
        const fy = flagY - 4 + r * s + Math.sin(this.completionTimer * 4 + c * 0.3) * wave;
        ctx.fillStyle = (r + c) % 2 === 0 ? '#fff' : '#222';
        ctx.fillRect(flagX + c * s, fy, s, s);
      }
    }

    // Golden glow
    const goldGlow = ctx.createRadialGradient(x, y, 5, x, y, 30);
    goldGlow.addColorStop(0, `rgba(255,215,0,${this.goldFlash * 0.12})`);
    goldGlow.addColorStop(1, 'rgba(255,215,0,0)');
    ctx.fillStyle = goldGlow;
    ctx.beginPath();
    ctx.arc(x, y, 30, 0, Math.PI * 2);
    ctx.fill();
  }

  drawInfo(ctx, x, y, color, activity) {
    // Session name and model name above
    ctx.fillStyle = '#ddd';
    ctx.font = 'bold 11px Courier New';
    ctx.textAlign = 'center';
    const labelText = `${this.state.name} • ${color.name}`;
    ctx.fillText(labelText, x, y - 22);

    // Model badge
    ctx.fillStyle = color.dark;
    const badgeText = color.name;
    const badgeWidth = ctx.measureText(badgeText).width + 8;
    ctx.fillRect(x - badgeWidth / 2, y - 38, badgeWidth, 14);
    ctx.fillStyle = '#fff';
    ctx.font = '9px Courier New';
    ctx.fillText(badgeText, x, y - 27);

    // Token count below (skip if tool badge will be shown)
    const tokenText = `${formatTokens(this.state.tokensUsed)}/${formatTokens(this.state.maxContextTokens)}`;
    ctx.fillStyle = '#999';
    ctx.font = '10px Courier New';
    ctx.textAlign = 'center';
    if (activity === 'tool_use' && this.state.currentTool) {
      // Token count higher up, tool badge below
      ctx.fillText(tokenText, x, y + 22);
    } else {
      ctx.fillText(tokenText, x, y + 24);
    }
  }
}
