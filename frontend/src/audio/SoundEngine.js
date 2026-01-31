export class SoundEngine {
  constructor() {
    this.ctx = null;
    this.masterGain = null;
    this.ambientBus = null;
    this.sfxBus = null;
    this.muted = false;
    this.ambientRunning = false;
    this.engineNodes = new Map(); // per-racer engine hum
    this.noiseBuffer = null;
    this.impulseBuffer = null;
    this.ambientNodes = [];
    this.duckTimeout = null;
  }

  _ensureCtx() {
    if (this.ctx) return this.ctx;
    this.ctx = new (window.AudioContext || window.webkitAudioContext)();

    // Master bus
    this.masterGain = this.ctx.createGain();
    this.masterGain.gain.value = 1.0;
    this.masterGain.connect(this.ctx.destination);

    // Ambient bus
    this.ambientBus = this.ctx.createGain();
    this.ambientBus.gain.value = 1.0;
    this.ambientBus.connect(this.masterGain);

    // SFX bus
    this.sfxBus = this.ctx.createGain();
    this.sfxBus.gain.value = 1.0;
    this.sfxBus.connect(this.masterGain);

    // Pre-compute shared buffers
    this._createNoiseBuffer();
    this._createImpulseBuffer();

    return this.ctx;
  }

  _createNoiseBuffer() {
    const sampleRate = this.ctx.sampleRate;
    const length = sampleRate; // 1 second
    this.noiseBuffer = this.ctx.createBuffer(1, length, sampleRate);
    const data = this.noiseBuffer.getChannelData(0);
    for (let i = 0; i < length; i++) {
      data[i] = Math.random() * 2 - 1;
    }
  }

  _createImpulseBuffer() {
    const sampleRate = this.ctx.sampleRate;
    const length = Math.floor(sampleRate * 1.5); // 1.5s reverb
    this.impulseBuffer = this.ctx.createBuffer(2, length, sampleRate);
    for (let ch = 0; ch < 2; ch++) {
      const data = this.impulseBuffer.getChannelData(ch);
      for (let i = 0; i < length; i++) {
        data[i] = (Math.random() * 2 - 1) * Math.exp(-i / (sampleRate * 0.4));
      }
    }
  }

  _makeNoise(loop = false) {
    const src = this.ctx.createBufferSource();
    src.buffer = this.noiseBuffer;
    src.loop = loop;
    return src;
  }

  _makeReverb() {
    const conv = this.ctx.createConvolver();
    conv.buffer = this.impulseBuffer;
    return conv;
  }

  _duck() {
    if (!this.ambientBus) return;
    const now = this.ctx.currentTime;
    this.ambientBus.gain.cancelScheduledValues(now);
    this.ambientBus.gain.setValueAtTime(this.ambientBus.gain.value, now);
    this.ambientBus.gain.linearRampToValueAtTime(0.3, now + 0.1);
    clearTimeout(this.duckTimeout);
    this.duckTimeout = setTimeout(() => {
      if (!this.ctx) return;
      const t = this.ctx.currentTime;
      this.ambientBus.gain.cancelScheduledValues(t);
      this.ambientBus.gain.setValueAtTime(this.ambientBus.gain.value, t);
      this.ambientBus.gain.linearRampToValueAtTime(1.0, t + 0.5);
    }, 300);
  }

  // --- Ambient sounds ---

  startAmbient() {
    if (this.ambientRunning) return;
    try {
      this._ensureCtx();
    } catch { return; }
    this.ambientRunning = true;

    if (this.ctx.state === 'suspended') {
      this.ctx.resume();
    }

    this._startCrowd();
    this._startWind();
  }

  _startCrowd() {
    // White noise through bandpass for crowd murmur
    const noise = this._makeNoise(true);
    const bandpass = this.ctx.createBiquadFilter();
    bandpass.type = 'bandpass';
    bandpass.frequency.value = 750;
    bandpass.Q.value = 0.5;

    // LFO on filter cutoff for organic murmur
    const lfo = this.ctx.createOscillator();
    const lfoGain = this.ctx.createGain();
    lfo.frequency.value = 0.1;
    lfoGain.gain.value = 300;
    lfo.connect(lfoGain);
    lfoGain.connect(bandpass.frequency);
    lfo.start();

    const gain = this.ctx.createGain();
    gain.gain.value = 0.02;

    noise.connect(bandpass);
    bandpass.connect(gain);
    gain.connect(this.ambientBus);
    noise.start();

    this.ambientNodes.push({ noise, lfo, gain, bandpass });
  }

  _startWind() {
    // Brown noise (filtered white noise through lowpass)
    const noise = this._makeNoise(true);
    const lowpass = this.ctx.createBiquadFilter();
    lowpass.type = 'lowpass';
    lowpass.frequency.value = 400;

    const gain = this.ctx.createGain();
    gain.gain.value = 0.015;

    noise.connect(lowpass);
    lowpass.connect(gain);
    gain.connect(this.ambientBus);
    noise.start();

    this.ambientNodes.push({ noise, gain, lowpass });

    // Occasional gusts
    this._scheduleGust(gain);
  }

  _scheduleGust(gainNode) {
    const delay = 5000 + Math.random() * 10000; // 5-15s
    setTimeout(() => {
      if (!this.ambientRunning || !this.ctx) return;
      const now = this.ctx.currentTime;
      gainNode.gain.cancelScheduledValues(now);
      gainNode.gain.setValueAtTime(0.015, now);
      gainNode.gain.linearRampToValueAtTime(0.04, now + 0.5);
      gainNode.gain.linearRampToValueAtTime(0.015, now + 2.5);
      this._scheduleGust(gainNode);
    }, delay);
  }

  stopAmbient() {
    this.ambientRunning = false;
    for (const nodes of this.ambientNodes) {
      try { nodes.noise.stop(); } catch { /* ok */ }
      try { nodes.lfo?.stop(); } catch { /* ok */ }
    }
    this.ambientNodes = [];
  }

  // --- Per-racer engine hum ---

  startEngine(racerId, activity) {
    if (this.muted || !this.ctx) return;
    if (activity !== 'thinking' && activity !== 'tool_use') {
      this.stopEngine(racerId);
      return;
    }

    const pitchMult = activity === 'tool_use' ? 1.4 : 1.0;
    const existing = this.engineNodes.get(racerId);

    if (existing) {
      // Update pitch smoothly
      const now = this.ctx.currentTime;
      existing.osc1.frequency.linearRampToValueAtTime(80 * pitchMult, now + 0.15);
      existing.osc2.frequency.linearRampToValueAtTime(82 * pitchMult, now + 0.15);
      existing.filter.frequency.linearRampToValueAtTime(200 * pitchMult, now + 0.15);
      return;
    }

    // Create new engine hum: 2 detuned sawtooth through lowpass
    const osc1 = this.ctx.createOscillator();
    const osc2 = this.ctx.createOscillator();
    osc1.type = 'sawtooth';
    osc2.type = 'sawtooth';
    osc1.frequency.value = 80 * pitchMult;
    osc2.frequency.value = 82 * pitchMult;

    const filter = this.ctx.createBiquadFilter();
    filter.type = 'lowpass';
    filter.frequency.value = 200 * pitchMult;

    const gain = this.ctx.createGain();
    gain.gain.value = 0;
    const now = this.ctx.currentTime;
    gain.gain.linearRampToValueAtTime(0.04, now + 0.1);

    osc1.connect(filter);
    osc2.connect(filter);
    filter.connect(gain);
    gain.connect(this.ambientBus);
    osc1.start();
    osc2.start();

    this.engineNodes.set(racerId, { osc1, osc2, filter, gain });
  }

  stopEngine(racerId) {
    const nodes = this.engineNodes.get(racerId);
    if (!nodes) return;

    if (this.ctx) {
      const now = this.ctx.currentTime;
      nodes.gain.gain.cancelScheduledValues(now);
      nodes.gain.gain.setValueAtTime(nodes.gain.gain.value, now);
      nodes.gain.gain.linearRampToValueAtTime(0, now + 0.2);
      setTimeout(() => {
        try { nodes.osc1.stop(); } catch { /* ok */ }
        try { nodes.osc2.stop(); } catch { /* ok */ }
      }, 300);
    } else {
      try { nodes.osc1.stop(); } catch { /* ok */ }
      try { nodes.osc2.stop(); } catch { /* ok */ }
    }
    this.engineNodes.delete(racerId);
  }

  // --- One-shot SFX ---

  playGearShift() {
    if (this.muted) return;
    try { this._ensureCtx(); } catch { return; }
    this._duck();
    const now = this.ctx.currentTime;

    const osc = this.ctx.createOscillator();
    osc.type = 'triangle';
    osc.frequency.setValueAtTime(300, now);
    osc.frequency.linearRampToValueAtTime(600, now + 0.08);

    const gain = this.ctx.createGain();
    gain.gain.setValueAtTime(0.15, now);
    gain.gain.exponentialRampToValueAtTime(0.001, now + 0.12);

    osc.connect(gain);
    gain.connect(this.sfxBus);
    osc.start(now);
    osc.stop(now + 0.12);
  }

  playVictory() {
    if (this.muted) return;
    try { this._ensureCtx(); } catch { return; }
    this._duck();
    const now = this.ctx.currentTime;
    const reverb = this._makeReverb();
    reverb.connect(this.sfxBus);

    const chords = [
      { freqs: [261.63, 329.63, 392.00], start: 0, dur: 0.3 },    // C4+E4+G4
      { freqs: [392.00, 493.88, 587.33], start: 0.3, dur: 0.3 },  // G4+B4+D5
      { freqs: [523.25, 659.25, 783.99], start: 0.6, dur: 0.6 },  // C5+E5+G5
    ];

    for (const chord of chords) {
      for (const freq of chord.freqs) {
        const osc = this.ctx.createOscillator();
        osc.type = 'sine';
        osc.frequency.value = freq;

        const gain = this.ctx.createGain();
        const t = now + chord.start;
        gain.gain.setValueAtTime(0, t);
        gain.gain.linearRampToValueAtTime(0.12, t + 0.02); // 20ms attack
        gain.gain.setValueAtTime(0.12, t + chord.dur - 0.05);
        gain.gain.exponentialRampToValueAtTime(0.001, t + chord.dur + 0.4); // 400ms release

        osc.connect(gain);
        gain.connect(reverb);
        osc.start(t);
        osc.stop(t + chord.dur + 0.4);
      }
    }
  }

  playCrash() {
    if (this.muted) return;
    try { this._ensureCtx(); } catch { return; }
    this._duck();
    const now = this.ctx.currentTime;

    // White noise burst through bandpass
    const noise = this._makeNoise(false);
    const bandpass = this.ctx.createBiquadFilter();
    bandpass.type = 'bandpass';
    bandpass.frequency.value = 500;
    bandpass.Q.value = 0.8;
    const noiseGain = this.ctx.createGain();
    noiseGain.gain.setValueAtTime(0.3, now);
    noiseGain.gain.exponentialRampToValueAtTime(0.001, now + 0.2);
    noise.connect(bandpass);
    bandpass.connect(noiseGain);
    noiseGain.connect(this.sfxBus);
    noise.start(now);
    noise.stop(now + 0.25);

    // Descending sawtooth
    const saw = this.ctx.createOscillator();
    saw.type = 'sawtooth';
    saw.frequency.setValueAtTime(400, now);
    saw.frequency.exponentialRampToValueAtTime(50, now + 0.5);
    const sawGain = this.ctx.createGain();
    sawGain.gain.setValueAtTime(0.15, now);
    sawGain.gain.exponentialRampToValueAtTime(0.001, now + 0.5);
    saw.connect(sawGain);
    sawGain.connect(this.sfxBus);
    saw.start(now);
    saw.stop(now + 0.55);

    // Low rumble
    const rumble = this.ctx.createOscillator();
    rumble.type = 'sine';
    rumble.frequency.value = 40;
    const rumbleGain = this.ctx.createGain();
    rumbleGain.gain.setValueAtTime(0.12, now);
    rumbleGain.gain.exponentialRampToValueAtTime(0.001, now + 1.0);
    rumble.connect(rumbleGain);
    rumbleGain.connect(this.sfxBus);
    rumble.start(now);
    rumble.stop(now + 1.05);
  }

  playToolClick() {
    if (this.muted) return;
    try { this._ensureCtx(); } catch { return; }
    const now = this.ctx.currentTime;

    const osc = this.ctx.createOscillator();
    osc.type = 'square';
    osc.frequency.value = 2000;
    const gain = this.ctx.createGain();
    gain.gain.setValueAtTime(0.08, now);
    gain.gain.exponentialRampToValueAtTime(0.001, now + 0.015);
    osc.connect(gain);
    gain.connect(this.sfxBus);
    osc.start(now);
    osc.stop(now + 0.02);
  }

  playAppear() {
    if (this.muted) return;
    try { this._ensureCtx(); } catch { return; }
    this._duck();
    const now = this.ctx.currentTime;

    // Rising whoosh: filtered white noise, highpass sweep 4000 -> 200
    const noise = this._makeNoise(false);
    const hp = this.ctx.createBiquadFilter();
    hp.type = 'highpass';
    hp.frequency.setValueAtTime(4000, now);
    hp.frequency.exponentialRampToValueAtTime(200, now + 0.3);
    const gain = this.ctx.createGain();
    gain.gain.setValueAtTime(0.001, now);
    gain.gain.linearRampToValueAtTime(0.15, now + 0.1);
    gain.gain.exponentialRampToValueAtTime(0.001, now + 0.35);
    noise.connect(hp);
    hp.connect(gain);
    gain.connect(this.sfxBus);
    noise.start(now);
    noise.stop(now + 0.4);
  }

  playDisappear() {
    if (this.muted) return;
    try { this._ensureCtx(); } catch { return; }
    const now = this.ctx.currentTime;

    // Falling whoosh: highpass sweep 200 -> 4000
    const noise = this._makeNoise(false);
    const hp = this.ctx.createBiquadFilter();
    hp.type = 'highpass';
    hp.frequency.setValueAtTime(200, now);
    hp.frequency.exponentialRampToValueAtTime(4000, now + 0.4);
    const gain = this.ctx.createGain();
    gain.gain.setValueAtTime(0.15, now);
    gain.gain.exponentialRampToValueAtTime(0.001, now + 0.4);
    noise.connect(hp);
    hp.connect(gain);
    gain.connect(this.sfxBus);
    noise.start(now);
    noise.stop(now + 0.45);
  }

  // --- Controls ---

  setMuted(muted) {
    this.muted = muted;
    if (this.masterGain) {
      this.masterGain.gain.value = muted ? 0 : 1;
    }
    if (muted) {
      // Stop all engine hums
      for (const id of [...this.engineNodes.keys()]) {
        this.stopEngine(id);
      }
    }
  }
}
