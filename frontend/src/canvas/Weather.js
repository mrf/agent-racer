/**
 * Weather system — ambient visual overlay driven by aggregate session metrics.
 *
 * Weather states:
 *   clear    – no sessions: dark sky, twinkling stars
 *   sunny    – 1-2 sessions, low activity
 *   cloudy   – 3+ sessions, heavy tool use: grey tint, wind streaks
 *   storm    – errors present: dark sky, rain, lightning
 *   haze     – high aggregate burn rate (>5K tok/min): shimmer distortion
 *   golden   – all sessions complete: warm orange glow
 *   fog      – context compactions happening: misty overlay
 *
 * Rendered as canvas overlays — call drawBehind() before track, drawFront() after.
 */

// ── Weather state constants ─────────────────────────────────────────
const CLEAR  = 'clear';
const SUNNY  = 'sunny';
const CLOUDY = 'cloudy';
const STORM  = 'storm';
const HAZE   = 'haze';
const GOLDEN = 'golden';
const FOG    = 'fog';

const TRANSITION_DURATION = 2.0; // seconds for cross-fade
const EVAL_INTERVAL = 2.0;      // seconds between metric evaluations
const BURN_RATE_THRESHOLD = 5000; // tokens/min aggregate for heat haze
const EFFECT_THRESHOLD = 0.3;    // minimum blend weight to activate an effect

// Sky gradient palettes per state  [top, bottom]
const SKY_PALETTES = {
  [CLEAR]:  { top: [10, 10, 30],   bottom: [26, 26, 46], alpha: 0.35 },
  [SUNNY]:  { top: [40, 50, 90],   bottom: [26, 26, 46], alpha: 0.20 },
  [CLOUDY]: { top: [50, 50, 60],   bottom: [35, 35, 45], alpha: 0.30 },
  [STORM]:  { top: [20, 15, 30],   bottom: [26, 26, 46], alpha: 0.45 },
  [HAZE]:   { top: [50, 35, 20],   bottom: [26, 26, 46], alpha: 0.20 },
  [GOLDEN]: { top: [162, 98, 48],  bottom: [104, 56, 26], alpha: 0.42 },
  [FOG]:    { top: [40, 40, 50],   bottom: [30, 30, 40], alpha: 0.40 },
};

const STATE_LABELS = {
  [CLEAR]:  'Clear',
  [SUNNY]:  'Sunny',
  [CLOUDY]: 'Overcast',
  [STORM]:  'Storm',
  [HAZE]:   'Heat Haze',
  [GOLDEN]: 'Golden Hour',
  [FOG]:    'Fog',
};

// ── Helpers ─────────────────────────────────────────────────────────
function lerp(a, b, t) { return a + (b - a) * t; }
function lerpRGB(a, b, t) {
  return [
    Math.round(lerp(a[0], b[0], t)),
    Math.round(lerp(a[1], b[1], t)),
    Math.round(lerp(a[2], b[2], t)),
  ];
}

// ── Star pool (static once generated) ───────────────────────────────
function generateStars(count) {
  const stars = [];
  for (let i = 0; i < count; i++) {
    stars.push({
      x: Math.random(),       // 0-1 normalized
      y: Math.random() * 0.4, // top 40% of canvas
      size: 0.5 + Math.random() * 1.5,
      phase: Math.random() * Math.PI * 2,
      speed: 0.8 + Math.random() * 1.5,
    });
  }
  return stars;
}

// ── Rain drop pool ──────────────────────────────────────────────────
function createRainDrop(w, h) {
  return {
    x: Math.random() * w * 1.3 - w * 0.15,
    y: -10 - Math.random() * h * 0.5,
    len: 8 + Math.random() * 14,
    speed: 6 + Math.random() * 6,
    alpha: 0.15 + Math.random() * 0.25,
  };
}

function createRainSplash(x, y) {
  return {
    x,
    y,
    age: 0,
    maxAge: 3,
    spread: 2 + Math.random() * 2,
    height: 2 + Math.random() * 2,
    alpha: 0.16 + Math.random() * 0.14,
  };
}

