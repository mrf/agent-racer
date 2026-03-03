// Crew member scale — small pixel-art figures
const S = 1.5;

// Positions relative to car center (car faces right: nose at +x, rear at -x)
// HIT_LEFT≈125 (rear), HIT_RIGHT≈60 (front), height≈56px
const MEMBER_DEFS = [
  { dx: -105, dy: 0,   tool: 'wrench', spawnDy: -55 }, // Rear — wrench on axle
  { dx:  -45, dy: 28,  tool: 'tire',   spawnDy:  55 }, // Below mid — rolling tire
  { dx:   45, dy: 0,   tool: 'fuel',   spawnDy: -55 }, // Front — refueling
];

export class PitCrew {
  constructor(carX, carY, modelColor) {
    this.modelColor = modelColor;
    this.carX = carX;
    this.carY = carY;
    this.leaving = false;
    this.leaveTimer = 0;
    this.boredomTimer = 0;
    this.celebrating = false;

    this.members = MEMBER_DEFS.map((def) => ({
      x: carX + def.dx,
      y: carY + def.dy + def.spawnDy, // Spawn offset — sprint in from edge
      targetX: carX + def.dx,
      targetY: carY + def.dy,
      tool: def.tool,
      spawnDy: def.spawnDy,
      legPhase: Math.random() * Math.PI * 2,
      workPhase: Math.random() * Math.PI * 2,
      zPhase: Math.random() * Math.PI * 2,
      state: 'running',
      atTarget: false,
    }));
  }

  updatePosition(carX, carY) {
    this.carX = carX;
    this.carY = carY;
    for (let i = 0; i < this.members.length; i++) {
      const m = this.members[i];
      if (!this.leaving) {
        const def = MEMBER_DEFS[i];
        m.targetX = carX + def.dx;
        m.targetY = carY + def.dy;
      }
    }
  }

  celebrate() {
    this.celebrating = true;
  }

  leave() {
    if (this.leaving) return;
    this.leaving = true;
    for (let i = 0; i < this.members.length; i++) {
      const m = this.members[i];
      // Sprint back the way they came (invert spawnDy offset)
      m.targetX = m.x;
      m.targetY = m.y + MEMBER_DEFS[i].spawnDy;
      m.state = 'leaving';
    }
  }

  isDone() {
    return this.leaving && this.leaveTimer > 1.4;
  }

  update(dt) {
    const dtScale = dt ? dt / (1 / 60) : 1;

    if (!this.celebrating && !this.leaving) this.boredomTimer += dt;
    if (this.leaving) this.leaveTimer += dt;

    for (const m of this.members) {
      const isMoving = m.state === 'running' || m.state === 'leaving';
      m.legPhase += (isMoving ? 0.22 : 0.04) * dtScale;
      m.workPhase += 0.09 * dtScale;
      m.zPhase += 0.03 * dtScale;

      const dx = m.targetX - m.x;
      const dy = m.targetY - m.y;
      const dist = Math.sqrt(dx * dx + dy * dy);
      const speed = m.state === 'leaving' ? 0.12 : 0.10;

      if (dist > 4) {
        m.x += dx * speed * dtScale;
        m.y += dy * speed * dtScale;
        m.atTarget = false;
        if (m.state !== 'leaving') m.state = 'running';
      } else if (!this.leaving) {
        m.x = m.targetX;
        m.y = m.targetY;
        m.atTarget = true;
        if (this.celebrating) {
          m.state = 'celebrating';
        } else if (this.boredomTimer > 30) {
          m.state = 'bored';
        } else {
          m.state = 'working';
        }
      }
    }
  }

  draw(ctx) {
    ctx.save();
    for (const m of this.members) {
      this._drawMember(ctx, m);
    }
    ctx.restore();
  }

