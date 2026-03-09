import { describe, it, expect, vi } from 'vitest';
import { Character } from './Character.js';

function makeState(overrides = {}) {
  return {
    id: 'test-1',
    activity: 'thinking',
    model: 'claude-sonnet-4-5-20250929',
    source: 'claude',
    name: 'test',
    ...overrides,
  };
}

function makeParticles() {
  return { emit: vi.fn(), emitWithColor: vi.fn() };
}

describe('construction and initial state', () => {
  it('sets id from state', () => {
    const ch = new Character(makeState({ id: 'abc-123' }));
    expect(ch.id).toBe('abc-123');
  });

  it('starts uninitialized', () => {
    const ch = new Character(makeState());
    expect(ch.initialized).toBe(false);
  });

  it('has default display position at origin', () => {
    const ch = new Character(makeState());
    expect(ch.displayX).toBe(0);
    expect(ch.displayY).toBe(0);
  });

  it('stores initial activity as prevActivity', () => {
    const ch = new Character(makeState({ activity: 'waiting' }));
    expect(ch.prevActivity).toBe('waiting');
  });

  it('starts with full opacity', () => {
    const ch = new Character(makeState());
    expect(ch.opacity).toBe(1.0);
  });

  it('has empty hamsters map', () => {
    const ch = new Character(makeState());
    expect(ch.hamsters).toBeInstanceOf(Map);
    expect(ch.hamsters.size).toBe(0);
  });

  it('detects tmux target', () => {
    const ch = new Character(makeState({ tmuxTarget: 'cc-main:0' }));
    expect(ch.hasTmux).toBe(true);
  });

  it('defaults hasTmux to false', () => {
    const ch = new Character(makeState());
    expect(ch.hasTmux).toBe(false);
  });
});

describe('spring suspension convergence', () => {
  it('converges springY toward zero after impulse', () => {
    const ch = new Character(makeState());
    ch.springVel = 5;

    for (let i = 0; i < 300; i++) ch.animate(null, 1 / 60);

    expect(Math.abs(ch.springY)).toBeLessThan(0.01);
    expect(Math.abs(ch.springVel)).toBeLessThan(0.01);
  });

  it('oscillates before settling', () => {
    const ch = new Character(makeState());
    ch.springVel = 10;

    const positions = [];
    for (let i = 0; i < 60; i++) {
      ch.animate(null, 1 / 60);
      positions.push(ch.springY);
    }

    const crossings = positions.filter(
      (p, i) => i > 0 && Math.sign(p) !== Math.sign(positions[i - 1]),
    );
    expect(crossings.length).toBeGreaterThan(0);
  });

  it('damping factor controls decay rate', () => {
    const fast = new Character(makeState());
    fast.springDamping = 0.8;
    fast.springVel = 5;

    const slow = new Character(makeState());
    slow.springDamping = 0.98;
    slow.springVel = 5;

    for (let i = 0; i < 30; i++) {
      fast.animate(null, 1 / 60);
      slow.animate(null, 1 / 60);
    }

    expect(Math.abs(fast.springVel)).toBeLessThan(Math.abs(slow.springVel));
  });
});

describe('lerp interpolation', () => {
  it('moves displayX toward targetX', () => {
    const ch = new Character(makeState());
    ch.initialized = true;
    ch.displayX = 0;
    ch.targetX = 100;

    ch.animate(null, 1 / 60);

    expect(ch.displayX).toBeGreaterThan(0);
    expect(ch.displayX).toBeLessThan(100);
  });

  it('converges to target over many frames', () => {
    const ch = new Character(makeState());
    ch.initialized = true;
    ch.displayX = 0;
    ch.targetX = 100;
    ch.displayY = 0;
    ch.targetY = 50;

    for (let i = 0; i < 300; i++) ch.animate(null, 1 / 60);

    expect(ch.displayX).toBeCloseTo(100, 0);
    expect(ch.displayY).toBeCloseTo(50, 0);
  });

  it('uses faster lerp during zone transitions', () => {
    const normal = new Character(makeState());
    normal.initialized = true;
    normal.displayX = 0;
    normal.targetX = 100;
    normal.animate(null, 1 / 60);

    const transitioning = new Character(makeState());
    transitioning.initialized = true;
    transitioning.displayX = 0;
    transitioning.transitionWaypoints = [{ x: 100, y: 0 }];
    transitioning.waypointIndex = 0;
    transitioning.animate(null, 1 / 60);

    expect(transitioning.displayX).toBeGreaterThan(normal.displayX);
  });

  it('snaps to initial position on first setTarget', () => {
    const ch = new Character(makeState());
    expect(ch.initialized).toBe(false);

    ch.setTarget(200, 150);

    expect(ch.displayX).toBe(200);
    expect(ch.displayY).toBe(150);
    expect(ch.initialized).toBe(true);
  });

  it('does not snap on subsequent setTarget calls', () => {
    const ch = new Character(makeState());
    ch.setTarget(100, 100);
    ch.setTarget(500, 500);

    expect(ch.displayX).toBe(100);
    expect(ch.displayY).toBe(100);
    expect(ch.targetX).toBe(500);
    expect(ch.targetY).toBe(500);
  });
});

