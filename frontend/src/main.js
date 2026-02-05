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

// Track known session IDs and their activities for detecting appear/disappear/transitions
let knownSessionIds = new Set();
let sessionActivities = new Map();

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
  renderDetailFlyout(state);
  positionFlyout(carX, carY);
  detailFlyout.classList.remove('hidden');
}

function positionFlyout(carX, carY) {
  const canvasRect = canvas.getBoundingClientRect();
  const flyoutWidth = 380; // match CSS width
  const flyoutMaxHeight = window.innerHeight * 0.8; // match CSS max-height
  const margin = 50; // distance from car (scaled up for larger cars)
  const padding = 10; // padding from screen edges

  // Calculate absolute position relative to viewport
  const absoluteCarX = canvasRect.left + carX;
  const absoluteCarY = canvasRect.top + carY;

  let left, top;
  let arrowClass = 'arrow-left';

  // Try positioning to the right of the car first
  if (absoluteCarX + margin + flyoutWidth + padding < window.innerWidth) {
    left = absoluteCarX + margin;
    arrowClass = 'arrow-left';
  }
  // If not enough space on right, try left
  else if (absoluteCarX - margin - flyoutWidth > padding) {
    left = absoluteCarX - margin - flyoutWidth;
    arrowClass = 'arrow-right';
  }
  // If neither works, center it horizontally
  else {
    left = Math.max(padding, Math.min(
      window.innerWidth - flyoutWidth - padding,
      absoluteCarX - flyoutWidth / 2
    ));
    // Position above or below based on vertical space
    if (absoluteCarY > window.innerHeight / 2) {
      top = absoluteCarY - margin - flyoutMaxHeight;
      arrowClass = 'arrow-down';
    } else {
      top = absoluteCarY + margin;
      arrowClass = 'arrow-up';
    }
  }

  // Calculate vertical position if not already set (for left/right positioning)
  if (top === undefined) {
    // Check if there's enough space below the car
    const spaceBelow = window.innerHeight - padding - absoluteCarY;
    if (spaceBelow >= margin + flyoutMaxHeight) {
      // Position below the car
      top = absoluteCarY + margin;
    } else {
      // Position above the car
      top = absoluteCarY - margin - flyoutMaxHeight;
    }
    // Clamp to viewport bounds
    top = Math.max(padding, Math.min(
      window.innerHeight - flyoutMaxHeight - padding,
      top
    ));
  }

  // Apply position
  detailFlyout.style.left = `${left}px`;
  detailFlyout.style.top = `${top}px`;

  // Update arrow class
  detailFlyout.className = detailFlyout.className.replace(/arrow-\w+/g, '').trim() + ` ${arrowClass}`;
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
      <span class="value"><span class="detail-activity thinking">churning</span></span>
    </div>` : ''}
    <div class="detail-progress">
      <div class="detail-progress-bar" style="width:${pct}%;background:${barColor}"></div>
      <span class="detail-progress-label">${formatTokens(state.tokensUsed)} / ${formatTokens(state.maxContextTokens)} (${pct}%)</span>
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

      // Detect activity transitions
      const prevActivity = sessionActivities.get(s.id);
      if (prevActivity && prevActivity !== s.activity) {
        // Play tool click when entering tool_use
        if (s.activity === 'tool_use') {
          engine.playToolClick();
        }
        // Play gear shift on any active transition
        if ((s.activity === 'thinking' || s.activity === 'tool_use') &&
            (prevActivity === 'thinking' || prevActivity === 'tool_use')) {
          engine.playGearShift();
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

// Detail flyout close
flyoutClose.addEventListener('click', () => {
  detailFlyout.classList.add('hidden');
  selectedSessionId = null;
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
log('Claude Racing Dashboard initialized', 'info');
log('Shortcuts: D=debug, M=mute, F=fullscreen, Click racer=details', 'info');
