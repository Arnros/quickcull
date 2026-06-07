//go:build !cgo

package review

import "fmt"

// HeicSupported returns false when CGo is not available.
func HeicSupported() bool {
	return false
}

// ConvertHEIC returns an error: HEIC support not available without CGo
func ConvertHEIC(src, fingerprint, cacheDir string, orientation int) (string, error) {
	return "", fmt.Errorf("HEIC conversion not available (compiled without CGo)")
}
