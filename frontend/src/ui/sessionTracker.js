const TRANSITION_SFX_COOLDOWN = 3000;
const ACTIVE_ACTIVITIES = new Set(['thinking', 'tool_use']);

export function createSessionTracker(engine) {
  const knownSessionIds = new Set();
  const sessionActivities = new Map();
  const lastTransitionSfx = new Map();

  function onSnapshot(sessionList) {
    const oldIds = new Set(knownSessionIds);
    knownSessionIds.clear();

    for (const s of sessionList) {
      knownSessionIds.add(s.id);
      sessionActivities.set(s.id, s.activity);

      if (!oldIds.has(s.id)) {
        engine.playAppear();
      }
    }

    for (const id of oldIds) {
      if (!knownSessionIds.has(id)) {
        engine.playDisappear();
      }
    }
  }

  function playTransitionSfx(sessionId, prevActivity, nextActivity) {
    const now = Date.now();
    const lastSfx = lastTransitionSfx.get(sessionId) || 0;
    if (now - lastSfx < TRANSITION_SFX_COOLDOWN) return;

    if (nextActivity === 'tool_use') {
      engine.playToolClick();
    }
    if (ACTIVE_ACTIVITIES.has(nextActivity) && ACTIVE_ACTIVITIES.has(prevActivity)) {
      engine.playGearShift();
    }
    lastTransitionSfx.set(sessionId, now);
  }

  function onDelta(updates, removed) {
    if (updates) {
      for (const s of updates) {
        if (!knownSessionIds.has(s.id)) {
          engine.playAppear();
          knownSessionIds.add(s.id);
        }

        const prevActivity = sessionActivities.get(s.id);
        if (prevActivity && prevActivity !== s.activity) {
          playTransitionSfx(s.id, prevActivity, s.activity);
        }
        sessionActivities.set(s.id, s.activity);
      }
    }

    if (removed) {
      for (const id of removed) {
        knownSessionIds.delete(id);
        sessionActivities.delete(id);
        lastTransitionSfx.delete(id);
        engine.playDisappear();
        engine.stopEngine(id);
      }
    }
  }

  return { onSnapshot, onDelta };
}
