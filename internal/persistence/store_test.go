package persistence

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNewMetadataStoreCreatesPrivateDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}
	privateDir := filepath.Join(t.TempDir(), "private", "metadata")
	store, err := NewMetadataStore(filepath.Join(privateDir, "state.db"))
	if err != nil {
		t.Fatalf("create metadata store: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close metadata store: %v", err)
	}

	info, err := os.Stat(privateDir)
	if err != nil {
		t.Fatalf("stat metadata directory: %v", err)
	}
	if got := info.Mode().Perm(); got&0o077 != 0 {
		t.Fatalf("metadata directory permissions = %04o, group/other access must be zero", got)
	}
}

func TestMetadataStore(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_metadata.db")

	store, err := NewMetadataStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	folderHash := "test-folder-1"
	relPath := "photo1.jpg"
	meta := PhotoMetadata{
		IsStarred: true,
		Label:     3,
		Rotation:  90,
		IsTrashed: false,
	}

	// Test Save
	err = store.SavePhotoMetadata(folderHash, relPath, meta)
	if err != nil {
		t.Errorf("failed to save metadata: %v", err)
	}

	// Test Load Single (via Folder Metadata)
	folderMeta, err := store.LoadFolderMetadata(folderHash)
	if err != nil {
		t.Errorf("failed to load folder metadata: %v", err)
	}
	loaded, found := folderMeta[relPath]
	if !found {
		t.Errorf("metadata not found")
	}
	if loaded != meta {
		t.Errorf("loaded metadata mismatch: got %+v, want %+v", loaded, meta)
	}

	// Test Overwrite
	meta.Label = 5
	_ = store.SavePhotoMetadata(folderHash, relPath, meta)
	folderMeta2, _ := store.LoadFolderMetadata(folderHash)
	if folderMeta2[relPath].Label != 5 {
		t.Errorf("expected label 5, got %d", folderMeta2[relPath].Label)
	}
}

func TestRemovePhotoMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewMetadataStore(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer store.Close()

	folderID := "folder-remove-test"
	relPath := "photo.jpg"

	if err := store.SavePhotoMetadata(folderID, relPath, PhotoMetadata{IsStarred: true, Label: 2}); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Verify it's present before removal.
	m, err := store.LoadFolderMetadata(folderID)
	if err != nil {
		t.Fatalf("load before remove: %v", err)
	}
	if _, ok := m[relPath]; !ok {
		t.Fatal("metadata not found before removal")
	}

	if err := store.RemovePhotoMetadata(folderID, relPath); err != nil {
		t.Fatalf("remove: %v", err)
	}

	// Entry must be gone.
	m2, err := store.LoadFolderMetadata(folderID)
	if err != nil {
		t.Fatalf("load after remove: %v", err)
	}
	if _, ok := m2[relPath]; ok {
		t.Fatal("metadata still present after removal")
	}
}

func TestRemovePhotoMetadata_MissingBucketIsNoOp(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewMetadataStore(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer store.Close()

	// Removing from a non-existent folder bucket must not error.
	if err := store.RemovePhotoMetadata("ghost-folder", "photo.jpg"); err != nil {
		t.Fatalf("expected no error for missing bucket, got: %v", err)
	}
}

func TestClearMetadataScope_Stars(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewMetadataStore(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer store.Close()

	folderID := "folder-clear-stars"
	for i, id := range []string{"a.jpg", "b.jpg"} {
		_ = store.SavePhotoMetadata(folderID, id, PhotoMetadata{IsStarred: true, Label: i + 1})
	}

	if err := store.ClearMetadataScope(folderID, "stars"); err != nil {
		t.Fatalf("clear stars: %v", err)
	}

	m, _ := store.LoadFolderMetadata(folderID)
	for id, meta := range m {
		if meta.IsStarred {
			t.Errorf("%s: IsStarred should be false after clear", id)
		}
		if meta.Label == 0 {
			t.Errorf("%s: Label should be preserved after stars-only clear", id)
		}
	}
}

func TestClearMetadataScope_All(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewMetadataStore(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer store.Close()

	folderID := "folder-clear-all"
	_ = store.SavePhotoMetadata(folderID, "p.jpg", PhotoMetadata{IsStarred: true, Label: 3})

	if err := store.ClearMetadataScope(folderID, "all"); err != nil {
		t.Fatalf("clear all: %v", err)
	}

	m, _ := store.LoadFolderMetadata(folderID)
	if meta, ok := m["p.jpg"]; ok {
		if meta.IsStarred || meta.Label != 0 {
			t.Errorf("expected cleared metadata, got %+v", meta)
		}
	}
}
