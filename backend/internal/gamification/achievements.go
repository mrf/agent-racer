package gamification

import (
	"strings"
	"time"
)

// Tier represents an achievement's difficulty level.
type Tier string

const (
	TierBronze   Tier = "bronze"
	TierSilver   Tier = "silver"
	TierGold     Tier = "gold"
	TierPlatinum Tier = "platinum"
)

// Category groups related achievements in the UI.
type Category string

const (
	CategorySessionMilestones    Category = "Session Milestones"
	CategorySourceDiversity      Category = "Source Diversity"
	CategoryModelCollection      Category = "Model Collection"
	CategoryPerformanceEndurance Category = "Performance & Endurance"
	CategorySpectacle            Category = "Spectacle"
	CategoryStreaks              Category = "Streaks"
)

// Achievement describes a single unlockable goal.
type Achievement struct {
	ID          string
	Name        string
	Description string
	Tier        Tier
	Category    Category
	// Condition reports whether the achievement should be awarded given a Stats snapshot.
	Condition func(*Stats) bool
}

// AchievementEngine holds the complete achievement registry and evaluates
// which achievements become newly unlocked against a Stats snapshot.
type AchievementEngine struct {
	registry []Achievement
}

// NewAchievementEngine creates an engine pre-loaded with the full achievement set.
func NewAchievementEngine() *AchievementEngine {
	return &AchievementEngine{registry: buildRegistry()}
}

// Registry returns a shallow copy of all registered achievements.
func (e *AchievementEngine) Registry() []Achievement {
	out := make([]Achievement, len(e.registry))
	copy(out, e.registry)
	return out
}

// Evaluate checks every not-yet-unlocked achievement against stats.
// Newly passing achievements are recorded in stats.AchievementsUnlocked
// with the current UTC timestamp and returned. The caller is responsible
// for persisting stats after this call.
func (e *AchievementEngine) Evaluate(stats *Stats) []Achievement {
	now := time.Now().UTC()
	var unlocked []Achievement
	for _, a := range e.registry {
		if _, already := stats.AchievementsUnlocked[a.ID]; already {
			continue
		}
		if a.Condition(stats) {
			stats.AchievementsUnlocked[a.ID] = now
			unlocked = append(unlocked, a)
		}
	}
	return unlocked
}

// modelFamilySessions returns the total session count across all models whose
// ID contains the given family substring (case-insensitive, e.g. "opus").
func modelFamilySessions(stats *Stats, family string) int {
	total := 0
	for model, n := range stats.SessionsPerModel {
		if strings.Contains(strings.ToLower(model), family) {
			total += n
		}
	}
	return total
}

// hasModelFamily reports whether at least one session used a model from the
// given family.
func hasModelFamily(stats *Stats, family string) bool {
	return modelFamilySessions(stats, family) > 0
}

