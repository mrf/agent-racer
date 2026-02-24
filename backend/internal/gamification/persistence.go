package gamification

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	// statsVersion is bumped when the schema changes. The Load function
	// can use it to apply migrations in the future.
	statsVersion = 1

	statsFileName = "stats.json"
	appDirName    = "agent-racer"
)

// Stats is the persistent aggregate data for the gamification system.
// It is loaded from and saved to ~/.local/state/agent-racer/stats.json
// (respecting XDG_STATE_HOME).
type Stats struct {
	Version int `json:"version"`

	// Aggregate counters
	TotalSessions          int `json:"totalSessions"`
	TotalCompletions       int `json:"totalCompletions"`
	TotalErrors            int `json:"totalErrors"`
	ConsecutiveCompletions int `json:"consecutiveCompletions"`

	// Per-dimension breakdowns
	SessionsPerSource   map[string]int `json:"sessionsPerSource"`
	SessionsPerModel    map[string]int `json:"sessionsPerModel"`
	DistinctModelsUsed  int            `json:"distinctModelsUsed"`
	DistinctSourcesUsed int            `json:"distinctSourcesUsed"`

	// Peak metrics (all-time highs)
	MaxContextUtilization float64 `json:"maxContextUtilization"`
	MaxBurnRate           float64 `json:"maxBurnRate"`
	MaxConcurrentActive   int     `json:"maxConcurrentActive"`
	MaxToolCalls          int     `json:"maxToolCalls"`
	MaxMessages           int     `json:"maxMessages"`
	MaxSessionDurationSec float64 `json:"maxSessionDurationSec"`

	// Gamification state
	AchievementsUnlocked map[string]time.Time `json:"achievementsUnlocked"`
	BattlePass           BattlePass           `json:"battlePass"`
	ArchivedSeasons      []ArchivedSeason     `json:"archivedSeasons,omitempty"`
	Equipped             Equipped             `json:"equipped"`
	WeeklyChallenges     WeeklyChallengeState `json:"weeklyChallenges"`

	LastUpdated time.Time `json:"lastUpdated"`
}

// BattlePass tracks seasonal progression.
type BattlePass struct {
	Season string `json:"season"`
	Tier   int    `json:"tier"`
	XP     int    `json:"xp"`
}

// UnmarshalJSON handles migration from older stats files where Season was an int.
func (bp *BattlePass) UnmarshalJSON(data []byte) error {
	type Alias BattlePass
	aux := &struct {
		Season json.RawMessage `json:"season"`
		*Alias
	}{Alias: (*Alias)(bp)}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if len(aux.Season) == 0 {
		return nil
	}
	// Try string first.
	var s string
	if err := json.Unmarshal(aux.Season, &s); err == nil {
		bp.Season = s
		return nil
	}
	// Fall back to number (legacy format).
	var n float64
	if err := json.Unmarshal(aux.Season, &n); err == nil {
		if n == 0 {
			bp.Season = ""
		} else {
			bp.Season = strconv.Itoa(int(n))
		}
		return nil
	}
	return fmt.Errorf("cannot parse season field: %s", string(aux.Season))
}

// ArchivedSeason holds the final state of a completed season.
type ArchivedSeason struct {
	Season   string `json:"season"`
	Tier     int    `json:"tier"`
	XP       int    `json:"xp"`
	Archived string `json:"archived"` // RFC 3339 timestamp
}

// Equipped tracks which cosmetic item is active in each slot.
// Each field holds a reward ID, or the empty string if the slot is empty.
type Equipped struct {
	Paint string `json:"paint,omitempty"`
	Trail string `json:"trail,omitempty"`
	Body  string `json:"body,omitempty"`
	Badge string `json:"badge,omitempty"`
	Sound string `json:"sound,omitempty"`
	Theme string `json:"theme,omitempty"`
	Title string `json:"title,omitempty"`
}

// Store handles loading and saving Stats to disk.
type Store struct {
	dir string // directory containing stats.json
}

// NewStore creates a Store that reads/writes stats in the given directory.
// The directory is created (with parents) on the first Save if it does not
// exist. Pass an empty string to use the default XDG state path.
func NewStore(dir string) *Store {
	if dir == "" {
		dir = defaultStatsDir()
	}
	return &Store{dir: dir}
}

// Path returns the full path to the stats file.
func (s *Store) Path() string {
	return filepath.Join(s.dir, statsFileName)
}

