import { Hamster } from './Hamster.js';
import { SpeechBubble } from './SpeechBubble.js';
import { getEquippedPaint, getEquippedBody, getEquippedBadge } from '../gamification/CosmeticRegistry.js';
import { TERMINAL_ACTIVITIES, isTerminalActivity } from '../session/constants.js';
import { MODEL_COLORS, DEFAULT_COLOR, SOURCE_COLORS, DEFAULT_SOURCE, getModelColor, hexToRgb, lightenHex, shortModelName } from '../session/colors.js';
import { formatBurnRate } from '../ui/formatters.js';

const CAR_SCALE = 2.3;
const LIMO_STRETCH = 35;
const FLAG_COLORS = { bg: '#ffffff', text: '#000', stripe: '#cccccc', pole: '#aaa', cap: '#ccc' };
const DIRECTORY_FLAG_MARGIN = 6;
const DIRECTORY_FLAG_MIN_FONT = 11;
const DIRECTORY_FLAG_MAX_FONT = 14;
const DIRECTORY_FLAG_MAX_WIDTH = 240;
const DIRECTORY_FLAG_MIN_WIDTH = 84;
const TRACK_COMPLETE_ALPHA = 0.72;

function clamp(value, min, max) {
  return Math.min(max, Math.max(min, value));
}

function basename(path) {
  return path.split('/').filter(Boolean).pop() || '';
}

function getCanvasViewport(ctx) {
  const canvas = ctx?.canvas;
  if (!canvas) {
    return { width: 1200, height: 800 };
  }

  const rect = typeof canvas.getBoundingClientRect === 'function'
    ? canvas.getBoundingClientRect()
    : null;

  return {
    width: rect?.width || canvas.width || 1200,
    height: rect?.height || canvas.height || 800,
  };
}

function truncateMiddleToWidth(ctx, text, maxWidth) {
  if (!text || ctx.measureText(text).width <= maxWidth) {
    return text;
  }

  const ellipsis = '...';
  let keep = text.length - 1;

  while (keep > 1) {
    const left = Math.ceil(keep / 2);
    const right = Math.floor(keep / 2);
    const candidate = `${text.slice(0, left)}${ellipsis}${text.slice(text.length - right)}`;
    if (ctx.measureText(candidate).width <= maxWidth) {
      return candidate;
    }
    keep--;
  }

  return ellipsis;
}

/**
 * Returns the index of the vertex with the smallest y value (visually highest).
 * @param {Array<{x: number, y: number}>} verts
 * @returns {number}
 */
function findTopmostIndex(verts) {
  let idx = 0;
  for (let i = 1; i < verts.length; i++) {
    if (verts[i].y < verts[idx].y) idx = i;
  }
  return idx;
}

/**
 * Convert HSL (0-360, 0-100, 0-100) to a hex color string.
 */
function hslToHex(h, s, l) {
  const sNorm = s / 100;
  const lNorm = l / 100;
  const a = sNorm * Math.min(lNorm, 1 - lNorm);
  const f = (n) => {
    const k = (n + h / 30) % 12;
    const c = lNorm - a * Math.max(Math.min(k - 3, 9 - k, 1), -1);
    return Math.round(255 * c).toString(16).padStart(2, '0');
  };
  return `#${f(0)}${f(8)}${f(4)}`;
}

/**
 * Applies equipped paint override to model color.
 * Returns { main, dark, light, name, paint } where paint is the raw paint
 * definition (or null if no paint equipped).
 */
function applyPaintOverride(baseColor, time) {
  const paint = getEquippedPaint();
  if (!paint) return { ...baseColor, paint: null };

  // Holographic: animated hue cycle, ignores base color
  if (paint.pattern === 'holographic') {
    const hue = ((time || 0) * 60) % 360;
    const main = hslToHex(hue, 80, 55);
    const dark = hslToHex(hue, 80, 40);
    const light = hslToHex(hue, 80, 70);
    return { main, dark, light, name: baseColor.name, paint };
  }

  // Racing stripe / gold stripe: keep base color, pattern applied in drawCar
  if (paint.pattern === 'racing_stripe' || paint.pattern === 'gold_stripe') {
    return { ...baseColor, paint };
  }

  // Solid / chrome / metallic / gradient / stripe: override fill + stroke
  if (paint.fill) {
    const light = lightenHex(paint.fill, 40);
    return { main: paint.fill, dark: paint.stroke, light, name: baseColor.name, paint };
  }

  return { ...baseColor, paint };
}

