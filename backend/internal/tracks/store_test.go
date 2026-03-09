package tracks

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

func TestStoreSaveRenameFailurePreservesExistingTrack(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore() error: %v", err)
	}

	initial := testTrack("atomic-track", "initial", testTiles(8, 8, "road"))
	if err := store.Save(initial); err != nil {
		t.Fatalf("initial Save() error: %v", err)
	}

	originalRenameFile := renameFile
	t.Cleanup(func() {
		renameFile = originalRenameFile
	})

	renameFile = func(oldPath, newPath string) error {
		return errors.New("rename failed")
	}

	updated := testTrack("atomic-track", "updated", testTiles(8, 8, "sand"))
	err = store.Save(updated)
	if err == nil {
		t.Fatal("Save() error = nil, want rename failure")
	}
	if err.Error() != "renaming track file: rename failed" {
		t.Fatalf("Save() error = %q, want rename failure", err)
	}

	loaded, err := store.Get("atomic-track")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if loaded.Name != "initial" {
		t.Fatalf("loaded.Name = %q, want %q", loaded.Name, "initial")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if entries[0].Name() != "atomic-track.json" {
		t.Fatalf("entries[0].Name() = %q, want %q", entries[0].Name(), "atomic-track.json")
	}
}

func TestStoreConcurrentSaveAndReadKeepsTrackReadable(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore() error: %v", err)
	}

	sharedTiles := testTiles(192, 192, "road")
	if err := store.Save(testTrack("concurrent-track", "seed", sharedTiles)); err != nil {
		t.Fatalf("initial Save() error: %v", err)
	}

	const (
		writerCount    = 3
		readerCount    = 4
		saveIterations = 12
	)

	start := make(chan struct{})
	writersDone := make(chan struct{})
	errCh := make(chan error, writerCount*saveIterations+readerCount)

	var writers sync.WaitGroup
	for writerIndex := 0; writerIndex < writerCount; writerIndex++ {
		writers.Add(1)
		go func(writer int) {
			defer writers.Done()
			<-start
			for iteration := 0; iteration < saveIterations; iteration++ {
				name := fmt.Sprintf("writer-%d-save-%d", writer, iteration)
				track := testTrack("concurrent-track", name, sharedTiles)
				if err := store.Save(track); err != nil {
					errCh <- fmt.Errorf("writer %d save %d: %w", writer, iteration, err)
					return
				}
			}
		}(writerIndex)
	}

	go func() {
		writers.Wait()
		close(writersDone)
	}()

	var readers sync.WaitGroup
	for readerIndex := 0; readerIndex < readerCount; readerIndex++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			<-start
			for {
				select {
				case <-writersDone:
					return
				default:
				}

				track, err := store.Get("concurrent-track")
				if err != nil {
					errCh <- fmt.Errorf("Get() error: %w", err)
					return
				}
				if track.ID != "concurrent-track" {
					errCh <- fmt.Errorf("Get() returned ID %q", track.ID)
					return
				}

				tracks, err := store.List()
				if err != nil {
					errCh <- fmt.Errorf("List() error: %w", err)
					return
				}
				found := false
				for i := 0; i < len(tracks); i++ {
					if tracks[i].ID == "concurrent-track" {
						found = true
						break
					}
				}
				if !found {
					errCh <- errors.New("List() omitted concurrent-track")
					return
				}
			}
		}()
	}

	close(start)
	writers.Wait()
	readers.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	data, err := os.ReadFile(store.path("concurrent-track"))
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}

	var loaded Track
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("final track file is invalid JSON: %v", err)
	}
	if loaded.ID != "concurrent-track" {
		t.Fatalf("loaded.ID = %q, want %q", loaded.ID, "concurrent-track")
	}
}

func testTrack(id string, name string, tiles [][]string) *Track {
	return &Track{
		ID:        id,
		Name:      name,
		Width:     len(tiles[0]),
		Height:    len(tiles),
		Tiles:     tiles,
		CreatedAt: time.Unix(1700000000, 0).UTC(),
	}
}

func testTiles(width int, height int, tile string) [][]string {
	tiles := make([][]string, height)
	for row := 0; row < height; row++ {
		tiles[row] = make([]string, width)
		for col := 0; col < width; col++ {
			tiles[row][col] = tile
		}
	}
	return tiles
}
