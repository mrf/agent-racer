package tracks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
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
	mu  sync.RWMutex
}

var validID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

var (
	syncOSFile = func(f *os.File) error {
		return f.Sync()
	}
	renameFile = os.Rename
	openDir    = func(path string) (*os.File, error) {
		return os.Open(path)
	}
)

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
	s.mu.RLock()
	defer s.mu.RUnlock()

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
		t, err := s.get(id)
		if err != nil {
			continue
		}
		result = append(result, t)
	}
	return result, nil
}

func (s *Store) Get(id string) (*Track, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.get(id)
}

func (s *Store) get(id string) (*Track, error) {
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
	s.mu.Lock()
	defer s.mu.Unlock()

	if !validID.MatchString(t.ID) {
		return fmt.Errorf("invalid track id")
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("tracks: mkdir %s: %w", s.dir, err)
	}
	t.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(s.dir, "."+t.ID+"-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := syncOSFile(tmp); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := renameFile(tmpPath, s.path(t.ID)); err != nil {
		return fmt.Errorf("renaming track file: %w", err)
	}
	committed = true
	if err := syncDir(s.dir); err != nil {
		return fmt.Errorf("syncing tracks dir: %w", err)
	}

	return nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !validID.MatchString(id) {
		return fmt.Errorf("invalid track id")
	}
	return os.Remove(s.path(id))
}

func syncDir(path string) error {
	dir, err := openDir(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = dir.Close()
	}()

	return syncOSFile(dir)
}