describe('activity transitions update animation state', () => {
  it('sets transitionTimer and prevActivity on change', () => {
    const ch = new Character(makeState({ activity: 'thinking' }));
    ch.update(makeState({ activity: 'waiting' }));

    expect(ch.transitionTimer).toBe(0.3);
    expect(ch.prevActivity).toBe('thinking');
  });

  it('skips spring energy for thinking↔tool_use', () => {
    const ch = new Character(makeState({ activity: 'thinking' }));
    const before = ch.springVel;
    ch.update(makeState({ activity: 'tool_use' }));
    expect(ch.springVel).toBe(before);
  });

  it('adds spring energy for other transitions', () => {
    const ch = new Character(makeState({ activity: 'waiting' }));
    ch.update(makeState({ activity: 'thinking' }));
    expect(ch.springVel).toBe(2.5);
  });

  it('resets error flags on errored transition', () => {
    const ch = new Character(makeState({ activity: 'thinking' }));
    ch.update(makeState({ activity: 'errored' }));

    expect(ch.errorStage).toBe(0);
    expect(ch.errorTimer).toBe(0);
    expect(ch.stumbleEmitted).toBe(false);
    expect(ch.starsEmitted).toBe(false);
    expect(ch.spinAngle).toBe(0);
  });

  it('resets completion flags on complete transition', () => {
    const ch = new Character(makeState({ activity: 'thinking' }));
    ch.update(makeState({ activity: 'complete' }));

    expect(ch.confettiEmitted).toBe(false);
    expect(ch.completionTimer).toBe(0);
    expect(ch.goldFlash).toBe(0);
  });

  it('restores opacity when resuming from terminal activity', () => {
    const ch = new Character(makeState({ activity: 'lost' }));
    ch.opacity = 0.2;
    ch.spinAngle = 1.5;

    ch.update(makeState({ activity: 'thinking' }));

    expect(ch.opacity).toBe(1.0);
    expect(ch.spinAngle).toBe(0);
  });

  it('does not restore opacity for terminal→terminal', () => {
    const ch = new Character(makeState({ activity: 'errored' }));
    ch.opacity = 0.5;

    ch.update(makeState({ activity: 'lost' }));

    expect(ch.opacity).toBe(0.5);
  });

  it('transitionTimer counts down in animate', () => {
    const ch = new Character(makeState({ activity: 'thinking' }));
    ch.update(makeState({ activity: 'waiting' }));
    expect(ch.transitionTimer).toBe(0.3);

    ch.animate(null, 0.1);
    expect(ch.transitionTimer).toBeCloseTo(0.2, 5);

    ch.animate(null, 0.1);
    expect(ch.transitionTimer).toBeCloseTo(0.1, 5);

    ch.animate(null, 0.15);
    expect(ch.transitionTimer).toBe(0);
  });

  it('adds spring bounce on churning start', () => {
    const ch = new Character(makeState({ activity: 'waiting', isChurning: false }));
    ch.update(makeState({ activity: 'waiting', isChurning: true }));
    expect(ch.springVel).toBeCloseTo(1.2);
  });

  it('no-ops when activity stays the same', () => {
    const ch = new Character(makeState({ activity: 'thinking' }));
    ch.transitionTimer = 0;

    ch.update(makeState({ activity: 'thinking' }));

    expect(ch.transitionTimer).toBe(0);
    expect(ch.prevActivity).toBe('thinking');
  });
});

