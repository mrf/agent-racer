package tracks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return NewHandler(store)
}

func doRequest(h *Handler, method, path, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// --- Collection routes: GET /api/tracks, POST /api/tracks ---

func TestListTracksReturnsPresetsWhenStoreEmpty(t *testing.T) {
	h := newTestHandler(t)
	w := doRequest(h, http.MethodGet, "/api/tracks", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var tracks []*Track
	if err := json.Unmarshal(w.Body.Bytes(), &tracks); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(tracks) != len(Presets()) {
		t.Fatalf("len = %d, want %d", len(tracks), len(Presets()))
	}
}

func TestListTracksIncludesUserTracks(t *testing.T) {
	h := newTestHandler(t)
	body := `{"id":"user-track","name":"My Track","width":4,"height":4,"tiles":[["straight-h","straight-h","straight-h","straight-h"],["straight-h","straight-h","straight-h","straight-h"],["straight-h","straight-h","straight-h","straight-h"],["straight-h","straight-h","straight-h","straight-h"]]}`
	w := doRequest(h, http.MethodPost, "/api/tracks", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want %d", w.Code, http.StatusCreated)
	}

	w = doRequest(h, http.MethodGet, "/api/tracks", "")
	var tracks []*Track
	if err := json.Unmarshal(w.Body.Bytes(), &tracks); err != nil {
		t.Fatalf("decode: %v", err)
	}
	expected := len(Presets()) + 1
	if len(tracks) != expected {
		t.Fatalf("len = %d, want %d", len(tracks), expected)
	}
	found := false
	for i := 0; i < len(tracks); i++ {
		if tracks[i].ID == "user-track" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("user-track not found in list")
	}
}

func TestCreateTrackReturns201(t *testing.T) {
	h := newTestHandler(t)
	body := `{"id":"new-track","name":"New","width":2,"height":2,"tiles":[["straight-h","straight-h"],["straight-h","straight-h"]]}`
	w := doRequest(h, http.MethodPost, "/api/tracks", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusCreated)
	}
	var created Track
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ID != "new-track" {
		t.Fatalf("ID = %q, want %q", created.ID, "new-track")
	}
	if created.Name != "New" {
		t.Fatalf("Name = %q, want %q", created.Name, "New")
	}
	if created.CreatedAt.IsZero() {
		t.Fatal("CreatedAt is zero")
	}
}

func TestCreateTrackAutoGeneratesID(t *testing.T) {
	h := newTestHandler(t)
	body := `{"name":"No ID","width":2,"height":2,"tiles":[["straight-h","straight-h"],["straight-h","straight-h"]]}`
	w := doRequest(h, http.MethodPost, "/api/tracks", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusCreated)
	}
	var created Track
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ID == "" {
		t.Fatal("ID is empty, expected auto-generated")
	}
	if !strings.HasPrefix(created.ID, "track-") {
		t.Fatalf("ID = %q, want prefix %q", created.ID, "track-")
	}
}