  _drawMember(ctx, m) {
    const x = m.x;
    const y = m.y;

    ctx.save();

    // Fade out crew as they leave
    if (m.state === 'leaving') {
      ctx.globalAlpha *= Math.max(0, 1 - this.leaveTimer / 1.2);
    }

    // Bored: gentle lean
    if (m.state === 'bored') {
      ctx.translate(x, y);
      ctx.rotate(0.10 * Math.sin(m.workPhase * 0.35));
      ctx.translate(-x, -y);
    }

    // Celebration: bounce up
    let jumpY = 0;
    if (m.state === 'celebrating') {
      jumpY = -Math.abs(Math.sin(m.workPhase * 2.5)) * 6 * S;
    }

    const ry = y + jumpY;

    // Ground shadow
    ctx.fillStyle = 'rgba(0,0,0,0.12)';
    ctx.beginPath();
    ctx.ellipse(x, y + 9 * S, 4 * S, 1.3 * S, 0, 0, Math.PI * 2);
    ctx.fill();

    // --- Legs ---
    const isRunning = m.state === 'running' || m.state === 'leaving';
    const legSwing = isRunning
      ? Math.sin(m.legPhase) * 0.55
      : Math.sin(m.legPhase * 0.25) * 0.06;
    const legLen = 6 * S;

    ctx.strokeStyle = this.modelColor.dark;
    ctx.lineWidth = 1.8 * S;
    ctx.lineCap = 'round';

    ctx.beginPath();
    ctx.moveTo(x - 1.5 * S, ry);
    ctx.lineTo(
      x - 1.5 * S + Math.sin(legSwing) * legLen * 0.35,
      ry + Math.cos(Math.abs(legSwing) * 0.8) * legLen,
    );
    ctx.stroke();

    ctx.beginPath();
    ctx.moveTo(x + 1.5 * S, ry);
    ctx.lineTo(
      x + 1.5 * S + Math.sin(-legSwing) * legLen * 0.35,
      ry + Math.cos(Math.abs(legSwing) * 0.8) * legLen,
    );
    ctx.stroke();

    // --- Body / jumpsuit (model color) ---
    const bodyH = 7 * S;
    const bodyTop = ry - bodyH;
    ctx.fillStyle = this.modelColor.main;
    ctx.fillRect(x - 3 * S, bodyTop, 6 * S, bodyH);
    ctx.strokeStyle = this.modelColor.dark;
    ctx.lineWidth = 0.5 * S;
    ctx.strokeRect(x - 3 * S, bodyTop, 6 * S, bodyH);

    // Chest stripe
    ctx.strokeStyle = this.modelColor.light;
    ctx.lineWidth = 0.8 * S;
    ctx.beginPath();
    ctx.moveTo(x - 3 * S, bodyTop + bodyH * 0.45);
    ctx.lineTo(x + 3 * S, bodyTop + bodyH * 0.45);
    ctx.stroke();

    // --- Arms ---
    const armY = bodyTop + 2 * S;
    ctx.strokeStyle = this.modelColor.dark;
    ctx.lineWidth = 1.4 * S;
    ctx.lineCap = 'round';

    if (m.state === 'celebrating') {
      // Arms raised
      ctx.beginPath();
      ctx.moveTo(x - 3 * S, armY);
      ctx.lineTo(x - 5.5 * S, armY - 5 * S + Math.sin(m.workPhase * 3) * S);
      ctx.stroke();
      ctx.beginPath();
      ctx.moveTo(x + 3 * S, armY);
      ctx.lineTo(x + 5.5 * S, armY - 5 * S + Math.sin(m.workPhase * 3 + 1) * S);
      ctx.stroke();
    } else {
      const armSwing = isRunning ? Math.sin(m.legPhase) * 0.4 : 0;
      ctx.beginPath();
      ctx.moveTo(x - 3 * S, armY);
      ctx.lineTo(
        x - 3 * S + Math.sin(armSwing) * 4 * S * 0.4,
        armY + Math.cos(armSwing) * 4 * S * 0.5,
      );
      ctx.stroke();
      ctx.beginPath();
      ctx.moveTo(x + 3 * S, armY);
      ctx.lineTo(
        x + 3 * S + Math.sin(-armSwing) * 4 * S * 0.4,
        armY + Math.cos(-armSwing) * 4 * S * 0.5,
      );
      ctx.stroke();
    }

    // --- Head ---
    const headY = bodyTop - 4 * S;
    ctx.fillStyle = '#fcd5b0';
    ctx.beginPath();
    ctx.arc(x, headY, 3.5 * S, 0, Math.PI * 2);
    ctx.fill();

    // Helmet (upper half, model color)
    ctx.fillStyle = this.modelColor.main;
    ctx.beginPath();
    ctx.arc(x, headY, 3.5 * S, Math.PI, 0);
    ctx.fill();
    ctx.strokeStyle = this.modelColor.light;
    ctx.lineWidth = 0.6 * S;
    ctx.beginPath();
    ctx.arc(x, headY, 3.5 * S, Math.PI + 0.2, -0.2);
    ctx.stroke();

    // Eyes
    ctx.fillStyle = '#222';
    ctx.beginPath();
    ctx.arc(x - 1.2 * S, headY + 0.5 * S, 0.7 * S, 0, Math.PI * 2);
    ctx.fill();
    ctx.beginPath();
    ctx.arc(x + 1.2 * S, headY + 0.5 * S, 0.7 * S, 0, Math.PI * 2);
    ctx.fill();

    // --- State-specific overlays ---
    if (m.state === 'working') {
      this._drawTool(ctx, x, ry, m);
    } else if (m.state === 'bored') {
      this._drawBored(ctx, x, ry, m);
    } else if (m.state === 'celebrating') {
      this._drawCelebrating(ctx, x, ry, m);
    }

    ctx.restore();
  }

