import { RaceConnection } from './websocket.js';
import { RaceCanvas } from './canvas/RaceCanvas.js';
import { SoundEngine } from './audio/SoundEngine.js';
import { requestPermission, notifyCompletion } from './notifications.js';

const debugPanel = document.getElementById('debug-panel');
const debugLog = document.getElementById('debug-log');
const debugClose = document.getElementById('debug-close');
const detailFlyout = document.getElementById('detail-flyout');
const flyoutContent = document.getElementById('flyout-content');
const flyoutClose = document.getElementById('flyout-close');
const statusDot = document.getElementById('connection-status');
const sessionCount = document.getElementById('session-count');
const canvas = document.getElementById('race-canvas');

let sessions = new Map();
let debugVisible = false;
let muted = false;
let selectedSessionId = null;
let ambientStarted = false;

// Flyout positioning state — tracks the current anchor side to avoid oscillation
let flyoutAnchor = null;       // 'right', 'left', 'above', 'below'
let flyoutCurrentX = null;     // smoothed X position
let flyoutCurrentY = null;     // smoothed Y position

// Track known session IDs and their activities for detecting appear/disappear/transitions
let knownSessionIds = new Set();
let sessionActivities = new Map();
let lastTransitionSfx = new Map(); // sessionId -> timestamp of last gear/tool SFX
const TRANSITION_SFX_COOLDOWN = 3000; // ms between gear shift / tool click per session

const engine = new SoundEngine();
const raceCanvas = new RaceCanvas(canvas);
raceCanvas.setEngine(engine);
window.raceCanvas = raceCanvas;

// Load sound configuration from backend
async function loadSoundConfig() {
  try {
    const response = await fetch('/api/config');
    if (response.ok) {
      const config = await response.json();
      engine.applyConfig(config);
      log('Sound configuration loaded from server', 'info');
    }
  } catch (err) {
    log(`Failed to load sound config: ${err.message}`, 'error');
  }
}

// Start ambient audio on first user interaction (autoplay policy)
function tryStartAmbient() {
  if (ambientStarted) return;
  ambientStarted = true;
  engine.startAmbient();
}

document.addEventListener('click', tryStartAmbient, { once: false });
document.addEventListener('keydown', tryStartAmbient, { once: false });

function log(msg, type = '') {
  const entry = document.createElement('div');
  entry.className = `log-entry ${type}`;
  const ts = new Date().toLocaleTimeString();
  entry.textContent = `[${ts}] ${msg}`;
  debugLog.appendChild(entry);
  debugLog.scrollTop = debugLog.scrollHeight;

  while (debugLog.children.length > 200) {
    debugLog.removeChild(debugLog.firstChild);
  }
}

function updateSessionCount() {
  const active = [...sessions.values()].filter(
    s => !['complete', 'errored', 'lost'].includes(s.activity)
  ).length;
  sessionCount.textContent = `${active} active / ${sessions.size} total`;
}

function formatTokens(tokens) {
  if (tokens >= 1000) return `${Math.round(tokens / 1000)}K`;
  return `${tokens}`;
}

function formatBurnRate(rate) {
  if (!rate || rate <= 0) return '-';
  if (rate >= 1000) return `${(rate / 1000).toFixed(1)}K/min`;
  return `${Math.round(rate)}/min`;
}

function formatTime(dateStr) {
  if (!dateStr) return '-';
  const d = new Date(dateStr);
  return d.toLocaleTimeString();
}