// Load reads stats from disk. If the file does not exist, a zero-value
// Stats with initialized maps and the current version is returned.
func (s *Store) Load() (*Stats, error) {
	data, err := os.ReadFile(s.Path())
	if err != nil {
		if os.IsNotExist(err) {
			return newStats(), nil
		}
		return nil, fmt.Errorf("reading stats: %w", err)
	}

	var st Stats
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parsing stats: %w", err)
	}
	st.initMaps()

	return &st, nil
}

// Save writes stats to disk using an atomic temp-file-then-rename pattern.
// The directory is created if it does not already exist.
func (s *Store) Save(st *Stats) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("creating stats dir: %w", err)
	}

	st.Version = statsVersion
	st.LastUpdated = time.Now().UTC()

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling stats: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(s.dir, ".stats-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpPath, s.Path()); err != nil {
		return fmt.Errorf("renaming stats file: %w", err)
	}
	committed = true

	return nil
}

// newStats returns a Stats with initialized maps and the current version.
func newStats() *Stats {
	return &Stats{
		Version:              statsVersion,
		SessionsPerSource:    make(map[string]int),
		SessionsPerModel:     make(map[string]int),
		AchievementsUnlocked: make(map[string]time.Time),
	}
}

// initMaps ensures all map fields are non-nil after deserialization.
func (st *Stats) initMaps() {
	if st.SessionsPerSource == nil {
		st.SessionsPerSource = make(map[string]int)
	}
	if st.SessionsPerModel == nil {
		st.SessionsPerModel = make(map[string]int)
	}
	if st.AchievementsUnlocked == nil {
		st.AchievementsUnlocked = make(map[string]time.Time)
	}
	initWeeklyChallengeState(&st.WeeklyChallenges)
}

// clone returns a deep copy of Stats with all maps duplicated.
func (st *Stats) clone() *Stats {
	cp := *st
	cp.SessionsPerSource = make(map[string]int, len(st.SessionsPerSource))
	for k, v := range st.SessionsPerSource {
		cp.SessionsPerSource[k] = v
	}
	cp.SessionsPerModel = make(map[string]int, len(st.SessionsPerModel))
	for k, v := range st.SessionsPerModel {
		cp.SessionsPerModel[k] = v
	}
	cp.AchievementsUnlocked = make(map[string]time.Time, len(st.AchievementsUnlocked))
	for k, v := range st.AchievementsUnlocked {
		cp.AchievementsUnlocked[k] = v
	}
	if len(st.ArchivedSeasons) > 0 {
		cp.ArchivedSeasons = make([]ArchivedSeason, len(st.ArchivedSeasons))
		copy(cp.ArchivedSeasons, st.ArchivedSeasons)
	}
	cp.WeeklyChallenges.ActiveIDs = make([]string, len(st.WeeklyChallenges.ActiveIDs))
	copy(cp.WeeklyChallenges.ActiveIDs, st.WeeklyChallenges.ActiveIDs)
	cp.WeeklyChallenges.Completed = make([]string, len(st.WeeklyChallenges.Completed))
	copy(cp.WeeklyChallenges.Completed, st.WeeklyChallenges.Completed)
	cp.WeeklyChallenges.Snapshot.SessionsPerModel = make(map[string]int, len(st.WeeklyChallenges.Snapshot.SessionsPerModel))
	for k, v := range st.WeeklyChallenges.Snapshot.SessionsPerModel {
		cp.WeeklyChallenges.Snapshot.SessionsPerModel[k] = v
	}
	cp.WeeklyChallenges.Snapshot.SessionsPerSource = make(map[string]int, len(st.WeeklyChallenges.Snapshot.SessionsPerSource))
	for k, v := range st.WeeklyChallenges.Snapshot.SessionsPerSource {
		cp.WeeklyChallenges.Snapshot.SessionsPerSource[k] = v
	}
	cp.WeeklyChallenges.XPAwarded = make(map[string]bool, len(st.WeeklyChallenges.XPAwarded))
	for k, v := range st.WeeklyChallenges.XPAwarded {
		cp.WeeklyChallenges.XPAwarded[k] = v
	}
	return &cp
}

// defaultStatsDir returns ~/.local/state/agent-racer, respecting
// XDG_STATE_HOME if set.
func defaultStatsDir() string {
	if base := os.Getenv("XDG_STATE_HOME"); base != "" {
		return filepath.Join(base, appDirName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, ".local", "state", appDirName)
}
