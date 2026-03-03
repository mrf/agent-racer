package tracks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Handler handles /api/tracks and /api/tracks/{id} routes.
type Handler struct {
	store *Store
}

// NewHandler creates a new Handler backed by the given store.
func NewHandler(store *Store) *Handler {
	return &Handler{store: store}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/tracks")
	path = strings.TrimPrefix(path, "/")

	if path == "" {
		switch r.Method {
		case http.MethodGet:
			h.listTracks(w, r)
		case http.MethodPost:
			h.createTrack(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	id := path
	switch r.Method {
	case http.MethodGet:
		h.getTrack(w, r, id)
	case http.MethodPut:
		h.updateTrack(w, r, id)
	case http.MethodDelete:
		h.deleteTrack(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func isPreset(id string) bool {
	for _, p := range Presets() {
		if p.ID == id {
			return true
		}
	}
	return false
}

func (h *Handler) listTracks(w http.ResponseWriter, r *http.Request) {
	userTracks, err := h.store.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	presets := Presets()
	presetIDs := make(map[string]bool)
	for i := 0; i < len(presets); i++ {
		presetIDs[presets[i].ID] = true
	}
	var filtered []*Track
	for i := 0; i < len(userTracks); i++ {
		if !presetIDs[userTracks[i].ID] {
			filtered = append(filtered, userTracks[i])
		}
	}
	all := make([]*Track, 0, len(presets)+len(filtered))
	all = append(all, presets...)
	all = append(all, filtered...)
	writeJSON(w, all)
}

func (h *Handler) getTrack(w http.ResponseWriter, r *http.Request, id string) {
	for _, p := range Presets() {
		if p.ID == id {
			writeJSON(w, p)
			return
		}
	}
	t, err := h.store.Get(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, t)
}

func (h *Handler) createTrack(w http.ResponseWriter, r *http.Request) {
	var t Track
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if t.ID == "" {
		t.ID = fmt.Sprintf("track-%d", time.Now().UnixMilli())
	}
	t.CreatedAt = time.Now()
	t.UpdatedAt = time.Now()
	if err := h.store.Save(&t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, t)
}

func (h *Handler) updateTrack(w http.ResponseWriter, r *http.Request, id string) {
	if isPreset(id) {
		http.Error(w, "preset tracks are read-only", http.StatusForbidden)
		return
	}
	var t Track
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	t.ID = id
	if err := h.store.Save(&t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, t)
}

func (h *Handler) deleteTrack(w http.ResponseWriter, r *http.Request, id string) {
	if isPreset(id) {
		http.Error(w, "preset tracks cannot be deleted", http.StatusForbidden)
		return
	}
	if err := h.store.Delete(id); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
