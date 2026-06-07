package review

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestFindIndexCaseSensitivity verifies that FindIndex performs exact (case-sensitive)
// matching and returns -1 for paths that differ only in case or do not exist.
func TestFindIndexCaseSensitivity(t *testing.T) {
	root := t.TempDir()
	files := []string{"photos/a.jpg", "photos/B.jpg", "photos/c.jpg"}
	for _, f := range files {
		abs := filepath.Join(root, f)
		_ = os.MkdirAll(filepath.Dir(abs), 0755)
		_ = os.WriteFile(abs, []byte("fake"), 0644)
	}
	s := NewState(root, files)

	// Exact match for first file.
	if got := s.FindIndex("photos/a.jpg"); got != 0 {
		t.Errorf("FindIndex(photos/a.jpg) = %d; want 0", got)
	}

	// Exact match for second file (uppercase B).
	if got := s.FindIndex("photos/B.jpg"); got != 1 {
		t.Errorf("FindIndex(photos/B.jpg) = %d; want 1", got)
	}

	// Lowercase variant of the second file — FindIndex is case-sensitive, so -1.
	if got := s.FindIndex("photos/b.jpg"); got != -1 {
		t.Errorf("FindIndex(photos/b.jpg) = %d; want -1 (case-sensitive)", got)
	}

	// Non-existent file.
	if got := s.FindIndex("photos/missing.jpg"); got != -1 {
		t.Errorf("FindIndex(photos/missing.jpg) = %d; want -1", got)
	}
}

// TestSortByDateFallsBackToMtimeWhenNoExifDate verifies that SortByDate sorts files
// by mtime when the cache contains no EXIF date metadata.
func TestSortByDateFallsBackToMtimeWhenNoExifDate(t *testing.T) {
	root := t.TempDir()

	// Create three files with distinct mtimes (oldest first in expected order).
	type fileSpec struct {
		name  string
		mtime time.Time
	}
	base := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	specs := []fileSpec{
		{"middle.jpg", base},
		{"newest.jpg", base.Add(2 * time.Hour)},
		{"oldest.jpg", base.Add(-2 * time.Hour)},
	}

	fileNames := make([]string, len(specs))
	for i, spec := range specs {
		fileNames[i] = spec.name
		abs := filepath.Join(root, spec.name)
		_ = os.WriteFile(abs, []byte("fake"), 0644)
		_ = os.Chtimes(abs, spec.mtime, spec.mtime)
	}

	s := NewState(root, fileNames)
	mc := NewMediaCache() // empty cache — no EXIF dates
	s.SortByDate(mc)

	// Expected order: oldest → middle → newest
	expected := []string{"oldest.jpg", "middle.jpg", "newest.jpg"}
	for i, want := range expected {
		got, err := s.Get(i)
		if err != nil {
			t.Fatalf("Get(%d) error: %v", i, err)
		}
		if got != want {
			t.Errorf("index %d: got %q, want %q", i, got, want)
		}
	}
}

// TestSortByDatePreferExifOverMtime verifies that SortByDate uses the EXIF date
// from the cache when available, ignoring the file's mtime.
func TestSortByDatePreferExifOverMtime(t *testing.T) {
	root := t.TempDir()

	// Both files have the same (recent) mtime so mtime alone cannot distinguish them.
	recentMtime := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	fileNames := []string{"a.jpg", "b.jpg"}
	for _, name := range fileNames {
		abs := filepath.Join(root, name)
		_ = os.WriteFile(abs, []byte("fake"), 0644)
		_ = os.Chtimes(abs, recentMtime, recentMtime)
	}

	s := NewState(root, fileNames)
	mc := NewMediaCache()

	// Inject EXIF dates directly into the in-memory cache:
	//   a.jpg → 2020-01-01  (should appear second)
	//   b.jpg → 2019-06-15  (should appear first, older date)
	absA := filepath.Join(root, "a.jpg")
	absB := filepath.Join(root, "b.jpg")
	mc.mu.Lock()
	mc.exifCache[absA] = &EXIFInfo{Date: "2020:01:01 10:00:00", Width: 1, Height: 1}
	mc.exifCache[absB] = &EXIFInfo{Date: "2019:06:15 08:00:00", Width: 1, Height: 1}
	mc.mu.Unlock()

	s.SortByDate(mc)

	// Expected order: b.jpg (2019) before a.jpg (2020).
	if got, _ := s.Get(0); got != "b.jpg" {
		t.Errorf("index 0: got %q, want %q", got, "b.jpg")
	}
	if got, _ := s.Get(1); got != "a.jpg" {
		t.Errorf("index 1: got %q, want %q", got, "a.jpg")
	}
}

func TestState(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "quickcull-state-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	files := []string{"a.jpg", "b.jpg", "c.jpg"}
	for _, f := range files {
		os.WriteFile(filepath.Join(tempDir, f), []byte("test"), 0644)
	}

	state := NewState(tempDir, files)

	t.Run("initial length", func(t *testing.T) {
		if state.Len() != 3 {
			t.Errorf("Expected 3 files, got %d", state.Len())
		}
	})

	t.Run("trash", func(t *testing.T) {
		// Trash b.jpg (index 1)
		newTotal, err := state.Trash(1)
		if err != nil {
			t.Fatalf("Trash failed: %v", err)
		}
		if newTotal != 2 {
			t.Errorf("Expected 2 files after trash, got %d", newTotal)
		}

		// Verify file moved to .trash
		if _, err := os.Stat(filepath.Join(tempDir, ".trash", "b.jpg")); err != nil {
			t.Errorf("File b.jpg not found in .trash")
		}
	})

	t.Run("trash multiple", func(t *testing.T) {
		tempDir3, _ := os.MkdirTemp("", "quickcull-state-test3-*")
		defer os.RemoveAll(tempDir3)
		files3 := []string{"x.jpg", "y.jpg", "z.jpg"}
		for _, f := range files3 {
			os.WriteFile(filepath.Join(tempDir3, f), []byte("test"), 0644)
		}
		s3 := NewState(tempDir3, files3)

		// Trash paths x.jpg and z.jpg
		newTotal, err := s3.TrashMultiplePaths([]string{"x.jpg", "z.jpg"})
		if err != nil {
			t.Fatalf("TrashMultiplePaths failed: %v", err)
		}

		if newTotal != 1 {
			t.Errorf("Expected 1 file remaining, got %d", newTotal)
		}

		// Remaining file should be y.jpg.
		// Since we deleted index 0 and 2, and previously the file y.jpg was at index 1,
		// it should now be at index 0.

		if f, err := s3.Get(0); err != nil {
			t.Errorf("Failed to get file at index 0: %v", err)
		} else if f != "y.jpg" {
			t.Errorf("Expected remaining file to be y.jpg, got %s", f)
		}
	})
}
