package review

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"quickcull/internal/domain"
	internalexif "quickcull/internal/exif"

	"github.com/corona10/goimagehash"
)

func TestMediaCacheHashLookupPriority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "photo.jpg")
	persistent := &mockCache{
		hashes:   map[string]uint64{path: 22},
		metadata: map[string]*EXIFInfo{},
	}
	cache := NewMediaCache()
	cache.persistence = persistent
	cache.hashCache[path] = goimagehash.NewImageHash(11, goimagehash.PHash)

	if got := cache.GetHash(path).GetHash(); got != 11 {
		t.Fatalf("memory hash = %d, want 11", got)
	}
	delete(cache.hashCache, path)
	if got := cache.GetHash(path).GetHash(); got != 22 {
		t.Fatalf("persistent hash = %d, want 22", got)
	}
	if got := cache.hashCache[path].GetHash(); got != 22 {
		t.Fatalf("persistent hash was not promoted to memory: got %d", got)
	}
}

func TestMetadataCacheRejectsRawFallbackWhenExiftoolBecomesAvailable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX executable script")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	original := domain.GetConfig()
	t.Cleanup(func() {
		_ = domain.UpdateConfig(original)
		internalexif.ResetExiftoolAvailabilityCache()
	})
	executable := filepath.Join(t.TempDir(), "exiftool")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write fake exiftool: %v", err)
	}
	cfg := original
	cfg.ExiftoolPath = executable
	if err := domain.UpdateConfig(cfg); err != nil {
		t.Fatalf("configure fake exiftool: %v", err)
	}
	internalexif.ResetExiftoolAvailabilityCache()
	if !internalexif.IsExiftoolAvailable() {
		t.Fatal("fake exiftool was not detected")
	}

	if metadataCacheUsable(&EXIFInfo{Camera: rawNoExiftoolCameraKey, Width: 10, Height: 10}) {
		t.Fatal("RAW fallback metadata remained usable after exiftool became available")
	}
}

func TestMediaCacheHashFallsBackToOriginalAndPersistsResult(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "photo.jpg")
	writeTinyJPEG(t, path)
	invalidCacheDir := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(invalidCacheDir, []byte("x"), 0o600); err != nil {
		t.Fatalf("write invalid cache path: %v", err)
	}
	persistent := &mockCache{hashes: map[string]uint64{}, metadata: map[string]*EXIFInfo{}}
	cache := NewMediaCache()
	cache.cacheDir = invalidCacheDir
	cache.persistence = persistent

	if got := preferredHashSource(path, invalidCacheDir, cache.computeSem); got != path {
		t.Fatalf("preferred source = %q, want original %q", got, path)
	}
	hash := cache.GetHash(path)
	if hash == nil {
		t.Fatal("hash computation from original returned nil")
	}
	if got, ok := persistent.hashes[path]; !ok || got != hash.GetHash() {
		t.Fatalf("persisted hash = (%d, %v), want (%d, true)", got, ok, hash.GetHash())
	}
}

func TestMediaCacheMetadataLookupPriorityAndUsability(t *testing.T) {
	path := filepath.Join(t.TempDir(), "photo.jpg")
	persistent := &mockCache{
		hashes: map[string]uint64{},
		metadata: map[string]*EXIFInfo{
			path: {Camera: "persistent"},
		},
	}
	cache := NewMediaCache()
	cache.persistence = persistent
	cache.exifCache[path] = &EXIFInfo{Camera: "memory"}

	if got := cache.GetMetadata(path); got == nil || got.Camera != "memory" {
		t.Fatalf("memory metadata = %+v, want camera memory", got)
	}
	delete(cache.exifCache, path)
	if got := cache.GetMetadata(path); got == nil || got.Camera != "persistent" {
		t.Fatalf("persistent metadata = %+v, want camera persistent", got)
	}
	if got := cache.exifCache[path]; got == nil || got.Camera != "persistent" {
		t.Fatalf("persistent metadata was not promoted to memory: %+v", got)
	}

	tests := []struct {
		name string
		info *EXIFInfo
		want bool
	}{
		{name: "empty", info: &EXIFInfo{}, want: false},
		{name: "fingerprint", info: &EXIFInfo{Fingerprint: "fp"}, want: true},
		{name: "camera", info: &EXIFInfo{Camera: "camera"}, want: true},
		{name: "dimensions", info: &EXIFInfo{Width: 1, Height: 1}, want: true},
		{name: "incomplete dimensions", info: &EXIFInfo{Width: 1}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := metadataCacheUsable(test.info); got != test.want {
				t.Fatalf("metadataCacheUsable(%+v) = %v, want %v", test.info, got, test.want)
			}
		})
	}
}
