const MODEL_COLORS = {
  'claude-opus-4-5-20251101': { main: '#a855f7', dark: '#7c3aed', light: '#c084fc', name: 'Opus' },
  'claude-sonnet-4-20250514': { main: '#3b82f6', dark: '#2563eb', light: '#60a5fa', name: 'Sonnet' },
  'claude-sonnet-4-5-20250929': { main: '#06b6d4', dark: '#0891b2', light: '#22d3ee', name: 'Sonnet' },
  'claude-haiku-3-5-20241022': { main: '#22c55e', dark: '#16a34a', light: '#4ade80', name: 'Haiku' },
  'claude-haiku-4-5-20251001': { main: '#22c55e', dark: '#16a34a', light: '#4ade80', name: 'Haiku' },
};

const DEFAULT_COLOR = { main: '#6b7280', dark: '#4b5563', light: '#9ca3af', name: '?' };

const SOURCE_COLORS = {
  claude: { bg: '#a855f7', label: 'C' },
  codex: { bg: '#10b981', label: 'X' },
  gemini: { bg: '#4285f4', label: 'G' },
};

const DEFAULT_SOURCE = { bg: '#6b7280', label: '?' };

const CAR_SCALE = 2.3;

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
    const lower = model.toLowerCase();
    if (lower.includes('opus')) {
      return { ...MODEL_COLORS['claude-opus-4-5-20251101'], name: 'Opus' };
    }
    if (lower.includes('sonnet')) {
      return { ...MODEL_COLORS['claude-sonnet-4-5-20250929'], name: 'Sonnet' };
    }
    if (lower.includes('haiku')) {
      return { ...MODEL_COLORS['claude-haiku-4-5-20251001'], name: 'Haiku' };
    }
    return { ...DEFAULT_COLOR, name: shortModelName(model) };
  }

  if (source) {
    return { ...DEFAULT_COLOR, name: source.toUpperCase() };
  }
  return DEFAULT_COLOR;
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

    // New: hammer animation for tool use
    this.hammerSwing = 0; // 0-1 animation progress
    this.hammerActive = false;
    this.hammerImpactEmitted = false;

    // Pit lane state
    this.inPit = false;
    this.pitDim = 0;       // current dimming (0=normal, 1=fully dimmed)
    this.pitDimTarget = 0;

    // Parking lot state
    this.inParkingLot = false;
    this.parkingLotDim = 0;       // 0=normal, 1=fully dimmed
    this.parkingLotDimTarget = 0;

    // Zone transition waypoints (track <-> pit <-> parking lot)
    this.transitionWaypoints = null;
    this.waypointIndex = 0;

    // Flag flutter animation
    this.flagPhase = Math.random() * Math.PI * 2;

    // Tmux focus state
    this.hovered = false;
    this.hasTmux = !!state.tmuxTarget;
  }

  update(state) {
    const oldActivity = this.state.activity;
    const wasChurning = this.state.isChurning;
    this.state = state;
    this.hasTmux = !!state.tmuxTarget;

    // Detect churning transition: add a subtle bounce when churning starts
    if (state.isChurning && !wasChurning) {
      this.springVel += 1.2;
    }

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
      if (state.activity === 'tool_use') {
        // Trigger hammer animation on tool_use transition
        this.hammerActive = true;
        this.hammerSwing = 0;
        this.hammerImpactEmitted = false;
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

  startZoneTransition(waypoints) {
    this.transitionWaypoints = waypoints;
    this.waypointIndex = 0;
  }

  animate(particles, dt) {
    const dtScale = dt ? dt / (1 / 60) : 1;

    // Zone transition waypoint pathing
    let lerpSpeed = 0.08;
    if (this.transitionWaypoints) {
      const wp = this.transitionWaypoints[this.waypointIndex];
      this.targetX = wp.x;
      this.targetY = wp.y;
      lerpSpeed = 0.12;
      const dx = this.displayX - wp.x;
      const dy = this.displayY - wp.y;
      if (Math.sqrt(dx * dx + dy * dy) < 10) {
        this.waypointIndex++;
        if (this.waypointIndex >= this.transitionWaypoints.length) {
          this.transitionWaypoints = null;
          this.waypointIndex = 0;
        }
      }
    }

    // Smooth lerp toward target
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

    // Pit dimming transition
    this.pitDim += (this.pitDimTarget - this.pitDim) * 0.08 * dtScale;

    // Parking lot dimming transition
    this.parkingLotDim += (this.parkingLotDimTarget - this.parkingLotDim) * 0.06 * dtScale;

    this.thoughtBubblePhase += 0.06 * dtScale;
    this.dotPhase += 0.04 * dtScale;
    this.flagPhase += 0.05 * dtScale;

    // Store position history for ghost trail
    this.posHistory.push({ x: this.displayX, y: this.displayY });
    if (this.posHistory.length > 4) this.posHistory.shift();

    const activity = this.state.activity;

    const S = CAR_SCALE;
    switch (activity) {
      case 'thinking':
        this.targetGlow = 0.08;
        if (particles && speed > 0.5 && Math.random() > 0.5) {
          particles.emit('exhaust', this.displayX - 17 * S, this.displayY + 1 * S, 1);
        }
        this.hammerActive = false; // Stop hammer animation
        break;

      case 'tool_use':
        this.targetGlow = 0.12;
        this.colorBrightness = Math.min(40, this.colorBrightness + 2 * dtScale);
        if (particles && Math.random() > 0.5) {
          particles.emit('exhaust', this.displayX - 17 * S, this.displayY + 1 * S, 1);
        }
        if (particles && Math.random() > 0.6) {
          particles.emit('sparks', this.displayX + 10 * S, this.displayY + 5 * S, 1);
        }
        // Speed lines for fast movement
        if (particles && speed > 1.5) {
          const color = getModelColor(this.state.model, this.state.source);
          const rgb = hexToRgb(color.main);
          particles.emitWithColor('speedLines', this.displayX - 20 * S, this.displayY, 1, rgb);
        }
        // Hammer animation
        if (this.hammerActive) {
          this.hammerSwing += 0.08 * dtScale; // Swing speed
          // Impact point (when hammer hits hood)
          if (this.hammerSwing >= 0.5 && this.hammerSwing < 0.6 && !this.hammerImpactEmitted && particles) {
            particles.emit('sparks', this.displayX + 12 * S, this.displayY - 8 * S, 3);
            this.hammerImpactEmitted = true;
            // Add bounce to suspension on impact
            this.springVel += 1.8;
          }
          // Reset for continuous animation
          if (this.hammerSwing >= 1.0) {
            this.hammerSwing = 0;
            this.hammerImpactEmitted = false;
          }
        }
        break;

      case 'waiting':
        this.hazardPhase += 0.1 * dtScale;
        this.targetGlow = 0.05;
        this.colorBrightness = Math.max(0, this.colorBrightness - 1 * dtScale);
        this.hammerActive = false; // Stop hammer animation
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
            particles.emit('skidMarks', this.displayX - 11 * S, this.displayY + 5 * S + 5 * S, 8);
            particles.emit('skidMarks', this.displayX + 12 * S, this.displayY + 5 * S + 5 * S, 8);
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
        this.hammerActive = false; // Stop hammer animation
    }

    // Churning animation: subtle activity when process is working but no
    // JSONL output yet. Only applies when car would otherwise be idle/starting.
    if (this.state.isChurning && activity !== 'thinking' && activity !== 'tool_use') {
      this.wheelAngle += 0.02 * dtScale;
      if (particles && Math.random() > 0.95) {
        const S = CAR_SCALE;
        particles.emit('exhaust', this.displayX - 17 * S, this.displayY + 1 * S, 1);
      }
      this.springVel += (Math.random() - 0.5) * 0.3;
      this.targetGlow = 0.04;
    }

    // Suppress effects when in pit or parking lot
    if (this.inPit || this.inParkingLot) {
      this.targetGlow = Math.min(this.targetGlow, 0.02);
    }
  }

  draw(ctx) {
    const x = this.displayX;
    const y = this.displayY;
    const color = getModelColor(this.state.model, this.state.source);
    const activity = this.state.activity;

    ctx.save();

    // Zone dimming: reduce opacity for pit and parking lot racers
    const pitAlpha = 1 - this.pitDim * 0.4;
    const parkingAlpha = 1 - this.parkingLotDim * 0.5;
    ctx.globalAlpha = this.opacity * pitAlpha * parkingAlpha;

    if (this.pitDim > 0.01 || this.parkingLotDim > 0.01) {
      const pitScale = 1 - this.pitDim * 0.15;
      const parkingScale = 1 - this.parkingLotDim * 0.1;
      ctx.translate(x, y);
      ctx.scale(pitScale * parkingScale, pitScale * parkingScale);
      ctx.translate(-x, -y);
    }

    // Parking lot: apply desaturation via filter if supported
    if (this.parkingLotDim > 0.01) {
      ctx.filter = `saturate(${1 - this.parkingLotDim * 0.7})`;
    }

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
    const S = CAR_SCALE;
    ctx.fillStyle = 'rgba(0,0,0,0.2)';
    ctx.beginPath();
    ctx.ellipse(x + 2, y + 12 * S, 18 * S, 3 * S, 0, 0, Math.PI * 2);
    ctx.fill();

    // Glow aura
    if (this.glowIntensity > 0.01) {
      const glowColor = hexToRgb(color.main);
      const glowR = 35 * S;
      const glow = ctx.createRadialGradient(x, y + yOff, 0, x, y + yOff, glowR);
      glow.addColorStop(0, `rgba(${glowColor.r},${glowColor.g},${glowColor.b},${this.glowIntensity})`);
      glow.addColorStop(1, `rgba(${glowColor.r},${glowColor.g},${glowColor.b},0)`);
      ctx.fillStyle = glow;
      ctx.beginPath();
      ctx.arc(x, y + yOff, glowR, 0, Math.PI * 2);
      ctx.fill();
    }

    this.drawCar(ctx, x, y + yOff, color, activity);

    // Hover highlight for tmux-focusable sessions
    if (this.hovered && this.hasTmux) {
      const rgb = hexToRgb(color.light);
      ctx.strokeStyle = `rgba(${rgb.r},${rgb.g},${rgb.b},0.5)`;
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.roundRect(x - 18 * S, (y + yOff) - 10 * S, 40 * S, 18 * S, 4);
      ctx.stroke();
    }

    this.drawActivityEffects(ctx, x, y + yOff, color, activity);
    this.drawInfo(ctx, x, y, color, activity);

    ctx.restore();
  }

  drawCar(ctx, x, y, color, activity) {
    ctx.save();
    ctx.translate(x, y);
    ctx.scale(CAR_SCALE, CAR_SCALE);
    ctx.translate(-x, -y);

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

    ctx.restore();
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
        if (this.hammerActive) {
          this._drawHammer(ctx, x, y);
        }
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
    const S = CAR_SCALE;
    // Rounded rect thought bubble with animated dots
    const bx = x + 22 * S + 4;
    const by = y - 10 * S - 4;
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
    ctx.arc(x + 21 * S, y - 3 * S, 3, 0, Math.PI * 2);
    ctx.fill();
    ctx.beginPath();
    ctx.arc(x + 20 * S, y - 1 * S, 2, 0, Math.PI * 2);
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
    const S = CAR_SCALE;
    const hx = x + 21 * S;
    // Headlight point
    ctx.fillStyle = 'rgba(255,255,200,0.8)';
    ctx.beginPath();
    ctx.arc(hx, y, 3, 0, Math.PI * 2);
    ctx.fill();

    // Headlight beam cone
    ctx.save();
    const beamLen = 40 * S;
    const beam = ctx.createRadialGradient(hx, y, 2, hx, y, beamLen);
    beam.addColorStop(0, 'rgba(255,255,200,0.2)');
    beam.addColorStop(1, 'rgba(255,255,200,0)');
    ctx.fillStyle = beam;
    ctx.beginPath();
    ctx.moveTo(hx, y - 3);
    ctx.lineTo(hx + beamLen, y - 15);
    ctx.lineTo(hx + beamLen, y + 15);
    ctx.lineTo(hx, y + 3);
    ctx.closePath();
    ctx.fill();
    ctx.restore();

    // Small glow
    const glow = ctx.createRadialGradient(hx, y, 0, hx, y, 20);
    glow.addColorStop(0, 'rgba(255,255,200,0.3)');
    glow.addColorStop(1, 'rgba(255,255,200,0)');
    ctx.fillStyle = glow;
    ctx.beginPath();
    ctx.arc(hx, y, 20, 0, Math.PI * 2);
    ctx.fill();
  }

  _drawToolBadge(ctx, x, y, color) {
    if (!this.state.currentTool) return;

    const S = CAR_SCALE;
    const text = this.state.currentTool;
    ctx.font = '9px Courier New';
    const textW = ctx.measureText(text).width;
    const bx = x - textW / 2 - 5;
    const by = y + 10 * S + 6;
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

    const S = CAR_SCALE;
    // Hazard lights at front and rear (side view)
    const positions = [
      [x + 20 * S, y],          // front
      [x - 17 * S, y - 1 * S],  // rear
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
    const glowR = 30 * S;
    const carGlow = ctx.createRadialGradient(x, y, 8, x, y, glowR);
    carGlow.addColorStop(0, `rgba(255,170,0,${pulseAlpha})`);
    carGlow.addColorStop(1, 'rgba(255,170,0,0)');
    ctx.fillStyle = carGlow;
    ctx.beginPath();
    ctx.arc(x, y, glowR, 0, Math.PI * 2);
    ctx.fill();
  }

  _drawWarningTriangle(ctx, x, y) {
    // Small warning triangle above car
    const tx = x;
    const ty = y - 9 * CAR_SCALE - 10;

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
    const S = CAR_SCALE;
    // Checkered flag waving above car
    const flagX = x - 5;
    const flagY = y - 9 * S - 14;

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
    const glowR = 30 * S;
    const goldGlow = ctx.createRadialGradient(x, y, 5, x, y, glowR);
    goldGlow.addColorStop(0, `rgba(255,215,0,${this.goldFlash * 0.12})`);
    goldGlow.addColorStop(1, 'rgba(255,215,0,0)');
    ctx.fillStyle = goldGlow;
    ctx.beginPath();
    ctx.arc(x, y, glowR, 0, Math.PI * 2);
    ctx.fill();
  }

  _drawHammer(ctx, x, y) {
    const S = CAR_SCALE;
    // Hammer positioned above hood, swinging down
    // Hood is around x+12 to x+15, y-9 to y-3

    // Calculate swing angle: -70deg (raised) -> 0deg (impact) -> -70deg
    // Use easeInOutQuad for smooth motion
    let t = this.hammerSwing;
    // Ease function for natural swing
    const easeInOutQuad = (t) => t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2;
    const swingProgress = easeInOutQuad(t);
    const angle = -70 * (1 - swingProgress) * (Math.PI / 180); // -70deg to 0deg

    // Pivot point (where hammer rotates from) - above and to the side of hood
    const pivotX = x + 10 * S;
    const pivotY = y - 18 * S;

    ctx.save();
    ctx.translate(pivotX, pivotY);
    ctx.rotate(angle);

    // Handle (wooden)
    const handleLength = 16;
    const handleWidth = 2;
    ctx.fillStyle = '#8B4513'; // Brown
    ctx.fillRect(-handleWidth / 2, 0, handleWidth, handleLength);

    // Hammer head (metallic gray)
    const hammerHeadWidth = 8;
    const hammerHeadHeight = 6;
    ctx.fillStyle = '#888';
    ctx.fillRect(-hammerHeadWidth / 2, handleLength - 1, hammerHeadWidth, hammerHeadHeight);

    // Metallic highlight on hammer head
    ctx.fillStyle = '#aaa';
    ctx.fillRect(-hammerHeadWidth / 2, handleLength - 1, hammerHeadWidth, 2);

    // Handle grip (darker bands)
    ctx.strokeStyle = '#5C3317';
    ctx.lineWidth = 1;
    for (let i = 0; i < 3; i++) {
      const gripY = 4 + i * 4;
      ctx.beginPath();
      ctx.moveTo(-handleWidth / 2 - 0.5, gripY);
      ctx.lineTo(handleWidth / 2 + 0.5, gripY);
      ctx.stroke();
    }

    ctx.restore();

    // Optional: Motion blur effect during fast swing (around impact point)
    if (t > 0.4 && t < 0.6) {
      ctx.save();
      ctx.globalAlpha = 0.2;
      ctx.translate(pivotX, pivotY);
      ctx.rotate(angle - 0.1);
      ctx.fillStyle = '#888';
      ctx.fillRect(-hammerHeadWidth / 2, handleLength - 1, hammerHeadWidth, hammerHeadHeight);
      ctx.restore();
    }
  }

  drawInfo(ctx, x, y, color, activity) {
    const S = CAR_SCALE;
    const carY = y + this.springY;

    // --- Directory flag: pennant on a pole from the rear spoiler ---
    const dirName = this.state.workingDir
      ? (this.state.workingDir.split('/').filter(Boolean).pop() || this.state.name || '')
      : (this.state.name || '');

    if (dirName) {
      ctx.font = 'bold 9px Courier New';
      const textW = ctx.measureText(dirName).width;
      const flagH = 13;
      const flagW = textW + 16;
      const notchDepth = 5;

      // Pole anchors at rear spoiler
      const poleBaseX = x - 15 * S;
      const poleBaseY = carY - 12 * S;
      const poleTopY = poleBaseY - 20;

      // Flag streams left from pole top (trailing behind moving car)
      const flagRight = poleBaseX;
      const flagLeft = flagRight - flagW;
      const flagTop = poleTopY;
      const flagBottom = flagTop + flagH;

      // Trailing-edge flutter: two phase-offset sine waves for natural motion
      const waveAngle = this.flagPhase * 3;
      const waveX = Math.sin(waveAngle) * 1.5;
      const waveY = Math.sin(waveAngle + 1.2);

      // Flagpole
      ctx.strokeStyle = '#aaa';
      ctx.lineWidth = 1.5;
      ctx.beginPath();
      ctx.moveTo(poleBaseX, poleBaseY);
      ctx.lineTo(poleBaseX, poleTopY - 1);
      ctx.stroke();

      // Pole cap
      ctx.fillStyle = '#ccc';
      ctx.beginPath();
      ctx.arc(poleBaseX, poleTopY - 2, 1.5, 0, Math.PI * 2);
      ctx.fill();

      // Flag shape: swallowtail pennant
      ctx.fillStyle = color.dark;
      ctx.beginPath();
      ctx.moveTo(flagRight, flagTop);
      ctx.lineTo(flagLeft + waveX, flagTop + waveY);
      ctx.lineTo(flagLeft + notchDepth + waveX * 0.6, flagTop + flagH / 2 + waveY * 0.5);
      ctx.lineTo(flagLeft + waveX, flagBottom + waveY);
      ctx.lineTo(flagRight, flagBottom);
      ctx.closePath();
      ctx.fill();

      // Top edge accent stripe
      ctx.strokeStyle = color.main;
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(flagRight, flagTop);
      ctx.lineTo(flagLeft + waveX, flagTop + waveY);
      ctx.stroke();

      // Flag text (offset right by half notch to center within swallowtail shape)
      ctx.fillStyle = '#fff';
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      const textCenterX = flagRight - flagW / 2 + notchDepth / 2 + waveX * 0.3;
      const textCenterY = flagTop + flagH / 2 + waveY * 0.3;
      ctx.fillText(dirName, textCenterX, textCenterY);
    }

    // --- Model decal on car body (door panel area) ---
    const panelX = x - 6 * S;
    const panelY = carY - 3 * S;
    ctx.fillStyle = 'rgba(255,255,255,0.8)';
    ctx.font = 'bold 9px Courier New';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(color.name.toUpperCase(), panelX, panelY);

    // --- Source badge on hood ---
    this._drawSourceBadge(ctx, x, carY);

    ctx.textBaseline = 'alphabetic';
    ctx.textAlign = 'center';
  }

  _drawSourceBadge(ctx, x, y) {
    const source = this.state.source;
    if (!source) return;

    const sc = SOURCE_COLORS[source] || DEFAULT_SOURCE;
    const S = CAR_SCALE;

    const cx = x + 14 * S;
    const cy = y - 4 * S;
    const r = 6;

    ctx.fillStyle = sc.bg;
    ctx.beginPath();
    ctx.arc(cx, cy, r, 0, Math.PI * 2);
    ctx.fill();

    ctx.strokeStyle = 'rgba(0,0,0,0.4)';
    ctx.lineWidth = 1;
    ctx.stroke();

    ctx.fillStyle = '#fff';
    ctx.font = 'bold 8px sans-serif';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(sc.label, cx, cy);
  }
}
