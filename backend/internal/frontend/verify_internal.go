package frontend

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"strings"
)

// verifyFS checks that all files in fsys match the SHA256 checksums
// recorded in .build-manifest. Returns nil if all files verify.
func verifyFS(fsys fs.FS) error {
	manifest, err := fs.ReadFile(fsys, ".build-manifest")
	if err != nil {
		return fmt.Errorf("manifest not found: %w", err)
	}

	expected := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(manifest)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// sha256sum format: "hash  ./path/to/file" (two-space separator)
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			return fmt.Errorf("malformed manifest line: %s", line)
		}
		path := strings.TrimPrefix(parts[1], "./")
		expected[path] = parts[0]
	}

	if len(expected) == 0 {
		return fmt.Errorf("manifest is empty")
	}

	err = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || path == ".build-manifest" {
			return nil
		}

		expectedHash, ok := expected[path]
		if !ok {
			return fmt.Errorf("file %q not listed in manifest", path)
		}

		f, openErr := fsys.Open(path)
		if openErr != nil {
			return fmt.Errorf("cannot open %q: %w", path, openErr)
		}
		defer func() { _ = f.Close() }()

		h := sha256.New()
		if _, copyErr := io.Copy(h, f); copyErr != nil {
			return fmt.Errorf("cannot hash %q: %w", path, copyErr)
		}

		actual := hex.EncodeToString(h.Sum(nil))
		if actual != expectedHash {
			return fmt.Errorf("integrity mismatch: %s (expected %s, got %s)", path, expectedHash, actual)
		}

		delete(expected, path)
		return nil
	})
	if err != nil {
		return err
	}

	// Files in manifest but not found on disk.
	for name := range expected {
		return fmt.Errorf("%q listed in manifest but not found", name)
	}

	return nil
}
