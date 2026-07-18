package review

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPurgeDerivedCacheFilesPreservesKeepSetAndPlaceholder(t *testing.T) {
	cacheDir := t.TempDir()
	thumbDir := filepath.Join(cacheDir, thumbsDirName)
	if err := os.MkdirAll(thumbDir, 0o700); err != nil {
		t.Fatalf("create thumbnail directory: %v", err)
	}
	keepThumb := filepath.Join(thumbDir, "keep.jpg")
	staleThumb := filepath.Join(thumbDir, "stale.jpg")
	placeholder := filepath.Join(thumbDir, placeholderError)
	keepConverted := filepath.Join(cacheDir, "keep.jpg")
	staleConverted := filepath.Join(cacheDir, "stale.jpg")
	stateFile := filepath.Join(cacheDir, "cache.db")
	for _, path := range []string{keepThumb, staleThumb, placeholder, keepConverted, staleConverted, stateFile} {
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	srv := NewServer()
	srv.cacheDir = cacheDir
	removed := srv.PurgeDerivedCacheFiles(map[string]struct{}{
		keepThumb:     {},
		keepConverted: {},
	})
	if removed != 2 {
		t.Fatalf("removed files = %d, want 2", removed)
	}
	for _, path := range []string{keepThumb, placeholder, keepConverted, stateFile} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("preserved file %s is missing: %v", path, err)
		}
	}
	for _, path := range []string{staleThumb, staleConverted} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("stale file %s still exists: %v", path, err)
		}
	}
}

func TestRemoveFilesCountsOnlySuccessfulRemovals(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "existing.jpg")
	missing := filepath.Join(root, "missing.jpg")
	if err := os.WriteFile(existing, []byte("x"), 0o600); err != nil {
		t.Fatalf("write existing file: %v", err)
	}

	if got := removeFiles([]string{existing, missing}); got != 1 {
		t.Fatalf("removeFiles count = %d, want 1", got)
	}
}
