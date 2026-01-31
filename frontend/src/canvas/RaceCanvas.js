import { ParticleSystem } from './Particles.js';
import { Track } from './Track.js';
import { Racer } from '../entities/Racer.js';

export class RaceCanvas {
  constructor(canvas, engine = null) {
    this.canvas = canvas;
    this.ctx = canvas.getContext('2d');
    this.track = new Track();
    this.particles = new ParticleSystem();
    this.racers = new Map();
    this.connected = false;
    this.animFrameId = null;
    this.onRacerClick = null;
    this.engine = engine;

    // Timing for dt-based animation
    this.lastFrameTime = 0;
    this.dt = 1 / 60;

    // Glow/bloom offscreen canvas
    this.glowCanvas = document.createElement('canvas');
    this.glowCtx = this.glowCanvas.getContext('2d');

    // Screen shake state
    this.shakeIntensity = 0;
    this.shakeDuration = 0;
    this.shakeTimer = 0;

    // Flash effect state
    this.flashAlpha = 0;

    this.resize();
    this._resizeHandler = () => this.resize();
    window.addEventListener('resize', this._resizeHandler);
    this.canvas.addEventListener('click', (e) => this.handleClick(e));
    this.startLoop();
  }

  setEngine(engine) {
    this.engine = engine;
  }

  resize() {
    const dpr = window.devicePixelRatio || 1;
    const rect = this.canvas.parentElement.getBoundingClientRect();
    this.canvas.width = rect.width * dpr;
    this.canvas.height = rect.height * dpr;
    this.ctx.scale(dpr, dpr);
    this.width = rect.width;
    this.height = rect.height;

    // Resize glow canvas to match (at reduced resolution for blur)
    this.glowCanvas.width = Math.ceil(rect.width / 4);
    this.glowCanvas.height = Math.ceil(rect.height / 4);
  }

  setConnected(connected) {
    this.connected = connected;
  }

  setAllRacers(sessions) {
    const newIds = new Set(sessions.map(s => s.id));

    // Remove racers no longer present
    for (const id of this.racers.keys()) {
      if (!newIds.has(id)) {
        this.racers.delete(id);
      }
    }

    // Add or update
    for (const s of sessions) {
      if (this.racers.has(s.id)) {
        this.racers.get(s.id).update(s);
      } else {
        this.racers.set(s.id, new Racer(s));
      }
    }
  }

  updateRacer(state) {
    if (this.racers.has(state.id)) {
      this.racers.get(state.id).update(state);
    } else {
      this.racers.set(state.id, new Racer(state));
    }
  }

  removeRacer(id) {
    this.racers.delete(id);
  }

  onComplete(sessionId) {
    const racer = this.racers.get(sessionId);
    if (racer) {
      racer.confettiEmitted = false;
    }
    // Flash effect on completion
    this.flashAlpha = 0.3;
  }

  onError(sessionId) {
    const racer = this.racers.get(sessionId);
    if (racer) {
      racer.smokeEmitted = false;
      racer.skidEmitted = false;
      racer.errorTimer = 0;
      racer.errorStage = 0;
    }
    // Screen shake on error
    this.shakeIntensity = 6;
    this.shakeDuration = 0.3;
    this.shakeTimer = 0;
  }

  startLoop() {
    const loop = (timestamp) => {
      // Compute delta time
      if (this.lastFrameTime === 0) {
        this.lastFrameTime = timestamp;
      }
      const rawDt = (timestamp - this.lastFrameTime) / 1000;
      this.dt = Math.min(rawDt, 0.05); // Cap at 50ms to avoid spiral
      this.lastFrameTime = timestamp;

      this.update();
      this.draw();
      this.animFrameId = requestAnimationFrame(loop);
    };
    this.animFrameId = requestAnimationFrame(loop);
  }

  update() {
    const dt = this.dt;
    const laneCount = this.racers.size || 1;
    const bounds = this.track.getTrackBounds(this.width, this.height, laneCount);

    // Sort racers by lane for consistent ordering
    const sorted = [...this.racers.values()].sort((a, b) => a.state.lane - b.state.lane);

    for (let i = 0; i < sorted.length; i++) {
      const racer = sorted[i];
      const targetX = this.track.getPositionX(bounds, racer.state.contextUtilization);
      const targetY = this.track.getLaneY(bounds, i);
      racer.setTarget(targetX, targetY);
      racer.animate(this.particles, dt);

      // Sync engine audio
      if (this.engine) {
        const activity = racer.state.activity;
        if (activity === 'thinking' || activity === 'tool_use') {
          this.engine.startEngine(racer.id, activity);
        } else {
          this.engine.stopEngine(racer.id);
        }
      }
    }

    this.particles.update(dt);

    // Update screen shake
    if (this.shakeTimer < this.shakeDuration) {
      this.shakeTimer += dt;
    }

    // Decay flash
    if (this.flashAlpha > 0) {
      this.flashAlpha = Math.max(0, this.flashAlpha - dt * 1.5); // fade over ~0.2s
    }
  }

