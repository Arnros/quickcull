package domain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetAppCacheDir(t *testing.T) {
	cacheDir := GetAppCacheDir()
	if cacheDir == "" {
		t.Fatal("GetAppCacheDir returned empty string")
	}
	if !strings.Contains(cacheDir, "quickcull") {
		t.Errorf("Expected cache dir to contain 'quickcull', got %s", cacheDir)
	}
}

func TestGetCacheDir(t *testing.T) {
	root := "/tmp/test-photos"
	// Check if we are on Windows
	if os.PathSeparator == '\\' {
		root = "C:\\test-photos"
	}
	
	cacheDir := GetCacheDir(root)
	if cacheDir == "" {
		t.Fatal("GetCacheDir returned empty string")
	}
	
	// Check that it's a subdirectory of AppCacheDir
	appCache := GetAppCacheDir()
	if !strings.HasPrefix(cacheDir, appCache) {
		t.Errorf("Expected cache dir %s to be under %s", cacheDir, appCache)
	}
	
	// Check that the folder ID is unique (length check)
	rel, _ := filepath.Rel(appCache, cacheDir)
	if len(rel) != 16 {
		t.Errorf("Expected 16-char folder ID, got %s (len %d)", rel, len(rel))
	}
}
