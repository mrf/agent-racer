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

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store
}

func TestStoreSaveAndGetRoundTrip(t *testing.T) {
	store := newTestStore(t)
	track := testTrack("round-trip", "Round Trip", testTiles(4, 4, "road"))
	if err := store.Save(track); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Get("round-trip")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != "round-trip" {
		t.Fatalf("ID = %q, want %q", got.ID, "round-trip")
	}
	if got.Name != "Round Trip" {
		t.Fatalf("Name = %q, want %q", got.Name, "Round Trip")
	}
	if got.Width != 4 || got.Height != 4 {
		t.Fatalf("dimensions = %dx%d, want 4x4", got.Width, got.Height)
	}
}

func TestStoreListEmpty(t *testing.T) {
	store := newTestStore(t)
	tracks, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tracks) != 0 {
		t.Fatalf("len = %d, want 0", len(tracks))
	}
}

func TestStoreListReturnsSavedTracks(t *testing.T) {
	store := newTestStore(t)
	tiles := testTiles(2, 2, "road")
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("track-%d", i)
		if err := store.Save(testTrack(id, "Track "+id, tiles)); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
	}
	tracks, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tracks) != 3 {
		t.Fatalf("len = %d, want 3", len(tracks))
	}
}

func TestStoreDelete(t *testing.T) {
	store := newTestStore(t)
	track := testTrack("del-target", "Delete Target", testTiles(2, 2, "road"))
	if err := store.Save(track); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Delete("del-target"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := store.Get("del-target")
	if err == nil {
		t.Fatal("Get after Delete returned nil error, want not found")
	}
}

func TestStoreGetInvalidID(t *testing.T) {
	store := newTestStore(t)
	badIDs := []string{"", "has spaces", "../escape", "semi;colon", "slash/id"}
	for i := 0; i < len(badIDs); i++ {
		_, err := store.Get(badIDs[i])
		if err == nil {
			t.Fatalf("Get(%q) returned nil error, want invalid id", badIDs[i])
		}
	}
}

func TestStoreGetNonexistent(t *testing.T) {
	store := newTestStore(t)
	_, err := store.Get("does-not-exist")
	if err == nil {
		t.Fatal("Get(nonexistent) returned nil error")
	}
}

func TestStoreSaveInvalidID(t *testing.T) {
	store := newTestStore(t)
	badIDs := []string{"", "has spaces", "../escape"}
	for i := 0; i < len(badIDs); i++ {
		track := testTrack(badIDs[i], "Bad", testTiles(2, 2, "road"))
		if err := store.Save(track); err == nil {
			t.Fatalf("Save(id=%q) returned nil error, want invalid id", badIDs[i])
		}
	}
}

func TestStoreDeleteInvalidID(t *testing.T) {
	store := newTestStore(t)
	if err := store.Delete("../escape"); err == nil {
		t.Fatal("Delete(invalid) returned nil error")
	}
}

func TestStoreDeleteNonexistent(t *testing.T) {
	store := newTestStore(t)
	if err := store.Delete("no-such-track"); err == nil {
		t.Fatal("Delete(nonexistent) returned nil error")
	}
}

func TestStoreSaveSetsUpdatedAt(t *testing.T) {
	store := newTestStore(t)
	track := testTrack("ts-test", "Timestamps", testTiles(2, 2, "road"))
	before := time.Now()
	if err := store.Save(track); err != nil {
		t.Fatalf("Save: %v", err)
	}
	after := time.Now()

	got, err := store.Get("ts-test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.UpdatedAt.Before(before) || got.UpdatedAt.After(after) {
		t.Fatalf("UpdatedAt = %v, want between %v and %v", got.UpdatedAt, before, after)
	}
}

func TestStoreSaveOverwriteExisting(t *testing.T) {
	store := newTestStore(t)
	v1 := testTrack("overwrite", "V1", testTiles(2, 2, "road"))
	if err := store.Save(v1); err != nil {
		t.Fatalf("Save v1: %v", err)
	}
	v2 := testTrack("overwrite", "V2", testTiles(2, 2, "sand"))
	if err := store.Save(v2); err != nil {
		t.Fatalf("Save v2: %v", err)
	}
	got, err := store.Get("overwrite")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "V2" {
		t.Fatalf("Name = %q, want %q", got.Name, "V2")
	}
}

func TestStoreListIgnoresNonJSONFiles(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Save(testTrack("real", "Real", testTiles(2, 2, "road"))); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Write a non-JSON file into the store directory
	if err := os.WriteFile(dir+"/notes.txt", []byte("not a track"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	tracks, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("len = %d, want 1", len(tracks))
	}
	if tracks[0].ID != "real" {
		t.Fatalf("ID = %q, want %q", tracks[0].ID, "real")
	}
}

func TestNewStoreCreatesDirectoryWithOwnerOnlyPerms(t *testing.T) {
	parent := t.TempDir()
	dir := parent + "/sub/tracks"
	_, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("dir perm = %o, want 0700", perm)
	}
}

func TestStoreSaveCreatesFileWithOwnerOnlyPerms(t *testing.T) {
	store := newTestStore(t)
	track := testTrack("perm-test", "Perm Test", testTiles(2, 2, "road"))
	if err := store.Save(track); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(store.path("perm-test"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file perm = %o, want 0600", perm)
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