// ── Fog cloud pool ──────────────────────────────────────────────────
function generateFogClouds() {
  const layerPattern = [0, 1, 2, 1];
  const clouds = [];
  for (let i = 0; i < layerPattern.length; i++) {
    const layer = layerPattern[i];
    clouds.push({
      layer,
      startX: Math.random() * (1.25 + layer * 0.12),
      y: 0.22 + layer * 0.13 + Math.random() * 0.08,
      width: 0.22 + layer * 0.05 + Math.random() * 0.07,
      height: 0.11 + layer * 0.02 + Math.random() * 0.04,
      speed: 0.012 + layer * 0.01 + Math.random() * 0.012,
      bobAmplitude: 6 + layer * 3 + Math.random() * 4,
      bobSpeed: 0.15 + Math.random() * 0.18,
      phase: Math.random() * Math.PI * 2,
      alpha: 0.14 + layer * 0.03 + Math.random() * 0.05,
    });
  }
  return clouds;
}

// ── Lightning ───────────────────────────────────────────────────────
const MAX_BOLT_SEGMENTS = 128;

function generateBolt(x, y, maxDepth) {
  const segments = [];
  const initialAngle = Math.PI / 2 + (Math.random() - 0.5) * 0.6;
  const stack = [{ sx: x, sy: y, angle: initialAngle, depth: 0, alpha: 1.0 }];

  while (stack.length > 0 && segments.length < MAX_BOLT_SEGMENTS) {
    const { sx, sy, angle, depth, alpha } = stack.pop();
    if (depth > maxDepth) continue;

    const len = 15 + Math.random() * 30;
    const ex = sx + Math.cos(angle) * len;
    const ey = sy + Math.sin(angle) * len;
    segments.push({ x1: sx, y1: sy, x2: ex, y2: ey, alpha, width: Math.max(1, 3 - depth) });

    // Main continuation
    stack.push({ sx: ex, sy: ey, angle: angle + (Math.random() - 0.5) * 0.8, depth: depth + 1, alpha: alpha * 0.85 });
    // Occasional fork
    if (Math.random() < 0.35) {
      stack.push({ sx: ex, sy: ey, angle: angle + (Math.random() - 0.5) * 1.5, depth: depth + 1, alpha: alpha * 0.5 });
    }
  }

  return segments;
}

// ─────────────────────────────────────────────────────────────────────
export class WeatherSystem {
  constructor() {
    this.enabled = true;
    this.currentState = CLEAR;
    this.targetState = CLEAR;
    this.transitionProgress = 1.0; // 1 = fully at targetState
    this.evalTimer = 0;
    this.time = 0;

    // Stars (persistent)
    this._stars = generateStars(80);

    // Rain particles
    this._rain = [];
    this._maxRain = 120;
    this._rainSplashes = [];
    this._maxRainSplashes = 48;

    // Lightning state
    this._lightning = null;  // { segments, flashAlpha, timer }
    this._lightningCooldown = 0;

    // Fog clouds
    this._fogClouds = generateFogClouds();

    // Heat shimmer phase
    this._shimmerPhase = 0;

    // Wind streaks
    this._windStreaks = [];

    // Cached aggregate metrics (updated every EVAL_INTERVAL)
    this._metrics = {
      sessionCount: 0,
      activeCount: 0,
      errorCount: 0,
      totalBurnRate: 0,
      compactionCount: 0,
      allComplete: false,
    };
  }

  /** Toggle weather on/off. */
  toggle() {
    this.enabled = !this.enabled;
    return this.enabled;
  }

  /** Feed session data — call from RaceCanvas.update(). */
  updateMetrics(sessions) {
    const m = this._metrics;
    m.sessionCount = sessions.length;
    m.activeCount = 0;
    m.errorCount = 0;
    m.totalBurnRate = 0;
    m.compactionCount = 0;
    m.allComplete = sessions.length > 0;

    for (let i = 0; i < sessions.length; i++) {
      const s = sessions[i];
      if (s.activity === 'errored' || s.activity === 'lost') {
        m.errorCount++;
      }
      if (s.activity !== 'complete' && s.activity !== 'errored' && s.activity !== 'lost') {
        m.activeCount++;
        m.allComplete = false;
      }
      m.totalBurnRate += (s.burnRatePerMinute || 0);
      m.compactionCount += (s.compactionCount || 0);
    }
  }

