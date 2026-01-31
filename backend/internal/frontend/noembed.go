//go:build !embed

package frontend

import "net/http"

func Handler() http.Handler {
	return nil
}