  _drawTool(ctx, x, y, m) {
    ctx.save();
    const bodyTop = y - 7 * S;

    switch (m.tool) {
      case 'wrench': {
        // Wrench held out to the side, oscillating up-down
        const wx = x + 5.5 * S;
        const wy = bodyTop + 3 * S + Math.sin(m.workPhase) * 2.5 * S;
        ctx.translate(wx, wy);
        ctx.rotate(-0.45 + Math.sin(m.workPhase) * 0.4);
        ctx.strokeStyle = '#888';
        ctx.lineWidth = 1.2 * S;
        ctx.lineCap = 'round';
        ctx.beginPath();
        ctx.moveTo(0, -4 * S);
        ctx.lineTo(0, 3 * S);
        ctx.stroke();
        // Open-end head
        ctx.beginPath();
        ctx.moveTo(-1.5 * S, -4 * S);
        ctx.lineTo(-1 * S, -6 * S);
        ctx.lineTo(1 * S, -6 * S);
        ctx.lineTo(1.5 * S, -4 * S);
        ctx.stroke();
        break;
      }

      case 'tire': {
        // Rolling tire on the near side
        const tx = x - 10 * S + Math.sin(m.workPhase * 0.35) * 1.5 * S;
        const ty = y + 2 * S;
        ctx.strokeStyle = '#222';
        ctx.lineWidth = 2.5 * S;
        ctx.beginPath();
        ctx.arc(tx, ty, 4 * S, 0, Math.PI * 2);
        ctx.stroke();
        // Hubcap
        ctx.fillStyle = '#444';
        ctx.beginPath();
        ctx.arc(tx, ty, 1.5 * S, 0, Math.PI * 2);
        ctx.fill();
        // Tread line (rotates to show rolling)
        ctx.strokeStyle = '#555';
        ctx.lineWidth = 0.6 * S;
        ctx.beginPath();
        ctx.moveTo(tx, ty);
        ctx.lineTo(
          tx + Math.cos(m.workPhase * 2) * 3.5 * S,
          ty + Math.sin(m.workPhase * 2) * 3.5 * S,
        );
        ctx.stroke();
        break;
      }

      case 'fuel': {
        // Red fuel can, nozzle angled toward car
        const fx = x - 6 * S;
        const fy = y - 4 * S + Math.sin(m.workPhase * 0.5) * S;
        // Can body
        ctx.fillStyle = '#e44';
        ctx.fillRect(fx - 2.5 * S, fy - 3 * S, 5 * S, 6.5 * S);
        ctx.strokeStyle = '#a00';
        ctx.lineWidth = 0.5 * S;
        ctx.strokeRect(fx - 2.5 * S, fy - 3 * S, 5 * S, 6.5 * S);
        // Handle top
        ctx.strokeStyle = '#a00';
        ctx.lineWidth = 1 * S;
        ctx.lineCap = 'round';
        ctx.beginPath();
        ctx.moveTo(fx - 1 * S, fy - 3 * S);
        ctx.lineTo(fx - 1 * S, fy - 5 * S);
        ctx.lineTo(fx + 1 * S, fy - 5 * S);
        ctx.lineTo(fx + 1 * S, fy - 3 * S);
        ctx.stroke();
        // Nozzle
        ctx.strokeStyle = '#555';
        ctx.lineWidth = 1 * S;
        ctx.beginPath();
        ctx.moveTo(fx + 2.5 * S, fy);
        ctx.lineTo(fx + 5 * S, fy + 2 * S);
        ctx.stroke();
        break;
      }
    }

    ctx.restore();
  }

