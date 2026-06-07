package review

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzExtractEXIFInfo(f *testing.F) {
	// Seed with some valid extensions to trigger different paths
	f.Add([]byte("fake data"), ".jpg")
	f.Add([]byte("fake data"), ".png")
	f.Add([]byte("fake data"), ".heic")
	f.Add([]byte("fake data"), ".arw")

	f.Fuzz(func(t *testing.T, data []byte, ext string) {
		// Create a temporary file with the fuzzed data and extension
		tmpDir := t.TempDir()
		// Clean extension to avoid path traversal or invalid filenames
		cleanExt := filepath.Clean("/" + ext)
		if cleanExt == "." || cleanExt == "/" {
			cleanExt = ".jpg"
		}
		
		path := filepath.Join(tmpDir, "fuzz_target"+cleanExt)
		if err := os.WriteFile(path, data, 0644); err != nil {
			return
		}

		// Run the extraction
		// We don't care about the result, only that it doesn't panic
		_ = ExtractEXIFInfo(path)
		_ = GetOrientation(path)
	})
}
