import { TERMINAL_ACTIVITIES, PIT_ACTIVITIES, DATA_FRESHNESS_MS } from './constants.js';

export function isParkingLotRacer(state) {
  return TERMINAL_ACTIVITIES.has(state.activity);
}

// Determines whether a racer belongs in the pit lane. Uses data freshness
// as the sole zone determinant — isChurning is reserved for visual effects
// only, since CPU jitter caused track/pit oscillation.
export function isPitRacer(state) {
  if (isParkingLotRacer(state)) return false;
  if (!PIT_ACTIVITIES.has(state.activity)) return false;

  if (state.lastDataReceivedAt) {
    const age = Date.now() - new Date(state.lastDataReceivedAt).getTime();
    if (age < DATA_FRESHNESS_MS) return false;
  }
  return true;
}

export function classifyZone(state) {
  if (isParkingLotRacer(state)) return 'parkingLot';
  if (isPitRacer(state)) return 'pit';
  return 'track';
}
