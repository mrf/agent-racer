package tracks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Validation limits for track tile data.
const (
	maxTrackWidth  = 64
	maxTrackHeight = 64
	maxTileCount   = 4096
	maxNameLength  = 128
)

// validTileTypes is the set of tile type strings accepted by the server.
// Empty string represents an empty cell.
var validTileTypes = map[string]bool{
	"":            true,
	"straight-h":  true,
	"straight-v":  true,
	"curve-ne":    true,
	"curve-nw":    true,
	"curve-se":    true,
	"curve-sw":    true,
	"chicane":     true,
	"pit-entry":   true,
	"pit-exit":    true,
	"grandstand":  true,
	"tree":        true,
	"barrier":     true,
	"start-line":  true,
	"finish-line": true,
}

// validateTrack checks that the track data is within acceptable bounds.
// Returns a human-readable error message or "" if valid.
func validateTrack(t *Track) string {
	if strings.TrimSpace(t.Name) == "" {
		return "name is required"
	}
	if len(t.Name) > maxNameLength {
		return fmt.Sprintf("name exceeds maximum length of %d characters", maxNameLength)
	}
	if t.Width <= 0 || t.Height <= 0 {
		return "width and height must be positive"
	}
	if t.Width > maxTrackWidth {
		return fmt.Sprintf("width %d exceeds maximum of %d", t.Width, maxTrackWidth)
	}
	if t.Height > maxTrackHeight {
		return fmt.Sprintf("height %d exceeds maximum of %d", t.Height, maxTrackHeight)
	}
	if t.Width*t.Height > maxTileCount {
		return fmt.Sprintf("tile count %d exceeds maximum of %d", t.Width*t.Height, maxTileCount)
	}
	if len(t.Tiles) != t.Height {
		return fmt.Sprintf("tiles has %d rows but height is %d", len(t.Tiles), t.Height)
	}
	for i := 0; i < len(t.Tiles); i++ {
		if len(t.Tiles[i]) != t.Width {
			return fmt.Sprintf("row %d has %d columns but width is %d", i, len(t.Tiles[i]), t.Width)
		}
		for j := 0; j < len(t.Tiles[i]); j++ {
			if !validTileTypes[t.Tiles[i][j]] {
				return fmt.Sprintf("invalid tile type %q at row %d, column %d", t.Tiles[i][j], i, j)
			}
		}
	}
	return ""
}

// maxRequestBodySize is the maximum allowed size for JSON request bodies (1 MB).
const maxRequestBodySize int64 = 1 << 20

// Handler handles /api/tracks and /api/tracks/{id} routes.
type Handler struct {
	store          *Store
	getActiveFn    func() string
	setActiveFn    func(id string) error
}

// NewHandler creates a new Handler backed by the given store.
func NewHandler(store *Store) *Handler {
	return &Handler{store: store}
}

// SetActiveTrackProvider registers callbacks for getting and setting the active track ID.
// get returns the current active track ID; set persists a new active track ID.
func (h *Handler) SetActiveTrackProvider(get func() string, set func(id string) error) {
	h.getActiveFn = get
	h.setActiveFn = set
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

	if path == "active" {
		switch r.Method {
		case http.MethodGet:
			h.getActiveTrack(w, r)
		case http.MethodPut:
			h.setActiveTrack(w, r)
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

func writeJSON(w http.ResponseWriter, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(buf.Bytes())
}

// decodeBody applies a MaxBytesReader limit, decodes JSON into dst, and writes
// the appropriate HTTP error response on failure. Returns true on success.
func decodeBody(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		}
		return false
	}
	return true
}

var presetIDs = func() map[string]bool {
	m := make(map[string]bool)
	for _, p := range Presets() {
		m[p.ID] = true
	}
	return m
}()

func isPreset(id string) bool {
	return presetIDs[id]
}

// activeTrackResponse is the JSON shape returned by GET /api/tracks/active.
type activeTrackResponse struct {
	ID string `json:"id"`
}

// activeTrackRequest is the JSON body expected by PUT /api/tracks/active.
type activeTrackRequest struct {
	ID string `json:"id"`
}

func (h *Handler) getActiveTrack(w http.ResponseWriter, r *http.Request) {
	if h.getActiveFn == nil {
		http.Error(w, "active track not available", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, activeTrackResponse{ID: h.getActiveFn()})
}

func (h *Handler) setActiveTrack(w http.ResponseWriter, r *http.Request) {
	if h.setActiveFn == nil {
		http.Error(w, "active track not available", http.StatusServiceUnavailable)
		return
	}
	var req activeTrackRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.ID == "" {
		http.Error(w, "bad request: id is required", http.StatusBadRequest)
		return
	}
	if !validID.MatchString(req.ID) {
		http.Error(w, "bad request: invalid track id", http.StatusBadRequest)
		return
	}
	// Verify the track exists (preset or user-defined).
	if _, err := h.GetByID(req.ID); err != nil {
		http.Error(w, "track not found", http.StatusNotFound)
		return
	}
	if err := h.setActiveFn(req.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, activeTrackResponse(req))
}

// GetByID resolves a track by ID, checking presets first then the user store.
// Returns nil, nil when id is empty (no active track configured).
// Returns nil, err when the track is not found or cannot be read.
func (h *Handler) GetByID(id string) (*Track, error) {
	if id == "" {
		return nil, nil
	}
	if isPreset(id) {
		presets := Presets()
		for i := 0; i < len(presets); i++ {
			if presets[i].ID == id {
				return presets[i], nil
			}
		}
	}
	return h.store.Get(id)
}

func (h *Handler) listTracks(w http.ResponseWriter, r *http.Request) {
	userTracks, err := h.store.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	presets := Presets()
	all := make([]*Track, 0, len(presets)+len(userTracks))
	all = append(all, presets...)
	for i := 0; i < len(userTracks); i++ {
		if !isPreset(userTracks[i].ID) {
			all = append(all, userTracks[i])
		}
	}
	writeJSON(w, all)
}

func (h *Handler) getTrack(w http.ResponseWriter, r *http.Request, id string) {
	t, err := h.GetByID(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, t)
}

func (h *Handler) createTrack(w http.ResponseWriter, r *http.Request) {
	var t Track
	if !decodeBody(w, r, &t) {
		return
	}
	if msg := validateTrack(&t); msg != "" {
		http.Error(w, "bad request: "+msg, http.StatusBadRequest)
		return
	}
	if t.ID == "" {
		t.ID = fmt.Sprintf("track-%d", time.Now().UnixMilli())
	}
	t.CreatedAt = time.Now()
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
	if !decodeBody(w, r, &t) {
		return
	}
	if msg := validateTrack(&t); msg != "" {
		http.Error(w, "bad request: "+msg, http.StatusBadRequest)
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