describe('animation states by activity', () => {
  it('thinking increases runPhase (running)', () => {
    const ch = new Character(makeState({ activity: 'thinking' }));
    const before = ch.runPhase;
    ch.animate(null, 1 / 60);
    expect(ch.runPhase).toBeGreaterThan(before);
  });

  it('tool_use increases runPhase faster than thinking (sprinting)', () => {
    const thinking = new Character(makeState({ activity: 'thinking' }));
    const sprinting = new Character(makeState({ activity: 'tool_use' }));

    for (let i = 0; i < 10; i++) {
      thinking.animate(null, 1 / 60);
      sprinting.animate(null, 1 / 60);
    }

    expect(sprinting.runPhase).toBeGreaterThan(thinking.runPhase);
  });

  it('default/idle increases stretchPhase', () => {
    const ch = new Character(makeState({ activity: 'idle' }));
    const before = ch.stretchPhase;
    ch.animate(null, 1 / 60);
    expect(ch.stretchPhase).toBeGreaterThan(before);
  });

  it('waiting advances headTurnPhase', () => {
    const ch = new Character(makeState({ activity: 'waiting' }));
    const before = ch.headTurnPhase;
    ch.animate(null, 1 / 60);
    expect(ch.headTurnPhase).toBeGreaterThan(before);
  });

  it('complete advances runPhase for jumping', () => {
    const ch = new Character(makeState({ activity: 'complete' }));
    const before = ch.runPhase;
    ch.animate(null, 1 / 60);
    expect(ch.runPhase).toBeGreaterThan(before);
  });

  it('lost reduces opacity toward 0.2', () => {
    const ch = new Character(makeState({ activity: 'lost' }));
    ch.opacity = 1.0;

    for (let i = 0; i < 200; i++) ch.animate(null, 1 / 60);

    expect(ch.opacity).toBeLessThan(1.0);
    expect(ch.opacity).toBeGreaterThanOrEqual(0.2);
  });

  it('lost advances runPhase slowly (ghost walk)', () => {
    const ch = new Character(makeState({ activity: 'lost' }));
    const before = ch.runPhase;
    ch.animate(null, 1 / 60);
    expect(ch.runPhase).toBeGreaterThan(before);
  });
});

describe('error stage progression', () => {
  it('stage 0 (stumble) for first 0.5s', () => {
    const ch = new Character(makeState({ activity: 'errored' }));

    for (let i = 0; i < 24; i++) ch.animate(null, 1 / 60);

    expect(ch.errorStage).toBe(0);
    expect(ch.errorTimer).toBeLessThan(0.5);
  });

  it('stage 1 (spin) between 0.5–1.0s', () => {
    const ch = new Character(makeState({ activity: 'errored' }));

    for (let i = 0; i < 45; i++) ch.animate(null, 1 / 60);

    expect(ch.errorStage).toBe(1);
  });

  it('stage 2 (face-plant) between 1.0–1.5s', () => {
    const ch = new Character(makeState({ activity: 'errored' }));

    for (let i = 0; i < 75; i++) ch.animate(null, 1 / 60);

    expect(ch.errorStage).toBe(2);
  });

  it('stage 3 (stars) after 1.5s', () => {
    const ch = new Character(makeState({ activity: 'errored' }));

    for (let i = 0; i < 120; i++) ch.animate(null, 1 / 60);

    expect(ch.errorStage).toBe(3);
  });

  it('emits dust on stumble', () => {
    const particles = makeParticles();
    const ch = new Character(makeState({ activity: 'errored' }));

    ch.animate(particles, 1 / 60);

    const dust = particles.emit.mock.calls.filter(c => c[0] === 'dust');
    expect(dust.length).toBeGreaterThan(0);
  });

  it('emits stars in stage 2', () => {
    const particles = makeParticles();
    const ch = new Character(makeState({ activity: 'errored' }));

    for (let i = 0; i < 65; i++) ch.animate(particles, 1 / 60);

    const stars = particles.emit.mock.calls.filter(c => c[0] === 'stars');
    expect(stars.length).toBeGreaterThan(0);
  });

  it('spin accelerates progressively through stages', () => {
    const ch = new Character(makeState({ activity: 'errored' }));

    const deltaByStage = [0, 0, 0, 0];
    let prevAngle = ch.spinAngle;

    for (let i = 0; i < 120; i++) {
      ch.animate(null, 1 / 60);
      const delta = ch.spinAngle - prevAngle;
      deltaByStage[ch.errorStage] += delta;
      prevAngle = ch.spinAngle;
    }

    expect(deltaByStage[1] / 30).toBeGreaterThan(deltaByStage[0] / 30);
  });
});

