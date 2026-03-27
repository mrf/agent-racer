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
	body := `{"id":"user-track","name":"My Track","width":4,"height":4,"tiles":[["road","road","road","road"],["road","road","road","road"],["road","road","road","road"],["road","road","road","road"]]}`
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
	body := `{"id":"new-track","name":"New","width":2,"height":2,"tiles":[["road","road"],["road","road"]]}`
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
	body := `{"name":"No ID","width":2,"height":2,"tiles":[["road","road"],["road","road"]]}`
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
	body := `{"id":"get-me","name":"Get Me","width":2,"height":2,"tiles":[["road","road"],["road","road"]]}`
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
	body := `{"id":"upd","name":"Original","width":2,"height":2,"tiles":[["road","road"],["road","road"]]}`
	doRequest(h, http.MethodPost, "/api/tracks", body)

	updated := `{"name":"Updated","width":2,"height":2,"tiles":[["sand","sand"],["sand","sand"]]}`
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
	body := `{"id":"upd2","name":"X","width":2,"height":2,"tiles":[["road","road"],["road","road"]]}`
	doRequest(h, http.MethodPost, "/api/tracks", body)

	w := doRequest(h, http.MethodPut, "/api/tracks/upd2", "not-json")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDeleteTrack(t *testing.T) {
	h := newTestHandler(t)
	body := `{"id":"del-me","name":"Delete","width":2,"height":2,"tiles":[["road","road"],["road","road"]]}`
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

// --- Response content type ---

func TestResponseContentTypeJSON(t *testing.T) {
	h := newTestHandler(t)
	w := doRequest(h, http.MethodGet, "/api/tracks", "")
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
}