export { CAR_SCALE, LIMO_STRETCH };

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

    // Wheel rotation
    this.wheelAngle = 0;

    // Spring-based suspension
    this.springY = 0;
    this.springVel = 0;
    this.springDamping = 0.92;
    this.springStiffness = 0.15;

    // Activity transitions
    this.prevActivity = state.activity;
    this.transitionTimer = 0;
    this.glowIntensity = 0;
    this.targetGlow = 0;
    this.colorBrightness = 0;

    // Error multi-stage: 0=skid, 1=spin accelerate, 2=smoke, 3=darken
    this.errorStage = 0;
    this.errorTimer = 0;
    this.skidEmitted = false;

    // Completion effects
    this.completionTimer = 0;
    this.goldFlash = 0;

    // Ghost trail for 'lost' (ring buffer of {x,y})
    this.posHistory = [];

    // Thought bubble dot animation
    this.dotPhase = 0;

    // Hammer animation for tool use (0-1 progress)
    this.hammerSwing = 0;
    this.hammerActive = false;
    this.hammerImpactEmitted = false;

    // Pit lane dimming (0=normal, 1=fully dimmed)
    this.inPit = false;
    this.pitDim = 0;
    this.pitDimTarget = 0;

    // Parking lot dimming (0=normal, 1=fully dimmed)
    this.inParkingLot = false;
    this.parkingLotDim = 0;
    this.parkingLotDimTarget = 0;

    // Zone transition waypoints (track <-> pit <-> parking lot)
    this.transitionWaypoints = null;
    this.waypointIndex = 0;

    // Subagent hamsters
    this.hamsters = new Map();

    // Flag flutter animation
    this.flagPhase = Math.random() * Math.PI * 2;

    // Tmux focus state
    this.hovered = false;
    this.hasTmux = !!state.tmuxTarget;
    this.hoverGlow = 0;
    this.hoverGlowPhase = Math.random() * Math.PI * 2;

    // Cumulative damage (0.0 = pristine, 1.0 = critical)
    this.damage = 0;
    this.prevCompactionCount = state.compactionCount || 0;
    this.damageSmokeCooldown = 0;
    this.damageSteamCooldown = 0;
    this.repairFlash = 0; // bright flash during repair completion

    // Procedural scratch/dent seeds (stable per racer so overlays don't jitter)
    this._scratchSeeds = [];
    for (let i = 0; i < 12; i++) {
      this._scratchSeeds.push({
        x: Math.random() * 0.8 + 0.1,
        y: Math.random() * 0.6 + 0.2,
        angle: (Math.random() - 0.5) * 0.8,
        len: 0.05 + Math.random() * 0.1,
      });
    }
    this._dentSeeds = [];
    for (let i = 0; i < 6; i++) {
      this._dentSeeds.push({
        x: Math.random() * 0.7 + 0.15,
        y: Math.random() * 0.5 + 0.25,
        r: 0.02 + Math.random() * 0.03,
      });
    }

    // Comic speech bubble
    this.bubble = new SpeechBubble();

    // Draft/overtake mechanics
    this.draftIntensity = 0;   // 0-1: how deep in draft zone (set by RaceCanvas)
    this.overtakeFlash = 0;    // 0-1: overtake flash intensity (decays over time)
    this.position = 0;         // current race position (1-based)
    this.teamColor = null;     // hex color string if session belongs to a team
  }

  _triggerBubble(state) {
    switch (state.activity) {
      case 'thinking':
        this.bubble.show('thought', '...');
        break;
      case 'tool_use':
        this.bubble.show('speech', state.currentTool || 'tool');
        break;
      case 'waiting':
      case 'idle':
      case 'starting':
        this.bubble.show('zzz', '');
        break;
      case 'errored':
        this.bubble.show('exclamation', '!');
        break;
      case 'complete':
        this.bubble.show('complete', '\u2713');
        break;
      default:
        this.bubble.hide();
    }
  }

  update(state) {
    const oldActivity = this.state.activity;
    const oldTool = this.state.currentTool;
    const wasChurning = this.state.isChurning;

    // Detect new compactions → add damage
    const newCompactions = (state.compactionCount || 0) - this.prevCompactionCount;
    if (newCompactions > 0) {
      this.damage = Math.min(1.0, this.damage + newCompactions * 0.15);
      this.prevCompactionCount = state.compactionCount || 0;
    }

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

      // Add spring energy on transition, but skip rapid thinking<->tool_use oscillation
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
        this.skidEmitted = false;
        this.smokeEmitted = false;
        this.spinAngle = 0;
        // Errors cause significant damage (+30%)
        this.damage = Math.min(1.0, this.damage + 0.30);
      }
      if (state.activity === 'complete') {
        this.confettiEmitted = false;
        this.completionTimer = 0;
        this.goldFlash = 0;
        // Completion resets damage with repair animation
        if (this.damage > 0) {
          this.repairFlash = 1.0;
        }
        this.damage = 0;
      }
      if (state.activity === 'tool_use') {
        // Trigger hammer animation on tool_use transition
        this.hammerActive = true;
        this.hammerSwing = 0;
        this.hammerImpactEmitted = false;
      }

      this._triggerBubble(state);
    } else if (state.activity === 'tool_use' && state.currentTool && state.currentTool !== oldTool) {
      // Same activity but tool changed — refresh speech bubble text
      this.bubble.show('speech', state.currentTool);
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
        this.hamsters.set(sub.id, new Hamster(sub));
      }
    }

    // Remove fully-faded completed hamsters after grace period
    for (const [id, hamster] of this.hamsters) {
      if (hamster.state.activity === 'complete' && hamster.fadeTimer > 8) {
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

    // Speed for wheel rotation and effects
    const speed = Math.abs(this.displayX - prevX);

    // Wheel rotation (disabled for parked cars)
    if (!this.inParkingLot) {
      this.wheelAngle += speed * 0.3 * dtScale;
    }

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

    // Hover glow interpolation
    const hoverTarget = this.hovered && this.hasTmux ? 1 : 0;
    this.hoverGlow += (hoverTarget - this.hoverGlow) * 0.15 * dtScale;
    if (this.hoverGlow > 0.01) this.hoverGlowPhase += 0.04 * dtScale;

    // Pit dimming transition
    this.pitDim += (this.pitDimTarget - this.pitDim) * 0.08 * dtScale;

    // Parking lot dimming transition
    this.parkingLotDim += (this.parkingLotDimTarget - this.parkingLotDim) * 0.06 * dtScale;

    // Draft intensity decays to 0 each frame (reset by RaceCanvas each update)
    this.draftIntensity = Math.max(0, this.draftIntensity - 0.1 * dtScale);

    // Overtake flash decays over ~0.6s
    if (this.overtakeFlash > 0) {
      this.overtakeFlash = Math.max(0, this.overtakeFlash - dt / 0.6);
    }

    this.thoughtBubblePhase += 0.06 * dtScale;
    this.dotPhase += 0.04 * dtScale;
    this.flagPhase += 0.05 * dtScale;

    // Store position history for ghost trail
    this.posHistory.push({ x: this.displayX, y: this.displayY });
    if (this.posHistory.length > 4) this.posHistory.shift();

    const activity = this.state.activity;

    const S = CAR_SCALE;

    // Burn rate drives exhaust intensity: higher burn = more/bigger flames
    const burnRate = this.state.burnRatePerMinute || 0;
    const burnIntensity = burnRate > 5000 ? 3 : burnRate > 2000 ? 2 : burnRate > 500 ? 1 : 0;

    // Draft boost: drafter car emits extra turbulent exhaust
    const draftBoost = this.draftIntensity > 0.3 ? 1 : 0;

    switch (activity) {
      case 'thinking':
        this.targetGlow = 0.08 + burnIntensity * 0.02;
        if (particles && speed > 0.5) {
          const exhaustChance = 0.5 - burnIntensity * 0.1; // more likely with higher burn
          if (Math.random() > exhaustChance) {
            if (burnIntensity >= 2 && Math.random() > 0.5) {
              particles.emit('flame', this.displayX - (17 + LIMO_STRETCH) * S, this.displayY + 1 * S, 1 + burnIntensity);
            } else {
              particles.emit('exhaust', this.displayX - (17 + LIMO_STRETCH) * S, this.displayY + 1 * S, 1);
            }
          }
          if (draftBoost && Math.random() > 0.6) {
            particles.emit('draftTurbulence', this.displayX - (17 + LIMO_STRETCH) * S, this.displayY, 1);
          }
        }
        this.hammerActive = false; // Stop hammer animation
        break;

      case 'tool_use':
        this.targetGlow = 0.12 + burnIntensity * 0.02;
        this.colorBrightness = Math.min(40, this.colorBrightness + 2 * dtScale);
        if (particles) {
          const exhaustChance = 0.5 - burnIntensity * 0.1;
          if (Math.random() > exhaustChance) {
            if (burnIntensity >= 1) {
              // Hot burn: emit flames
              particles.emit('flame', this.displayX - (17 + LIMO_STRETCH) * S, this.displayY + 1 * S, 1 + burnIntensity);
              if (burnIntensity >= 2) {
                particles.emit('exhaust', this.displayX - (17 + LIMO_STRETCH) * S, this.displayY + 2 * S, 1);
              }
            } else {
              particles.emit('exhaust', this.displayX - (17 + LIMO_STRETCH) * S, this.displayY + 1 * S, 1);
            }
          }
        }
        if (particles && Math.random() > 0.6) {
          particles.emit('sparks', this.displayX + 10 * S, this.displayY + 5 * S, 1);
        }
        // Speed lines for fast movement (uses paint color if equipped)
        if (particles && speed > 1.5) {
          const baseC = getModelColor(this.state.model, this.state.source);
          const paintedC = applyPaintOverride(baseC, performance.now() / 1000);
          const rgb = hexToRgb(paintedC.main);
          particles.emitWithColor('speedLines', this.displayX - (20 + LIMO_STRETCH) * S, this.displayY, 1, rgb);
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
            particles.emit('skidMarks', this.displayX - (11 + LIMO_STRETCH) * S, this.displayY + 5 * S + 5 * S, 8);
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
    // Disabled for parked cars (terminal states in the parking lot).
    if (this.state.isChurning && !this.inParkingLot && activity !== 'thinking' && activity !== 'tool_use') {
      this.wheelAngle += 0.02 * dtScale;
      if (particles && Math.random() > 0.95) {
        particles.emit('exhaust', this.displayX - (17 + LIMO_STRETCH) * S, this.displayY + 1 * S, 1);
      }
      this.springVel += (Math.random() - 0.5) * 0.3;
      this.targetGlow = 0.04;
    }

    // Continuous damage from high burn rate (>5K/min → heat marks)
    if (burnRate > 5000) {
      this.damage = Math.min(1.0, this.damage + 0.002 * dtScale);
    }

    // Context pressure damage (>95% utilization → overheating)
    const ctxUtil = this.state.contextUtilization || 0;
    if (ctxUtil > 0.95) {
      this.damage = Math.min(1.0, this.damage + 0.003 * dtScale);
    }

    // Pit repair: heal 1%/sec
    if (this.inPit && this.damage > 0) {
      this.damage = Math.max(0, this.damage - 0.01 * (dt || 1 / 60));
    }

    // Repair flash decay
    if (this.repairFlash > 0) {
      this.repairFlash = Math.max(0, this.repairFlash - 2.0 * (dt || 1 / 60));
    }

    // Damage-based particle effects
    if (particles && this.damage > 0.25 && !this.inParkingLot) {
      this.damageSmokeCooldown -= dt || 1 / 60;
      if (this.damageSmokeCooldown <= 0) {
        // Smoke intensity scales with damage
        let smokeRate = 0.3;
        if (this.damage > 0.75) smokeRate = 0.08;
        else if (this.damage > 0.5) smokeRate = 0.15;
        this.damageSmokeCooldown = smokeRate;
        const smokeX = this.displayX + (Math.random() - 0.3) * 20 * S;
        const smokeY = this.displayY - 8 * S;
        particles.emit('damageSmoke', smokeX, smokeY, 1);
      }
    }

    // Steam from overheating (context >95%)
    if (particles && ctxUtil > 0.95 && !this.inParkingLot) {
      this.damageSteamCooldown -= dt || 1 / 60;
      if (this.damageSteamCooldown <= 0) {
        this.damageSteamCooldown = 0.12;
        const steamX = this.displayX + 10 * S;
        const steamY = this.displayY - 6 * S;
        particles.emit('steam', steamX, steamY, 2);
      }
    }

    // Damage sparks for heavily damaged cars (>50%)
    if (particles && this.damage > 0.5 && !this.inParkingLot && Math.random() > 0.92) {
      const sparkX = this.displayX + (Math.random() - 0.3) * 15 * S;
      const sparkY = this.displayY + (Math.random() - 0.5) * 8 * S;
      particles.emit('damageSparks', sparkX, sparkY, 1 + Math.floor(this.damage * 3));
    }

    // Suppress effects when in pit or parking lot
    if (this.inPit || this.inParkingLot) {
      this.targetGlow = Math.min(this.targetGlow, 0.02);
    }

    // Position and animate hamsters in fan pattern behind car
    if (this.hamsters.size > 0) {
      const count = this.hamsters.size;
      const carRearOffset = (17 + LIMO_STRETCH) * S;
      const baseX = this.displayX - carRearOffset - 80;

      // Compress spacing when count > 4, cap visual spread at lane height
      const maxSpread = 50;
      const ySpacing = count > 1 ? Math.min(25, maxSpread / (count - 1)) : 25;

      let i = 0;
      for (const hamster of this.hamsters.values()) {
        const yOffset = (i - (count - 1) / 2) * ySpacing;
        const xStagger = -i * 15;
        hamster.setTarget(baseX + xStagger, this.displayY + yOffset);
        hamster.animate(particles, dt);
        i++;
      }

      // Force-snap ropes and fade orphaned hamsters when parent is terminal
      if (isTerminalActivity(activity)) {
        for (const hamster of this.hamsters.values()) {
          if (!isTerminalActivity(hamster.state.activity)) {
            if (!hamster.ropeSnapped) {
              hamster.ropeSnapped = true;
              hamster.fadeTimer = 0;
            }
            hamster.fadeTimer += dt || 1 / 60;
            hamster.opacity = Math.max(0.3, 1.0 - (hamster.fadeTimer / 5) * 0.7);
          }
        }
      }
    }

    this.bubble.update(dt || 1 / 60);
  }

  draw(ctx) {
    const x = this.displayX;
    const y = this.displayY;
    const baseColor = getModelColor(this.state.model, this.state.source);
    const color = applyPaintOverride(baseColor, performance.now() / 1000);
    const activity = this.state.activity;

    ctx.save();

    // Zone dimming: subtle opacity reduction so details remain readable
    const pitAlpha = 1 - this.pitDim * 0.15;
    const parkingAlpha = 1 - this.parkingLotDim * 0.2;
    const isTrackComplete = this._isTrackComplete();
    const completionAlpha = isTrackComplete ? TRACK_COMPLETE_ALPHA : 1;
    ctx.globalAlpha = this.opacity * pitAlpha * parkingAlpha * completionAlpha;

    // Parking lot: mild desaturation so completed sessions are still legible
    if (this.parkingLotDim > 0.01) {
      ctx.filter = `saturate(${1 - this.parkingLotDim * 0.3})`;
    } else if (isTrackComplete) {
      ctx.filter = 'grayscale(0.35) saturate(0.45)';
    } else {
      ctx.filter = 'none';
    }

    // Draw hamsters behind the car (before error spin so they don't rotate)
    const zoneAlpha = this.opacity * pitAlpha * parkingAlpha;
    for (const hamster of this.hamsters.values()) {
      this.drawTowRope(ctx, hamster);
      const origOpacity = hamster.opacity;
      hamster.opacity *= zoneAlpha;
      hamster.draw(ctx);
      hamster.opacity = origOpacity;
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

    // Car shadow — sized to match car body extents (rear=17, front=23)
    const S = CAR_SCALE;
    const carLength = 17 + LIMO_STRETCH + 23;
    const shadowRx = carLength / 2 * S;
    const shadowCx = x + (23 - 17 - LIMO_STRETCH) / 2 * S + 2;
    ctx.fillStyle = isTrackComplete ? 'rgba(34,197,94,0.16)' : 'rgba(0,0,0,0.2)';
    ctx.beginPath();
    ctx.ellipse(shadowCx, y + 12 * S, shadowRx, 3 * S, 0, 0, Math.PI * 2);
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
    if (isTrackComplete) {
      this._drawCompletionBadge(ctx, x, y + yOff);
    }

    // Damage overlay (scratches, dents, cracks)
    if (this.damage > 0.01) {
      this._drawDamageOverlay(ctx, x, y + yOff);
    }

    // Repair flash (bright white glow on completion)
    if (this.repairFlash > 0) {
      ctx.save();
      const flashR = 40 * CAR_SCALE;
      const grad = ctx.createRadialGradient(x, y + yOff, 0, x, y + yOff, flashR);
      grad.addColorStop(0, `rgba(200,255,200,${this.repairFlash * 0.4})`);
      grad.addColorStop(1, 'rgba(200,255,200,0)');
      ctx.fillStyle = grad;
      ctx.beginPath();
      ctx.arc(x, y + yOff, flashR, 0, Math.PI * 2);
      ctx.fill();
      ctx.restore();
    }

    // Hover glow for tmux-focusable sessions
    if (this.hoverGlow > 0.01) {
      const glowColor = hexToRgb(color.light);
      const pulse = 0.3 + 0.15 * Math.sin(this.hoverGlowPhase);
      const glowAlpha = this.hoverGlow * pulse;
      const glowR = (30 + LIMO_STRETCH * 0.5) * S;
      const glowCx = x - LIMO_STRETCH * 0.3 * S;
      const glow = ctx.createRadialGradient(glowCx, y + yOff, 0, glowCx, y + yOff, glowR);
      glow.addColorStop(0, `rgba(${glowColor.r},${glowColor.g},${glowColor.b},${glowAlpha})`);
      glow.addColorStop(1, `rgba(${glowColor.r},${glowColor.g},${glowColor.b},0)`);
      ctx.fillStyle = glow;
      ctx.beginPath();
      ctx.arc(glowCx, y + yOff, glowR, 0, Math.PI * 2);
      ctx.fill();
    }

    this.drawActivityEffects(ctx, x, y + yOff, color, activity);
    this.drawInfo(ctx, x, y, color, activity);

    // Position badge (top-left of car)
    if (this.position > 0 && !this.inPit && !this.inParkingLot) {
      this._drawPositionBadge(ctx, x, y + yOff, color);
    }

    // Overtake flash: brief golden overlay on the car
    if (this.overtakeFlash > 0) {
      const flashAlpha = this.overtakeFlash * 0.55;
      ctx.save();
      ctx.globalAlpha = flashAlpha;
      ctx.fillStyle = '#ffd700';
      const carLen = (17 + LIMO_STRETCH + 23) * S;
      ctx.fillRect(x - (17 + LIMO_STRETCH) * S, y + yOff - 12 * S, carLen, 14 * S);
      ctx.restore();
    }

    // Team livery stripe: thin colored bar along the car roofline
    if (this.teamColor) {
      ctx.save();
      ctx.globalAlpha = 0.55;
      ctx.fillStyle = this.teamColor;
      // Roof spans from rear roofline to windshield in car coords; scale to screen
      const roofLeft = x - (13 + LIMO_STRETCH) * S;
      const roofRight = x + 3 * S;
      ctx.fillRect(roofLeft, y + yOff - 9 * S, roofRight - roofLeft, 3 * S);
      ctx.restore();
    }

    ctx.restore();

    // Speech bubble: drawn outside the main save/restore so it is not affected
    // by the error-spin transform or parking-lot filter.
    if (this.bubble.isVisible) {
      ctx.save();
      ctx.globalAlpha = this.opacity * (1 - this.pitDim * 0.15) * (1 - this.parkingLotDim * 0.2);
      this.bubble.draw(ctx, x, y);
      ctx.restore();
    }
  }

  _drawPositionBadge(ctx, x, y, color) {
    const pos = this.position;
    const S = CAR_SCALE;
    const badgeX = x - (17 + LIMO_STRETCH) * S;
    const badgeY = y - 12 * S - 14;
    const badgeR = 9;

    // Background circle: gold for top 3, dark for rest
    const bgColor = pos <= 3 ? '#d4a017' : 'rgba(20,20,40,0.75)';
    ctx.fillStyle = bgColor;
    ctx.beginPath();
    ctx.arc(badgeX, badgeY, badgeR, 0, Math.PI * 2);
    ctx.fill();

    // Position number
    ctx.fillStyle = pos <= 3 ? '#1a1a2e' : '#aaa';
    ctx.font = `bold ${pos >= 10 ? 7 : 9}px Courier New`;
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(`${pos}`, badgeX, badgeY);
    ctx.textBaseline = 'alphabetic';
  }

  drawCar(ctx, x, y, color, activity) {
    ctx.save();
    ctx.translate(x, y);
    ctx.scale(CAR_SCALE, CAR_SCALE);
    ctx.translate(-x, -y);

    // Determine car color (grayscale for errored stage 3, gold tint for complete)
    const paint = color.paint;
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
    const L = LIMO_STRETCH;
    const rearWheelX = x - 11 - L;
    const midWheelX = x - 11;
    const frontWheelX = x + 12;
    const wheelY = y + 5;
    const wheelR = 5;
    this._drawWheel(ctx, rearWheelX, wheelY, wheelR);
    this._drawWheel(ctx, midWheelX, wheelY, wheelR);
    this._drawWheel(ctx, frontWheelX, wheelY, wheelR);

    // --- Car body - side profile racing car facing right ---
    // Chrome / metallic paints use a vertical gradient for a reflective look
    if (paint && (paint.pattern === 'chrome' || paint.pattern === 'metallic') &&
        activity !== 'errored') {
      const grad = ctx.createLinearGradient(x, y - 9, x, y + 2);
      const highlight = lightenHex(color.main, 80);
      grad.addColorStop(0, highlight);
      grad.addColorStop(0.4, color.main);
      grad.addColorStop(0.6, color.dark);
      grad.addColorStop(0.8, color.main);
      grad.addColorStop(1, highlight);
      ctx.fillStyle = grad;
    } else {
      ctx.fillStyle = bodyColor;
    }
    const bodyVerts = getEquippedBody(L);
    ctx.beginPath();
    if (bodyVerts) {
      this._tracePolygon(ctx, x, y, bodyVerts);
    } else {
      ctx.moveTo(x - 17 - L, y + 2);    // rear bottom (limo)
      ctx.lineTo(x - 17 - L, y - 3);    // rear face
      ctx.lineTo(x - 13 - L, y - 7);    // rear roofline
      ctx.lineTo(x - 4 - L, y - 9);     // roof
      ctx.lineTo(x + 3, y - 9);         // roof front
      ctx.lineTo(x + 9, y - 5);         // windshield slope
      ctx.lineTo(x + 15, y - 3);        // hood
      ctx.lineTo(x + 21, y - 1);        // nose top
      ctx.quadraticCurveTo(x + 23, y, x + 21, y + 1); // nose tip curve
      ctx.lineTo(x + 18, y + 2);        // front bottom
    }
    ctx.closePath();
    ctx.fill();

    // Body outline
    ctx.strokeStyle = color.dark;
    ctx.lineWidth = 1;
    ctx.stroke();

    // Lower panel / side skirt (darker shade for depth)
    ctx.fillStyle = color.dark;
    ctx.beginPath();
    if (bodyVerts) {
      const rear = bodyVerts[0];
      const front = bodyVerts[bodyVerts.length - 1];
      const frontPrev = bodyVerts[bodyVerts.length - 2];
      ctx.moveTo(x + rear.x, y + rear.y);
      ctx.lineTo(x + rear.x, y);
      ctx.lineTo(x + frontPrev.x, y);
      ctx.lineTo(x + frontPrev.x, y + frontPrev.y);
      ctx.lineTo(x + front.x, y + front.y);
    } else {
      ctx.moveTo(x - 17 - L, y + 2);
      ctx.lineTo(x - 17 - L, y);
      ctx.lineTo(x + 19, y);
      ctx.lineTo(x + 21, y + 1);
      ctx.lineTo(x + 18, y + 2);
    }
    ctx.closePath();
    ctx.fill();

    // Top edge highlight
    ctx.strokeStyle = lightenHex(color.main, 35);
    ctx.lineWidth = 1;
    ctx.beginPath();
    if (bodyVerts) {
      const topIdx = findTopmostIndex(bodyVerts);
      const start = Math.max(0, topIdx - 1);
      const end = Math.min(bodyVerts.length - 1, topIdx + 1);
      ctx.moveTo(x + bodyVerts[start].x, y + bodyVerts[start].y);
      for (let i = start + 1; i <= end; i++) {
        ctx.lineTo(x + bodyVerts[i].x, y + bodyVerts[i].y);
      }
    } else {
      ctx.moveTo(x - 12 - L, y - 7);
      ctx.lineTo(x - 4 - L, y - 9);
      ctx.lineTo(x + 3, y - 9);
    }
    ctx.stroke();

    // Racing stripe (paint overrides: racing_stripe uses white, gold_stripe uses gold)
    if (paint && (paint.pattern === 'racing_stripe' || paint.pattern === 'gold_stripe')) {
      ctx.strokeStyle = paint.stripeColor;
      ctx.lineWidth = 2.5;
    } else {
      ctx.strokeStyle = lightenHex(color.main, 60);
      ctx.lineWidth = 1.5;
    }
    ctx.beginPath();
    if (bodyVerts) {
      ctx.moveTo(x + bodyVerts[0].x + 2, y - 2);
      ctx.lineTo(x + bodyVerts[bodyVerts.length - 2].x, y - 2);
    } else {
      ctx.moveTo(x - 15 - L, y - 2);
      ctx.lineTo(x + 19, y - 2);
    }
    ctx.stroke();

    // Windshield, limo windows, and chrome trim — skip for custom bodies
    if (!bodyVerts) {
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

      // Limo passenger windows along the stretched body
      const winY = y - 7;
      const winH = 5;
      const winW = 6;
      const winGap = 2;
      const winStartX = x - L + 2;
      const numWindows = Math.floor((L - 4) / (winW + winGap));
      ctx.fillStyle = 'rgba(100,180,255,0.25)';
      for (let i = 0; i < numWindows; i++) {
        const wx = winStartX + i * (winW + winGap);
        const wr = 1.5;
        ctx.beginPath();
        ctx.moveTo(wx + wr, winY);
        ctx.lineTo(wx + winW - wr, winY);
        ctx.quadraticCurveTo(wx + winW, winY, wx + winW, winY + wr);
        ctx.lineTo(wx + winW, winY + winH - wr);
        ctx.quadraticCurveTo(wx + winW, winY + winH, wx + winW - wr, winY + winH);
        ctx.lineTo(wx + wr, winY + winH);
        ctx.quadraticCurveTo(wx, winY + winH, wx, winY + winH - wr);
        ctx.lineTo(wx, winY + wr);
        ctx.quadraticCurveTo(wx, winY, wx + wr, winY);
        ctx.closePath();
        ctx.fill();
      }
      // Chrome trim along window line
      ctx.strokeStyle = 'rgba(200,200,220,0.5)';
      ctx.lineWidth = 0.5;
      ctx.beginPath();
      ctx.moveTo(x - L + 1, winY + winH + 1);
      ctx.lineTo(x + 2, winY + winH + 1);
      ctx.stroke();
    }

    // Headlight — position at front of body
    const headlightX = bodyVerts ? bodyVerts[bodyVerts.length - 2].x : 20;
    ctx.fillStyle = 'rgba(255,255,220,0.7)';
    ctx.beginPath();
    ctx.arc(x + headlightX, y, 2, 0, Math.PI * 2);
    ctx.fill();

    // Taillight — position at rear of body
    const rearX = bodyVerts ? bodyVerts[1].x : -17 - L;
    ctx.fillStyle = 'rgba(255,30,30,0.7)';
    ctx.beginPath();
    ctx.arc(x + rearX, y - 1, 2, 0, Math.PI * 2);
    ctx.fill();

    // Exhaust pipe
    ctx.fillStyle = '#333';
    ctx.beginPath();
    ctx.arc(x + rearX, y + 1, 1.5, 0, Math.PI * 2);
    ctx.fill();

    // X overlay for errored stage 3
    if (activity === 'errored' && this.errorStage >= 3) {
      ctx.strokeStyle = '#e94560';
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.moveTo(x - 14 - L, y - 8);
      ctx.lineTo(x + 16, y + 4);
      ctx.stroke();
      ctx.beginPath();
      ctx.moveTo(x + 16, y - 8);
      ctx.lineTo(x - 14 - L, y + 4);
      ctx.stroke();
    }

    ctx.restore();
  }

  _tracePolygon(ctx, cx, cy, verts) {
    ctx.moveTo(cx + verts[0].x, cy + verts[0].y);
    for (let i = 1; i < verts.length; i++) {
      ctx.lineTo(cx + verts[i].x, cy + verts[i].y);
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
        // Suppressed when comic speech bubbles are on (SpeechBubble handles it)
        if (!SpeechBubble.enabled) this._drawThoughtBubble(ctx, x, y);
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
        break;

      case 'complete':
        if (!this._isTrackComplete()) {
          this._drawCheckerFlag(ctx, x, y);
        }
        break;
    }

    // Show warning triangle only when churning contradicts the activity
    // (e.g. idle/waiting + CPU active is unexpected; thinking + CPU active is normal)
    if (this.state.isChurning && activity !== 'thinking' && activity !== 'tool_use') {
      this._drawWarningTriangle(ctx, x, y);
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
      [x - (17 + LIMO_STRETCH) * S, y - 1 * S],  // rear
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

  _drawCompletionBadge(ctx, x, y) {
    const outerR = 16;
    const innerR = 12;
    const badgeX = x + 3;
    const badgeY = y - 1;

    ctx.save();

    const halo = ctx.createRadialGradient(badgeX, badgeY, 0, badgeX, badgeY, 30);
    halo.addColorStop(0, 'rgba(34,197,94,0.22)');
    halo.addColorStop(1, 'rgba(34,197,94,0)');
    ctx.fillStyle = halo;
    ctx.beginPath();
    ctx.arc(badgeX, badgeY, 30, 0, Math.PI * 2);
    ctx.fill();

    ctx.fillStyle = 'rgba(20,83,45,0.92)';
    ctx.beginPath();
    ctx.arc(badgeX, badgeY, outerR, 0, Math.PI * 2);
    ctx.fill();

    ctx.strokeStyle = 'rgba(220,252,231,0.95)';
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.arc(badgeX, badgeY, outerR - 1, 0, Math.PI * 2);
    ctx.stroke();

    ctx.fillStyle = 'rgba(240,253,244,0.96)';
    ctx.beginPath();
    ctx.arc(badgeX, badgeY, innerR, 0, Math.PI * 2);
    ctx.fill();

    const poleX = badgeX - 8;
    const poleTop = badgeY - 7;
    const cell = 3;

    ctx.strokeStyle = '#166534';
    ctx.lineWidth = 1.2;
    ctx.beginPath();
    ctx.moveTo(poleX, badgeY + 8);
    ctx.lineTo(poleX, poleTop);
    ctx.stroke();

    for (let row = 0; row < 3; row++) {
      for (let col = 0; col < 4; col++) {
        ctx.fillStyle = (row + col) % 2 === 0 ? '#ffffff' : '#111827';
        ctx.fillRect(poleX + col * cell, poleTop + row * cell, cell, cell);
      }
    }

    ctx.restore();
  }

  _drawHammer(ctx, x, y) {
    const S = CAR_SCALE;

    // Swing angle: -70deg (raised) -> 0deg (impact) with easeInOutQuad
    const t = this.hammerSwing;
    const easeInOutQuad = (v) => v < 0.5 ? 2 * v * v : 1 - Math.pow(-2 * v + 2, 2) / 2;
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
    ctx.fillStyle = '#8B4513';
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

    // Motion blur during fast swing near impact point
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

  _drawDamageOverlay(ctx, x, y) {
    const S = CAR_SCALE;
    const L = LIMO_STRETCH;
    const dmg = this.damage;

    ctx.save();
    ctx.translate(x, y);
    ctx.scale(S, S);

    // Car body bounds in local space (approx)
    const bodyLeft = -17 - L;
    const bodyRight = 21;
    const bodyTop = -9;
    const bodyBottom = 2;
    const bodyW = bodyRight - bodyLeft;
    const bodyH = bodyBottom - bodyTop;

    // Tier 1 (0-25%): minor scratches
    if (dmg > 0) {
      const scratchCount = Math.min(this._scratchSeeds.length, Math.ceil(dmg * 12));
      ctx.strokeStyle = `rgba(255,255,255,${Math.min(0.35, dmg * 0.5)})`;
      ctx.lineWidth = 0.5;
      for (let i = 0; i < scratchCount; i++) {
        const s = this._scratchSeeds[i];
        const sx = bodyLeft + s.x * bodyW;
        const sy = bodyTop + s.y * bodyH;
        const len = s.len * bodyW;
        ctx.beginPath();
        ctx.moveTo(sx, sy);
        ctx.lineTo(sx + Math.cos(s.angle) * len, sy + Math.sin(s.angle) * len);
        ctx.stroke();
      }
    }

    // Tier 2 (25-50%): dents + faint smoke tint
    if (dmg > 0.25) {
      const dentAlpha = Math.min(0.3, (dmg - 0.25) * 0.6);
      const dentCount = Math.min(this._dentSeeds.length, Math.ceil((dmg - 0.25) * 8));
      for (let i = 0; i < dentCount; i++) {
        const d = this._dentSeeds[i];
        const dx = bodyLeft + d.x * bodyW;
        const dy = bodyTop + d.y * bodyH;
        const dr = d.r * bodyW;
        // Dark dent shadow
        ctx.fillStyle = `rgba(0,0,0,${dentAlpha})`;
        ctx.beginPath();
        ctx.ellipse(dx, dy, dr, dr * 0.6, 0, 0, Math.PI * 2);
        ctx.fill();
        // Highlight edge
        ctx.strokeStyle = `rgba(255,255,255,${dentAlpha * 0.5})`;
        ctx.lineWidth = 0.3;
        ctx.beginPath();
        ctx.arc(dx - dr * 0.3, dy - dr * 0.3, dr * 0.5, Math.PI * 1.2, Math.PI * 1.8);
        ctx.stroke();
      }
    }

    // Tier 3 (50-75%): cracked windshield + heavy marks
    if (dmg > 0.5) {
      const crackAlpha = Math.min(0.6, (dmg - 0.5) * 1.2);
      // Windshield crack pattern radiating from center (local x 3..9, y -9..-5)
      const crackOrigin = [6, -7];
      const crackEnds = [[4, -5.5], [8, -6], [5, -8.5], [7.5, -8]];
      ctx.strokeStyle = `rgba(200,220,255,${crackAlpha})`;
      ctx.lineWidth = 0.6;
      ctx.beginPath();
      for (let i = 0; i < crackEnds.length; i++) {
        ctx.moveTo(crackOrigin[0], crackOrigin[1]);
        ctx.lineTo(crackEnds[i][0], crackEnds[i][1]);
      }
      ctx.stroke();

      // Dark scorch marks on body
      const scorchAlpha = (dmg - 0.5) * 0.4;
      ctx.fillStyle = `rgba(30,20,10,${scorchAlpha})`;
      ctx.beginPath();
      ctx.ellipse(-5 - L * 0.5, -3, 8, 3, 0.2, 0, Math.PI * 2);
      ctx.fill();
      ctx.beginPath();
      ctx.ellipse(8, -1, 5, 2, -0.1, 0, Math.PI * 2);
      ctx.fill();
    }

    // Tier 4 (75-100%): loose panels + heavy damage overlay
    if (dmg > 0.75) {
      const critAlpha = Math.min(0.5, (dmg - 0.75) * 1.0);

      // Loose panel gaps (dark lines suggesting panels separating)
      ctx.strokeStyle = `rgba(20,15,10,${critAlpha})`;
      ctx.lineWidth = 0.8;
      ctx.beginPath();
      ctx.moveTo(-4, bodyTop);
      ctx.lineTo(-4, bodyTop + 3);
      ctx.stroke();
      ctx.beginPath();
      ctx.moveTo(-10 - L * 0.3, bodyTop + 2);
      ctx.lineTo(-10 - L * 0.3, bodyBottom - 1);
      ctx.stroke();

      // Panel offset (slight shift to suggest looseness)
      const jitter = Math.sin(performance.now() * 0.01) * 0.3;
      ctx.fillStyle = `rgba(60,40,20,${critAlpha * 0.3})`;
      ctx.fillRect(bodyLeft + 2 + jitter, bodyTop + 1, 6, 4);

      // Overall damage tint
      ctx.fillStyle = `rgba(40,20,0,${critAlpha * 0.15})`;
      ctx.fillRect(bodyLeft, bodyTop, bodyW, bodyH);
    }

    ctx.restore();
  }

  drawInfo(ctx, x, y, color, activity) {
    const S = CAR_SCALE;
    const carY = y + this.springY;
    const dirName = this._getDirectoryFlagLabel();

    if (dirName) {
      const badge = getEquippedBadge();
      const layout = this._getDirectoryFlagLayout(ctx, x, y, dirName, badge?.emoji || '');

      // Trailing-edge flutter: two phase-offset sine waves for natural motion
      const waveAngle = this.flagPhase * 3;
      const waveX = Math.sin(waveAngle) * 1.5;
      const waveY = Math.sin(waveAngle + 1.2);

      // Flagpole
      ctx.strokeStyle = FLAG_COLORS.pole;
      ctx.lineWidth = 1.5;
      ctx.beginPath();
      ctx.moveTo(layout.poleBaseX, layout.poleBaseY);
      ctx.lineTo(layout.poleBaseX, layout.poleTopY - 1);
      ctx.stroke();

      if (Math.abs(layout.flagRight - layout.poleBaseX) > 1) {
        ctx.beginPath();
        ctx.moveTo(layout.poleBaseX, layout.poleTopY);
        ctx.lineTo(layout.flagRight, layout.poleTopY);
        ctx.stroke();
      }

      // Pole cap
      ctx.fillStyle = FLAG_COLORS.cap;
      ctx.beginPath();
      ctx.arc(layout.poleBaseX, layout.poleTopY - 2, 1.5, 0, Math.PI * 2);
      ctx.fill();

      // Flag shape: swallowtail pennant
      ctx.fillStyle = FLAG_COLORS.bg;
      ctx.beginPath();
      ctx.moveTo(layout.flagRight, layout.flagTop);
      ctx.lineTo(layout.flagLeft + waveX, layout.flagTop + waveY);
      ctx.lineTo(
        layout.flagLeft + layout.notchDepth + waveX * 0.6,
        layout.flagTop + layout.flagH / 2 + waveY * 0.5,
      );
      ctx.lineTo(layout.flagLeft + waveX, layout.flagBottom + waveY);
      ctx.lineTo(layout.flagRight, layout.flagBottom);
      ctx.closePath();
      ctx.fill();

      // Top edge accent stripe
      ctx.strokeStyle = FLAG_COLORS.stripe;
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(layout.flagRight, layout.flagTop);
      ctx.lineTo(layout.flagLeft + waveX, layout.flagTop + waveY);
      ctx.stroke();

      ctx.font = layout.font;
      ctx.fillStyle = FLAG_COLORS.text;
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText(layout.label, layout.textX + waveX * 0.2, layout.textY + waveY * 0.3);

      if (badge) {
        ctx.fillText(
          badge.emoji,
          layout.textX + layout.textW / 2 + layout.badgeW / 2 + 1 + waveX * 0.2,
          layout.textY + waveY * 0.3,
        );
      }
    }

    // --- Model decal on limo body (centered on stretched side panel) ---
    const panelX = x - (6 + LIMO_STRETCH / 2) * S;
    const panelY = carY - 4.5 * S;
    ctx.fillStyle = 'rgba(255,255,255,0.9)';
    ctx.font = 'bold 14px Courier New';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(color.name.toUpperCase(), panelX, panelY);

    // --- Source badge on hood ---
    this._drawSourceBadge(ctx, x, carY);

    // --- Metrics label on car body, below model name ---
    this._drawMetricsLabel(ctx, x, carY);

    ctx.textBaseline = 'alphabetic';
    ctx.textAlign = 'center';
  }

  _getDirectoryFlagLabel() {
    const workingDirBase = this.state.workingDir ? basename(this.state.workingDir) : '';
    const sessionName = this.state.name || '';
    const slug = (workingDirBase && workingDirBase !== 'unknown')
      ? workingDirBase
      : ((sessionName && sessionName !== 'unknown') ? sessionName : '');
    const branch = this.state.branch || '';

    if (!slug) {
      return branch || this.state.source || '';
    }
    if (!branch || branch === slug) {
      return slug;
    }
    if (slug.endsWith(`--${branch}`)) {
      return branch;
    }

    return `${slug} | ${branch}`;
  }

  _getDirectoryFlagLayout(ctx, x, y, dirName, badgeText = '') {
    const S = CAR_SCALE;
    const carY = y + this.springY;
    const viewport = getCanvasViewport(ctx);
    const viewportScale = clamp(viewport.width / 1280, 0.95, 1.35);
    const fontSize = Math.round(clamp(11 * viewportScale, DIRECTORY_FLAG_MIN_FONT, DIRECTORY_FLAG_MAX_FONT));
    const font = `bold ${fontSize}px Courier New`;

    ctx.font = font;
    const badgeW = badgeText ? ctx.measureText(badgeText).width + 4 : 0;
    const maxFlagW = clamp(viewport.width * 0.28, 120, DIRECTORY_FLAG_MAX_WIDTH);
    const maxTextW = Math.max(56, maxFlagW - badgeW - 18);
    const label = truncateMiddleToWidth(ctx, dirName, maxTextW);
    const textW = ctx.measureText(label).width;
    const flagH = fontSize + 8;
    const flagW = clamp(textW + badgeW + 18, DIRECTORY_FLAG_MIN_WIDTH, maxFlagW);
    const notchDepth = Math.max(6, Math.round(flagH * 0.4));

    const poleBaseX = x - (15 + LIMO_STRETCH) * S;
    const poleBaseY = carY - 5 * S;
    const poleTopY = Math.max(DIRECTORY_FLAG_MARGIN, poleBaseY - (22 + (fontSize - DIRECTORY_FLAG_MIN_FONT) * 2));
    const flagTop = clamp(
      poleTopY - 1,
      DIRECTORY_FLAG_MARGIN,
      Math.max(DIRECTORY_FLAG_MARGIN, viewport.height - flagH - DIRECTORY_FLAG_MARGIN),
    );
    const flagLeft = clamp(
      poleBaseX - flagW + 6,
      DIRECTORY_FLAG_MARGIN,
      Math.max(DIRECTORY_FLAG_MARGIN, viewport.width - flagW - DIRECTORY_FLAG_MARGIN),
    );
    const flagRight = flagLeft + flagW;
    const flagBottom = flagTop + flagH;
    const contentCenterX = flagLeft + notchDepth + (flagW - notchDepth) / 2;
    const textY = flagTop + flagH / 2;

    return {
      badgeW,
      flagBottom,
      flagH,
      flagLeft,
      flagRight,
      flagTop,
      font,
      fontSize,
      label,
      notchDepth,
      poleBaseX,
      poleBaseY,
      poleTopY: flagTop + 1,
      textW,
      textX: contentCenterX - badgeW / 2,
      textY,
    };
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

  _drawMetricsLabel(ctx, x, y) {
    const state = this.state;
    if (!state.tokensUsed && !state.contextUtilization) return;

    const S = CAR_SCALE;
    const label = this._buildMetricsLabel(state);

    // Centered on limo side panel, offset below the model name decal
    const labelX = x - (6 + LIMO_STRETCH / 2) * S;
    const labelY = y - 4.5 * S + 12;

    ctx.font = 'bold 9px Courier New';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';

    const textW = ctx.measureText(label).width;
    const textH = 10;
    const padX = 4;
    const padY = 3;
    const isComplete = state.activity === 'complete';

    ctx.fillStyle = isComplete ? 'rgba(20,83,45,0.58)' : 'rgba(0,0,0,0.25)';
    ctx.beginPath();
    ctx.roundRect(
      labelX - textW / 2 - padX,
      labelY - textH / 2 - padY,
      textW + padX * 2,
      textH + padY * 2,
      2,
    );
    ctx.fill();

    ctx.fillStyle = isComplete ? 'rgba(220,252,231,0.96)' : 'rgba(255,255,255,0.9)';
    ctx.fillText(label, labelX, labelY);
  }

  _buildMetricsLabel(state) {
    const parts = [];

    // Context utilization percentage
    if (state.activity === 'complete') {
      parts.push('DONE');
    } else {
      const pct = Math.round((state.contextUtilization || 0) * 100);
      parts.push(`${pct}%`);
    }

    // Token usage
    if (state.tokensUsed) {
      const usedFormatted = this._formatTokenCount(state.tokensUsed);
      if (state.maxContextTokens) {
        const maxFormatted = this._formatTokenCount(state.maxContextTokens);
        parts.push(`${usedFormatted}/${maxFormatted}`);
      } else {
        parts.push(usedFormatted);
      }
    }

    if (isTerminalActivity(state.activity)) {
      parts.push('-');
    } else if (state.burnRatePerMinute) {
      parts.push(formatBurnRate(state.burnRatePerMinute));
    }

    // Session duration
    if (state.startedAt) {
      const elapsedSeconds = Math.floor((Date.now() - new Date(state.startedAt).getTime()) / 1000);
      if (elapsedSeconds >= 60) {
        const minutes = Math.floor(elapsedSeconds / 60);
        parts.push(`${minutes}m`);
      } else if (elapsedSeconds > 0) {
        parts.push(`${elapsedSeconds}s`);
      }
    }

    return parts.join(' · ');
  }

  _formatTokenCount(count) {
    if (count >= 1000) {
      return `${Math.round(count / 1000)}K`;
    }
    return `${count}`;
  }

  _isTrackComplete() {
    return this.state.activity === 'complete' && !this.inPit && !this.inParkingLot;
  }

  drawTowRope(ctx, hamster) {
    // Skip rope entirely once hamster is fading — rope fades with hamster opacity
    if (hamster.ropeSnapped) return;

    const S = CAR_SCALE;

    // Attachment points: car rear bumper to skateboard front
    const carX = this.displayX - (17 + LIMO_STRETCH) * S;
    const carY = this.displayY + this.springY + 1 * S;
    const hamsterX = hamster.displayX + 10;
    const hamsterY = hamster.displayY;

    // Rope sag increases with distance
    const dx = hamsterX - carX;
    const dy = hamsterY - carY;
    const sag = 8 + Math.sqrt(dx * dx + dy * dy) * 0.02;

    // Control point (midpoint with sag)
    const cpX = (carX + hamsterX) / 2;
    const cpY = (carY + hamsterY) / 2 + sag;

    // Main rope
    ctx.strokeStyle = '#8B7355';
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.moveTo(carX, carY);
    ctx.quadraticCurveTo(cpX, cpY, hamsterX, hamsterY);
    ctx.stroke();

    // Highlight stroke
    ctx.strokeStyle = 'rgba(255,255,255,0.15)';
    ctx.lineWidth = 0.5;
    ctx.beginPath();
    ctx.moveTo(carX, carY);
    ctx.quadraticCurveTo(cpX, cpY - 1, hamsterX, hamsterY);
    ctx.stroke();
  }
}
