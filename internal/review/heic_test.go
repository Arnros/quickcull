package review

import "testing"

func TestHeicSupported(t *testing.T) {
	// This test just ensures the function exists and returns a boolean.
	// The exact value depends on the build tags (cgo or not).
	_ = HeicSupported()
}
