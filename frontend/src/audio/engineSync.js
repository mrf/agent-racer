/**
 * Syncs engine audio state for a single entity based on zone and activity.
 * Pure decision function — no side effects beyond engine calls.
 *
 * @param {object} engine - SoundEngine instance (must have startEngine/stopEngine)
 * @param {string} id - Entity ID
 * @param {object} state - Session state (activity, isChurning)
 * @param {string} zone - 'track' | 'pit' | 'parkingLot'
 */
export function syncEngineForEntity(engine, id, state, zone) {
  if (!engine) return;

  if (zone === 'track') {
    const activity = state.activity;
    if (activity === 'thinking' || activity === 'tool_use') {
      engine.startEngine(id, activity);
    } else if (state.isChurning && (activity === 'idle' || activity === 'starting')) {
      engine.startEngine(id, 'churning');
    } else {
      engine.stopEngine(id);
    }
  } else {
    // pit and parkingLot: engines always off
    engine.stopEngine(id);
  }
}
