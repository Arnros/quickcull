package review

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBoltDBCorruptionRecovery(t *testing.T) {
	cacheDir := t.TempDir()
	dbPath := filepath.Join(cacheDir, "cache.db")

	// 1. Create a "corrupted" BoltDB file (just write garbage)
	err := os.WriteFile(dbPath, []byte("this is not a valid boltdb file, it is just garbage data"), 0644)
	if err != nil {
		t.Fatalf("failed to write corrupted db file: %v", err)
	}

	// 2. Initialize the cache, which should detect the corruption, delete it, and recreate it
	cache := NewMediaCache()
	cache.LoadCache(cacheDir)
	defer cache.Close()

	// 3. Verify the cache is operational
	err = cache.persistence.PutHash("test.jpg", 12345)
	if err != nil {
		t.Errorf("failed to write to recovered cache: %v", err)
	}

	hash, ok := cache.persistence.GetHash("test.jpg")
	if !ok || hash != 12345 {
		t.Errorf("failed to read from recovered cache, got %d, expected 12345", hash)
	}
}