function esc(s) {
  if (!s) return '';
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function formatElapsed(startStr) {
  if (!startStr) return '-';
  const start = new Date(startStr);
  const elapsed = Math.floor((Date.now() - start.getTime()) / 1000);
  const mins = Math.floor(elapsed / 60);
  const secs = elapsed % 60;
  return `${mins}m ${secs}s`;
}

function showDetailFlyout(state, carX, carY) {
  selectedSessionId = state.id;
  flyoutAnchor = null;       // reset anchor for fresh placement
  flyoutCurrentX = null;
  flyoutCurrentY = null;
  renderDetailFlyout(state);
  detailFlyout.classList.remove('hidden');
  positionFlyout(carX, carY);
}

function positionFlyout(carX, carY) {
  const canvasRect = canvas.getBoundingClientRect();
  const margin = 50;
  const padding = 10;

  // Use actual rendered dimensions (falls back to CSS values if not yet laid out)
  const flyoutWidth = detailFlyout.offsetWidth || 380;
  const flyoutHeight = detailFlyout.offsetHeight || 200;

  // Absolute car position in viewport coordinates
  const carVX = canvasRect.left + carX;
  const carVY = canvasRect.top + carY;

  // Helper: check if a given anchor side can fit the flyout on screen
  const canFit = (anchor) => {
    switch (anchor) {
      case 'right': return carVX + margin + flyoutWidth + padding < window.innerWidth;
      case 'left':  return carVX - margin - flyoutWidth > padding;
      case 'below': return carVY + margin + flyoutHeight + padding < window.innerHeight;
      case 'above': return carVY - margin - flyoutHeight > padding;
      default: return false;
    }
  };

  // Determine anchor — keep existing anchor if it still fits, otherwise pick best
  const preferredOrder = ['right', 'left', 'below', 'above'];
  if (!flyoutAnchor || !canFit(flyoutAnchor)) {
    flyoutAnchor = preferredOrder.find(canFit) || 'right';
  }

  let targetX, targetY, arrowClass;

  switch (flyoutAnchor) {
    case 'right':
      targetX = carVX + margin;
      targetY = carVY - flyoutHeight / 2;
      arrowClass = 'arrow-left';
      break;
    case 'left':
      targetX = carVX - margin - flyoutWidth;
      targetY = carVY - flyoutHeight / 2;
      arrowClass = 'arrow-right';
      break;
    case 'below':
      targetX = carVX - flyoutWidth / 2;
      targetY = carVY + margin;
      arrowClass = 'arrow-up';
      break;
    case 'above':
      targetX = carVX - flyoutWidth / 2;
      targetY = carVY - margin - flyoutHeight;
      arrowClass = 'arrow-down';
      break;
  }

  // Clamp to viewport
  targetX = Math.max(padding, Math.min(window.innerWidth - flyoutWidth - padding, targetX));
  targetY = Math.max(padding, Math.min(window.innerHeight - flyoutHeight - padding, targetY));

  // Smooth the position to avoid jitter from lerping car coordinates
  if (flyoutCurrentX === null) {
    flyoutCurrentX = targetX;
    flyoutCurrentY = targetY;
  } else {
    const smoothing = 0.25;
    flyoutCurrentX += (targetX - flyoutCurrentX) * smoothing;
    flyoutCurrentY += (targetY - flyoutCurrentY) * smoothing;
    // Snap to target when within 1px to prevent perpetual sub-pixel drift
    if (Math.abs(flyoutCurrentX - targetX) < 1) flyoutCurrentX = targetX;
    if (Math.abs(flyoutCurrentY - targetY) < 1) flyoutCurrentY = targetY;
  }

  detailFlyout.style.left = `${Math.round(flyoutCurrentX)}px`;
  detailFlyout.style.top = `${Math.round(flyoutCurrentY)}px`;

  // Update arrow class only if changed
  const currentArrow = detailFlyout.className.match(/arrow-\w+/)?.[0];
  if (currentArrow !== arrowClass) {
    detailFlyout.className = detailFlyout.className.replace(/arrow-\w+/g, '').trim() + ` ${arrowClass}`;
  }
}

function renderDetailFlyout(state) {
  const pct = (state.contextUtilization * 100).toFixed(1);
  const barColor = state.contextUtilization > 0.8 ? '#e94560' :
                   state.contextUtilization > 0.5 ? '#d97706' : '#22c55e';

  flyoutContent.innerHTML = `
    <div class="detail-row">
      <span class="label">Activity</span>
      <span class="value"><span class="detail-activity ${esc(state.activity)}">${esc(state.activity)}</span></span>
    </div>
    ${state.isChurning ? `<div class="detail-row">
      <span class="label">Process</span>
      <span class="value"><span class="detail-activity thinking">CPU Active</span></span>
    </div>` : ''}
    <div class="detail-progress">
      <div class="detail-progress-bar" style="width:${pct}%;background:${barColor}"></div>
      <span class="detail-progress-label">${formatTokens(state.tokensUsed)} / ${formatTokens(state.maxContextTokens)} (${pct}%)</span>
    </div>
    <div class="detail-row">
      <span class="label">Burn Rate</span>
      <span class="value burn-rate">${formatBurnRate(state.burnRatePerMinute)}</span>
    </div>
    <div class="detail-row">
      <span class="label">Model</span>
      <span class="value">${esc(state.model) || 'unknown'}</span>
    </div>
    <div class="detail-row">
      <span class="label">Source</span>
      <span class="value"><span class="source-badge${state.source ? ` source-${esc(state.source)}` : ''}">${esc(state.source) || 'unknown'}</span></span>
    </div>
    <div class="detail-row">
      <span class="label">Working Dir</span>
      <span class="value">${esc(state.workingDir) || '-'}</span>
    </div>
    <div class="detail-row">
      <span class="label">Branch</span>
      <span class="value">${esc(state.branch) || '-'}</span>
    </div>
    <div class="detail-row">
      <span class="label">Tmux</span>
      <span class="value">${state.tmuxTarget ? esc(state.tmuxTarget) : 'not in tmux'}</span>
    </div>
    <div class="detail-row">
      <span class="label">Session ID</span>
      <span class="value">${esc(state.id)}</span>
    </div>
    <div class="detail-row">
      <span class="label">PID</span>
      <span class="value">${state.pid || '-'}</span>
    </div>
    <div class="detail-row">
      <span class="label">Messages</span>
      <span class="value">${state.messageCount}</span>
    </div>
    <div class="detail-row">
      <span class="label">Tool Calls</span>
      <span class="value">${state.toolCallCount}</span>
    </div>
    <div class="detail-row">
      <span class="label">Current Tool</span>
      <span class="value">${esc(state.currentTool) || '-'}</span>
    </div>
    <div class="detail-row">
      <span class="label">Started</span>
      <span class="value">${formatTime(state.startedAt)}</span>
    </div>
    <div class="detail-row">
      <span class="label">Last Activity</span>
      <span class="value">${formatTime(state.lastActivityAt)}</span>
    </div>
    <div class="detail-row">
      <span class="label">Elapsed</span>
      <span class="value">${formatElapsed(state.startedAt)}</span>
    </div>
    ${state.completedAt ? `
    <div class="detail-row">
      <span class="label">Completed</span>
      <span class="value">${formatTime(state.completedAt)}</span>
    </div>` : ''}
    <div class="detail-row">
      <span class="label">Input Tokens</span>
      <span class="value">${formatTokens(state.tokensUsed)}</span>
    </div>
    <div class="detail-row">
      <span class="label">Max Tokens</span>
      <span class="value">${formatTokens(state.maxContextTokens)}</span>
    </div>
    <div class="detail-row">
      <span class="label">Context %</span>
      <span class="value">${pct}%</span>
    </div>
  `;
}

function handleSnapshot(payload) {
  const oldIds = new Set(knownSessionIds);
  sessions.clear();
  knownSessionIds.clear();

  for (const s of payload.sessions) {
    sessions.set(s.id, s);
    knownSessionIds.add(s.id);
    sessionActivities.set(s.id, s.activity);

    // Play appear sound for new sessions
    if (!oldIds.has(s.id)) {
      engine.playAppear();
    }
  }

  // Play disappear sound for removed sessions
  for (const id of oldIds) {
    if (!knownSessionIds.has(id)) {
      engine.playDisappear();
    }
  }

  updateSessionCount();
  log(`Snapshot: ${payload.sessions.length} sessions`, 'info');
  raceCanvas.setAllRacers(payload.sessions);

  // Update crowd excitement based on current race state
  engine.updateExcitement(payload.sessions);

  // Update detail flyout if open
  if (selectedSessionId && sessions.has(selectedSessionId)) {
    const state = sessions.get(selectedSessionId);
    renderDetailFlyout(state);
  }
}

function handleDelta(payload) {
  if (payload.updates) {
    for (const s of payload.updates) {
      const isNew = !knownSessionIds.has(s.id);
      if (isNew) {
        engine.playAppear();
        knownSessionIds.add(s.id);
      }

      // Detect activity transitions (debounced per session)
      const prevActivity = sessionActivities.get(s.id);
      if (prevActivity && prevActivity !== s.activity) {
        const now = Date.now();
        const lastSfx = lastTransitionSfx.get(s.id) || 0;
        const cooledDown = now - lastSfx >= TRANSITION_SFX_COOLDOWN;

        if (cooledDown) {
          // Play tool click when entering tool_use
          if (s.activity === 'tool_use') {
            engine.playToolClick();
          }
          // Play gear shift on any active transition
          if ((s.activity === 'thinking' || s.activity === 'tool_use') &&
              (prevActivity === 'thinking' || prevActivity === 'tool_use')) {
            engine.playGearShift();
          }
          lastTransitionSfx.set(s.id, now);
        }
      }
      sessionActivities.set(s.id, s.activity);

      sessions.set(s.id, s);
      raceCanvas.updateRacer(s);
    }
  }
  if (payload.removed) {
    for (const id of payload.removed) {
      sessions.delete(id);
      knownSessionIds.delete(id);
      sessionActivities.delete(id);
      lastTransitionSfx.delete(id);
      raceCanvas.removeRacer(id);
      engine.playDisappear();
      engine.stopEngine(id);
    }
  }
  updateSessionCount();

  // Update crowd excitement based on current race state
  engine.updateExcitement([...sessions.values()]);

  // Update detail flyout if open
  if (selectedSessionId && sessions.has(selectedSessionId)) {
    const state = sessions.get(selectedSessionId);
    renderDetailFlyout(state);
  }
}

function handleCompletion(payload) {
  log(`Session "${payload.name}" ${payload.activity}`, payload.activity === 'complete' ? 'info' : 'error');
  notifyCompletion(payload.name, payload.activity);

  if (payload.activity === 'complete') {
    raceCanvas.onComplete(payload.sessionId);
    engine.playVictory();
    engine.recordCompletion();
  } else if (payload.activity === 'errored') {
    raceCanvas.onError(payload.sessionId);
    engine.playCrash();
    engine.recordCrash();
  } else if (payload.activity === 'lost') {
    raceCanvas.onError(payload.sessionId);
    engine.playCrash();
    engine.recordCrash();
  }
}

function handleStatus(status) {
  statusDot.className = `status-dot ${status}`;
  raceCanvas.setConnected(status === 'connected');
  log(`Connection: ${status}`, status === 'connected' ? 'info' : 'error');
}

// Racer click -> detail flyout
raceCanvas.onRacerClick = (state) => {
  const racer = raceCanvas.racers.get(state.id);
  if (racer) {
    showDetailFlyout(state, racer.displayX, racer.displayY);
  }
};

// Keep flyout attached to car as it moves
raceCanvas.onAfterDraw = () => {
  if (selectedSessionId && !detailFlyout.classList.contains('hidden')) {
    const racer = raceCanvas.racers.get(selectedSessionId);
    if (racer) {
      positionFlyout(racer.displayX, racer.displayY);
    }
  }
};

// Detail flyout close
flyoutClose.addEventListener('click', () => {
  detailFlyout.classList.add('hidden');
  selectedSessionId = null;
  flyoutAnchor = null;
  flyoutCurrentX = null;
  flyoutCurrentY = null;
});

// Debug panel close
debugClose.addEventListener('click', () => {
  debugPanel.classList.add('hidden');
  debugVisible = false;
});

// Keyboard shortcuts
document.addEventListener('keydown', (e) => {
  if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') return;

  switch (e.key.toLowerCase()) {
    case 'd':
      debugVisible = !debugVisible;
      debugPanel.classList.toggle('hidden', !debugVisible);
      break;
    case 'm':
      muted = !muted;
      engine.setMuted(muted);
      log(`Sound ${muted ? 'muted' : 'unmuted'}`, 'info');
      break;
    case 'f':
      if (!document.fullscreenElement) {
        document.documentElement.requestFullscreen().catch(() => {});
      } else {
        document.exitFullscreen();
      }
      break;
    case 'escape':
      if (!detailFlyout.classList.contains('hidden')) {
        detailFlyout.classList.add('hidden');
        selectedSessionId = null;
      }
      break;
  }
});

// Connect
const conn = new RaceConnection({
  onSnapshot: handleSnapshot,
  onDelta: handleDelta,
  onCompletion: handleCompletion,
  onStatus: handleStatus,
});

conn.connect();
requestPermission();
loadSoundConfig();
log('Agent Racing Dashboard initialized', 'info');
log('Shortcuts: D=debug, M=mute, F=fullscreen, Click racer=details', 'info');
