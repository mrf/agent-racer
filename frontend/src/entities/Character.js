import { isTerminalActivity } from '../session/constants.js';
import { getModelColor, hexToRgb } from '../session/colors.js';

const CHAR_SCALE = 2.0;

/**
 * Procedural Canvas2D pixel character for the footrace view.
 * Drop-in replacement for Racer — same public interface, different visuals.
 */
export class Character {
  constructor(state) {
    this.id = state.id;
    this.state = state;

    // Position / interpolation
    this.displayX = 0;
    this.displayY = 0;
    this.targetX = 0;
    this.targetY = 0;
    this.initialized = false;

    // Opacity (fades for lost state)
    this.opacity = 1.0;

    // Spring-based bounce
    this.springY = 0;
    this.springVel = 0;
    this.springDamping = 0.92;
    this.springStiffness = 0.15;

    // Activity transitions
    this.prevActivity = state.activity;
    this.transitionTimer = 0;
    this.glowIntensity = 0;
    this.targetGlow = 0;

    // Animation phase (drives legs/arms cycle)
    this.runPhase = 0;
    this.armPhase = 0;

    // Error multi-stage: 0=stumble, 1=spin, 2=face-plant, 3=stars
    this.errorStage = 0;
    this.errorTimer = 0;
    this.stumbleEmitted = false;
    this.starsEmitted = false;
    this.spinAngle = 0;

    // Completion effects
    this.completionTimer = 0;
    this.goldFlash = 0;
    this.confettiEmitted = false;

    // Ghost trail for 'lost' (ring buffer of {x,y})
    this.posHistory = [];

    // Thought bubble dot animation
    this.dotPhase = 0;

    // Stretch/idle bounce
    this.stretchPhase = 0;

    // Yawn timer for waiting
    this.yawnTimer = 0;
    this.yawnActive = false;
    this.headTurnPhase = 0;

    this.inPit = false;

    this.inParkingLot = false;

    // Zone transition waypoints
    this.transitionWaypoints = null;
    this.waypointIndex = 0;

    // Subagent hamsters (optional, for interface compatibility)
    this.hamsters = new Map();

    // Tmux focus state
    this.hovered = false;
    this.hasTmux = !!state.tmuxTarget;
    this.hoverGlow = 0;
    this.hoverGlowPhase = Math.random() * Math.PI * 2;
  }

