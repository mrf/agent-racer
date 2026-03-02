import { RaceConnection } from './websocket.js';
import { createView, getViewTypes } from './canvas/ViewRenderer.js';
import { SoundEngine } from './audio/SoundEngine.js';
import { requestPermission, notifyCompletion } from './notifications.js';
import { AchievementPanel } from './gamification/AchievementPanel.js';
import { UnlockToast } from './gamification/UnlockToast.js';
import { RewardSelector } from './gamification/RewardSelector.js';
import { BattlePassBar } from './gamification/BattlePassBar.js';
import { setEquipped } from './gamification/CosmeticRegistry.js';
import { authFetch, getAuthToken } from './auth.js';
import { isTerminalActivity } from './session/constants.js';
import { createFlyout } from './ui/detailFlyout.js';
import { createSessionTracker } from './ui/sessionTracker.js';
import { initAmbientAudio } from './ui/ambientAudio.js';
import { ShortcutBar } from './ui/ShortcutBar.js';
import { HelpPopup } from './ui/HelpPopup.js';

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

// View management — createView registers built-in views via ViewRenderer import
let activeViewType = 'race';
let activeView = createView(activeViewType, canvas, engine);
window.raceCanvas = activeView;

function switchView() {
  const types = getViewTypes();
  const idx = types.indexOf(activeViewType);
  const nextType = types[(idx + 1) % types.length];

  // Capture state before destroying old view
  const currentSessions = [...sessions.values()];
  const wasConnected = activeView.connected;

  activeView.destroy();

  activeViewType = nextType;
  activeView = createView(activeViewType, canvas, engine);
  window.raceCanvas = activeView;

  // Restore state
  activeView.setConnected(wasConnected);
  if (currentSessions.length > 0) {
    activeView.setAllRacers(currentSessions);
  }

  wireViewCallbacks(activeView, flyout, unlockToast);
  log(`Switched to ${activeViewType} view`, 'info');
}

const achievementPanel = new AchievementPanel();
const unlockToast = new UnlockToast(engine);
const rewardSelector = new RewardSelector();
const battlePassBar = new BattlePassBar(document.getElementById('battlepass-bar'));

const flyout = createFlyout({ detailFlyout, flyoutContent, canvas });
const tracker = createSessionTracker(engine);
const shortcutBar = new ShortcutBar(document.getElementById('shortcut-bar'));
const helpPopup = new HelpPopup();

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
    s => !isTerminalActivity(s.activity)
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
  activeView.setAllRacers(payload.sessions);
  engine.updateExcitement(payload.sessions);
  flyout.updateContent(sessions);
}

function handleDelta(payload) {
  if (payload.updates) {
    for (const s of payload.updates) {
      sessions.set(s.id, s);
      activeView.updateRacer(s);
    }
  }
  if (payload.removed) {
    for (const id of payload.removed) {
      sessions.delete(id);
      activeView.removeRacer(id);
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
    activeView.onComplete(payload.sessionId);
    engine.playVictory();
    engine.recordCompletion();
  } else {
    activeView.onError(payload.sessionId);
    engine.playCrash();
    engine.recordCrash();
  }
}

function handleAchievementUnlocked(payload) {
  log(`Achievement unlocked: ${payload.name} (${payload.tier})`, 'info');
  unlockToast.show(payload);
  achievementPanel.markDirty();
}

function handleEquipped(payload) {
  if (payload.loadout) {
    setEquipped(payload.loadout);
    log('Cosmetic loadout updated via WebSocket', 'info');
  }
}

function handleBattlePassProgress(payload) {
  const xpGained = (payload.recentXP || []).reduce((sum, e) => sum + e.amount, 0);
  log(`Battle Pass: Tier ${payload.tier}, +${xpGained} XP`, 'info');
  battlePassBar.onProgress(payload);
}

function handleStatus(status) {
  statusDot.className = `status-dot ${status}`;
  activeView.setConnected(status === 'connected');
  log(`Connection: ${status}`, status === 'connected' ? 'info' : 'error');
}

export function wireViewCallbacks(view, flyout, unlockToast) {
  function getEntities() {
    return view.entities || view.racers;
  }

  view.onRacerClick = (state) => {
    const racer = getEntities().get(state.id);
    if (racer) {
      flyout.show(state, racer.displayX, racer.displayY);
    }
  };

  view.onHamsterClick = ({ hamsterState, parentState }) => {
    const racer = getEntities().get(parentState.id);
    if (!racer) return;
    const hamster = racer.hamsters && racer.hamsters.get(hamsterState.id);
    if (hamster) {
      flyout.showHamster(hamsterState, parentState, hamster.displayX, hamster.displayY);
    }
  };

  // Keep flyout attached to car/hamster as it moves, and draw toast overlays
  view.onAfterDraw = () => {
    unlockToast.update(view.dt);
    unlockToast.draw(view.ctx, view.width);

    if (!flyout.isVisible()) return;

    const entities = getEntities();
    const hamsterId = flyout.getSelectedHamsterId();
    const sessionId = flyout.getSelectedSessionId();

    if (hamsterId && sessionId) {
      const racer = entities.get(sessionId);
      if (racer && racer.hamsters) {
        const hamster = racer.hamsters.get(hamsterId);
        if (hamster) {
          flyout.updatePosition(hamster.displayX, hamster.displayY);
          return;
        }
      }
    }

    if (sessionId) {
      const racer = entities.get(sessionId);
      if (racer) {
        flyout.updatePosition(racer.displayX, racer.displayY);
      }
    }
  };
}

wireViewCallbacks(activeView, flyout, unlockToast);

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

function updateShortcutHighlights() {
  shortcutBar.setActive('achievements', achievementPanel.isVisible);
  shortcutBar.setActive('garage', rewardSelector.isVisible);
  shortcutBar.setActive('debug', debugVisible);
  shortcutBar.setActive('mute', muted);
  shortcutBar.setActive('fullscreen', !!document.fullscreenElement);
  shortcutBar.setActive('help', helpPopup.isVisible);
}

document.addEventListener('keydown', (e) => {
  if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') return;

  switch (e.key.toLowerCase()) {
    case '?':
      helpPopup.toggle();
      break;
    case 'a':
      achievementPanel.toggle();
      break;
    case 'g':
      rewardSelector.toggle();
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
    case 'v':
      switchView();
      break;
    case 'f':
      if (!e.shiftKey) break;
      if (!document.fullscreenElement) {
        document.documentElement.requestFullscreen().catch(() => {});
      } else {
        document.exitFullscreen();
      }
      break;
    case 'escape':
      if (helpPopup.isVisible) {
        helpPopup.hide();
      } else if (rewardSelector.isVisible) {
        rewardSelector.hide();
      } else if (achievementPanel.isVisible) {
        achievementPanel.hide();
      } else if (flyout.isVisible()) {
        flyout.hide();
      }
      break;
  }
  updateShortcutHighlights();
});

document.addEventListener('fullscreenchange', () => updateShortcutHighlights());

const conn = new RaceConnection({
  onSnapshot: handleSnapshot,
  onDelta: handleDelta,
  onCompletion: handleCompletion,
  onStatus: handleStatus,
  authToken: getAuthToken(),
  onSourceHealth: handleSourceHealth,
  onAchievementUnlocked: handleAchievementUnlocked,
  onEquipped: handleEquipped,
  onBattlePassProgress: handleBattlePassProgress,
});

conn.connect();
requestPermission();
loadSoundConfig();
log('Agent Racing Dashboard initialized', 'info');
log('Shortcuts: A=achievements, G=garage, D=debug, M=mute, V=view, Shift+F=fullscreen, Click racer=details', 'info');
