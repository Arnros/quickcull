package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveConfigDurableWriteSuccess(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	configMu.Lock()
	orig := config
	config = Config{
		Debug:              true,
		Theme:              "light",
		DuplicateThreshold: 88.8,
		AutoRefresh:        true,
		AutoRefreshSeconds: 7,
		WindowWidth:        1200,
		WindowHeight:       800,
		Shortcuts:          map[string]string{"x": "y"},
	}
	configMu.Unlock()
	t.Cleanup(func() {
		configMu.Lock()
		config = orig
		configMu.Unlock()
	})

	if err := saveConfig(); err != nil {
		t.Fatalf("saveConfig failed: %v", err)
	}

	cacheRoot, _ := os.UserCacheDir()
	path := filepath.Join(cacheRoot, AppName, configFile)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config failed: %v", err)
	}

	var got Config
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal config failed: %v", err)
	}
	if got.Theme != "light" || got.WindowWidth != 1200 || got.WindowHeight != 800 {
		t.Fatalf("unexpected config values: %+v", got)
	}
}

func TestSaveConfigDurableWriteRecoversWhenTempPathIsDirectory(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	cacheRoot, _ := os.UserCacheDir()
	dir := filepath.Join(cacheRoot, AppName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir cache dir failed: %v", err)
	}
	path := filepath.Join(dir, configFile)

	original := Config{
		Theme:              "dark",
		DuplicateThreshold: 90.0,
		AutoRefreshSeconds: 5,
		WindowWidth:        1600,
		WindowHeight:       1000,
	}
	originalData, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("marshal original config failed: %v", err)
	}
	if err := os.WriteFile(path, originalData, 0o600); err != nil {
		t.Fatalf("write original config failed: %v", err)
	}

	// Simulate stale/corrupted temp path left as a directory.
	if err := os.Mkdir(path+".tmp", 0o700); err != nil {
		t.Fatalf("mkdir temp-path blocker failed: %v", err)
	}

	configMu.Lock()
	orig := config
	config = Config{
		Theme:              "light",
		DuplicateThreshold: 99.0,
		AutoRefreshSeconds: 15,
		WindowWidth:        1024,
		WindowHeight:       768,
	}
	configMu.Unlock()
	t.Cleanup(func() {
		configMu.Lock()
		config = orig
		configMu.Unlock()
	})

	if err := saveConfig(); err != nil {
		t.Fatalf("expected saveConfig to recover when temp path is a directory, got: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config failed: %v", err)
	}
	var got Config
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal config failed: %v", err)
	}
	if got.Theme != "light" || got.WindowWidth != 1024 || got.WindowHeight != 768 {
		t.Fatalf("config was not updated after recovering from temp directory: %+v", got)
	}
}