  draw() {
    const ctx = this.ctx;

    // Clear
    ctx.clearRect(0, 0, this.width, this.height);

    ctx.save();

    // Apply screen shake
    if (this.shakeTimer < this.shakeDuration && this.shakeIntensity > 0) {
      const progress = this.shakeTimer / this.shakeDuration;
      const currentIntensity = this.shakeIntensity * (1 - progress); // linear decay
      const sx = (Math.random() * 2 - 1) * currentIntensity;
      const sy = (Math.random() * 2 - 1) * currentIntensity;
      ctx.translate(sx, sy);
    }

    // Background
    ctx.fillStyle = '#1a1a2e';
    ctx.fillRect(-10, -10, this.width + 20, this.height + 20);

    const laneCount = this.racers.size || 1;
    this.track.draw(ctx, this.width, this.height, laneCount);

    // Draw particles behind racers
    this.particles.drawBehind(ctx);

    // Draw car shadows first (under all cars)
    const sorted = [...this.racers.values()].sort((a, b) => a.state.lane - b.state.lane);

    // Draw racers
    for (const racer of sorted) {
      racer.draw(ctx);
    }

    // Draw particles in front of racers
    this.particles.drawFront(ctx);

    // Glow/bloom composite pass
    this._drawBloom(ctx);

    // Flash effect
    if (this.flashAlpha > 0) {
      ctx.fillStyle = `rgba(255,255,255,${this.flashAlpha})`;
      ctx.fillRect(-10, -10, this.width + 20, this.height + 20);
    }

    ctx.restore();

    // Connection overlay (drawn outside shake transform)
    if (!this.connected) {
      ctx.fillStyle = 'rgba(0,0,0,0.6)';
      ctx.fillRect(0, 0, this.width, this.height);
      ctx.fillStyle = '#e94560';
      ctx.font = 'bold 20px Courier New';
      ctx.textAlign = 'center';
      ctx.fillText('Connecting...', this.width / 2, this.height / 2);
    }

    // Empty state
    if (this.connected && this.racers.size === 0) {
      ctx.fillStyle = '#666';
      ctx.font = '16px Courier New';
      ctx.textAlign = 'center';
      ctx.fillText('No active Claude sessions detected', this.width / 2, this.height / 2 - 10);
      ctx.font = '12px Courier New';
      ctx.fillText('Start a Claude Code session to see it race', this.width / 2, this.height / 2 + 14);
    }
  }

  _drawBloom(ctx) {
    // Draw bright elements (glows, headlights) to small offscreen canvas,
    // which acts as a blur due to the downscale, then composite back.
    const gc = this.glowCtx;
    const gw = this.glowCanvas.width;
    const gh = this.glowCanvas.height;
    const scaleX = gw / this.width;
    const scaleY = gh / this.height;

    gc.clearRect(0, 0, gw, gh);

    // Draw just the glow elements at reduced resolution
    gc.save();
    gc.scale(scaleX, scaleY);

    for (const racer of this.racers.values()) {
      const x = racer.displayX;
      const y = racer.displayY + racer.springY;

      // Racer glow aura
      if (racer.glowIntensity > 0.02) {
        const glowR = 35;
        const grad = gc.createRadialGradient(x, y, 0, x, y, glowR);
        grad.addColorStop(0, `rgba(255,255,255,${racer.glowIntensity * 2})`);
        grad.addColorStop(1, 'rgba(255,255,255,0)');
        gc.fillStyle = grad;
        gc.beginPath();
        gc.arc(x, y, glowR, 0, Math.PI * 2);
        gc.fill();
      }

      // Headlight glow for tool_use
      if (racer.state.activity === 'tool_use') {
        const hlGrad = gc.createRadialGradient(x + 21, y, 0, x + 21, y, 20);
        hlGrad.addColorStop(0, 'rgba(255,255,200,0.4)');
        hlGrad.addColorStop(1, 'rgba(255,255,200,0)');
        gc.fillStyle = hlGrad;
        gc.beginPath();
        gc.arc(x + 22, y, 20, 0, Math.PI * 2);
        gc.fill();
      }

      // Hazard glow for waiting
      if (racer.state.activity === 'waiting' && Math.sin(racer.hazardPhase) > 0) {
        const hzGrad = gc.createRadialGradient(x, y, 0, x, y, 25);
        hzGrad.addColorStop(0, 'rgba(255,170,0,0.3)');
        hzGrad.addColorStop(1, 'rgba(255,170,0,0)');
        gc.fillStyle = hzGrad;
        gc.beginPath();
        gc.arc(x, y, 25, 0, Math.PI * 2);
        gc.fill();
      }
    }

    gc.restore();

    // Composite the glow canvas onto main canvas with additive blending
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    ctx.globalAlpha = 0.5;
    ctx.drawImage(this.glowCanvas, 0, 0, gw, gh, 0, 0, this.width, this.height);
    ctx.restore();
  }

  handleClick(e) {
    const rect = this.canvas.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const y = e.clientY - rect.top;

    // Find clicked racer (within 30px radius)
    for (const racer of this.racers.values()) {
      const dx = x - racer.displayX;
      const dy = y - racer.displayY;
      if (Math.sqrt(dx * dx + dy * dy) < 30) {
        if (this.onRacerClick) {
          this.onRacerClick(racer.state);
        }
        return;
      }
    }
  }

  destroy() {
    if (this.animFrameId) {
      cancelAnimationFrame(this.animFrameId);
    }
    window.removeEventListener('resize', this._resizeHandler);
  }
}