describe('glow intensity by activity', () => {
  it('thinking → targetGlow 0.08', () => {
    const ch = new Character(makeState({ activity: 'thinking' }));
    ch.animate(null, 1 / 60);
    expect(ch.targetGlow).toBeCloseTo(0.08);
  });

  it('tool_use → targetGlow 0.12', () => {
    const ch = new Character(makeState({ activity: 'tool_use' }));
    ch.animate(null, 1 / 60);
    expect(ch.targetGlow).toBeCloseTo(0.12);
  });

  it('waiting → targetGlow 0.05', () => {
    const ch = new Character(makeState({ activity: 'waiting' }));
    ch.animate(null, 1 / 60);
    expect(ch.targetGlow).toBeCloseTo(0.05);
  });

  it('complete → targetGlow 0.15', () => {
    const ch = new Character(makeState({ activity: 'complete' }));
    ch.animate(null, 1 / 60);
    expect(ch.targetGlow).toBeCloseTo(0.15);
  });

  it('lost → targetGlow 0', () => {
    const ch = new Character(makeState({ activity: 'lost' }));
    ch.animate(null, 1 / 60);
    expect(ch.targetGlow).toBe(0);
  });

  it('clamps targetGlow in pit', () => {
    const ch = new Character(makeState({ activity: 'thinking' }));
    ch.inPit = true;
    ch.animate(null, 1 / 60);
    expect(ch.targetGlow).toBeLessThanOrEqual(0.02);
  });

  it('clamps targetGlow in parking lot', () => {
    const ch = new Character(makeState({ activity: 'tool_use' }));
    ch.inParkingLot = true;
    ch.animate(null, 1 / 60);
    expect(ch.targetGlow).toBeLessThanOrEqual(0.02);
  });

  it('glowIntensity interpolates toward targetGlow', () => {
    const ch = new Character(makeState({ activity: 'complete' }));
    ch.glowIntensity = 0;

    for (let i = 0; i < 80; i++) ch.animate(null, 1 / 60);

    expect(ch.glowIntensity).toBeGreaterThan(0.1);
  });

  it('higher burn rate increases targetGlow for thinking', () => {
    const ch = new Character(makeState({ activity: 'thinking', burnRatePerMinute: 6000 }));
    ch.animate(null, 1 / 60);
    expect(ch.targetGlow).toBeCloseTo(0.14);
  });

  it('higher burn rate increases targetGlow for tool_use', () => {
    const ch = new Character(makeState({ activity: 'tool_use', burnRatePerMinute: 3000 }));
    ch.animate(null, 1 / 60);
    expect(ch.targetGlow).toBeCloseTo(0.16);
  });
});

describe('setTarget and animate movement', () => {
  it('setTarget sets target coordinates', () => {
    const ch = new Character(makeState());
    ch.setTarget(300, 200);

    expect(ch.targetX).toBe(300);
    expect(ch.targetY).toBe(200);
  });

  it('animate moves character toward target', () => {
    const ch = new Character(makeState());
    ch.initialized = true;
    ch.displayX = 0;
    ch.targetX = 200;

    for (let i = 0; i < 60; i++) ch.animate(null, 1 / 60);

    expect(ch.displayX).toBeGreaterThan(0);
    expect(ch.displayX).toBeLessThan(200);
  });
});

describe('zone transition waypoints', () => {
  it('startZoneTransition sets waypoints', () => {
    const ch = new Character(makeState());
    const waypoints = [{ x: 100, y: 50 }, { x: 200, y: 100 }];
    ch.startZoneTransition(waypoints);

    expect(ch.transitionWaypoints).toBe(waypoints);
    expect(ch.waypointIndex).toBe(0);
  });

  it('advances through waypoints during animate', () => {
    const ch = new Character(makeState());
    ch.initialized = true;
    ch.displayX = 100;
    ch.displayY = 50;
    ch.startZoneTransition([{ x: 100, y: 50 }, { x: 200, y: 100 }]);

    // Character is at first waypoint, should advance
    ch.animate(null, 1 / 60);
    expect(ch.waypointIndex).toBe(1);
  });

  it('clears waypoints after reaching last one', () => {
    const ch = new Character(makeState());
    ch.initialized = true;
    ch.displayX = 200;
    ch.displayY = 100;
    ch.startZoneTransition([{ x: 200, y: 100 }]);

    ch.animate(null, 1 / 60);

    expect(ch.transitionWaypoints).toBe(null);
    expect(ch.waypointIndex).toBe(0);
  });
});

