package review

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"quickcull/internal/persistence"
)

func TestBuildFolderSignature_IsDeterministic(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a.jpg"), []byte("a"))
	mustWriteFile(t, filepath.Join(root, "b.jpg"), []byte("b"))

	sig1 := BuildFolderSignature(root)
	sig2 := BuildFolderSignature(root)

	if sig1 == "" || sig2 == "" {
		t.Fatalf("signature must not be empty: sig1=%q sig2=%q", sig1, sig2)
	}
	if sig1 != sig2 {
		t.Fatalf("signature must be deterministic: sig1=%q sig2=%q", sig1, sig2)
	}
}

func TestBuildFolderSignature_ChangesWhenSampleChanges(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "sample.jpg")
	mustWriteFile(t, target, []byte("one"))

	sig1 := BuildFolderSignature(root)
	if sig1 == "" {
		t.Fatalf("initial signature must not be empty")
	}

	// Ensure mtime differs even on coarse-resolution filesystems.
	time.Sleep(10 * time.Millisecond)
	mustWriteFile(t, target, []byte("one-two-three"))

	sig2 := BuildFolderSignature(root)
	if sig2 == "" {
		t.Fatalf("updated signature must not be empty")
	}
	if sig1 == sig2 {
		t.Fatalf("signature must change when sampled file changes: sig=%q", sig1)
	}
}

func TestIsSnapshotUsable(t *testing.T) {
	root := t.TempDir()
	sig := "abc123"

	good := persistence.FolderSnapshot{
		Version:   folderSnapshotVersion,
		RootPath:  root,
		Signature: sig,
	}

	if !IsSnapshotUsable(root, good, sig) {
		t.Fatalf("expected snapshot to be usable")
	}

	badVersion := good
	badVersion.Version++
	if IsSnapshotUsable(root, badVersion, sig) {
		t.Fatalf("snapshot with wrong version must not be usable")
	}

	badRoot := good
	badRoot.RootPath = filepath.Join(root, "other")
	if IsSnapshotUsable(root, badRoot, sig) {
		t.Fatalf("snapshot with wrong root must not be usable")
	}

	badSig := good
	badSig.Signature = "other"
	if IsSnapshotUsable(root, badSig, sig) {
		t.Fatalf("snapshot with mismatched signature must not be usable")
	}

	emptyPersistedSig := good
	emptyPersistedSig.Signature = ""
	if IsSnapshotUsable(root, emptyPersistedSig, sig) {
		t.Fatalf("snapshot with empty persisted signature must not be usable")
	}

	if IsSnapshotUsable(root, good, "") {
		t.Fatalf("snapshot with empty runtime signature must not be usable")
	}
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

// TestBuildFolderSignature_ChangesWhenSubdirChanges verifies that adding a file
// inside an existing sub-directory invalidates the snapshot signature.
// This is the critical case for fast-startup correctness: if photos are added to
// a sub-folder between two app sessions, the stale snapshot must be rejected.
func TestBuildFolderSignature_ChangesWhenSubdirChanges(t *testing.T) {
	root := t.TempDir()
	subdir := filepath.Join(root, "2024")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	// Capture signature before any file is present in the sub-folder.
	sig1 := BuildFolderSignature(root)
	if sig1 == "" {
		t.Fatalf("initial signature must not be empty")
	}

	// Ensure mtime differs even on coarse-resolution filesystems.
	time.Sleep(10 * time.Millisecond)

	// Add a new file inside the existing sub-directory.
	mustWriteFile(t, filepath.Join(subdir, "new_photo.jpg"), []byte("new"))

	sig2 := BuildFolderSignature(root)
	if sig2 == "" {
		t.Fatalf("updated signature must not be empty")
	}
	if sig1 == sig2 {
		t.Fatalf("signature must change when a file is added to a sub-directory: sig=%q", sig1)
	}
}
