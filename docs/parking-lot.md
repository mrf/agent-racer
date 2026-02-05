# Parking Lot Feature Implementation Plan

## Summary

Add a "parking lot" area below the pit for sessions whose tmux window has closed. This creates a clear visual hierarchy:
- **Track**: Active sessions (thinking, tool_use, churning)
- **Pit**: Idle sessions with open windows (waiting for input)
- **Parking Lot**: Closed sessions we've seen this daemon run (terminal: complete/errored/lost)

## Key Insight

Terminal sessions (`complete`, `errored`, `lost`) = window closed. When a tmux window closes:
1. Claude process ends → SessionEnd hook fires → session marked `complete` or `errored`
2. Or process disappears without hook → session marked `lost`

Currently these terminal sessions stay on track briefly (8s) then get removed. With parking lot, they move to a dedicated area instead.

## Files to Modify

### Backend (minimal changes)

**`config.yaml`**
- Increase `completion_remove_after` significantly (e.g., `24h` or `0` for indefinite)
- This keeps terminal sessions visible in parking lot instead of removing them

### Frontend

**`frontend/src/canvas/RaceCanvas.js`**
- Add `isParkingLotRacer()` function: returns true for terminal activities
- Partition racers into track/pit/parkingLot groups
- Track `_parkingLotLaneCount` for dynamic canvas sizing
- Handle transitions: track/pit → parkingLot (one-way, sessions don't leave parking lot)

**`frontend/src/canvas/Track.js`**
- Add parking lot constants: `PARKING_LOT_LANE_HEIGHT`, `PARKING_LOT_GAP`, etc.
- Add `getRequiredParkingLotHeight(parkingLotLaneCount)`
- Add `getParkingLotBounds(canvasWidth, canvasHeight, activeLaneCount, pitLaneCount, parkingLotLaneCount)`
- Reuse existing `getLaneY()`, `getPositionX()` (bounds carry `laneHeight`)
- Add `drawParkingLot()` method with distinct visual style

**`frontend/src/entities/Racer.js`**
- Add `inParkingLot` state tracking (similar to `inPit`)
- Add `parkingLotDim` for additional dimming (grayer than pit)
- Apply parking lot visual treatment: more faded, grayscale-ish

## Implementation Details

### 1. Classification Logic (RaceCanvas.js)

```javascript
function isParkingLotRacer(state) {
  return state.activity === 'complete' ||
         state.activity === 'errored' ||
         state.activity === 'lost';
}
```

Update `isPitRacer()` to exclude terminal sessions:
```javascript
function isPitRacer(state) {
  const { activity, isChurning } = state;
  // Terminal sessions go to parking lot, not pit
  if (activity === 'complete' || activity === 'errored' || activity === 'lost') {
    return false;
  }
  if (activity === 'idle' || activity === 'waiting' || activity === 'starting') {
    return !isChurning;
  }
  return false;
}
```

### 2. Update Loop (RaceCanvas.js)

In `update()`, partition into three groups:
```javascript
const trackRacers = [];
const pitRacers = [];
const parkingLotRacers = [];

for (const racer of this.racers.values()) {
  if (isParkingLotRacer(racer.state)) {
    parkingLotRacers.push(racer);
  } else if (isPitRacer(racer.state)) {
    pitRacers.push(racer);
  } else {
    trackRacers.push(racer);
  }
}
```

### 3. Track.js Additions

Layout constants:
```javascript
const PARKING_LOT_LANE_HEIGHT = 45;  // Slightly smaller than pit
const PARKING_LOT_GAP = 20;          // Gap between pit and parking lot
const PARKING_LOT_BOTTOM_PADDING = 40;
```

New methods:
- `getRequiredParkingLotHeight(count)`
- `getParkingLotBounds(...)` - positioned below pit
- Reuses `getLaneY()`, `getPositionX()` (generic, bounds-driven)
- `drawParkingLot()` - darker background, "PARKED" label, no chevrons

Visual style:
- Darker surface than pit (`#1a1a28`)
- Dashed border like pit but dimmer
- Label: "PARKED" on the left
- No connecting lane (one-way entry)

### 4. Racer.js Updates

Add parking lot state:
```javascript
this.inParkingLot = false;
this.parkingLotDim = 0;
this.parkingLotDimTarget = 0;
```

In `animate()`, interpolate `parkingLotDim` toward target (similar to `pitDim`).

In `draw()`, apply additional opacity/grayscale for parking lot:
```javascript
if (this.parkingLotDim > 0) {
  ctx.globalAlpha *= (1 - this.parkingLotDim * 0.5);  // 50% additional fade
  // Apply grayscale filter via compositing or CSS filter
}
```

### 5. Config Change

In `config.yaml`:
```yaml
completion_remove_after: 0  # Or 24h - keep parked sessions visible
```

Setting to `0` disables auto-removal entirely, keeping all terminal sessions until daemon restart.

## Visual Hierarchy

```
┌─────────────────────────────────────────────┐
│  TRACK  (active racers)                     │
│  [🏎️ thinking] [🏎️ tool_use] [🏎️ churning] │
├─────────────────────────────────────────────┤
│  PIT  (idle with open window)               │
│  [🏎️ idle] [🏎️ waiting]                     │  ← dimmed 40%
├─────────────────────────────────────────────┤
│  PARKED  (window closed)                    │
│  [🏎️ complete✓] [🏎️ errored✗]              │  ← dimmed 60%, grayscale
└─────────────────────────────────────────────┘
```

## Transitions

- **Track → Parking Lot**: When session becomes terminal (complete/errored/lost)
- **Pit → Parking Lot**: When idle session's window closes (becomes terminal)
- **Parking Lot → anywhere**: Never (terminal is final)

Waypoint animation: Drive through pit entry column → down to parking lot

## Verification

1. Start daemon with new config
2. Start a Claude session → appears on track
3. Let it go idle → moves to pit
4. Close the tmux window → moves to parking lot (marked complete)
5. Verify session stays in parking lot until daemon restart
6. Force-kill a session → moves to parking lot (marked errored/lost)
7. Start new daemon → parking lot is empty (sessions only persist in daemon memory)

## Notes

- No backend data model changes needed - we use existing terminal activity states
- Parking lot only shows sessions from current daemon run (not persisted to disk)
- Completed sessions show checkered flag, errored shows X, lost shows ghost effect
- Parking lot cars remain in their final animation state (gold for complete, grayscale for error)