describe('draw calls ctx methods without errors', () => {
  function makeCtx() {
    return {
      save: vi.fn(),
      restore: vi.fn(),
      translate: vi.fn(),
      rotate: vi.fn(),
      scale: vi.fn(),
      fillRect: vi.fn(),
      strokeRect: vi.fn(),
      fillText: vi.fn(),
      beginPath: vi.fn(),
      closePath: vi.fn(),
      moveTo: vi.fn(),
      lineTo: vi.fn(),
      quadraticCurveTo: vi.fn(),
      arc: vi.fn(),
      ellipse: vi.fn(),
      fill: vi.fn(),
      stroke: vi.fn(),
      createRadialGradient: vi.fn(() => ({
        addColorStop: vi.fn(),
      })),
      globalAlpha: 1,
      fillStyle: '',
      strokeStyle: '',
      lineWidth: 1,
      lineCap: 'butt',
      font: '',
      textAlign: 'left',
      textBaseline: 'alphabetic',
      filter: 'none',
    };
  }

  it('draws without error for thinking activity', () => {
    const ch = new Character(makeState({ activity: 'thinking' }));
    ch.setTarget(100, 100);
    ch.animate(null, 1 / 60);
    const ctx = makeCtx();
    expect(() => ch.draw(ctx)).not.toThrow();
    expect(ctx.save).toHaveBeenCalled();
    expect(ctx.restore).toHaveBeenCalled();
  });

  it('draws without error for tool_use activity', () => {
    const ch = new Character(makeState({ activity: 'tool_use' }));
    ch.setTarget(100, 100);
    ch.animate(null, 1 / 60);
    const ctx = makeCtx();
    expect(() => ch.draw(ctx)).not.toThrow();
  });

  it('draws without error for waiting activity', () => {
    const ch = new Character(makeState({ activity: 'waiting' }));
    ch.setTarget(100, 100);
    ch.animate(null, 1 / 60);
    const ctx = makeCtx();
    expect(() => ch.draw(ctx)).not.toThrow();
  });

  it('draws without error for complete activity', () => {
    const ch = new Character(makeState({ activity: 'complete' }));
    ch.setTarget(100, 100);
    ch.animate(null, 1 / 60);
    const ctx = makeCtx();
    expect(() => ch.draw(ctx)).not.toThrow();
  });

  it('draws without error for errored activity', () => {
    const ch = new Character(makeState({ activity: 'errored' }));
    ch.setTarget(100, 100);
    for (let i = 0; i < 100; i++) ch.animate(null, 1 / 60);
    const ctx = makeCtx();
    expect(() => ch.draw(ctx)).not.toThrow();
  });

  it('draws without error for lost activity with ghost trail', () => {
    const ch = new Character(makeState({ activity: 'lost' }));
    ch.setTarget(100, 100);
    for (let i = 0; i < 10; i++) ch.animate(null, 1 / 60);
    const ctx = makeCtx();
    expect(() => ch.draw(ctx)).not.toThrow();
  });

  it('draws glow when glowIntensity > 0', () => {
    const ch = new Character(makeState({ activity: 'thinking' }));
    ch.setTarget(100, 100);
    ch.glowIntensity = 0.1;
    const ctx = makeCtx();
    ch.draw(ctx);
    expect(ctx.createRadialGradient).toHaveBeenCalled();
  });

});

describe('each animation state produces expected visual changes', () => {
  it('thinking emits dust particles when moving', () => {
    const particles = makeParticles();
    const ch = new Character(makeState({ activity: 'thinking' }));
    ch.initialized = true;
    ch.displayX = 0;
    ch.targetX = 500;

    // Run many frames to get particle emission (random)
    for (let i = 0; i < 100; i++) ch.animate(particles, 1 / 60);

    const dust = particles.emit.mock.calls.filter(c => c[0] === 'dust');
    expect(dust.length).toBeGreaterThan(0);
  });

  it('tool_use emits speed lines when moving fast', () => {
    const particles = makeParticles();
    const ch = new Character(makeState({ activity: 'tool_use' }));
    ch.initialized = true;
    ch.displayX = 0;
    ch.targetX = 500;

    for (let i = 0; i < 100; i++) ch.animate(particles, 1 / 60);

    const speedLines = particles.emitWithColor.mock.calls.filter(c => c[0] === 'speedLines');
    expect(speedLines.length).toBeGreaterThan(0);
  });

  it('complete emits celebration particles', () => {
    const particles = makeParticles();
    const ch = new Character(makeState({ activity: 'complete' }));

    ch.animate(particles, 1 / 60);

    const celebration = particles.emit.mock.calls.filter(c => c[0] === 'celebration');
    expect(celebration.length).toBe(1);
  });

  it('complete only emits confetti once', () => {
    const particles = makeParticles();
    const ch = new Character(makeState({ activity: 'complete' }));

    for (let i = 0; i < 10; i++) ch.animate(particles, 1 / 60);

    const celebration = particles.emit.mock.calls.filter(c => c[0] === 'celebration');
    expect(celebration.length).toBe(1);
  });

  it('waiting activates yawn after delay', () => {
    const ch = new Character(makeState({ activity: 'waiting' }));

    // Run for > 4 seconds to trigger yawn
    for (let i = 0; i < 300; i++) ch.animate(null, 1 / 60);

    expect(ch.yawnActive).toBe(true);
  });
});