  /** Called every frame. Evaluates weather state periodically, updates particles. */
  update(dt, canvasWidth, canvasHeight) {
    if (!this.enabled) return;

    this.time += dt;
    this.evalTimer += dt;

    // Periodic state evaluation
    if (this.evalTimer >= EVAL_INTERVAL) {
      this.evalTimer = 0;
      this._evaluateState();
    }

    // Advance transition
    if (this.transitionProgress < 1.0) {
      this.transitionProgress = Math.min(1.0, this.transitionProgress + dt / TRANSITION_DURATION);
      if (this.transitionProgress >= 1.0) {
        this.currentState = this.targetState;
      }
    }

    // Check which effects are active (blend weight above threshold)
    const isStormy = this._getEffectWeight(STORM) > EFFECT_THRESHOLD;
    const isHazy   = this._getEffectWeight(HAZE) > EFFECT_THRESHOLD;
    const isCloudy = this._getEffectWeight(CLOUDY) > EFFECT_THRESHOLD;

    // ── Rain ────────────────────────────────────────────────────────
    if (isStormy) {
      // Spawn rain
      const targetCount = Math.min(this._maxRain, Math.floor(80 + this._metrics.errorCount * 20));
      while (this._rain.length < targetCount) {
        this._rain.push(createRainDrop(canvasWidth, canvasHeight));
      }
    }
    // Update rain drops
    for (let i = this._rain.length - 1; i >= 0; i--) {
      const drop = this._rain[i];
      drop.x += 2.0 * dt * 60;  // diagonal wind
      drop.y += drop.speed * dt * 60;
      if (drop.y + drop.len >= canvasHeight) {
        this._spawnRainSplash(drop.x + drop.len * 0.15, canvasHeight - 1);
        if (isStormy) {
          // Recycle
          drop.x = Math.random() * canvasWidth * 1.3 - canvasWidth * 0.15;
          drop.y = -10 - Math.random() * 40;
        } else {
          this._rain.splice(i, 1);
        }
      }
    }

    for (let i = this._rainSplashes.length - 1; i >= 0; i--) {
      const splash = this._rainSplashes[i];
      splash.age += dt * 60;
      if (splash.age >= splash.maxAge) {
        this._rainSplashes.splice(i, 1);
      }
    }

    // ── Lightning ───────────────────────────────────────────────────
    this._lightningCooldown = Math.max(0, this._lightningCooldown - dt);
    if (isStormy && this._lightningCooldown <= 0 && Math.random() < 0.005) {
      const boltX = canvasWidth * (0.1 + Math.random() * 0.8);
      this._lightning = {
        segments: generateBolt(boltX, 0, 5),
        flashAlpha: 0.25,
        timer: 0.3,
      };
      this._lightningCooldown = 2 + Math.random() * 4;
    }
    if (this._lightning) {
      this._lightning.timer -= dt;
      this._lightning.flashAlpha = Math.max(0, this._lightning.flashAlpha - dt * 1.5);
      if (this._lightning.timer <= 0) {
        this._lightning = null;
      }
    }

    // ── Heat shimmer ────────────────────────────────────────────────
    if (isHazy) {
      this._shimmerPhase += dt * 3;
    }

    // ── Wind streaks (cloudy / stormy) ──────────────────────────────
    if (isCloudy || isStormy) {
      if (this._windStreaks.length < 15 && Math.random() < 0.08) {
        this._windStreaks.push({
          x: canvasWidth + 10,
          y: Math.random() * canvasHeight * 0.6,
          len: 30 + Math.random() * 60,
          speed: 3 + Math.random() * 4,
          alpha: 0.04 + Math.random() * 0.06,
        });
      }
    }
    for (let i = this._windStreaks.length - 1; i >= 0; i--) {
      const s = this._windStreaks[i];
      s.x -= s.speed * dt * 60;
      if (s.x + s.len < 0) {
        this._windStreaks.splice(i, 1);
      }
    }
  }

  /** Draw weather effects behind the track (sky tint, stars). */
  drawBehind(ctx, width, height) {
    if (!this.enabled) return;

    const t = this.transitionProgress;
    const fromPal = SKY_PALETTES[this.currentState] || SKY_PALETTES[CLEAR];
    const toPal = SKY_PALETTES[this.targetState] || SKY_PALETTES[CLEAR];

    const topRGB = lerpRGB(fromPal.top, toPal.top, t);
    const botRGB = lerpRGB(fromPal.bottom, toPal.bottom, t);
    const alpha = lerp(fromPal.alpha, toPal.alpha, t);

    // Atmospheric tint should influence the full scene, not just the sky band.
    const grad = ctx.createLinearGradient(0, 0, 0, height);
    grad.addColorStop(0, `rgba(${topRGB[0]},${topRGB[1]},${topRGB[2]},${alpha})`);
    grad.addColorStop(1, `rgba(${botRGB[0]},${botRGB[1]},${botRGB[2]},${alpha})`);
    ctx.fillStyle = grad;
    ctx.fillRect(0, 0, width, height);

    // Stars (visible in clear/night state)
    const starAlpha = this._getEffectWeight(CLEAR);
    if (starAlpha > 0.01) {
      this._drawStars(ctx, width, height, starAlpha);
    }

    // Wind streaks
    if (this._windStreaks.length > 0) {
      ctx.save();
      ctx.lineCap = 'round';
      for (let i = 0; i < this._windStreaks.length; i++) {
        const s = this._windStreaks[i];
        ctx.strokeStyle = `rgba(180,180,210,${s.alpha})`;
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(s.x, s.y);
        ctx.lineTo(s.x + s.len, s.y);
        ctx.stroke();
      }
      ctx.restore();
    }
  }