func TestCreateTrackBadJSON(t *testing.T) {
	h := newTestHandler(t)
	w := doRequest(h, http.MethodPost, "/api/tracks", "{invalid")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// --- Resource routes: GET/PUT/DELETE /api/tracks/{id} ---

func TestGetTrackReturnsUserTrack(t *testing.T) {
	h := newTestHandler(t)
	body := `{"id":"get-me","name":"Get Me","width":2,"height":2,"tiles":[["straight-h","straight-h"],["straight-h","straight-h"]]}`
	doRequest(h, http.MethodPost, "/api/tracks", body)

	w := doRequest(h, http.MethodGet, "/api/tracks/get-me", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var track Track
	if err := json.Unmarshal(w.Body.Bytes(), &track); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if track.ID != "get-me" {
		t.Fatalf("ID = %q, want %q", track.ID, "get-me")
	}
}

func TestGetTrackReturnsPreset(t *testing.T) {
	h := newTestHandler(t)
	presets := Presets()
	for i := 0; i < len(presets); i++ {
		p := presets[i]
		w := doRequest(h, http.MethodGet, "/api/tracks/"+p.ID, "")
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want %d", p.ID, w.Code, http.StatusOK)
		}
		var track Track
		if err := json.Unmarshal(w.Body.Bytes(), &track); err != nil {
			t.Fatalf("GET %s: decode: %v", p.ID, err)
		}
		if track.ID != p.ID {
			t.Fatalf("GET %s: ID = %q", p.ID, track.ID)
		}
	}
}

func TestGetTrackNotFound(t *testing.T) {
	h := newTestHandler(t)
	w := doRequest(h, http.MethodGet, "/api/tracks/nonexistent", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestUpdateTrack(t *testing.T) {
	h := newTestHandler(t)
	body := `{"id":"upd","name":"Original","width":2,"height":2,"tiles":[["straight-h","straight-h"],["straight-h","straight-h"]]}`
	doRequest(h, http.MethodPost, "/api/tracks", body)

	updated := `{"name":"Updated","width":2,"height":2,"tiles":[["tree","tree"],["tree","tree"]]}`
	w := doRequest(h, http.MethodPut, "/api/tracks/upd", updated)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var track Track
	if err := json.Unmarshal(w.Body.Bytes(), &track); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if track.Name != "Updated" {
		t.Fatalf("Name = %q, want %q", track.Name, "Updated")
	}
	if track.ID != "upd" {
		t.Fatalf("ID = %q, want %q (URL id should override body)", track.ID, "upd")
	}
}

func TestUpdatePresetTrackForbidden(t *testing.T) {
	h := newTestHandler(t)
	presets := Presets()
	for i := 0; i < len(presets); i++ {
		p := presets[i]
		w := doRequest(h, http.MethodPut, "/api/tracks/"+p.ID, `{"name":"hacked"}`)
		if w.Code != http.StatusForbidden {
			t.Fatalf("PUT %s: status = %d, want %d", p.ID, w.Code, http.StatusForbidden)
		}
	}
}

func TestUpdateTrackBadJSON(t *testing.T) {
	h := newTestHandler(t)
	body := `{"id":"upd2","name":"X","width":2,"height":2,"tiles":[["straight-h","straight-h"],["straight-h","straight-h"]]}`
	doRequest(h, http.MethodPost, "/api/tracks", body)

	w := doRequest(h, http.MethodPut, "/api/tracks/upd2", "not-json")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDeleteTrack(t *testing.T) {
	h := newTestHandler(t)
	body := `{"id":"del-me","name":"Delete","width":2,"height":2,"tiles":[["straight-h","straight-h"],["straight-h","straight-h"]]}`
	doRequest(h, http.MethodPost, "/api/tracks", body)

	w := doRequest(h, http.MethodDelete, "/api/tracks/del-me", "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}

	w = doRequest(h, http.MethodGet, "/api/tracks/del-me", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("after delete: status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestDeletePresetTrackForbidden(t *testing.T) {
	h := newTestHandler(t)
	presets := Presets()
	for i := 0; i < len(presets); i++ {
		p := presets[i]
		w := doRequest(h, http.MethodDelete, "/api/tracks/"+p.ID, "")
		if w.Code != http.StatusForbidden {
			t.Fatalf("DELETE %s: status = %d, want %d", p.ID, w.Code, http.StatusForbidden)
		}
	}
}

func TestDeleteTrackNotFound(t *testing.T) {
	h := newTestHandler(t)
	w := doRequest(h, http.MethodDelete, "/api/tracks/no-such-track", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// --- Method not allowed ---

func TestCollectionMethodNotAllowed(t *testing.T) {
	h := newTestHandler(t)
	methods := []string{http.MethodPut, http.MethodDelete, http.MethodPatch}
	for i := 0; i < len(methods); i++ {
		w := doRequest(h, methods[i], "/api/tracks", "")
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s /api/tracks: status = %d, want %d", methods[i], w.Code, http.StatusMethodNotAllowed)
		}
	}
}

func TestResourceMethodNotAllowed(t *testing.T) {
	h := newTestHandler(t)
	methods := []string{http.MethodPost, http.MethodPatch}
	for i := 0; i < len(methods); i++ {
		w := doRequest(h, methods[i], "/api/tracks/some-id", "")
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s /api/tracks/some-id: status = %d, want %d", methods[i], w.Code, http.StatusMethodNotAllowed)
		}
	}
}

// --- Track validation ---

func TestCreateTrackValidationRejects(t *testing.T) {
	longName := strings.Repeat("a", 129)
	cases := []struct {
		name string
		body string
	}{
		{"empty name", `{"name":"","width":2,"height":2,"tiles":[["",""],["",""]]}`},
		{"long name", `{"name":"` + longName + `","width":2,"height":2,"tiles":[["",""],["",""]]}`},
		{"zero width", `{"name":"Zero","width":0,"height":2,"tiles":[]}`},
		{"oversized width", `{"name":"Big","width":100,"height":2,"tiles":[[],[]]}`},
		{"oversized height", `{"name":"Tall","width":2,"height":100,"tiles":[[],[]]}`},
		{"excessive tile count", `{"name":"Huge","width":65,"height":64,"tiles":[]}`},
		{"mismatched rows", `{"name":"Bad","width":2,"height":2,"tiles":[["",""],["",""],["",""]]}`},
		{"mismatched columns", `{"name":"Bad","width":2,"height":2,"tiles":[["","",""],["",""]]}`},
		{"invalid tile type", `{"name":"Bad","width":2,"height":2,"tiles":[["straight-h","bogus"],["",""]]}`},
	}
	for i := 0; i < len(cases); i++ {
		tc := cases[i]
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(t)
			w := doRequest(h, http.MethodPost, "/api/tracks", tc.body)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestCreateTrackAcceptsAllValidTileTypes(t *testing.T) {
	h := newTestHandler(t)
	// All 14 non-empty tile types + 1 empty = 15 cells in a 5x3 grid.
	body := `{"name":"All Tiles","width":5,"height":3,"tiles":[` +
		`["straight-h","straight-v","curve-ne","curve-nw","curve-se"],` +
		`["curve-sw","chicane","pit-entry","pit-exit","grandstand"],` +
		`["tree","barrier","start-line","finish-line",""]]}`
	w := doRequest(h, http.MethodPost, "/api/tracks", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusCreated, w.Body.String())
	}
}

func TestUpdateTrackValidation(t *testing.T) {
	h := newTestHandler(t)
	create := `{"id":"val-upd","name":"Valid","width":2,"height":2,"tiles":[["",""],["",""]]}`
	w := doRequest(h, http.MethodPost, "/api/tracks", create)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want %d", w.Code, http.StatusCreated)
	}

	update := `{"name":"Updated","width":2,"height":2,"tiles":[["nope",""],["",""]]}`
	w = doRequest(h, http.MethodPut, "/api/tracks/val-upd", update)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PUT status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// --- Response content type ---

func TestResponseContentTypeJSON(t *testing.T) {
	h := newTestHandler(t)
	w := doRequest(h, http.MethodGet, "/api/tracks", "")
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
}
