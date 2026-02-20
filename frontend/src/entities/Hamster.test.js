import { describe, it, expect, vi } from 'vitest';
import { Hamster } from './Hamster.js';

function makeState(overrides = {}) {
  return {
    id: 'hamster-1',
    activity: 'thinking',
    model: 'claude-sonnet-4-5-20250929',
    source: 'claude',
    name: 'test-hamster',
    ...overrides,
  };
}

function simulateFrames(hamster, count) {
  for (let i = 0; i < count; i++) {
    hamster.animate(null, 1 / 60);
  }
}

function makeMockCtx() {
  return {
    save: vi.fn(),
    restore: vi.fn(),
    globalAlpha: 1,
    translate: vi.fn(),
    scale: vi.fn(),
    rotate: vi.fn(),
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 1,
    lineCap: '',
    beginPath: vi.fn(),
    moveTo: vi.fn(),
    lineTo: vi.fn(),
    quadraticCurveTo: vi.fn(),
    arc: vi.fn(),
    ellipse: vi.fn(),
    fill: vi.fn(),
    stroke: vi.fn(),
    closePath: vi.fn(),
    createRadialGradient: vi.fn(() => ({
      addColorStop: vi.fn(),
    })),
  };
}

describe('spring physics', () => {
  it('converges springY toward zero after impulse', () => {
    const hamster = new Hamster(makeState());
    hamster.springVel = 5;

    simulateFrames(hamster, 300);

    expect(Math.abs(hamster.springY)).toBeLessThan(0.01);
    expect(Math.abs(hamster.springVel)).toBeLessThan(0.01);
  });

  it('oscillates before settling', () => {
    const hamster = new Hamster(makeState());
    hamster.springVel = 10;

    const positions = [];
    for (let i = 0; i < 60; i++) {
      hamster.animate(null, 1 / 60);
      positions.push(hamster.springY);
    }

    const crossings = positions.filter(
      (p, i) => i > 0 && Math.sign(p) !== Math.sign(positions[i - 1]),
    );
    expect(crossings.length).toBeGreaterThan(0);
  });

  it('damping factor controls decay rate', () => {
    const fast = new Hamster(makeState());
    fast.springDamping = 0.8;
    fast.springVel = 5;

    const slow = new Hamster(makeState());
    slow.springDamping = 0.98;
    slow.springVel = 5;

    for (let i = 0; i < 30; i++) {
      fast.animate(null, 1 / 60);
      slow.animate(null, 1 / 60);
    }

    expect(Math.abs(fast.springVel)).toBeLessThan(Math.abs(slow.springVel));
  });

  it('stiffness determines spring force magnitude', () => {
    const stiff = new Hamster(makeState());
    stiff.springStiffness = 0.2;
    stiff.springY = 10;

    const soft = new Hamster(makeState());
    soft.springStiffness = 0.05;
    soft.springY = 10;

    stiff.animate(null, 1 / 60);
    soft.animate(null, 1 / 60);

    expect(Math.abs(stiff.springVel)).toBeGreaterThan(Math.abs(soft.springVel));
  });

  it('applies spring force opposing displacement', () => {
    const hamster = new Hamster(makeState());
    hamster.springY = 5;
    hamster.springVel = 0;

    hamster.animate(null, 1 / 60);

    expect(hamster.springVel).toBeLessThan(0);
  });
});