  /** Draw weather effects in front of the scene (rain, lightning, fog, haze). */
  drawFront(ctx, width, height) {
    if (!this.enabled) return;

    // Rain
    if (this._rain.length > 0) {
      const stormWeight = this._getEffectWeight(STORM);
      ctx.save();
      ctx.lineCap = 'round';
      ctx.lineWidth = 1;
      for (let i = 0; i < this._rain.length; i++) {
        const d = this._rain[i];
        ctx.strokeStyle = `rgba(170,190,220,${d.alpha * stormWeight})`;
        ctx.beginPath();
        ctx.moveTo(d.x, d.y);
        ctx.lineTo(d.x + d.len * 0.15, d.y + d.len);
        ctx.stroke();
      }
      ctx.restore();
    }

    if (this._rainSplashes.length > 0) {
      const stormWeight = this._getEffectWeight(STORM);
      ctx.save();
      ctx.lineCap = 'round';
      ctx.lineWidth = 1;
      for (let i = 0; i < this._rainSplashes.length; i++) {
        const splash = this._rainSplashes[i];
        const t = splash.age / splash.maxAge;
        const spread = splash.spread * (0.7 + t * 0.9);
        const rise = splash.height * (1 - t * 0.35);
        const alpha = splash.alpha * (1 - t) * stormWeight;
        if (alpha <= 0) continue;
        ctx.strokeStyle = `rgba(190,205,230,${alpha})`;
        ctx.beginPath();
        ctx.moveTo(splash.x - spread, splash.y);
        ctx.lineTo(splash.x - spread * 0.25, splash.y - rise);
        ctx.moveTo(splash.x, splash.y);
        ctx.lineTo(splash.x, splash.y - rise * 0.7);
        ctx.moveTo(splash.x + spread, splash.y);
        ctx.lineTo(splash.x + spread * 0.25, splash.y - rise);
        ctx.stroke();
      }
      ctx.restore();
    }

    // Lightning bolt
    if (this._lightning) {
      const { segments, flashAlpha } = this._lightning;
      // Screen flash
      if (flashAlpha > 0) {
        ctx.fillStyle = `rgba(220,220,255,${flashAlpha})`;
        ctx.fillRect(0, 0, width, height);
      }
      // Bolt segments
      ctx.save();
      ctx.lineCap = 'round';
      ctx.lineJoin = 'round';
      const boltAlpha = Math.min(1, this._lightning.timer * 5);
      for (let i = 0; i < segments.length; i++) {
        const seg = segments[i];
        ctx.strokeStyle = `rgba(200,200,255,${seg.alpha * boltAlpha})`;
        ctx.lineWidth = seg.width;
        ctx.beginPath();
        ctx.moveTo(seg.x1, seg.y1);
        ctx.lineTo(seg.x2, seg.y2);
        ctx.stroke();
      }
      ctx.restore();
    }

    // Fog clouds
    const fogWeight = this._getEffectWeight(FOG);
    if (fogWeight > 0.01) {
      this._drawFogClouds(ctx, width, height, fogWeight);
    }

    // Heat shimmer (distortion lines on track surface)
    const hazeWeight = this._getEffectWeight(HAZE);
    if (hazeWeight > 0.01) {
      ctx.save();
      ctx.globalAlpha = 0.06 * hazeWeight;
      const shimmerY = height * 0.25;
      const shimmerH = height * 0.35;
      for (let row = 0; row < 8; row++) {
        const y = shimmerY + (row / 8) * shimmerH;
        const offset = Math.sin(this._shimmerPhase + row * 0.7) * 3;
        ctx.fillStyle = `rgba(255,200,100,0.5)`;
        ctx.fillRect(0, y + offset, width, 1);
      }
      ctx.restore();
    }

    // Golden glow
    const goldenWeight = this._getEffectWeight(GOLDEN);
    if (goldenWeight > 0.01) {
      this._drawGoldenHour(ctx, width, height, goldenWeight);
    }
  }