func buildRegistry() []Achievement {
	return []Achievement{

		// ── Session Milestones ─────────────────────────────────────────────

		{
			ID: "first_lap", Name: "First Lap",
			Description: "Observe your first agent session",
			Tier:        TierBronze, Category: CategorySessionMilestones,
			Condition: func(s *Stats) bool { return s.TotalSessions >= 1 },
		},
		{
			ID: "pit_crew", Name: "Pit Crew",
			Description: "Run 10 sessions",
			Tier:        TierBronze, Category: CategorySessionMilestones,
			Condition: func(s *Stats) bool { return s.TotalSessions >= 10 },
		},
		{
			ID: "veteran_driver", Name: "Veteran Driver",
			Description: "Run 50 sessions",
			Tier:        TierSilver, Category: CategorySessionMilestones,
			Condition: func(s *Stats) bool { return s.TotalSessions >= 50 },
		},
		{
			ID: "century_club", Name: "Century Club",
			Description: "Run 100 sessions",
			Tier:        TierGold, Category: CategorySessionMilestones,
			Condition: func(s *Stats) bool { return s.TotalSessions >= 100 },
		},
		{
			ID: "track_legend", Name: "Track Legend",
			Description: "Run 500 sessions",
			Tier:        TierPlatinum, Category: CategorySessionMilestones,
			Condition: func(s *Stats) bool { return s.TotalSessions >= 500 },
		},

		// ── Source Diversity ───────────────────────────────────────────────

		{
			ID: "home_turf", Name: "Home Turf",
			Description: "Run 5 Claude sessions",
			Tier:        TierBronze, Category: CategorySourceDiversity,
			Condition: func(s *Stats) bool { return s.SessionsPerSource["claude"] >= 5 },
		},
		{
			ID: "gemini_rising", Name: "Gemini Rising",
			Description: "Complete your first Gemini CLI session",
			Tier:        TierBronze, Category: CategorySourceDiversity,
			Condition: func(s *Stats) bool { return s.SessionsPerSource["gemini"] >= 1 },
		},
		{
			ID: "codex_curious", Name: "Codex Curious",
			Description: "Complete your first Codex session",
			Tier:        TierBronze, Category: CategorySourceDiversity,
			Condition: func(s *Stats) bool { return s.SessionsPerSource["codex"] >= 1 },
		},
		{
			ID: "triple_threat", Name: "Triple Threat",
			Description: "Use all 3 agent sources (Claude, Gemini, Codex)",
			Tier:        TierSilver, Category: CategorySourceDiversity,
			Condition: func(s *Stats) bool {
				return s.SessionsPerSource["claude"] >= 1 &&
					s.SessionsPerSource["gemini"] >= 1 &&
					s.SessionsPerSource["codex"] >= 1
			},
		},
		{
			ID: "polyglot", Name: "Polyglot",
			Description: "Run 10+ sessions from each agent source",
			Tier:        TierGold, Category: CategorySourceDiversity,
			Condition: func(s *Stats) bool {
				return s.SessionsPerSource["claude"] >= 10 &&
					s.SessionsPerSource["gemini"] >= 10 &&
					s.SessionsPerSource["codex"] >= 10
			},
		},

		// ── Model Collection ───────────────────────────────────────────────

		{
			ID: "opus_enthusiast", Name: "Opus Enthusiast",
			Description: "Run 5 sessions using any Opus model",
			Tier:        TierBronze, Category: CategoryModelCollection,
			Condition: func(s *Stats) bool { return modelFamilySessions(s, "opus") >= 5 },
		},
		{
			ID: "sonnet_fan", Name: "Sonnet Fan",
			Description: "Run 5 sessions using any Sonnet model",
			Tier:        TierBronze, Category: CategoryModelCollection,
			Condition: func(s *Stats) bool { return modelFamilySessions(s, "sonnet") >= 5 },
		},
		{
			ID: "haiku_speedster", Name: "Haiku Speedster",
			Description: "Run 5 sessions using any Haiku model",
			Tier:        TierBronze, Category: CategoryModelCollection,
			Condition: func(s *Stats) bool { return modelFamilySessions(s, "haiku") >= 5 },
		},
		{
			ID: "full_spectrum", Name: "Full Spectrum",
			Description: "Use at least one Opus, one Sonnet, and one Haiku model",
			Tier:        TierSilver, Category: CategoryModelCollection,
			Condition: func(s *Stats) bool {
				return hasModelFamily(s, "opus") &&
					hasModelFamily(s, "sonnet") &&
					hasModelFamily(s, "haiku")
			},
		},
		{
			ID: "model_collector", Name: "Model Collector",
			Description: "Use 5 or more distinct model IDs",
			Tier:        TierSilver, Category: CategoryModelCollection,
			Condition: func(s *Stats) bool { return s.DistinctModelsUsed >= 5 },
		},
		{
			ID: "connoisseur", Name: "Connoisseur",
			Description: "Use 10 or more distinct model IDs",
			Tier:        TierGold, Category: CategoryModelCollection,
			Condition: func(s *Stats) bool { return s.DistinctModelsUsed >= 10 },
		},

		// ── Performance & Endurance ────────────────────────────────────────

		{
			ID: "redline", Name: "Redline",
			Description: "A session reaches 95%+ context utilization",
			Tier:        TierBronze, Category: CategoryPerformanceEndurance,
			Condition: func(s *Stats) bool { return s.MaxContextUtilization >= 0.95 },
		},
		{
			ID: "afterburner", Name: "Afterburner",
			Description: "A session burns tokens at 5,000+ tokens per minute",
			Tier:        TierSilver, Category: CategoryPerformanceEndurance,
			Condition: func(s *Stats) bool { return s.MaxBurnRate >= 5000 },
		},
		{
			ID: "marathon", Name: "Marathon",
			Description: "A single session runs for 2 or more hours",
			Tier:        TierSilver, Category: CategoryPerformanceEndurance,
			Condition: func(s *Stats) bool { return s.MaxSessionDurationSec >= 7200 },
		},
		{
			ID: "tool_fiend", Name: "Tool Fiend",
			Description: "A single session makes 500 or more tool calls",
			Tier:        TierSilver, Category: CategoryPerformanceEndurance,
			Condition: func(s *Stats) bool { return s.MaxToolCalls >= 500 },
		},
		{
			ID: "conversationalist", Name: "Conversationalist",
			Description: "A single session exchanges 200 or more messages",
			Tier:        TierBronze, Category: CategoryPerformanceEndurance,
			Condition: func(s *Stats) bool { return s.MaxMessages >= 200 },
		},
		// NOTE: Same threshold as "on_a_roll" (Streaks) -- intentional cross-category award.
		{
			ID: "clean_sweep", Name: "Clean Sweep",
			Description: "Complete 10 sessions in a row without any errors",
			Tier:        TierSilver, Category: CategoryPerformanceEndurance,
			Condition: func(s *Stats) bool { return s.ConsecutiveCompletions >= 10 },
		},
		{
			ID: "photo_finish", Name: "Photo Finish",
			Description: "Two sessions complete within 10 seconds of each other",
			Tier:        TierGold, Category: CategoryPerformanceEndurance,
			Condition: func(s *Stats) bool { return s.PhotoFinishSeen },
		},

		// ── Spectacle ──────────────────────────────────────────────────────

		{
			ID: "grid_start", Name: "Grid Start",
			Description: "Have 3 or more sessions racing simultaneously",
			Tier:        TierBronze, Category: CategorySpectacle,
			Condition: func(s *Stats) bool { return s.MaxConcurrentActive >= 3 },
		},
		{
			ID: "full_grid", Name: "Full Grid",
			Description: "Have 5 or more sessions racing simultaneously",
			Tier:        TierSilver, Category: CategorySpectacle,
			Condition: func(s *Stats) bool { return s.MaxConcurrentActive >= 5 },
		},
		{
			ID: "grid_full", Name: "Grid Full",
			Description: "Have 10 or more sessions racing simultaneously",
			Tier:        TierGold, Category: CategorySpectacle,
			Condition: func(s *Stats) bool { return s.MaxConcurrentActive >= 10 },
		},
		{
			ID: "crash_survivor", Name: "Crash Survivor",
			Description: "Have a session error, then complete a new session successfully",
			Tier:        TierBronze, Category: CategorySpectacle,
			Condition: func(s *Stats) bool {
				return s.TotalErrors >= 1 && s.TotalCompletions >= 1
			},
		},
		{
			ID: "burning_rubber", Name: "Burning Rubber",
			Description: "3 or more sessions all above 50% context utilization simultaneously",
			Tier:        TierSilver, Category: CategorySpectacle,
			Condition: func(s *Stats) bool {
				return s.MaxHighUtilizationSimultaneous >= 3
			},
		},

		// ── Streaks ────────────────────────────────────────────────────────

		{
			ID: "hat_trick", Name: "Hat Trick",
			Description: "Complete 3 sessions in a row without any errors",
			Tier:        TierBronze, Category: CategoryStreaks,
			Condition: func(s *Stats) bool { return s.ConsecutiveCompletions >= 3 },
		},
		// NOTE: Same threshold as "clean_sweep" (Performance & Endurance) -- intentional cross-category award.
		{
			ID: "on_a_roll", Name: "On a Roll",
			Description: "Complete 10 sessions in a row without any errors",
			Tier:        TierSilver, Category: CategoryStreaks,
			Condition: func(s *Stats) bool { return s.ConsecutiveCompletions >= 10 },
		},
		{
			ID: "untouchable", Name: "Untouchable",
			Description: "Complete 25 sessions in a row without any errors",
			Tier:        TierGold, Category: CategoryStreaks,
			Condition: func(s *Stats) bool { return s.ConsecutiveCompletions >= 25 },
		},
	}
}
