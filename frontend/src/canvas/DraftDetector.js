// Draft threshold: racer must be within this context utilization gap to be drafting.
export const DRAFT_GAP = 0.05;

// Battle window: if the same pair swaps positions within this many seconds, show "battle".
const BATTLE_WINDOW_S = 10;

export class DraftDetector {
  constructor() {
    // Previous sorted order: array of racer IDs, highest util first
    this._prevOrder = [];
    // Battle tracker: key = `${idA}:${idB}` (sorted), value = { lastOvertakeTime }
    this._battles = new Map();
  }

  /**
   * Detect draft pairs and overtakes among the given track racers.
   * Racers must be on the same track group (same maxContextTokens).
   *
   * @param {Racer[]} trackRacers - racers currently on the track (not pit/parked)
   * @param {number} nowS - current time in seconds (for battle window)
   * @returns {{ draftPairs: Array, overtakes: Array, battles: Set }}
   */
  detect(trackRacers, nowS) {
    if (trackRacers.length < 2) {
      this._prevOrder = trackRacers.map(r => r.id);
      return { draftPairs: [], overtakes: [], battles: new Set() };
    }

    // Sort by contextUtilization descending — position 0 is leader
    const sorted = [...trackRacers].sort((a, b) =>
      (b.state.contextUtilization || 0) - (a.state.contextUtilization || 0)
    );

    // --- Draft pairs: racer within DRAFT_GAP of the car directly ahead ---
    const draftPairs = [];
    for (let i = 1; i < sorted.length; i++) {
      const leader = sorted[i - 1];
      const drafter = sorted[i];
      const leaderUtil = leader.state.contextUtilization || 0;
      const drafterUtil = drafter.state.contextUtilization || 0;
      const gap = leaderUtil - drafterUtil;
      if (gap >= 0 && gap <= DRAFT_GAP) {
        draftPairs.push({ drafter, leader, gap });
      }
    }

    // --- Overtake detection: compare current order to previous ---
    const currentIds = sorted.map(r => r.id);
    const overtakes = this._detectOvertakes(this._prevOrder, currentIds, sorted, nowS);
    this._prevOrder = currentIds;

    // --- Active battles: pairs that have swapped within BATTLE_WINDOW_S ---
    const battles = new Set();
    for (const [key, rec] of this._battles) {
      if (nowS - rec.lastOvertakeTime <= BATTLE_WINDOW_S) {
        battles.add(key);
      } else {
        this._battles.delete(key);
      }
    }

    return { draftPairs, overtakes, battles };
  }

  _detectOvertakes(prev, curr, sorted, nowS) {
    if (prev.length === 0) return [];

    // Build rank maps: id -> rank (0 = first/leader)
    const prevRank = new Map();
    for (let i = 0; i < prev.length; i++) prevRank.set(prev[i], i);
    const currRank = new Map();
    for (let i = 0; i < curr.length; i++) currRank.set(curr[i], i);

    const overtakes = [];
    for (const racer of sorted) {
      const pr = prevRank.get(racer.id);
      const cr = currRank.get(racer.id);
      if (pr === undefined || cr === undefined) continue;
      if (cr >= pr) continue; // same rank or dropped — not an overtake

      // This racer moved up. Find who they passed (the racer at their new rank in prev order).
      const passedId = prev[cr];
      if (!passedId || passedId === racer.id) continue;

      // Look up the passed racer object
      const passedRacer = sorted.find(r => r.id === passedId);
      if (!passedRacer) continue;

      overtakes.push({ overtaker: racer, overtaken: passedRacer });

      // Record for battle detection
      const key = [racer.id, passedId].sort().join(':');
      this._battles.set(key, { lastOvertakeTime: nowS });
    }

    return overtakes;
  }
}
