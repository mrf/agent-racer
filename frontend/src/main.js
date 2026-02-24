import { RaceConnection } from './websocket.js';
import { RaceCanvas } from './canvas/RaceCanvas.js';
import { SoundEngine } from './audio/SoundEngine.js';
import { requestPermission, notifyCompletion } from './notifications.js';
import { AchievementPanel } from './gamification/AchievementPanel.js';
import { UnlockToast } from './gamification/UnlockToast.js';
import { authFetch, getAuthToken } from './auth.js';
import { createFlyout } from './ui/detailFlyout.js';
import { createSessionTracker } from './ui/sessionTracker.js';
import { initAmbientAudio } from './ui/ambientAudio.js';

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

const engine = new SoundEngine();
const raceCanvas = new RaceCanvas(canvas);
raceCanvas.setEngine(engine);
window.raceCanvas = raceCanvas;

const achievementPanel = new AchievementPanel();
const unlockToast = new UnlockToast(engine);

const flyout = createFlyout({ detailFlyout, flyoutContent, canvas });
const tracker = createSessionTracker(engine);

initAmbientAudio(engine);

async function loadSoundConfig() {
  try {
    const response = await authFetch('/api/config');
    if (response.ok) {
      const config = await response.json();
      engine.applyConfig(config);
      log('Sound configuration loaded from server', 'info');
    }
  } catch (err) {
    log(`Failed to load sound config: ${err.message}`, 'error');
  }
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

function handleSourceHealth(payload) {
  const status = payload.status || 'unknown';
  const src = payload.source || 'unknown';
  const errMsg = payload.lastError ? ` — ${payload.lastError}` : '';
  const level = status === 'healthy' ? 'info' : 'error';
  log(`Source [${src}] health: ${status} (discover=${payload.discoverFailures}, parse=${payload.parseFailures})${errMsg}`, level);
}

function handleSnapshot(payload) {
  sessions.clear();
  for (const s of payload.sessions) {
    sessions.set(s.id, s);
  }

  tracker.onSnapshot(payload.sessions);

  if (payload.sourceHealth) {
    for (const sh of payload.sourceHealth) {
      handleSourceHealth(sh);
    }
  }

  updateSessionCount();
  log(`Snapshot: ${payload.sessions.length} sessions`, 'info');
  raceCanvas.setAllRacers(payload.sessions);
  engine.updateExcitement(payload.sessions);
  flyout.updateContent(sessions);
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

  tracker.onDelta(payload.updates, payload.removed);

  updateSessionCount();
  engine.updateExcitement([...sessions.values()]);
  flyout.updateContent(sessions);
}

function handleCompletion(payload) {
  const isSuccess = payload.activity === 'complete';
  log(`Session "${payload.name}" ${payload.activity}`, isSuccess ? 'info' : 'error');
  notifyCompletion(payload.name, payload.activity);

  if (isSuccess) {
    raceCanvas.onComplete(payload.sessionId);
    engine.playVictory();
    engine.recordCompletion();
  } else {
    raceCanvas.onError(payload.sessionId);
    engine.playCrash();
    engine.recordCrash();
  }
}

function handleAchievementUnlocked(payload) {
  log(`Achievement unlocked: ${payload.name} (${payload.tier})`, 'info');
  unlockToast.show(payload);
}

function handleStatus(status) {
  statusDot.className = `status-dot ${status}`;
  raceCanvas.setConnected(status === 'connected');
  log(`Connection: ${status}`, status === 'connected' ? 'info' : 'error');
}

raceCanvas.onRacerClick = (state) => {
  const racer = raceCanvas.racers.get(state.id);
  if (racer) {
    flyout.show(state, racer.displayX, racer.displayY);
  }
};

raceCanvas.onHamsterClick = ({ hamsterState, parentState }) => {
  const racer = raceCanvas.racers.get(parentState.id);
  if (racer) {
    const hamster = racer.hamsters && racer.hamsters.get(hamsterState.id);
    if (hamster) {
      flyout.showHamster(hamsterState, parentState, hamster.displayX, hamster.displayY);
    }
  }
};

// Keep flyout attached to car/hamster as it moves, and draw toast overlays
raceCanvas.onAfterDraw = () => {
  // Update and draw unlock toasts on top of everything
  unlockToast.update(raceCanvas.dt);
  unlockToast.draw(raceCanvas.ctx, raceCanvas.width);

  if (!flyout.isVisible()) return;

  const hamsterId = flyout.getSelectedHamsterId();
  const sessionId = flyout.getSelectedSessionId();

  if (hamsterId && sessionId) {
    const racer = raceCanvas.racers.get(sessionId);
    if (racer && racer.hamsters) {
      const hamster = racer.hamsters.get(hamsterId);
      if (hamster) {
        flyout.updatePosition(hamster.displayX, hamster.displayY);
        return;
      }
    }
  }

  if (sessionId) {
    const racer = raceCanvas.racers.get(sessionId);
    if (racer) {
      flyout.updatePosition(racer.displayX, racer.displayY);
    }
  }
};

flyoutClose.addEventListener('click', () => flyout.hide());

detailFlyout.addEventListener('click', (e) => {
  const btn = e.target.closest('.copy-btn');
  if (!btn) return;
  const text = btn.dataset.copy;
  navigator.clipboard.writeText(text).then(() => {
    btn.textContent = '\u2713';
    setTimeout(() => { btn.innerHTML = '&#x2398;'; }, 1500);
  });
});

debugClose.addEventListener('click', () => {
  debugPanel.classList.add('hidden');
  debugVisible = false;
});

document.addEventListener('keydown', (e) => {
  if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') return;

  switch (e.key.toLowerCase()) {
    case 'a':
      achievementPanel.toggle();
      break;
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
      if (achievementPanel.isVisible) {
        achievementPanel.hide();
      } else if (flyout.isVisible()) {
        flyout.hide();
      }
      break;
  }
});

const conn = new RaceConnection({
  onSnapshot: handleSnapshot,
  onDelta: handleDelta,
  onCompletion: handleCompletion,
  onStatus: handleStatus,
  authToken: getAuthToken(),
  onSourceHealth: handleSourceHealth,
  onAchievementUnlocked: handleAchievementUnlocked,
});

conn.connect();
requestPermission();
loadSoundConfig();
log('Agent Racing Dashboard initialized', 'info');
log('Shortcuts: A=achievements, D=debug, M=mute, F=fullscreen, Click racer=details', 'info');
