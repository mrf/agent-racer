import { RaceConnection } from './websocket.js';
import { RaceCanvas } from './canvas/RaceCanvas.js';
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

const raceCanvas = new RaceCanvas(canvas);
window.raceCanvas = raceCanvas;

// Sound effects (generated via AudioContext)
let audioCtx = null;

function getAudioCtx() {
  if (!audioCtx) {
    audioCtx = new (window.AudioContext || window.webkitAudioContext)();
  }
  return audioCtx;
}

function playTone(freq, duration, type = 'sine') {
  if (muted) return;
  try {
    const ctx = getAudioCtx();
    const osc = ctx.createOscillator();
    const gain = ctx.createGain();
    osc.type = type;
    osc.frequency.value = freq;
    gain.gain.setValueAtTime(0.15, ctx.currentTime);
    gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + duration);
    osc.connect(gain);
    gain.connect(ctx.destination);
    osc.start();
    osc.stop(ctx.currentTime + duration);
  } catch {
    // Audio may not be available
  }
}

function playVictoryFanfare() {
  if (muted) return;
  playTone(523, 0.15, 'square');
  setTimeout(() => playTone(659, 0.15, 'square'), 150);
  setTimeout(() => playTone(784, 0.15, 'square'), 300);
  setTimeout(() => playTone(1047, 0.4, 'square'), 450);
}

function playErrorSound() {
  if (muted) return;
  playTone(300, 0.2, 'sawtooth');
  setTimeout(() => playTone(200, 0.3, 'sawtooth'), 200);
}

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
      <span class="value"><span class="detail-activity ${state.activity}">${state.activity}</span></span>
    </div>
    <div class="detail-progress">
      <div class="detail-progress-bar" style="width:${pct}%;background:${barColor}"></div>
      <span class="detail-progress-label">${formatTokens(state.tokensUsed)} / ${formatTokens(state.maxContextTokens)} (${pct}%)</span>
    </div>
    <div class="detail-row">
      <span class="label">Model</span>
      <span class="value">${state.model || 'unknown'}</span>
    </div>
    <div class="detail-row">
      <span class="label">Working Dir</span>
      <span class="value" title="${state.workingDir}">${state.workingDir || '-'}</span>
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
      <span class="value">${state.currentTool || '-'}</span>
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
      <span class="value" title="${state.id}">${state.id.substring(0, 12)}...</span>
    </div>
  `;
}

function handleSnapshot(payload) {
  sessions.clear();
  for (const s of payload.sessions) {
    sessions.set(s.id, s);
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
      sessions.set(s.id, s);
      raceCanvas.updateRacer(s);
    }
  }
  if (payload.removed) {
    for (const id of payload.removed) {
      sessions.delete(id);
      raceCanvas.removeRacer(id);
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
    playVictoryFanfare();
  } else if (payload.activity === 'errored') {
    raceCanvas.onError(payload.sessionId);
    playErrorSound();
  }
}

function handleStatus(status) {
  statusDot.className = `status-dot ${status}`;
  raceCanvas.setConnected(status === 'connected');
  log(`Connection: ${status}`, status === 'connected' ? 'info' : 'error');
}

// Racer click → detail panel
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