  update(state) {
    const oldActivity = this.state.activity;
    const wasChurning = this.state.isChurning;
    this.state = state;
    this.hasTmux = !!state.tmuxTarget;

    // Detect churning transition
    if (state.isChurning && !wasChurning) {
      this.springVel += 1.2;
    }

    // Detect activity transition
    if (state.activity !== oldActivity) {
      this.prevActivity = oldActivity;
      this.transitionTimer = 0.3;

      // Add spring energy on transition, skip rapid thinking<->tool_use
      const bothActive = (oldActivity === 'thinking' || oldActivity === 'tool_use') &&
                         (state.activity === 'thinking' || state.activity === 'tool_use');
      if (!bothActive) {
        this.springVel += 2.5;
      }

      // Reset terminal visual effects when session resumes
      if (isTerminalActivity(oldActivity) && !isTerminalActivity(state.activity)) {
        this.opacity = 1.0;
        this.spinAngle = 0;
      }

      // Reset stage-specific flags on new activity
      if (state.activity === 'errored') {
        this.errorStage = 0;
        this.errorTimer = 0;
        this.stumbleEmitted = false;
        this.starsEmitted = false;
        this.spinAngle = 0;
      } else if (state.activity === 'complete') {
        this.confettiEmitted = false;
        this.completionTimer = 0;
        this.goldFlash = 0;
      }
    }

    // Sync hamsters from subagents
    const subagents = state.subagents || [];
    const activeIds = new Set(subagents.map(s => s.id));

    for (const id of this.hamsters.keys()) {
      if (!activeIds.has(id)) this.hamsters.delete(id);
    }
    for (const sub of subagents) {
      if (this.hamsters.has(sub.id)) {
        this.hamsters.get(sub.id).update(sub);
      } else {
        // Hamster import is optional — characters may not have subagent entities
        this.hamsters.set(sub.id, { state: sub, displayX: 0, displayY: 0, opacity: 1 });
      }
    }

    // Remove fully-faded completed hamsters
    for (const [id, hamster] of this.hamsters) {
      if (hamster.state && hamster.state.activity === 'complete' && hamster.fadeTimer > 8) {
        this.hamsters.delete(id);
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

    const speed = Math.abs(this.displayX - prevX);

    // Spring-based bounce
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

    // Hover glow interpolation
    const hoverTarget = this.hovered && this.hasTmux ? 1 : 0;
    this.hoverGlow += (hoverTarget - this.hoverGlow) * 0.15 * dtScale;
    if (this.hoverGlow > 0.01) this.hoverGlowPhase += 0.04 * dtScale;

    this.dotPhase += 0.04 * dtScale;

    // Store position history for ghost trail
    this.posHistory.push({ x: this.displayX, y: this.displayY });
    if (this.posHistory.length > 4) this.posHistory.shift();

    const activity = this.state.activity;

    // Burn rate intensity
    const burnRate = this.state.burnRatePerMinute || 0;
    let burnIntensity = 0;
    if (burnRate > 5000) burnIntensity = 3;
    else if (burnRate > 2000) burnIntensity = 2;
    else if (burnRate > 500) burnIntensity = 1;

    switch (activity) {
      case 'thinking':
        // Running: arms pumping, legs cycling, lean forward
        this.targetGlow = 0.08 + burnIntensity * 0.02;
        this.runPhase += 0.12 * dtScale;
        this.armPhase += 0.12 * dtScale;
        if (particles && speed > 0.5 && Math.random() > 0.7) {
          particles.emit('dust', this.displayX - 10 * CHAR_SCALE, this.displayY + 14 * CHAR_SCALE, 1);
        }
        break;

      case 'tool_use':
        // Sprinting: exaggerated motion, speed lines
        this.targetGlow = 0.12 + burnIntensity * 0.02;
        this.runPhase += 0.2 * dtScale;
        this.armPhase += 0.2 * dtScale;
        if (particles && speed > 0.5) {
          if (Math.random() > 0.5) {
            particles.emit('dust', this.displayX - 12 * CHAR_SCALE, this.displayY + 14 * CHAR_SCALE, 2);
          }
          if (speed > 1.5) {
            const baseC = getModelColor(this.state.model, this.state.source);
            const rgb = hexToRgb(baseC.main);
            particles.emitWithColor('speedLines', this.displayX - 16 * CHAR_SCALE, this.displayY, 1, rgb);
          }
        }
        break;

      case 'waiting':
        // Standing: head turns, occasional yawn
        this.targetGlow = 0.05;
        this.headTurnPhase += 0.03 * dtScale;
        this.yawnTimer += dt || 1 / 60;
        if (this.yawnActive) {
          if (this.yawnTimer > 1.5) {
            this.yawnActive = false;
            this.yawnTimer = 0;
          }
        } else if (this.yawnTimer > 4) {
          this.yawnActive = true;
          this.yawnTimer = 0;
        }
        break;

      case 'complete':
        // Celebrating: arms raised, jumping
        this.completionTimer += dt || 1 / 60;
        this.goldFlash = this.completionTimer < 2 ?
          0.5 + 0.5 * Math.sin(this.completionTimer * 8) : 1.0;
        if (!this.confettiEmitted && particles) {
          particles.emit('celebration', this.displayX, this.displayY, 60);
          this.confettiEmitted = true;
        }
        this.targetGlow = 0.15;
        this.runPhase += 0.15 * dtScale; // Jumping animation
        break;

      case 'errored':
        // Tripping: stumble, face-plant, stars
        this.errorTimer += dt || 1 / 60;
        if (this.errorTimer < 0.5) {
          this.errorStage = 0;
          if (!this.stumbleEmitted && particles) {
            particles.emit('dust', this.displayX, this.displayY + 14 * CHAR_SCALE, 5);
            this.stumbleEmitted = true;
          }
          this.spinAngle += 0.05 * dtScale;
        } else if (this.errorTimer < 1.0) {
          this.errorStage = 1;
          this.spinAngle += 0.2 * dtScale;
        } else if (this.errorTimer < 1.5) {
          this.errorStage = 2;
          if (!this.starsEmitted && particles) {
            particles.emit('stars', this.displayX, this.displayY - 10 * CHAR_SCALE, 8);
            this.starsEmitted = true;
          }
          this.spinAngle += 0.1 * dtScale;
        } else {
          this.errorStage = 3;
          this.spinAngle += 0.02 * dtScale;
        }
        break;

      case 'lost':
        // Ghost: translucent, slow walk
        this.opacity = Math.max(0.2, this.opacity - 0.005 * dtScale);
        this.targetGlow = 0;
        this.runPhase += 0.03 * dtScale;
        break;

      default:
        // idle/starting: stretching, gentle bounce, jogging in place
        this.targetGlow = 0;
        this.stretchPhase += 0.06 * dtScale;
        this.runPhase += 0.04 * dtScale;
    }

    // Churning animation
    if (this.state.isChurning && !this.inParkingLot && activity !== 'thinking' && activity !== 'tool_use') {
      this.runPhase += 0.02 * dtScale;
      this.springVel += (Math.random() - 0.5) * 0.3;
      this.targetGlow = 0.04;
    }

    // Suppress effects in pit/parking lot
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

    ctx.globalAlpha = this.opacity;

    // Error spin
    if (activity === 'errored') {
      ctx.translate(x, y);
      ctx.rotate(this.spinAngle);
      ctx.translate(-x, -y);
    }

    const yOff = this.springY;

    // Ghost trail for 'lost'
    if (activity === 'lost' && this.posHistory.length > 1) {
      for (let i = 0; i < this.posHistory.length - 1; i++) {
        const pos = this.posHistory[i];
        const ghostAlpha = (i / this.posHistory.length) * 0.15;
        ctx.save();
        ctx.globalAlpha = ghostAlpha;
        this._drawCharacter(ctx, pos.x, pos.y, color, activity);
        ctx.restore();
      }
    }

    // Shadow
    const S = CHAR_SCALE;
    ctx.fillStyle = 'rgba(0,0,0,0.18)';
    ctx.beginPath();
    ctx.ellipse(x, y + 15 * S, 7 * S, 2 * S, 0, 0, Math.PI * 2);
    ctx.fill();

    // Glow aura
    if (this.glowIntensity > 0.01) {
      const glowColor = hexToRgb(color.main);
      const glowR = 25 * S;
      const glow = ctx.createRadialGradient(x, y + yOff, 0, x, y + yOff, glowR);
      glow.addColorStop(0, `rgba(${glowColor.r},${glowColor.g},${glowColor.b},${this.glowIntensity})`);
      glow.addColorStop(1, `rgba(${glowColor.r},${glowColor.g},${glowColor.b},0)`);
      ctx.fillStyle = glow;
      ctx.beginPath();
      ctx.arc(x, y + yOff, glowR, 0, Math.PI * 2);
      ctx.fill();
    }

    // Draw the character
    this._drawCharacter(ctx, x, y + yOff, color, activity);

    // Activity indicators
    this._drawActivityIndicator(ctx, x, y + yOff, color, activity);

    // Hover glow for tmux-focusable sessions
    if (this.hoverGlow > 0.01) {
      const glowColor = hexToRgb(color.light);
      const pulse = 0.3 + 0.15 * Math.sin(this.hoverGlowPhase);
      const glowAlpha = this.hoverGlow * pulse;
      const glowR = 22 * S;
      const glow = ctx.createRadialGradient(x, y + yOff, 0, x, y + yOff, glowR);
      glow.addColorStop(0, `rgba(${glowColor.r},${glowColor.g},${glowColor.b},${glowAlpha})`);
      glow.addColorStop(1, `rgba(${glowColor.r},${glowColor.g},${glowColor.b},0)`);
      ctx.fillStyle = glow;
      ctx.beginPath();
      ctx.arc(x, y + yOff, glowR, 0, Math.PI * 2);
      ctx.fill();
    }

    ctx.restore();
  }

  _drawCharacter(ctx, x, y, color, activity) {
    const S = CHAR_SCALE;

    ctx.save();

    // Lean forward when running/sprinting; face-plant for error stage 2+
    let lean = 0;
    if (activity === 'errored' && this.errorStage >= 2) lean = 0.5;
    else if (activity === 'tool_use') lean = 0.2;
    else if (activity === 'thinking') lean = 0.12;

    // Celebration jump offset
    let jumpOffset = 0;
    if (activity === 'complete') {
      jumpOffset = -Math.abs(Math.sin(this.runPhase * 2)) * 6 * S;
    }

    ctx.translate(x, y + jumpOffset);
    if (lean) ctx.rotate(lean);

    // --- Legs ---
    const legSwing = Math.sin(this.runPhase * 2) * this._getLegAmplitude(activity);
    const legY = 5 * S;
    const legLen = 9 * S;

    ctx.strokeStyle = color.dark;
    ctx.lineWidth = 2.5 * S;
    ctx.lineCap = 'round';

    // Left leg
    ctx.beginPath();
    ctx.moveTo(- 2 * S, legY);
    ctx.lineTo(- 2 * S + Math.sin(legSwing) * legLen * 0.5, legY + Math.cos(legSwing) * legLen);
    ctx.stroke();

    // Right leg (opposite phase)
    ctx.beginPath();
    ctx.moveTo(2 * S, legY);
    ctx.lineTo(2 * S + Math.sin(-legSwing) * legLen * 0.5, legY + Math.cos(-legSwing) * legLen);
    ctx.stroke();

    // --- Torso (rectangle with jersey) ---
    const torsoW = 10 * S;
    const torsoH = 10 * S;
    const torsoX = -torsoW / 2;
    const torsoY = -5 * S;

    // Jersey (model color)
    let jerseyColor = color.main;
    if (activity === 'errored' && this.errorStage >= 3) {
      jerseyColor = '#555';
    } else if (activity === 'complete' && this.goldFlash > 0) {
      const mc = hexToRgb(color.main);
      const gold = { r: 255, g: 215, b: 0 };
      const f = this.goldFlash;
      jerseyColor = `rgb(${Math.round(mc.r + (gold.r - mc.r) * f)},${Math.round(mc.g + (gold.g - mc.g) * f)},${Math.round(mc.b + (gold.b - mc.b) * f)})`;
    }

    ctx.fillStyle = jerseyColor;
    ctx.fillRect(torsoX, torsoY, torsoW, torsoH);
    ctx.strokeStyle = color.dark;
    ctx.lineWidth = 0.8 * S;
    ctx.strokeRect(torsoX, torsoY, torsoW, torsoH);

    // Number bib
    this._drawBib(ctx, 0, 0, S, color);

    // --- Arms ---
    const armSwing = Math.sin(this.armPhase * 2) * this._getArmAmplitude(activity);
    const shoulderY = -3 * S;
    const armLen = 8 * S;

    ctx.strokeStyle = color.dark;
    ctx.lineWidth = 2 * S;
    ctx.lineCap = 'round';

    if (activity === 'complete') {
      // Arms raised for celebration
      ctx.beginPath();
      ctx.moveTo(-torsoW / 2, shoulderY);
      ctx.lineTo(-torsoW / 2 - 4 * S, shoulderY - armLen + Math.sin(this.runPhase * 3) * 2 * S);
      ctx.stroke();

      ctx.beginPath();
      ctx.moveTo(torsoW / 2, shoulderY);
      ctx.lineTo(torsoW / 2 + 4 * S, shoulderY - armLen + Math.sin(this.runPhase * 3 + 1) * 2 * S);
      ctx.stroke();
    } else {
      // Normal arm pump
      ctx.beginPath();
      ctx.moveTo(-torsoW / 2, shoulderY);
      ctx.lineTo(-torsoW / 2 + Math.sin(armSwing) * armLen * 0.4, shoulderY + Math.cos(armSwing) * armLen * 0.6);
      ctx.stroke();

      ctx.beginPath();
      ctx.moveTo(torsoW / 2, shoulderY);
      ctx.lineTo(torsoW / 2 + Math.sin(-armSwing) * armLen * 0.4, shoulderY + Math.cos(-armSwing) * armLen * 0.6);
      ctx.stroke();
    }

    // --- Head ---
    const headR = 5 * S;
    const headY = -5 * S - headR;

    // Head turn for waiting
    let headOffsetX = 0;
    if (activity === 'waiting') {
      headOffsetX = Math.sin(this.headTurnPhase) * 2 * S;
    }

    // Head circle (skin color)
    ctx.fillStyle = '#fcd5b0';
    ctx.beginPath();
    ctx.arc(headOffsetX, headY, headR, 0, Math.PI * 2);
    ctx.fill();
    ctx.strokeStyle = '#d4a574';
    ctx.lineWidth = 0.6 * S;
    ctx.stroke();

    // Eyes
    const eyeY = headY - 1 * S;
    ctx.fillStyle = '#333';
    ctx.beginPath();
    ctx.arc(headOffsetX - 2 * S, eyeY, 0.8 * S, 0, Math.PI * 2);
    ctx.fill();
    ctx.beginPath();
    ctx.arc(headOffsetX + 2 * S, eyeY, 0.8 * S, 0, Math.PI * 2);
    ctx.fill();

    // Yawn mouth for waiting
    if (activity === 'waiting' && this.yawnActive) {
      ctx.fillStyle = '#333';
      ctx.beginPath();
      ctx.ellipse(headOffsetX, headY + 2 * S, 1.5 * S, 2 * S, 0, 0, Math.PI * 2);
      ctx.fill();
    }

    // Headband (model color)
    ctx.strokeStyle = color.main;
    ctx.lineWidth = 2 * S;
    ctx.beginPath();
    ctx.arc(headOffsetX, headY, headR + 0.5 * S, -Math.PI * 0.8, -Math.PI * 0.2);
    ctx.stroke();

    // Headband tail (flowing behind)
    const tailLen = 4 * S;
    const tailWave = Math.sin(this.runPhase * 2) * 1.5 * S;
    const bandEndX = headOffsetX - (headR + 0.5 * S) * Math.cos(Math.PI * 0.8);
    const bandEndY = headY - (headR + 0.5 * S) * Math.sin(Math.PI * 0.8);
    ctx.strokeStyle = color.main;
    ctx.lineWidth = 1.5 * S;
    ctx.beginPath();
    ctx.moveTo(bandEndX, bandEndY);
    ctx.quadraticCurveTo(bandEndX - tailLen * 0.5, bandEndY + tailWave, bandEndX - tailLen, bandEndY + tailWave * 0.5);
    ctx.stroke();

    ctx.restore();
  }

  _drawBib(ctx, cx, cy, S, color) {
    // White bib rectangle on torso
    const bibW = 7 * S;
    const bibH = 5 * S;
    ctx.fillStyle = '#fff';
    ctx.fillRect(cx - bibW / 2, cy - bibH / 2 - 1 * S, bibW, bibH);
    ctx.strokeStyle = '#ccc';
    ctx.lineWidth = 0.3 * S;
    ctx.strokeRect(cx - bibW / 2, cy - bibH / 2 - 1 * S, bibW, bibH);

    // Session name abbreviation
    const abbrev = this._getNameAbbrev();
    ctx.fillStyle = '#333';
    ctx.font = `bold ${3 * S}px monospace`;
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(abbrev, cx, cy - 0.5 * S);
  }

  _getNameAbbrev() {
    const name = this.state.name || this.state.id || '?';
    // Take first 3 chars, uppercase
    return name.slice(0, 3).toUpperCase();
  }

  _getLegAmplitude(activity) {
    switch (activity) {
      case 'tool_use': return 0.8;  // Sprinting — big strides
      case 'thinking': return 0.5;  // Running
      case 'complete': return 0.3;  // Jumping celebration
      case 'lost': return 0.15;     // Ghost slow walk
      default: return 0.2;          // Idle jog
    }
  }

  _getArmAmplitude(activity) {
    switch (activity) {
      case 'tool_use': return 0.9;  // Exaggerated pumping
      case 'thinking': return 0.6;  // Normal pumping
      case 'complete': return 0;    // Arms raised (handled separately)
      case 'lost': return 0.1;      // Slow swing
      default: return 0.2;          // Gentle
    }
  }

  _drawActivityIndicator(ctx, x, y, color, activity) {
    const S = CHAR_SCALE;

    switch (activity) {
      case 'thinking':
        this._drawThoughtBubble(ctx, x + 8 * S, y - 18 * S, S);
        break;
      case 'tool_use':
        this._drawWrenchBadge(ctx, x + 10 * S, y - 16 * S, S);
        break;
      case 'errored':
        if (this.errorStage >= 2) {
          this._drawStars(ctx, x, y - 16 * S, S);
        }
        break;
      case 'complete':
        this._drawCheckerFlag(ctx, x + 12 * S, y - 14 * S, S);
        break;
      case 'waiting':
        this._drawZzz(ctx, x + 6 * S, y - 20 * S, S);
        break;
    }
  }

  _drawThoughtBubble(ctx, x, y, S) {
    ctx.save();
    ctx.fillStyle = 'rgba(255,255,255,0.92)';
    ctx.strokeStyle = '#bbb';
    ctx.lineWidth = 0.5 * S;

    // Main bubble
    const bw = 12 * S;
    const bh = 7 * S;
    const br = 3 * S;
    ctx.beginPath();
    ctx.moveTo(x - bw / 2 + br, y - bh);
    ctx.lineTo(x + bw / 2 - br, y - bh);
    ctx.quadraticCurveTo(x + bw / 2, y - bh, x + bw / 2, y - bh + br);
    ctx.lineTo(x + bw / 2, y - br);
    ctx.quadraticCurveTo(x + bw / 2, y, x + bw / 2 - br, y);
    ctx.lineTo(x - bw / 2 + br, y);
    ctx.quadraticCurveTo(x - bw / 2, y, x - bw / 2, y - br);
    ctx.lineTo(x - bw / 2, y - bh + br);
    ctx.quadraticCurveTo(x - bw / 2, y - bh, x - bw / 2 + br, y - bh);
    ctx.closePath();
    ctx.fill();
    ctx.stroke();

    // Tail circles
    ctx.beginPath();
    ctx.arc(x - 4 * S, y + 2 * S, 1.5 * S, 0, Math.PI * 2);
    ctx.fill();
    ctx.stroke();
    ctx.beginPath();
    ctx.arc(x - 6 * S, y + 4 * S, 1 * S, 0, Math.PI * 2);
    ctx.fill();
    ctx.stroke();

    // Animated dots
    const dotCount = 3;
    const dotR = 1 * S;
    for (let i = 0; i < dotCount; i++) {
      const phase = (this.dotPhase + i * 0.3) % 1;
      const dotY = y - bh / 2 - Math.sin(phase * Math.PI) * 1.5 * S;
      ctx.fillStyle = '#888';
      ctx.beginPath();
      ctx.arc(x - 3 * S + i * 3 * S, dotY, dotR, 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.restore();
  }

  _drawWrenchBadge(ctx, x, y, S) {
    ctx.save();
    ctx.translate(x, y);
    ctx.rotate(-0.3);

    // Wrench handle
    ctx.strokeStyle = '#666';
    ctx.lineWidth = 1.5 * S;
    ctx.lineCap = 'round';
    ctx.beginPath();
    ctx.moveTo(0, 0);
    ctx.lineTo(0, 6 * S);
    ctx.stroke();

    // Wrench head (open-end)
    ctx.beginPath();
    ctx.moveTo(-2 * S, 0);
    ctx.lineTo(-1 * S, -1.5 * S);
    ctx.lineTo(1 * S, -1.5 * S);
    ctx.lineTo(2 * S, 0);
    ctx.stroke();

    ctx.restore();
  }

  _drawStars(ctx, x, y, S) {
    ctx.save();
    const time = this.errorTimer;
    const starCount = 4;
    const radius = 8 * S;

    for (let i = 0; i < starCount; i++) {
      const angle = (i / starCount) * Math.PI * 2 + time * 3;
      const sx = x + Math.cos(angle) * radius;
      const sy = y + Math.sin(angle) * radius * 0.5;

      ctx.fillStyle = '#ffd700';
      ctx.beginPath();
      this._starPath(ctx, sx, sy, 2 * S, 1 * S, 5);
      ctx.fill();
    }
    ctx.restore();
  }

  _starPath(ctx, cx, cy, outerR, innerR, points) {
    for (let i = 0; i < points * 2; i++) {
      const r = i % 2 === 0 ? outerR : innerR;
      const angle = (i / (points * 2)) * Math.PI * 2 - Math.PI / 2;
      const px = cx + Math.cos(angle) * r;
      const py = cy + Math.sin(angle) * r;
      if (i === 0) ctx.moveTo(px, py);
      else ctx.lineTo(px, py);
    }
    ctx.closePath();
  }

  _drawCheckerFlag(ctx, x, y, S) {
    ctx.save();
    const flagW = 8 * S;
    const flagH = 6 * S;
    const cellW = flagW / 4;
    const cellH = flagH / 3;

    // Pole
    ctx.strokeStyle = '#aaa';
    ctx.lineWidth = 1 * S;
    ctx.beginPath();
    ctx.moveTo(x, y);
    ctx.lineTo(x, y + flagH + 4 * S);
    ctx.stroke();

    // Checker pattern
    for (let row = 0; row < 3; row++) {
      for (let col = 0; col < 4; col++) {
        ctx.fillStyle = (row + col) % 2 === 0 ? '#fff' : '#000';
        ctx.fillRect(x + col * cellW, y + row * cellH, cellW, cellH);
      }
    }

    // Border
    ctx.strokeStyle = '#333';
    ctx.lineWidth = 0.3 * S;
    ctx.strokeRect(x, y, flagW, flagH);

    ctx.restore();
  }

  _drawZzz(ctx, x, y, S) {
    ctx.save();
    ctx.fillStyle = 'rgba(150,150,200,0.7)';
    ctx.font = `bold ${3 * S}px monospace`;
    ctx.textAlign = 'left';

    const phase = this.dotPhase;
    for (let i = 0; i < 3; i++) {
      const alpha = 0.4 + 0.3 * Math.sin(phase + i * 0.8);
      ctx.globalAlpha = alpha;
      ctx.fillText('Z', x + i * 3 * S, y - i * 3 * S);
    }
    ctx.restore();
  }
}
