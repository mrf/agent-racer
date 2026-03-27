package frontend

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
)

func sha256hex(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

func buildManifest(files map[string]string) string {
	var lines []string
	for path, content := range files {
		lines = append(lines, fmt.Sprintf("%s  ./%s", sha256hex(content), path))
	}
	return strings.Join(lines, "\n") + "\n"
}

func TestVerifyFS_Valid(t *testing.T) {
	files := map[string]string{
		"index.html":     "<html>hello</html>",
		"assets/main.js": "console.log('hi')",
		"styles.css":     "body { color: red }",
	}

	fsys := fstest.MapFS{
		".build-manifest": &fstest.MapFile{Data: []byte(buildManifest(files))},
	}
	for path, content := range files {
		fsys[path] = &fstest.MapFile{Data: []byte(content)}
	}

	if err := verifyFS(fsys); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestVerifyFS_TamperedFile(t *testing.T) {
	files := map[string]string{
		"index.html": "<html>hello</html>",
		"app.js":     "original content",
	}

	fsys := fstest.MapFS{
		".build-manifest": &fstest.MapFile{Data: []byte(buildManifest(files))},
		"index.html":      &fstest.MapFile{Data: []byte("<html>hello</html>")},
		"app.js":          &fstest.MapFile{Data: []byte("TAMPERED content")},
	}

	err := verifyFS(fsys)
	if err == nil {
		t.Fatal("expected integrity mismatch error")
	}
	if !strings.Contains(err.Error(), "integrity mismatch") {
		t.Fatalf("expected integrity mismatch, got: %v", err)
	}
	if !strings.Contains(err.Error(), "app.js") {
		t.Fatalf("expected error to mention app.js, got: %v", err)
	}
}

func TestVerifyFS_ExtraFile(t *testing.T) {
	files := map[string]string{
		"index.html": "<html>hello</html>",
	}

	fsys := fstest.MapFS{
		".build-manifest": &fstest.MapFile{Data: []byte(buildManifest(files))},
		"index.html":      &fstest.MapFile{Data: []byte("<html>hello</html>")},
		"injected.js":     &fstest.MapFile{Data: []byte("evil()")},
	}

	err := verifyFS(fsys)
	if err == nil {
		t.Fatal("expected error for extra file")
	}
	if !strings.Contains(err.Error(), "not listed in manifest") {
		t.Fatalf("expected 'not listed in manifest', got: %v", err)
	}
}

func TestVerifyFS_MissingFile(t *testing.T) {
	files := map[string]string{
		"index.html": "<html>hello</html>",
		"missing.js": "should exist",
	}

	fsys := fstest.MapFS{
		".build-manifest": &fstest.MapFile{Data: []byte(buildManifest(files))},
		"index.html":      &fstest.MapFile{Data: []byte("<html>hello</html>")},
		// missing.js intentionally absent
	}

	err := verifyFS(fsys)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "missing.js") {
		t.Fatalf("expected error to mention missing.js, got: %v", err)
	}
}

func TestVerifyFS_NoManifest(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html></html>")},
	}

	err := verifyFS(fsys)
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
	if !strings.Contains(err.Error(), "manifest not found") {
		t.Fatalf("expected 'manifest not found', got: %v", err)
	}
}

func TestVerifyFS_EmptyManifest(t *testing.T) {
	fsys := fstest.MapFS{
		".build-manifest": &fstest.MapFile{Data: []byte("")},
		"index.html":      &fstest.MapFile{Data: []byte("<html></html>")},
	}

	err := verifyFS(fsys)
	if err == nil {
		t.Fatal("expected error for empty manifest")
	}
	if !strings.Contains(err.Error(), "manifest is empty") {
		t.Fatalf("expected 'manifest is empty', got: %v", err)
	}
}

func TestVerifyFS_MalformedManifest(t *testing.T) {
	fsys := fstest.MapFS{
		".build-manifest": &fstest.MapFile{Data: []byte("not-a-valid-manifest-line\n")},
	}

	err := verifyFS(fsys)
	if err == nil {
		t.Fatal("expected error for malformed manifest")
	}
	if !strings.Contains(err.Error(), "malformed manifest") {
		t.Fatalf("expected 'malformed manifest', got: %v", err)
	}
}
