package tracks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// Track is a custom race circuit defined by a tile grid.
type Track struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Width     int        `json:"width"`
	Height    int        `json:"height"`
	Tiles     [][]string `json:"tiles"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// Store manages track persistence in the XDG data directory.
type Store struct {
	dir string
}

var validID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// NewStore creates a Store using XDG_DATA_HOME (~/.local/share/agent-racer/tracks).
func NewStore(dataDir string) (*Store, error) {
	if dataDir == "" {
		xdgData := os.Getenv("XDG_DATA_HOME")
		if xdgData == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("tracks: no home dir: %w", err)
			}
			xdgData = filepath.Join(home, ".local", "share")
		}
		dataDir = filepath.Join(xdgData, "agent-racer", "tracks")
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("tracks: mkdir %s: %w", dataDir, err)
	}
	return &Store{dir: dataDir}, nil
}

func (s *Store) path(id string) string {
	return filepath.Join(s.dir, id+".json")
}

func (s *Store) List() ([]*Track, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var result []*Track
	for i := 0; i < len(entries); i++ {
		e := entries[i]
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := e.Name()[:len(e.Name())-5]
		t, err := s.Get(id)
		if err != nil {
			continue
		}
		result = append(result, t)
	}
	return result, nil
}

func (s *Store) Get(id string) (*Track, error) {
	if !validID.MatchString(id) {
		return nil, fmt.Errorf("invalid track id")
	}
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return nil, err
	}
	var t Track
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) Save(t *Track) error {
	if !validID.MatchString(t.ID) {
		return fmt.Errorf("invalid track id")
	}
	t.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(t.ID), data, 0644)
}

func (s *Store) Delete(id string) error {
	if !validID.MatchString(id) {
		return fmt.Errorf("invalid track id")
	}
	return os.Remove(s.path(id))
}
