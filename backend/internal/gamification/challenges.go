package gamification

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"
	"strings"
	"time"
)

// Challenge describes a single weekly challenge goal.
type Challenge struct {
	ID          string
	Description string
	// Progress evaluates how far the player is toward completing this challenge.
	// It returns (current, target) where current/target >= target means complete.
	Progress func(snap *WeekSnapshot) (current, target int)
}

// WeekSnapshot captures the stats delta for the current challenge week.
// Challenges evaluate progress against these values, not all-time Stats.
type WeekSnapshot struct {
	SessionsPerModel  map[string]int
	SessionsPerSource map[string]int
	TotalSessions     int
	TotalCompletions  int
	TotalErrors       int
	TokensBurned      int
	Context90PctCount int
	DistinctModels    int
}

// ChallengeProgress is the JSON-serializable progress for a single active challenge.
type ChallengeProgress struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Current     int    `json:"current"`
	Target      int    `json:"target"`
	Complete    bool   `json:"complete"`
}

// WeeklyChallengeState is persisted in Stats to track the current week's challenges.
type WeeklyChallengeState struct {
	WeekStart  time.Time    `json:"weekStart"`
	ActiveIDs  []string     `json:"activeIds"`
	Snapshot   WeekSnapshot `json:"snapshot"`
	Completed  []string     `json:"completed"`
	XPAwarded  map[string]bool `json:"xpAwarded"`
}

const challengesPerWeek = 3

// challengePool returns the full set of available challenges.
func challengePool() []Challenge {
	return []Challenge{
		{
			ID:          "run_5_haiku",
			Description: "Run 5 Haiku sessions this week",
			Progress: func(snap *WeekSnapshot) (int, int) {
				return snapModelFamilyCount(snap.SessionsPerModel, "haiku"), 5
			},
		},
		{
			ID:          "complete_3_no_errors",
			Description: "Complete 3 sessions without errors",
			Progress: func(snap *WeekSnapshot) (int, int) {
				return snap.TotalCompletions, 3
			},
		},
		{
			ID:          "context_90_twice",
			Description: "Hit 90% context utilization twice",
			Progress: func(snap *WeekSnapshot) (int, int) {
				return snap.Context90PctCount, 2
			},
		},
		{
			ID:          "3_models_one_week",
			Description: "Use 3 different models this week",
			Progress: func(snap *WeekSnapshot) (int, int) {
				return snap.DistinctModels, 3
			},
		},
		{
			ID:          "burn_1m_tokens",
			Description: "Burn 1M total tokens this week",
			Progress: func(snap *WeekSnapshot) (int, int) {
				return snap.TokensBurned, 1_000_000
			},
		},
		{
			ID:          "run_10_sessions",
			Description: "Run 10 sessions this week",
			Progress: func(snap *WeekSnapshot) (int, int) {
				return snap.TotalSessions, 10
			},
		},
		{
			ID:          "complete_5_sessions",
			Description: "Complete 5 sessions this week",
			Progress: func(snap *WeekSnapshot) (int, int) {
				return snap.TotalCompletions, 5
			},
		},
		{
			ID:          "use_2_sources",
			Description: "Use 2 different agent sources this week",
			Progress: func(snap *WeekSnapshot) (int, int) {
				count := 0
				for _, n := range snap.SessionsPerSource {
					if n > 0 {
						count++
					}
				}
				return count, 2
			},
		},
		{
			ID:          "run_3_opus",
			Description: "Run 3 Opus sessions this week",
			Progress: func(snap *WeekSnapshot) (int, int) {
				return snapModelFamilyCount(snap.SessionsPerModel, "opus"), 3
			},
		},
		{
			ID:          "burn_500k_tokens",
			Description: "Burn 500K tokens this week",
			Progress: func(snap *WeekSnapshot) (int, int) {
				return snap.TokensBurned, 500_000
			},
		},
	}
}