describe('lerp interpolation', () => {
  it('moves displayX toward targetX', () => {
    const hamster = new Hamster(makeState());
    hamster.initialized = true;
    hamster.displayX = 0;
    hamster.targetX = 100;

    hamster.animate(null, 1 / 60);

    expect(hamster.displayX).toBeGreaterThan(0);
    expect(hamster.displayX).toBeLessThan(100);
  });

  it('moves displayY toward targetY', () => {
    const hamster = new Hamster(makeState());
    hamster.initialized = true;
    hamster.displayY = 0;
    hamster.targetY = 100;

    hamster.animate(null, 1 / 60);

    expect(hamster.displayY).toBeGreaterThan(0);
    expect(hamster.displayY).toBeLessThan(100);
  });

  it('converges to target over many frames', () => {
    const hamster = new Hamster(makeState());
    hamster.initialized = true;
    hamster.displayX = 0;
    hamster.targetX = 100;
    hamster.displayY = 0;
    hamster.targetY = 50;

    simulateFrames(hamster, 300);

    // Jitter introduces ~1 unit of drift around the target
    expect(hamster.displayX).toBeCloseTo(100, -1);
    expect(hamster.displayY).toBeCloseTo(50, -1);
  });

  it('snaps to initial position on first setTarget', () => {
    const hamster = new Hamster(makeState());
    expect(hamster.initialized).toBe(false);

    hamster.setTarget(200, 150);

    expect(hamster.displayX).toBe(200);
    expect(hamster.displayY).toBe(150);
    expect(hamster.initialized).toBe(true);
  });

  it('does not snap on subsequent setTarget calls', () => {
    const hamster = new Hamster(makeState());
    hamster.setTarget(100, 100);
    hamster.setTarget(500, 500);

    expect(hamster.displayX).toBe(100);
    expect(hamster.displayY).toBe(100);
    expect(hamster.targetX).toBe(500);
    expect(hamster.targetY).toBe(500);
  });

  it('lerp factor is 0.15 for follow delay', () => {
    const hamster = new Hamster(makeState());
    hamster.initialized = true;
    hamster.displayX = 0;
    hamster.targetX = 100;

    hamster.animate(null, 1 / 60);

    // First frame: 100 * 0.15 = 15
    expect(hamster.displayX).toBeCloseTo(15, 0);
  });

  it('applies jitter to position', () => {
    const hamster = new Hamster(makeState());
    hamster.initialized = true;
    hamster.displayX = 100;
    hamster.targetX = 100;
    hamster.displayY = 100;
    hamster.targetY = 100;
    hamster.jitterTimer = 0.35;

    hamster.animate(null, 1 / 60);

    expect(Math.abs(hamster.jitterX)).toBeGreaterThan(0);
    expect(Math.abs(hamster.jitterY)).toBeGreaterThan(0);
  });
});

describe('activity transitions', () => {
  it('updates state on activity change', () => {
    const hamster = new Hamster(makeState({ activity: 'thinking' }));

    hamster.update(makeState({ activity: 'tool_use' }));

    expect(hamster.state.activity).toBe('tool_use');
  });

  it('triggers spring on activity change', () => {
    const hamster = new Hamster(makeState({ activity: 'thinking' }));
    hamster.springVel = 0;

    hamster.update(makeState({ activity: 'tool_use' }));

    expect(hamster.springVel).toBe(1.5);
  });

  it('does not trigger spring if activity does not change', () => {
    const hamster = new Hamster(makeState({ activity: 'thinking' }));
    hamster.springVel = 0;

    hamster.update(makeState({ activity: 'thinking' }));

    expect(hamster.springVel).toBe(0);
  });

  it('resets completion state on complete activity', () => {
    const hamster = new Hamster(makeState({ activity: 'thinking' }));
    hamster.ropeSnapped = true;
    hamster.ropeSnapTimer = 5;
    hamster.completionBurst = true;
    hamster.fadeTimer = 2;
    hamster.goldFlash = 0.5;

    hamster.update(makeState({ activity: 'complete' }));

    expect(hamster.ropeSnapped).toBe(false);
    expect(hamster.ropeSnapTimer).toBe(0);
    expect(hamster.completionBurst).toBe(false);
    expect(hamster.fadeTimer).toBe(0);
    expect(hamster.goldFlash).toBe(0);
  });

  it('increments spring velocity additively on multiple transitions', () => {
    const hamster = new Hamster(makeState({ activity: 'thinking' }));
    hamster.springVel = 0;

    hamster.update(makeState({ activity: 'tool_use' }));
    expect(hamster.springVel).toBe(1.5);

    hamster.update(makeState({ activity: 'waiting' }));
    expect(hamster.springVel).toBe(3);
  });
});

describe('tow rope rendering', () => {
  it('initializes rope state', () => {
    const hamster = new Hamster(makeState());

    expect(hamster.ropeSnapped).toBe(false);
    expect(hamster.ropeSnapTimer).toBe(0);
  });

  it('increments ropeSnapTimer during complete activity', () => {
    const hamster = new Hamster(makeState({ activity: 'thinking' }));

    hamster.update(makeState({ activity: 'complete' }));
    hamster.animate(null, 1 / 60);

    expect(hamster.ropeSnapTimer).toBeCloseTo(1 / 60, 3);
  });

  it('snaps rope after 0.3 seconds in complete activity', () => {
    const hamster = new Hamster(makeState());
    hamster.update(makeState({ activity: 'complete' }));

    simulateFrames(hamster, 20);

    expect(hamster.ropeSnapped).toBe(true);
  });

  it('sets completionBurst when rope snaps', () => {
    const hamster = new Hamster(makeState());
    hamster.update(makeState({ activity: 'complete' }));

    simulateFrames(hamster, 20);

    expect(hamster.completionBurst).toBe(true);
  });

  it('does not snap rope before 0.3 seconds', () => {
    const hamster = new Hamster(makeState());
    hamster.update(makeState({ activity: 'complete' }));

    simulateFrames(hamster, 10);

    expect(hamster.ropeSnapped).toBe(false);
  });
});

