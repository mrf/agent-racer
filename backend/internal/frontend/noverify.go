//go:build !embed

package frontend

// Verify is a no-op when the frontend is not embedded.
func Verify() error {
	return nil
}
