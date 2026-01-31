import { ParticleSystem } from './Particles.js';
import { Track } from './Track.js';
import { Racer } from '../entities/Racer.js';

export class RaceCanvas {
  constructor(canvas) {
    this.canvas = canvas;
    this.ctx = canvas.getContext('2d');
    this.track = new Track();
    this.particles = new ParticleSystem();
    this.racers = new Map();
    this.connected = false;
    this.animFrameId = null;
    this.onRacerClick = null;

    this.resize();
    window.addEventListener('resize', () => this.resize());
    this.canvas.addEventListener('click', (e) => this.handleClick(e));
    this.startLoop();
  }

  resize() {
    const dpr = window.devicePixelRatio || 1;
    const rect = this.canvas.parentElement.getBoundingClientRect();
    this.canvas.width = rect.width * dpr;
    this.canvas.height = rect.height * dpr;
    this.ctx.scale(dpr, dpr);
    this.width = rect.width;
    this.height = rect.height;
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
      racer.confettiEmitted = false; // reset so it emits
    }
  }

  onError(sessionId) {
    const racer = this.racers.get(sessionId);
    if (racer) {
      racer.smokeEmitted = false;
    }
  }

  startLoop() {
    const loop = () => {
      this.update();
      this.draw();
      this.animFrameId = requestAnimationFrame(loop);
    };
    loop();
  }

  update() {
    const laneCount = this.racers.size || 1;
    const bounds = this.track.getTrackBounds(this.width, this.height, laneCount);

    // Sort racers by lane for consistent ordering
    const sorted = [...this.racers.values()].sort((a, b) => a.state.lane - b.state.lane);

    for (let i = 0; i < sorted.length; i++) {
      const racer = sorted[i];
      const targetX = this.track.getPositionX(bounds, racer.state.contextUtilization);
      const targetY = this.track.getLaneY(bounds, i);
      racer.setTarget(targetX, targetY);
      racer.animate(this.particles);
    }

    this.particles.update();
  }

  draw() {
    const ctx = this.ctx;

    // Clear
    ctx.clearRect(0, 0, this.width, this.height);

    // Background
    ctx.fillStyle = '#1a1a2e';
    ctx.fillRect(0, 0, this.width, this.height);

    const laneCount = this.racers.size || 1;
    this.track.draw(ctx, this.width, this.height, laneCount);

    // Draw particles behind racers
    this.particles.draw(ctx);

    // Draw racers sorted by lane
    const sorted = [...this.racers.values()].sort((a, b) => a.state.lane - b.state.lane);
    for (const racer of sorted) {
      racer.draw(ctx);
    }

    // Connection overlay
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
    window.removeEventListener('resize', this.resize);
  }
}
