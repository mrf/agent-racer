package track

import (
	"time"

	"github.com/agent-racer/tui/internal/client"
)

// Zone identifies a track zone.
type Zone int

const (
	ZoneRacing Zone = iota
	ZonePit
	ZoneParked
)

// DataFreshnessThreshold matches frontend/src/canvas/RaceCanvas.js DATA_FRESHNESS_MS.
const DataFreshnessThreshold = 30 * time.Second

// Classify returns the zone a session belongs in.
// Logic mirrors frontend/src/canvas/RaceCanvas.js:33-50.
func Classify(s *client.SessionState) Zone {
	switch s.Activity {
	case client.ActivityComplete, client.ActivityErrored, client.ActivityLost:
		return ZoneParked
	case client.ActivityIdle, client.ActivityWaiting, client.ActivityStarting:
		if !s.LastDataReceivedAt.IsZero() {
			if time.Since(s.LastDataReceivedAt) < DataFreshnessThreshold {
				return ZoneRacing
			}
		}
		return ZonePit
	default:
		return ZoneRacing
	}
}

// ZoneName returns a display label.
func ZoneName(z Zone) string {
	switch z {
	case ZoneRacing:
		return "TRACK"
	case ZonePit:
		return "PIT"
	case ZoneParked:
		return "PARKED"
	default:
		return "?"
	}
}
