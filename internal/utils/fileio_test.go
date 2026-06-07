package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveFile_Exists(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "file.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := RemoveFile(path); err != nil {
		t.Fatalf("RemoveFile: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("file should be gone after RemoveFile")
	}
}

func TestRemoveFile_NonExistent(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "ghost.txt")
	// On non-Windows, os.Remove on a missing file returns an error.
	// RemoveFile must propagate it (consistent with os.Remove).
	err := RemoveFile(path)
	if err == nil {
		t.Fatal("expected error removing non-existent file, got nil")
	}
}

func TestAtomicWriteFile_Success(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "out.txt")
	data := []byte("atomic content")

	if err := AtomicWriteFile(path, data, 0o600); err != nil {
		t.Fatalf("AtomicWriteFile: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("content mismatch: got %q, want %q", got, data)
	}

	// The .tmp file must be cleaned up.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal(".tmp file should not exist after successful atomic write")
	}
}

func TestAtomicWriteFile_OverwritesExisting(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "out.txt")

	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := AtomicWriteFile(path, []byte("new"), 0o600); err != nil {
		t.Fatalf("AtomicWriteFile: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Fatalf("expected 'new', got %q", string(got))
	}
}

func TestAtomicWriteFileDurable_NoTempLeft(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "durable.txt")

	if err := AtomicWriteFileDurable(path, []byte("durable"), 0o600); err != nil {
		t.Fatalf("AtomicWriteFileDurable: %v", err)
	}

	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal(".tmp should not exist after successful durable write")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "durable" {
		t.Fatalf("content mismatch: %q", got)
	}
}
