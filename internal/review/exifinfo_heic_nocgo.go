//go:build !cgo

package review

// extractEXIFInfoFromHEIC: HEIC support not available without CGo
func extractEXIFInfoFromHEIC(_ string, _ *EXIFInfo) {}

// extractHEICOrientation always returns 1 (standard) without CGo
func extractHEICOrientation(_ string) int { return 1 }
