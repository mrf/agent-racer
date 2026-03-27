package replay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fixedTime is a stable reference point for test file timestamps.
var fixedTime = func() time.Time {
	t, _ := time.Parse(time.RFC3339, "2026-01-15T12:00:00Z")
	return t
}()

// serveReplay creates a Handler with the given dir and authFn, registers routes,
// and executes a single HTTP request, returning the recorded response.
func serveReplay(dir string, authFn func(*http.Request) bool, method, path string) *httptest.ResponseRecorder {
	h := NewHandler(dir, authFn)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// denyAll is an authFn that rejects every request.
func denyAll(_ *http.Request) bool { return false }

func TestHandleList_Unauthorized(t *testing.T) {
	rec := serveReplay(t.TempDir(), denyAll, http.MethodGet, "/api/replays")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleList_NilAuthAllowsAccess(t *testing.T) {
	rec := serveReplay(t.TempDir(), nil, http.MethodGet, "/api/replays")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleList_MissingDirReturnsEmptyArray(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent")
	rec := serveReplay(dir, nil, http.MethodGet, "/api/replays")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var result []ReplayInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d replays, want 0", len(result))
	}
}

func TestHandleList_EmptyDirReturnsEmptyArray(t *testing.T) {
	rec := serveReplay(t.TempDir(), nil, http.MethodGet, "/api/replays")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var result []ReplayInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d replays, want 0", len(result))
	}
}

func TestHandleList_ReturnsSortedNewestFirst(t *testing.T) {
	dir := t.TempDir()

	files := []struct {
		name string
		age  int // days old
	}{
		{"older.jsonl", 3},
		{"newest.jsonl", 0},
		{"middle.jsonl", 1},
	}
	for _, f := range files {
		path := filepath.Join(dir, f.name)
		if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}
		if f.age > 0 {
			mtime := fixedTime.AddDate(0, 0, -f.age)
			if err := os.Chtimes(path, mtime, mtime); err != nil {
				t.Fatal(err)
			}
		}
	}

	rec := serveReplay(dir, nil, http.MethodGet, "/api/replays")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var result []ReplayInfo
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wantOrder := []string{"newest", "middle", "older"}
	if len(result) != len(wantOrder) {
		t.Fatalf("got %d replays, want %d", len(result), len(wantOrder))
	}
	for i, want := range wantOrder {
		if result[i].ID != want {
			t.Fatalf("result[%d].ID = %q, want %q", i, result[i].ID, want)
		}
	}
}

func TestHandleList_IgnoresNonJSONL(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "replay.jsonl"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}

	rec := serveReplay(dir, nil, http.MethodGet, "/api/replays")

	var result []ReplayInfo
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d replays, want 1 (only .jsonl)", len(result))
	}
	if result[0].ID != "replay" {
		t.Fatalf("ID = %q, want %q", result[0].ID, "replay")
	}
}

func TestHandleGet_Unauthorized(t *testing.T) {
	rec := serveReplay(t.TempDir(), denyAll, http.MethodGet, "/api/replays/test")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleGet_InvalidIDs(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{"empty", "/api/replays/"},
		{"slash", "/api/replays/a/b"},
		{"backslash", "/api/replays/a\\b"},
		{"dotdot", "/api/replays/..%2F..%2Fetc"},
	}

	dir := t.TempDir()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveReplay(dir, nil, http.MethodGet, tc.path)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d for path %q", rec.Code, http.StatusBadRequest, tc.path)
			}
		})
	}
}

func TestHandleGet_NotFound(t *testing.T) {
	rec := serveReplay(t.TempDir(), nil, http.MethodGet, "/api/replays/nonexistent")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleGet_StreamsFileContent(t *testing.T) {
	dir := t.TempDir()
	content := `{"t":"2026-01-15T12:00:00Z","s":[]}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "demo.jsonl"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	rec := serveReplay(dir, nil, http.MethodGet, "/api/replays/demo")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/x-ndjson" {
		t.Fatalf("Content-Type = %q, want application/x-ndjson", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", cc)
	}
	if rec.Body.String() != content {
		t.Fatalf("body = %q, want %q", rec.Body.String(), content)
	}
}

func TestHandleGet_NilAuthAllowsAccess(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok.jsonl"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	rec := serveReplay(dir, nil, http.MethodGet, "/api/replays/ok")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