describe('fan positioning (wheel spin)', () => {
  it('increments wheelAngle based on movement delta', () => {
    const hamster = new Hamster(makeState());
    hamster.initialized = true;
    hamster.displayX = 0;
    hamster.targetX = 100;
    const initialWheel = hamster.wheelAngle;

    hamster.animate(null, 1 / 60);

    expect(hamster.wheelAngle).toBeGreaterThan(initialWheel);
  });

  it('spins wheels faster with larger movement delta', () => {
    const slow = new Hamster(makeState());
    slow.initialized = true;
    slow.displayX = 0;
    slow.targetX = 10;
    slow.animate(null, 1 / 60);
    const slowWheel = slow.wheelAngle;

    const fast = new Hamster(makeState());
    fast.initialized = true;
    fast.displayX = 0;
    fast.targetX = 100;
    fast.animate(null, 1 / 60);
    const fastWheel = fast.wheelAngle;

    expect(fastWheel).toBeGreaterThan(slowWheel);
  });

  it('wheelAngle continues to increment continuously', () => {
    const hamster = new Hamster(makeState());
    hamster.initialized = true;
    hamster.displayX = 0;
    hamster.targetX = 100;

    const angles = [];
    for (let i = 0; i < 5; i++) {
      hamster.animate(null, 1 / 60);
      angles.push(hamster.wheelAngle);
    }

    for (let i = 1; i < angles.length; i++) {
      expect(angles[i]).toBeGreaterThan(angles[i - 1]);
    }
  });
});

describe('completion fade', () => {
  it('starts with opacity 1.0', () => {
    const hamster = new Hamster(makeState());

    expect(hamster.opacity).toBe(1.0);
  });

  it('decreases opacity after rope snap in complete activity', () => {
    const hamster = new Hamster(makeState());
    hamster.update(makeState({ activity: 'complete' }));

    simulateFrames(hamster, 20);
    const opacityAfterSnap = hamster.opacity;

    simulateFrames(hamster, 60);

    expect(hamster.opacity).toBeLessThan(opacityAfterSnap);
  });

  it('fades to minimum 0.3 opacity', () => {
    const hamster = new Hamster(makeState());
    hamster.update(makeState({ activity: 'complete' }));

    simulateFrames(hamster, 600);

    expect(hamster.opacity).toBeGreaterThanOrEqual(0.3);
  });

  it('maintains opacity during non-complete activities', () => {
    const hamster = new Hamster(makeState({ activity: 'thinking' }));
    const initialOpacity = hamster.opacity;

    hamster.animate(null, 1 / 60);

    expect(hamster.opacity).toBe(initialOpacity);
  });

  it('increments fadeTimer only while ropeSnapped', () => {
    const hamster = new Hamster(makeState());
    hamster.update(makeState({ activity: 'complete' }));

    simulateFrames(hamster, 20);
    const fadeTimerAfterSnap = hamster.fadeTimer;

    hamster.animate(null, 1 / 60);

    expect(hamster.fadeTimer).toBeGreaterThan(fadeTimerAfterSnap);
  });

  it('fade rate is 0.7 per 5 seconds', () => {
    const hamster = new Hamster(makeState());
    hamster.update(makeState({ activity: 'complete' }));

    // Wait for rope to snap (~20 frames at 60fps > 0.3s threshold)
    simulateFrames(hamster, 20);

    // Animate for 5 seconds (300 frames at 60fps)
    simulateFrames(hamster, 300);

    const expectedOpacity = Math.max(0.3, 1.0 - 0.7);
    expect(hamster.opacity).toBeCloseTo(expectedOpacity, 0);
  });
});

