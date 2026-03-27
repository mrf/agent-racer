function buildDOM() {
  const overlay = document.createElement('div');
  overlay.id = 'help-popup';
  overlay.className = 'help-popup hidden';
  overlay.setAttribute('role', 'dialog');
  overlay.setAttribute('aria-label', 'Help');
  overlay.setAttribute('aria-modal', 'true');

  overlay.innerHTML = `
    <div class="help-inner">
      <div class="help-header">
        <span class="help-title">Agent Racer — Quick Reference</span>
        <button class="help-close" aria-label="Close help">\u00D7</button>
      </div>
      <div class="help-body">

        <div class="help-section">
          <h3 class="help-section-title">The Track</h3>
          <dl class="help-dl">
            <dt>Race Track</dt>
            <dd>Active coding sessions. Car speed reflects how fast the agent is producing output.</dd>
            <dt>Pit Lane</dt>
            <dd>Stale sessions — no recent output. Cars pull off the track to wait.</dd>
            <dt>Parking Lot</dt>
            <dd>Completed, errored, or lost sessions park here when finished.</dd>
          </dl>
        </div>

        <div class="help-section">
          <h3 class="help-section-title">Cars</h3>
          <dl class="help-dl">
            <dt>Labels</dt>
            <dd>Each car shows its session name. Click a car for full details.</dd>
            <dt>Tmux Jump</dt>
            <dd>Click on a car running in a tmux window to jump to that session (cursor highlights clickable cars).</dd>
            <dt>Colors</dt>
            <dd>
              <span class="help-swatch help-swatch-opus"></span> Opus&ensp;
              <span class="help-swatch help-swatch-sonnet"></span> Sonnet&ensp;
              <span class="help-swatch help-swatch-haiku"></span> Haiku&ensp;
              <span class="help-swatch help-swatch-other"></span> Other
            </dd>
            <dt>Source Badges</dt>
            <dd>
              <span class="help-badge">C</span> Claude&ensp;
              <span class="help-badge">X</span> Codex&ensp;
              <span class="help-badge">G</span> Gemini
            </dd>
            <dt>Hamsters</dt>
            <dd>Sub-agent sessions ride inside their parent car. Click them for details too.</dd>
          </dl>
        </div>

        <div class="help-section">
          <h3 class="help-section-title">Animations</h3>
          <dl class="help-dl">
            <dt>Exhaust Particles</dt>
            <dd>Trail behind active cars — more particles means more output.</dd>
            <dt>Speed Lines</dt>
            <dd>Appear at high activity rates as a car accelerates.</dd>
            <dt>Victory / Crash</dt>
            <dd>Confetti on completion, sparks on error.</dd>
          </dl>
        </div>

        <div class="help-section">
          <h3 class="help-section-title">Sounds</h3>
          <dl class="help-dl">
            <dt>Engine Hum</dt>
            <dd>Each active car has a synthesized engine — pitch rises with activity.</dd>
            <dt>Crowd Noise</dt>
            <dd>Ambient cheering scales with the number of active sessions.</dd>
            <dt>Victory Fanfare</dt>
            <dd>Plays when a session completes successfully.</dd>
            <dt>Crash Sound</dt>
            <dd>Plays when a session errors out.</dd>
          </dl>
        </div>

        <div class="help-section">
          <h3 class="help-section-title">Keyboard Shortcuts</h3>
          <table class="help-shortcuts">
            <tr><td class="help-shortcut-key">?</td><td>Toggle this help</td></tr>
            <tr><td class="help-shortcut-key">A</td><td>Achievements panel</td></tr>
            <tr><td class="help-shortcut-key">B</td><td>Toggle speech bubbles</td></tr>
            <tr><td class="help-shortcut-key">G</td><td>Garage (cosmetics)</td></tr>
            <tr><td class="help-shortcut-key">C</td><td>Cycle commentary (ticker / announcer / off)</td></tr>
            <tr><td class="help-shortcut-key">D</td><td>Debug log</td></tr>
            <tr><td class="help-shortcut-key">E</td><td>Track editor</td></tr>
            <tr><td class="help-shortcut-key">M</td><td>Mute / unmute sound</td></tr>
            <tr><td class="help-shortcut-key">S</td><td>Volume sliders (master / ambient / sfx)</td></tr>
            <tr><td class="help-shortcut-key">N</td><td>Toggle mini-map radar (bottom-right)</td></tr>
            <tr><td class="help-shortcut-key">Shift+N</td><td>Toggle mini-map zoom (also: Tab to focus)</td></tr>
            <tr><td class="help-shortcut-key">R</td><td>Replay mode — browse &amp; scrub recorded sessions</td></tr>
            <tr><td class="help-shortcut-key">W</td><td>Toggle weather effects</td></tr>
            <tr><td class="help-shortcut-key">V</td><td>Toggle view (Race / Footrace)</td></tr>
            <tr><td class="help-shortcut-key">Shift+F</td><td>Toggle fullscreen</td></tr>
            <tr><td class="help-shortcut-key">Esc</td><td>Close open panel</td></tr>
            <tr><td class="help-shortcut-key">Click</td><td>Car details flyout</td></tr>
          </table>
        </div>

      </div>
    </div>
  `;

  document.body.appendChild(overlay);
  return overlay;
}

export class HelpPopup {
  constructor() {
    this._overlay = null;
    this._visible = false;
  }

  get isVisible() {
    return this._visible;
  }

  _ensureDOM() {
    if (this._overlay) return this._overlay;

    this._overlay = buildDOM();
    this._overlay.querySelector('.help-close').addEventListener('click', () => this.hide());
    this._overlay.addEventListener('click', (e) => {
      if (e.target === this._overlay) this.hide();
    });

    return this._overlay;
  }

  show() {
    if (this._visible) return;
    this._visible = true;
    this._ensureDOM().classList.remove('hidden');
  }

  hide() {
    if (!this._visible) return;
    this._visible = false;
    this._overlay.classList.add('hidden');
  }

  toggle() {
    if (this._visible) this.hide();
    else this.show();
  }
}