  _drawBored(ctx, x, y, m) {
    // One arm raised to check watch
    const watchOscillate = Math.sin(m.workPhase * 0.25);
    const bodyTop = y - 7 * S;
    const armBaseY = bodyTop + 2 * S;

    ctx.save();
    ctx.strokeStyle = this.modelColor.dark;
    ctx.lineWidth = 1.4 * S;
    ctx.lineCap = 'round';

    // Right arm raised to face level
    const wristX = x + 4 * S + watchOscillate * 1.5 * S;
    const wristY = armBaseY - 3.5 * S;
    ctx.beginPath();
    ctx.moveTo(x + 3 * S, armBaseY);
    ctx.lineTo(wristX, wristY);
    ctx.stroke();

    // Watch face
    ctx.fillStyle = '#b8b8b8';
    ctx.beginPath();
    ctx.arc(wristX, wristY - 1.5 * S, 2 * S, 0, Math.PI * 2);
    ctx.fill();
    ctx.strokeStyle = '#666';
    ctx.lineWidth = 0.4 * S;
    ctx.stroke();
    // Watch hands
    ctx.strokeStyle = '#333';
    ctx.lineWidth = 0.5 * S;
    ctx.beginPath();
    ctx.moveTo(wristX, wristY - 1.5 * S);
    ctx.lineTo(
      wristX + Math.cos(m.workPhase * 0.5) * 1.2 * S,
      wristY - 1.5 * S + Math.sin(m.workPhase * 0.5) * 1.2 * S,
    );
    ctx.stroke();

    // Occasional floating 'z'
    if (Math.sin(m.zPhase) > 0.55) {
      const zAlpha = (Math.sin(m.zPhase) - 0.55) / 0.45;
      ctx.fillStyle = `rgba(150,150,200,${zAlpha * 0.65})`;
      ctx.font = `bold ${3 * S}px monospace`;
      ctx.textAlign = 'center';
      ctx.fillText('z', x + 6 * S, y - 17 * S);
    }

    ctx.restore();
  }

  _drawCelebrating(ctx, x, y, m) {
    // Gold sparkles orbiting the crew figure
    ctx.save();
    ctx.fillStyle = '#ffd700';
    const numSparks = 5;
    for (let i = 0; i < numSparks; i++) {
      const angle = (i / numSparks) * Math.PI * 2 + m.workPhase * 1.8;
      const r = 9 * S;
      const jumpY = -Math.abs(Math.sin(m.workPhase * 2.5)) * 6 * S;
      const sx = x + Math.cos(angle) * r;
      const sy = y + jumpY + Math.sin(angle) * r * 0.55;
      const sparkR = (0.7 + 0.5 * Math.sin(m.workPhase * 3 + i)) * S;
      ctx.beginPath();
      ctx.arc(sx, sy, sparkR, 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.restore();
  }
}
