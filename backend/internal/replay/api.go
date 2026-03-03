package replay

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ReplayInfo describes a single replay file available for playback.
type ReplayInfo struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"createdAt"`
}

// Handler serves the /api/replays REST endpoints.
type Handler struct {
	dir    string
	authFn func(r *http.Request) bool
}

// NewHandler returns a Handler that serves replay files from dir.
// authFn is called on each request; pass nil to allow unauthenticated access.
func NewHandler(dir string, authFn func(r *http.Request) bool) *Handler {
	return &Handler{dir: dir, authFn: authFn}
}

// RegisterRoutes registers /api/replays and /api/replays/ on mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/replays", h.handleList)
	mux.HandleFunc("/api/replays/", h.handleGet)
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	if h.authFn != nil && !h.authFn(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	entries, err := os.ReadDir(h.dir)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]\n"))
			return
		}
		http.Error(w, "failed to list replays", http.StatusInternalServerError)
		return
	}

	var replays []ReplayInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		replays = append(replays, ReplayInfo{
			ID:        strings.TrimSuffix(e.Name(), ".jsonl"),
			Name:      e.Name(),
			Size:      info.Size(),
			CreatedAt: info.ModTime().UTC(),
		})
	}

	sort.Slice(replays, func(i, j int) bool {
		return replays[i].CreatedAt.After(replays[j].CreatedAt)
	})

	w.Header().Set("Content-Type", "application/json")
	if replays == nil {
		_, _ = w.Write([]byte("[]\n"))
		return
	}
	_ = json.NewEncoder(w).Encode(replays)
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	if h.authFn != nil && !h.authFn(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/replays/")
	if id == "" || strings.ContainsAny(id, "/\\") || strings.Contains(id, "..") {
		http.Error(w, "invalid replay id", http.StatusBadRequest)
		return
	}

	path := filepath.Join(h.dir, id+".jsonl")
	// Guard against path traversal after Join resolves ".." etc.
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(h.dir)+string(filepath.Separator)) {
		http.Error(w, "invalid replay id", http.StatusBadRequest)
		return
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "replay not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to open replay", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = io.Copy(w, f)
}
