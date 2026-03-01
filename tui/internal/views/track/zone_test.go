package track

import (
	"testing"
	"time"

	"github.com/agent-racer/tui/internal/client"
)

func TestClassify(t *testing.T) {
	freshTime := time.Now()
	staleTime := time.Now().Add(-(DataFreshnessThreshold + time.Second))

	tests := []struct {
		name     string
		session  *client.SessionState
		expected Zone
	}{
		// Terminal states → parked
		{"complete", &client.SessionState{Activity: client.ActivityComplete}, ZoneParked},
		{"errored", &client.SessionState{Activity: client.ActivityErrored}, ZoneParked},
		{"lost", &client.SessionState{Activity: client.ActivityLost}, ZoneParked},

		// Active states → racing
		{"thinking", &client.SessionState{Activity: client.ActivityThinking}, ZoneRacing},
		{"tool_use", &client.SessionState{Activity: client.ActivityToolUse}, ZoneRacing},

		// Idle + fresh data → racing
		{
			"idle_fresh",
			&client.SessionState{Activity: client.ActivityIdle, LastDataReceivedAt: freshTime},
			ZoneRacing,
		},
		// Waiting + fresh data → racing
		{
			"waiting_fresh",
			&client.SessionState{Activity: client.ActivityWaiting, LastDataReceivedAt: freshTime},
			ZoneRacing,
		},
		// Starting + fresh data → racing
		{
			"starting_fresh",
			&client.SessionState{Activity: client.ActivityStarting, LastDataReceivedAt: freshTime},
			ZoneRacing,
		},

		// Idle + stale data → pit
		{
			"idle_stale",
			&client.SessionState{Activity: client.ActivityIdle, LastDataReceivedAt: staleTime},
			ZonePit,
		},
		// Waiting + stale data → pit
		{
			"waiting_stale",
			&client.SessionState{Activity: client.ActivityWaiting, LastDataReceivedAt: staleTime},
			ZonePit,
		},
		// Starting + stale data → pit
		{
			"starting_stale",
			&client.SessionState{Activity: client.ActivityStarting, LastDataReceivedAt: staleTime},
			ZonePit,
		},

		// Zero LastDataReceivedAt → pit (IsZero guard)
		{"idle_zero_time", &client.SessionState{Activity: client.ActivityIdle}, ZonePit},
		{"waiting_zero_time", &client.SessionState{Activity: client.ActivityWaiting}, ZonePit},
		{"starting_zero_time", &client.SessionState{Activity: client.ActivityStarting}, ZonePit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.session)
			if got != tt.expected {
				t.Errorf("Classify(%s) = %v, want %v", tt.session.Activity, got, tt.expected)
			}
		})
	}
}

func TestClassifyFreshnessThreshold(t *testing.T) {
	// Just under threshold → racing.
	justFresh := &client.SessionState{
		Activity:           client.ActivityIdle,
		LastDataReceivedAt: time.Now().Add(-(DataFreshnessThreshold - time.Millisecond)),
	}
	if got := Classify(justFresh); got != ZoneRacing {
		t.Errorf("just-fresh idle should be ZoneRacing, got %v", got)
	}

	// Just over threshold → pit.
	justStale := &client.SessionState{
		Activity:           client.ActivityIdle,
		LastDataReceivedAt: time.Now().Add(-(DataFreshnessThreshold + time.Millisecond)),
	}
	if got := Classify(justStale); got != ZonePit {
		t.Errorf("just-stale idle should be ZonePit, got %v", got)
	}
}

func TestZoneName(t *testing.T) {
	tests := []struct {
		name     string
		zone     Zone
		expected string
	}{
		{"racing", ZoneRacing, "TRACK"},
		{"pit", ZonePit, "PIT"},
		{"parked", ZoneParked, "PARKED"},
		{"unknown", Zone(99), "?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ZoneName(tt.zone)
			if got != tt.expected {
				t.Errorf("ZoneName(%d) = %q, want %q", tt.zone, got, tt.expected)
			}
		})
	}
}
