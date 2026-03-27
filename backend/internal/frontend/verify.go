//go:build embed

package frontend

import (
	"fmt"
	"io/fs"
)

// Verify checks that all embedded frontend files match the SHA256
// checksums recorded in .build-manifest at build time.
func Verify() error {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("frontend integrity: %w", err)
	}
	return verifyFS(sub)
}
