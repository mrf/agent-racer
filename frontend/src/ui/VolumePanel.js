const STORAGE_KEY = 'agent-racer-volume';
const CHANNELS = ['master', 'ambient', 'sfx'];

const CHANNEL_SETTERS = {
  master: 'setMasterVolume',
  ambient: 'setAmbientVolume',
  sfx: 'setSfxVolume',
};

const CHANNEL_GETTERS = {
  master: 'getMasterVolume',
  ambient: 'getAmbientVolume',
  sfx: 'getSfxVolume',
};

function buildDOM() {
  const panel = document.createElement('div');
  panel.id = 'volume-panel';
  panel.className = 'volume-panel hidden';
  panel.setAttribute('role', 'dialog');
  panel.setAttribute('aria-label', 'Volume Controls');

  panel.innerHTML = `
    <div class="vol-header">
      <span class="vol-title">Sound</span>
      <label class="vol-mute-label">
        <input type="checkbox" class="vol-mute-cb" />
        <span class="vol-mute-text">Mute</span>
      </label>
    </div>
    <div class="vol-sliders">
      <div class="vol-row">
        <span class="vol-label">Master</span>
        <input type="range" min="0" max="100" class="vol-slider" data-channel="master" />
        <span class="vol-value" data-channel="master">100</span>
      </div>
      <div class="vol-row">
        <span class="vol-label">Ambient</span>
        <input type="range" min="0" max="100" class="vol-slider" data-channel="ambient" />
        <span class="vol-value" data-channel="ambient">100</span>
      </div>
      <div class="vol-row">
        <span class="vol-label">SFX</span>
        <input type="range" min="0" max="100" class="vol-slider" data-channel="sfx" />
        <span class="vol-value" data-channel="sfx">100</span>
      </div>
    </div>
  `;

  document.body.appendChild(panel);
  return panel;
}

export class VolumePanel {
  constructor(engine) {
    this.engine = engine;
    this._panel = null;
    this._visible = false;
    this._outsideClickHandler = (e) => {
      if (this._visible && this._panel && !this._panel.contains(e.target)) {
        this.hide();
      }
    };
  }

  get isVisible() { return this._visible; }

  _ensureDOM() {
    if (this._panel) return this._panel;
    this._panel = buildDOM();

    // Wire slider inputs
    const sliders = this._panel.querySelectorAll('.vol-slider');
    for (let i = 0; i < sliders.length; i++) {
      const slider = sliders[i];
      slider.addEventListener('input', () => this._onSliderChange(slider));
    }

    // Wire mute checkbox
    this._panel.querySelector('.vol-mute-cb').addEventListener('change', (e) => {
      this._onMuteChange(e.target.checked);
    });

    // Load saved volumes and apply
    this._loadSaved();

    return this._panel;
  }

  _onSliderChange(slider) {
    const channel = slider.dataset.channel;
    const value = parseInt(slider.value, 10);

    this._panel.querySelector(`.vol-value[data-channel="${channel}"]`).textContent = value;
    this._applyChannelVolume(channel, value / 100);
    this._save();
  }

  _onMuteChange(muted) {
    this.engine.setMuted(muted);
    this._save();
    if (this._onMuteCallback) this._onMuteCallback(muted);
  }

  _save() {
    const data = { muted: this._panel.querySelector('.vol-mute-cb').checked };
    for (let i = 0; i < CHANNELS.length; i++) {
      const ch = CHANNELS[i];
      data[ch] = parseInt(this._panel.querySelector(`.vol-slider[data-channel="${ch}"]`).value, 10);
    }
    localStorage.setItem(STORAGE_KEY, JSON.stringify(data));
  }

  _loadSaved() {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return;
    try {
      const data = JSON.parse(raw);
      for (let i = 0; i < CHANNELS.length; i++) {
        const ch = CHANNELS[i];
        if (typeof data[ch] === 'number') this._setSlider(ch, data[ch]);
      }
      if (typeof data.muted === 'boolean') {
        this._panel.querySelector('.vol-mute-cb').checked = data.muted;
        this.engine.setMuted(data.muted);
        if (this._onMuteCallback) this._onMuteCallback(data.muted);
      }
    } catch { /* ignore corrupt data */ }
  }

  _applyChannelVolume(channel, normalized) {
    const setter = CHANNEL_SETTERS[channel];
    if (setter) this.engine[setter](normalized);
  }

  _setSlider(channel, value) {
    this._panel.querySelector(`.vol-slider[data-channel="${channel}"]`).value = value;
    this._panel.querySelector(`.vol-value[data-channel="${channel}"]`).textContent = value;
    this._applyChannelVolume(channel, value / 100);
  }

  syncMuteState(muted) {
    this._ensureDOM();
    this._panel.querySelector('.vol-mute-cb').checked = muted;
  }

  onMuteChange(callback) {
    this._onMuteCallback = callback;
  }

  show() {
    if (this._visible) return;
    this._visible = true;
    this._ensureDOM();
    // Sync current engine state into sliders
    for (let i = 0; i < CHANNELS.length; i++) {
      const ch = CHANNELS[i];
      this._setSlider(ch, Math.round(this.engine[CHANNEL_GETTERS[ch]]() * 100));
    }
    this._panel.querySelector('.vol-mute-cb').checked = this.engine.muted;
    this._panel.classList.remove('hidden');
    // Defer outside-click so the opening keypress doesn't immediately close
    setTimeout(() => document.addEventListener('click', this._outsideClickHandler), 0);
  }

  hide() {
    if (!this._visible) return;
    this._visible = false;
    if (this._panel) this._panel.classList.add('hidden');
    document.removeEventListener('click', this._outsideClickHandler);
  }

  toggle() {
    if (this._visible) this.hide();
    else this.show();
  }

  applyLocalPreferences() {
    this._ensureDOM();
  }
}
