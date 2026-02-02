import { RaceConnection } from './websocket.js';
import { RaceCanvas } from './canvas/RaceCanvas.js';
import { SoundEngine } from './audio/SoundEngine.js';
import { requestPermission, notifyCompletion } from './notifications.js';

const debugPanel = document.getElementById('debug-panel');
const debugLog = document.getElementById('debug-log');
const debugClose = document.getElementById('debug-close');
const detailPanel = document.getElementById('detail-panel');
const detailContent = document.getElementById('detail-content');
const detailClose = document.getElementById('detail-close');
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

function showDetailPanel(state) {
  selectedSessionId = state.id;
  renderDetailPanel(state);
  detailPanel.classList.remove('hidden');
}

function renderDetailPanel(state) {
  const pct = (state.contextUtilization * 100).toFixed(1);
  const barColor = state.contextUtilization > 0.8 ? '#e94560' :
                   state.contextUtilization > 0.5 ? '#d97706' : '#22c55e';

  detailContent.innerHTML = `
    <div class="detail-row">
      <span class="label">Activity</span>
      <span class="value"><span class="detail-activity ${esc(state.activity)}">${esc(state.activity)}</span></span>
    </div>
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
      <span class="value">${esc(state.source) || 'unknown'}</span>
    </div>
    <div class="detail-row">
      <span class="label">Working Dir</span>
      <span class="value" title="${esc(state.workingDir)}">${esc(state.workingDir) || '-'}</span>
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
      <span class="label">Elapsed</span>
      <span class="value">${formatElapsed(state.startedAt)}</span>
    </div>
    <div class="detail-row">
      <span class="label">Last Activity</span>
      <span class="value">${formatTime(state.lastActivityAt)}</span>
    </div>
    ${state.completedAt ? `
    <div class="detail-row">
      <span class="label">Completed</span>
      <span class="value">${formatTime(state.completedAt)}</span>
    </div>` : ''}
    <div class="detail-row">
      <span class="label">PID</span>
      <span class="value">${state.pid || '-'}</span>
    </div>
    <div class="detail-row">
      <span class="label">Session ID</span>
      <span class="value" title="${esc(state.id)}">${esc(state.id.substring(0, 12))}...</span>
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

  // Update detail panel if open
  if (selectedSessionId && sessions.has(selectedSessionId)) {
    renderDetailPanel(sessions.get(selectedSessionId));
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

  // Update detail panel if open
  if (selectedSessionId && sessions.has(selectedSessionId)) {
    renderDetailPanel(sessions.get(selectedSessionId));
  }
}

function handleCompletion(payload) {
  log(`Session "${payload.name}" ${payload.activity}`, payload.activity === 'complete' ? 'info' : 'error');
  notifyCompletion(payload.name, payload.activity);

  if (payload.activity === 'complete') {
    raceCanvas.onComplete(payload.sessionId);
    engine.playVictory();
  } else if (payload.activity === 'errored') {
    raceCanvas.onError(payload.sessionId);
    engine.playCrash();
  }
}

function handleStatus(status) {
  statusDot.className = `status-dot ${status}`;
  raceCanvas.setConnected(status === 'connected');
  log(`Connection: ${status}`, status === 'connected' ? 'info' : 'error');
}

// Racer click -> detail panel
raceCanvas.onRacerClick = (state) => {
  showDetailPanel(state);
};

// Detail panel close
detailClose.addEventListener('click', () => {
  detailPanel.classList.add('hidden');
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
      if (!detailPanel.classList.contains('hidden')) {
        detailPanel.classList.add('hidden');
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
log('Claude Racing Dashboard initialized', 'info');
log('Shortcuts: D=debug, M=mute, F=fullscreen, Click racer=details', 'info');