// challengeByID returns the Challenge from the pool with the given ID, or ok=false.
func challengeByID(id string) (Challenge, bool) {
	for _, c := range challengePool() {
		if c.ID == id {
			return c, true
		}
	}
	return Challenge{}, false
}

// weekStart returns the Monday 00:00 UTC of the ISO week containing t.
func weekStart(t time.Time) time.Time {
	t = t.UTC()
	y, w := t.ISOWeek()
	// Jan 4 is always in week 1 of its year.
	jan4 := time.Date(y, 1, 4, 0, 0, 0, 0, time.UTC)
	_, jan4Week := jan4.ISOWeek()
	// Monday of week 1
	monday := jan4.AddDate(0, 0, -int(jan4.Weekday()-time.Monday))
	if jan4.Weekday() == time.Sunday {
		monday = jan4.AddDate(0, 0, -6)
	}
	return monday.AddDate(0, 0, (w-jan4Week)*7)
}

// selectChallenges deterministically picks challengesPerWeek challenges
// for the given week start time using a hash-based shuffle.
func selectChallenges(ws time.Time) []string {
	pool := challengePool()
	n := len(pool)

	// Seed a deterministic ordering from the week timestamp.
	h := sha256.Sum256([]byte(ws.Format(time.RFC3339)))
	seed := binary.BigEndian.Uint64(h[:8])

	// Build index array and shuffle using Fisher-Yates with the seed.
	indices := make([]int, n)
	for i := range indices {
		indices[i] = i
	}
	for i := n - 1; i > 0; i-- {
		seed = seed*6364136223846793005 + 1442695040888963407 // LCG step
		j := int(seed % uint64(i+1))
		indices[i], indices[j] = indices[j], indices[i]
	}

	count := challengesPerWeek
	if count > n {
		count = n
	}
	ids := make([]string, count)
	for i := 0; i < count; i++ {
		ids[i] = pool[indices[i]].ID
	}
	sort.Strings(ids)
	return ids
}

// EvaluateChallenges computes progress for the active weekly challenges.
func EvaluateChallenges(state *WeeklyChallengeState) []ChallengeProgress {
	out := make([]ChallengeProgress, 0, len(state.ActiveIDs))
	for _, id := range state.ActiveIDs {
		c, ok := challengeByID(id)
		if !ok {
			continue
		}
		cur, tgt := c.Progress(&state.Snapshot)
		out = append(out, ChallengeProgress{
			ID:          c.ID,
			Description: c.Description,
			Current:     cur,
			Target:      tgt,
			Complete:    cur >= tgt,
		})
	}
	return out
}

// RotateChallengesIfNeeded checks whether the current week has changed and
// rotates the active challenge set. Returns true if rotation occurred.
func RotateChallengesIfNeeded(state *WeeklyChallengeState, now time.Time) bool {
	ws := weekStart(now)
	if !state.WeekStart.IsZero() && ws.Equal(state.WeekStart) {
		return false
	}
	state.WeekStart = ws
	state.ActiveIDs = selectChallenges(ws)
	state.Snapshot = WeekSnapshot{
		SessionsPerModel:  make(map[string]int),
		SessionsPerSource: make(map[string]int),
	}
	state.Completed = nil
	state.XPAwarded = make(map[string]bool)
	return true
}

// initWeeklyChallengeState ensures the state has initialized maps.
func initWeeklyChallengeState(s *WeeklyChallengeState) {
	if s.Snapshot.SessionsPerModel == nil {
		s.Snapshot.SessionsPerModel = make(map[string]int)
	}
	if s.Snapshot.SessionsPerSource == nil {
		s.Snapshot.SessionsPerSource = make(map[string]int)
	}
	if s.XPAwarded == nil {
		s.XPAwarded = make(map[string]bool)
	}
}

// snapModelFamilyCount returns the total session count across all models whose
// ID contains the given family substring (case-insensitive, e.g. "haiku").
func snapModelFamilyCount(sessions map[string]int, family string) int {
	total := 0
	for model, n := range sessions {
		if strings.Contains(strings.ToLower(model), family) {
			total += n
		}
	}
	return total
}
