package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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

func TestExiftoolPath(t *testing.T) {
	// Trigger lazy load once so subsequent calls don't read from disk and overwrite mock
	_ = GetConfig()

	configMu.Lock()
	orig := config
	configMu.Unlock()
	t.Cleanup(func() {
		configMu.Lock()
		config = orig
		configMu.Unlock()
	})

	// Case 1: Configured path is absolute and exists
	dummyExe := filepath.Join(t.TempDir(), "exiftool")
	if err := os.WriteFile(dummyExe, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	configMu.Lock()
	config = Config{ExiftoolPath: dummyExe}
	configMu.Unlock()
	if got := ExiftoolPath(); got != dummyExe {
		t.Errorf("ExiftoolPath() = %q, expected %q", got, dummyExe)
	}

	// Make fallback resolution deterministic and independent of the host.
	fallbackName := defaultExiftoolBinary
	if runtime.GOOS == "windows" {
		fallbackName = "exiftool.exe"
	}
	fallbackExe := filepath.Join(t.TempDir(), fallbackName)
	if err := os.WriteFile(fallbackExe, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(fallbackExe))

	// Case 2: Configured path does not exist or is insecure: falls back.
	configMu.Lock()
	config = Config{ExiftoolPath: "/some/custom/path/exiftool"}
	configMu.Unlock()
	if got := ExiftoolPath(); got != fallbackExe {
		t.Errorf("ExiftoolPath() fallback = %q, expected %q", got, fallbackExe)
	}

	// Case 3: Configured path is empty, falls back to PATH lookup or auto-detection
	configMu.Lock()
	config = Config{ExiftoolPath: ""}
	configMu.Unlock()
	got := ExiftoolPath()
	if got != fallbackExe {
		t.Errorf("ExiftoolPath() empty-config fallback = %q, expected %q", got, fallbackExe)
	}
}

func TestApplyNewConfigDefaults(t *testing.T) {
	var cfg Config
	applyNewConfigDefaults(&cfg)

	if cfg.WindowWidth != defaultWindowWidth || cfg.WindowHeight != defaultWindowHeight {
		t.Fatalf("window defaults = %dx%d, want %dx%d", cfg.WindowWidth, cfg.WindowHeight, defaultWindowWidth, defaultWindowHeight)
	}
	if cfg.DuplicateThreshold != defaultDuplicateThreshold || cfg.AutoRefreshSeconds != defaultAutoRefreshSeconds {
		t.Fatalf("review defaults = threshold %.1f refresh %d", cfg.DuplicateThreshold, cfg.AutoRefreshSeconds)
	}
	if cfg.BurstSeconds != defaultBurstSeconds || cfg.BurstMaxFiles != defaultBurstMaxFiles {
		t.Fatalf("burst defaults = %d/%d, want %d/%d", cfg.BurstSeconds, cfg.BurstMaxFiles, defaultBurstSeconds, defaultBurstMaxFiles)
	}
	if !cfg.StartupSnapshotEnabled {
		t.Fatal("startup snapshot must default to enabled")
	}
}

func TestNormalizeLoadedConfigDefaultsMissingFieldsAndPreservesExplicitValues(t *testing.T) {
	cfg := Config{WindowX: 24, WindowY: 42, StartupSnapshotEnabled: false}
	raw := map[string]json.RawMessage{
		"windowX":                json.RawMessage(`24`),
		"windowY":                json.RawMessage(`42`),
		"startupSnapshotEnabled": json.RawMessage(`false`),
	}
	normalizeLoadedConfig(&cfg, raw)

	if cfg.WindowX != 24 || cfg.WindowY != 42 {
		t.Fatalf("explicit position changed to (%d,%d)", cfg.WindowX, cfg.WindowY)
	}
	if cfg.StartupSnapshotEnabled {
		t.Fatal("explicit disabled startup snapshot was replaced by default")
	}
	if cfg.Shortcuts == nil {
		t.Fatal("nil shortcuts map was not initialized")
	}
	if cfg.WindowWidth != defaultWindowWidth || cfg.WindowHeight != defaultWindowHeight || cfg.DuplicateThreshold != defaultDuplicateThreshold {
		t.Fatalf("invalid loaded values were not normalized: %+v", cfg)
	}
}

func TestNormalizeLoadedConfigUsesOSPositionWhenCoordinatesAreMissing(t *testing.T) {
	var cfg Config
	normalizeLoadedConfig(&cfg, map[string]json.RawMessage{})
	if cfg.WindowX != -1 || cfg.WindowY != -1 {
		t.Fatalf("missing position normalized to (%d,%d), want (-1,-1)", cfg.WindowX, cfg.WindowY)
	}
	if !cfg.StartupSnapshotEnabled {
		t.Fatal("missing startup snapshot setting must default to enabled")
	}
}
