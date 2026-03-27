package replay

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleGet_SetsContentLength(t *testing.T) {
	dir := t.TempDir()
	data := `{"t":"2024-01-01T00:00:00Z","s":[]}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "test.jsonl"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(dir, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/replays/test", nil)
	w := httptest.NewRecorder()

	h.handleGet(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	cl := w.Header().Get("Content-Length")
	if cl == "" {
		t.Fatal("expected Content-Length header to be set")
	}
	want := fmt.Sprintf("%d", len(data))
	if cl != want {
		t.Fatalf("Content-Length = %q, want %q", cl, want)
	}
	if w.Body.String() != data {
		t.Fatalf("body = %q, want %q", w.Body.String(), data)
	}
}

func TestHandleGet_RejectsTooLargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.jsonl")

	// Create a sparse file that exceeds the limit without writing actual data.
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxReplayResponseBytes + 1); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	_ = f.Close()

	h := NewHandler(dir, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/replays/huge", nil)
	w := httptest.NewRecorder()

	h.handleGet(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
	if !strings.Contains(w.Body.String(), "replay too large") {
		t.Fatalf("body = %q, want it to contain 'replay too large'", w.Body.String())
	}
}

func TestHandleGet_NotFound(t *testing.T) {
	dir := t.TempDir()
	h := NewHandler(dir, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/replays/missing", nil)
	w := httptest.NewRecorder()

	h.handleGet(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleGet_InvalidID(t *testing.T) {
	dir := t.TempDir()
	h := NewHandler(dir, nil)

	for _, id := range []string{"", "../etc/passwd", "foo/bar", "foo\\bar"} {
		req := httptest.NewRequest(http.MethodGet, "/api/replays/"+id, nil)
		w := httptest.NewRecorder()
		h.handleGet(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("id=%q: status = %d, want %d", id, w.Code, http.StatusBadRequest)
		}
	}
}