  /** Get the current weather state label (for UI display). */
  getStateLabel() {
    return STATE_LABELS[this.targetState] || 'Clear';
  }

  // ── Private ───────────────────────────────────────────────────────

  /** Evaluate aggregate metrics and pick the target weather state. */
  _evaluateState() {
    const m = this._metrics;
    let next = CLEAR;

    if (m.sessionCount === 0) {
      next = CLEAR;
    } else if (m.allComplete) {
      next = GOLDEN;
    } else if (m.errorCount >= 2) {
      next = STORM;
    } else if (m.compactionCount >= 2) {
      next = FOG;
    } else if (m.totalBurnRate >= BURN_RATE_THRESHOLD) {
      next = HAZE;
    } else if (m.activeCount >= 3) {
      next = CLOUDY;
    } else if (m.activeCount >= 1) {
      next = SUNNY;
    }

    if (next !== this.targetState) {
      this.currentState = this.targetState;
      this.targetState = next;
      this.transitionProgress = 0;
    }
  }

  /** Compute effective weight (0-1) of a state accounting for transition blend. */
  _getEffectWeight(state) {
    const t = this.transitionProgress;
    let weight = 0;
    if (this.currentState === state) weight += (1 - t);
    if (this.targetState === state) weight += t;
    return weight;
  }

  _drawStars(ctx, width, height, alpha) {
    ctx.save();
    for (let i = 0; i < this._stars.length; i++) {
      const star = this._stars[i];
      const twinkle = 0.7 + 0.3 * Math.sin(this.time * star.speed + star.phase);
      const a = alpha * twinkle;
      if (a < 0.02) continue;
      ctx.fillStyle = `rgba(220,220,255,${a})`;
      ctx.beginPath();
      ctx.arc(star.x * width, star.y * height, star.size, 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.restore();
  }

  _drawFogClouds(ctx, width, height, fogWeight) {
    ctx.save();
    ctx.filter = 'blur(18px)';
    for (let i = 0; i < this._fogClouds.length; i++) {
      const cloud = this._fogClouds[i];
      const frame = this._getFogCloudFrame(cloud, width, height);
      const alpha = cloud.alpha * fogWeight;

      ctx.fillStyle = `rgba(195,200,215,${alpha})`;
      ctx.beginPath();
      ctx.ellipse(frame.x, frame.y, frame.width / 2, frame.height / 2, 0, 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.restore();
  }

  _drawGoldenHour(ctx, width, height, goldenWeight) {
    const warmWash = ctx.createLinearGradient(0, 0, 0, height);
    warmWash.addColorStop(0, `rgba(255,210,140,${0.12 * goldenWeight})`);
    warmWash.addColorStop(0.55, `rgba(224,168,112,${0.08 * goldenWeight})`);
    warmWash.addColorStop(1, `rgba(140,92,58,${0.06 * goldenWeight})`);
    ctx.fillStyle = warmWash;
    ctx.fillRect(0, 0, width, height);

    ctx.fillStyle = `rgba(190,170,155,${0.06 * goldenWeight})`;
    ctx.fillRect(0, 0, width, height);

    const glow = ctx.createRadialGradient(
      width * 0.55, height * 0.14, 0,
      width * 0.55, height * 0.14, width * 0.7
    );
    glow.addColorStop(0, `rgba(255,220,150,${0.14 * goldenWeight})`);
    glow.addColorStop(0.45, `rgba(255,168,78,${0.08 * goldenWeight})`);
    glow.addColorStop(1, 'rgba(255,120,20,0)');
    ctx.fillStyle = glow;
    ctx.fillRect(0, 0, width, height);
  }

  _getFogCloudFrame(cloud, width, height) {
    const travel = 1 + cloud.width * 2;
    const wrappedX = (cloud.startX + this.time * cloud.speed) % travel;

    return {
      x: (wrappedX - cloud.width) * width,
      y: cloud.y * height + Math.sin(this.time * cloud.bobSpeed + cloud.phase) * cloud.bobAmplitude,
      width: cloud.width * width,
      height: cloud.height * height,
    };
  }

  _spawnRainSplash(x, y) {
    if (this._rainSplashes.length >= this._maxRainSplashes) {
      this._rainSplashes.splice(0, 1);
    }
    this._rainSplashes.push(createRainSplash(x, y));
  }
}
