package review

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"quickcull/internal/domain"
)

func newStateWithFiles(t *testing.T, files []string) (*State, string) {
	root := t.TempDir()
	for _, f := range files {
		abs := filepath.Join(root, f)
		_ = os.MkdirAll(filepath.Dir(abs), 0755)
		_ = os.WriteFile(abs, []byte("fake"), 0644)
	}
	s := NewState(root, files)
	s.cacheDir = t.TempDir()
	if err := os.MkdirAll(s.cacheDir, 0o755); err != nil {
		t.Fatalf("create cache dir failed: %v", err)
	}
	return s, root
}

func TestStateResolveIndex(t *testing.T) {
	s, _ := newStateWithFiles(t, []string{"a.jpg", "b.jpg", "c.jpg"})

	// Correct match
	if idx := s.ResolveIndex(1, "b.jpg"); idx != 1 {
		t.Errorf("ResolveIndex(1, b.jpg) = %d; want 1", idx)
	}

	// Index mismatch but file exists elsewhere
	if idx := s.ResolveIndex(0, "b.jpg"); idx != 1 {
		t.Errorf("ResolveIndex(0, b.jpg) = %d; want 1", idx)
	}

	// Nonexistent file
	if idx := s.ResolveIndex(1, "nonexistent.jpg"); idx != -1 {
		t.Errorf("ResolveIndex(1, nonexistent.jpg) = %d; want -1", idx)
	}

	// Empty path (trust index)
	if idx := s.ResolveIndex(2, ""); idx != 2 {
		t.Errorf("ResolveIndex(2, '') = %d; want 2", idx)
	}
}

func TestStateBounds(t *testing.T) {
	s, _ := newStateWithFiles(t, []string{"a.jpg"})

	if _, err := s.Get(99); err != domain.ErrIndexOutOfBounds {
		t.Fatalf("expected ErrIndexOutOfBounds from Get(99), got %v", err)
	}
	if _, err := s.AbsPath(99); err != domain.ErrIndexOutOfBounds {
		t.Fatalf("expected ErrIndexOutOfBounds from AbsPath(99), got %v", err)
	}
	if _, err := s.FileSize(99); err != domain.ErrIndexOutOfBounds {
		t.Fatalf("expected ErrIndexOutOfBounds from FileSize(99), got %v", err)
	}
	if _, err := s.Trash(-1); err != domain.ErrIndexOutOfBounds {
		t.Fatalf("expected ErrIndexOutOfBounds from Trash(-1), got %v", err)
	}
}

func TestRestoreFromTrashRejectsEscapingPaths(t *testing.T) {
	s, root := newStateWithFiles(t, []string{"a.jpg"})
	if _, err := s.Trash(0); err != nil {
		t.Fatalf("trash: %v", err)
	}

	restored, err := s.RestoreFromTrash([]string{"../a.jpg", filepath.Join(root, ".trash", "a.jpg")})
	if err != nil {
		t.Fatalf("restore invalid paths: %v", err)
	}
	if len(restored) != 0 || s.Len() != 0 {
		t.Fatalf("invalid paths must not restore files: restored=%v len=%d", restored, s.Len())
	}
	if err := s.RestoreFromTrashAt(filepath.Join(root, ".trash", "a.jpg"), 0); err != domain.ErrInvalidPath {
		t.Fatalf("RestoreFromTrashAt absolute path error = %v, want ErrInvalidPath", err)
	}
	if _, err := os.Stat(filepath.Join(root, domain.DirTrash, "a.jpg")); err != nil {
		t.Fatalf("invalid restore must retain trash source: %v", err)
	}
}

func TestTrashMultiplePathsDetailedReportsOnlySuccessfulUniqueMoves(t *testing.T) {
	s, root := newStateWithFiles(t, []string{"a.jpg", "b.jpg", "c.jpg"})

	count, moved, err := s.TrashMultiplePathsDetailed([]string{"a.jpg", "a.jpg", "missing.jpg", "c.jpg"})
	if err != nil {
		t.Fatalf("trash batch: %v", err)
	}
	if count != 1 || len(moved) != 2 || moved[0] != "a.jpg" || moved[1] != "c.jpg" {
		t.Fatalf("count=%d moved=%v, want count=1 and [a.jpg c.jpg]", count, moved)
	}
	if s.FindIndex("a.jpg") >= 0 || s.FindIndex("c.jpg") >= 0 || s.FindIndex("b.jpg") != 0 {
		t.Fatalf("unexpected active index after batch: a=%d b=%d c=%d", s.FindIndex("a.jpg"), s.FindIndex("b.jpg"), s.FindIndex("c.jpg"))
	}
	for _, rel := range moved {
		if _, err := os.Stat(filepath.Join(root, domain.DirTrash, rel)); err != nil {
			t.Fatalf("moved file %s missing from trash: %v", rel, err)
		}
	}
}