describe('activity glow', () => {
  it('initializes glowIntensity to 0', () => {
    const hamster = new Hamster(makeState());

    expect(hamster.glowIntensity).toBe(0);
  });

  it('sets targetGlow based on activity', () => {
    const thinking = new Hamster(makeState({ activity: 'thinking' }));
    thinking.animate(null, 1 / 60);
    expect(thinking.targetGlow).toBe(0.06);

    const toolUse = new Hamster(makeState({ activity: 'tool_use' }));
    toolUse.animate(null, 1 / 60);
    expect(toolUse.targetGlow).toBe(0.10);

    const complete = new Hamster(makeState({ activity: 'complete' }));
    complete.animate(null, 1 / 60);
    expect(complete.targetGlow).toBe(0.12);

    const other = new Hamster(makeState({ activity: 'waiting' }));
    other.animate(null, 1 / 60);
    expect(other.targetGlow).toBe(0.03);
  });

  it('interpolates glowIntensity toward targetGlow', () => {
    const hamster = new Hamster(makeState({ activity: 'thinking' }));

    // First frame sets targetGlow; second frame interpolates toward it
    hamster.animate(null, 1 / 60);
    hamster.animate(null, 1 / 60);

    expect(hamster.glowIntensity).toBeGreaterThan(0);
    expect(hamster.glowIntensity).toBeLessThan(0.06);
  });

  it('converges glow intensity over many frames', () => {
    const hamster = new Hamster(makeState({ activity: 'tool_use' }));

    simulateFrames(hamster, 120);

    expect(hamster.glowIntensity).toBeCloseTo(0.10, 1);
  });
});

describe('continuous animations', () => {
  it('increments earWigglePhase', () => {
    const hamster = new Hamster(makeState());
    const initialPhase = hamster.earWigglePhase;

    hamster.animate(null, 1 / 60);

    expect(hamster.earWigglePhase).toBeGreaterThan(initialPhase);
  });

  it('increments tailWagPhase', () => {
    const hamster = new Hamster(makeState());
    const initialPhase = hamster.tailWagPhase;

    hamster.animate(null, 1 / 60);

    expect(hamster.tailWagPhase).toBeGreaterThan(initialPhase);
  });

  it('increments dotPhase', () => {
    const hamster = new Hamster(makeState());
    const initialPhase = hamster.dotPhase;

    hamster.animate(null, 1 / 60);

    expect(hamster.dotPhase).toBeGreaterThan(initialPhase);
  });

  it('tail wags faster than ear wiggles', () => {
    const hamster = new Hamster(makeState());
    const initialEar = hamster.earWigglePhase;
    const initialTail = hamster.tailWagPhase;

    simulateFrames(hamster, 60);

    const earDelta = hamster.earWigglePhase - initialEar;
    const tailDelta = hamster.tailWagPhase - initialTail;

    expect(tailDelta).toBeGreaterThan(earDelta);
  });
});

describe('goldFlash on completion', () => {
  it('initializes goldFlash to 0', () => {
    const hamster = new Hamster(makeState());

    expect(hamster.goldFlash).toBe(0);
  });

  it('increments goldFlash during complete activity', () => {
    const hamster = new Hamster(makeState());
    hamster.update(makeState({ activity: 'complete' }));

    hamster.animate(null, 1 / 60);

    expect(hamster.goldFlash).toBeGreaterThan(0);
  });

  it('caps goldFlash at 1.0', () => {
    const hamster = new Hamster(makeState());
    hamster.update(makeState({ activity: 'complete' }));

    simulateFrames(hamster, 100);

    expect(hamster.goldFlash).toBeLessThanOrEqual(1.0);
  });

  it('goldFlash is proportional to ropeSnapTimer', () => {
    const hamster = new Hamster(makeState());
    hamster.update(makeState({ activity: 'complete' }));

    simulateFrames(hamster, 20);

    expect(hamster.goldFlash).toBeCloseTo(hamster.ropeSnapTimer * 2, 2);
  });
});

describe('drawing and rendering', () => {
  it('accepts draw() method with canvas context', () => {
    const hamster = new Hamster(makeState());
    const mockCtx = makeMockCtx();

    expect(() => {
      hamster.draw(mockCtx);
    }).not.toThrow();
  });

  it('respects opacity in draw context', () => {
    const hamster = new Hamster(makeState());
    hamster.opacity = 0.5;

    const mockCtx = makeMockCtx();
    hamster.draw(mockCtx);

    expect(mockCtx.globalAlpha).toBe(0.5);
  });
});

describe('star burst animation', () => {
  it('initializes starBurstPhase to 0', () => {
    const hamster = new Hamster(makeState());

    expect(hamster.starBurstPhase).toBe(0);
  });

  it('increments starBurstPhase during complete activity', () => {
    const hamster = new Hamster(makeState());
    hamster.update(makeState({ activity: 'complete' }));

    hamster.animate(null, 1 / 60);

    expect(hamster.starBurstPhase).toBeGreaterThan(0);
  });

  it('does not increment starBurstPhase in non-complete activities', () => {
    const hamster = new Hamster(makeState({ activity: 'thinking' }));
    const initialPhase = hamster.starBurstPhase;

    hamster.animate(null, 1 / 60);

    expect(hamster.starBurstPhase).toBe(initialPhase);
  });
});