func TestStateFoldersUpdateVerifyTrashAndPosition(t *testing.T) {
	files := []string{
		"a.jpg",
		"sub/b.jpg",
		"sub/c.jpg",
		"other/d.jpg",
	}
	s, root := newStateWithFiles(t, files)

	// Test Folders()
	folders := s.Folders()
	if len(folders) != 3 { // root (.), sub, other
		t.Fatalf("expected 3 folders, got %d", len(folders))
	}

	// Test UpdateFiles (Cleanup)
	newFiles := []string{"a.jpg", "sub/b.jpg"}

	s.UpdateFiles(newFiles)
	if s.Len() != 2 {
		t.Fatalf("expected length 2 after UpdateFiles, got %d", s.Len())
	}

	// Test corrupted position file (migration path)
	if err := os.MkdirAll(s.cacheDir, 0755); err != nil {
		t.Fatalf("create cache dir failed: %v", err)
	}
	posPath := filepath.Join(s.cacheDir, positionFile)
	if err := os.WriteFile(posPath, []byte("not-a-number"), 0o600); err != nil {
		t.Fatalf("write position failed: %v", err)
	}
	if got := s.LoadPosition(); got != 0 {
		t.Fatalf("expected 0 for corrupted position file, got %d", got)
	}

	// Rebuild with original files for trash verification.
	s = NewState(root, files)
	if _, err := s.Trash(0); err != nil {
		t.Fatalf("Trash failed: %v", err)
	}
	if s.Len() != 3 {
		t.Fatalf("expected length 3 after trashing 1 file, got %d", s.Len())
	}
	if s.TrashedCount() != 1 {
		t.Fatalf("expected trashed count 1, got %d", s.TrashedCount())
	}

	// Verify ListTrash
	trashItems, err := s.ListTrash()
	if err != nil || len(trashItems) != 1 {
		t.Fatalf("ListTrash failed: %v, items=%v", err, trashItems)
	}

	// Verify RestoreFromTrash
	restored, err := s.RestoreFromTrash([]string{"a.jpg"})
	if err != nil || len(restored) != 1 {
		t.Fatalf("RestoreFromTrash failed: %v", err)
	}
	if s.Len() != 4 {
		t.Fatalf("expected length 4 after restore, got %d", s.Len())
	}
	if s.TrashedCount() != 0 {
		t.Fatalf("expected trashed count 0, got %d", s.TrashedCount())
	}
}

func TestStateSortByNameAndDate(t *testing.T) {
	// Name sort
	s, _ := newStateWithFiles(t, []string{"z.jpg", "a.jpg", "m.jpg"})
	s.SortByName()
	if f, _ := s.Get(0); f != "a.jpg" {
		t.Errorf("expected a.jpg at index 0 after SortByName, got %s", f)
	}

	// Date sort (Mocked cache)
	root := t.TempDir()
	files := []string{"old.jpg", "new.jpg"}
	oldAbs := filepath.Join(root, "old.jpg")
	newAbs := filepath.Join(root, "new.jpg")
	_ = os.WriteFile(oldAbs, []byte("old"), 0644)
	_ = os.WriteFile(newAbs, []byte("new"), 0644)

	// Set file times
	now := time.Now()
	_ = os.Chtimes(oldAbs, now.Add(-1*time.Hour), now.Add(-1*time.Hour))
	_ = os.Chtimes(newAbs, now, now)

	s = NewState(root, files)
	mc := NewMediaCache() // empty cache
	s.SortByDate(mc)

	if f, _ := s.Get(0); f != "old.jpg" {
		t.Errorf("expected old.jpg at index 0 after SortByDate, got %s", f)
	}
}
