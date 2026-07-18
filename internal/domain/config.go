package domain

import (
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"quickcull/internal/utils"
	"runtime"
	"strings"
	"sync"
)

const configFile = "config.json"

// Default values used when a config key is absent or out of range.
const (
	defaultWindowWidth        = 1600
	defaultWindowHeight       = 1000
	defaultDuplicateThreshold = 90.0
	defaultAutoRefreshSeconds = 5
	defaultBurstSeconds       = 5
	defaultBurstMaxFiles      = 100
	defaultStartupSnapshot    = true
	minWindowWidth            = 800
	minWindowHeight           = 600

	// defaultExiftoolBinary is the binary name resolved via $PATH when no
	// absolute ExiftoolPath is configured.
	defaultExiftoolBinary = "exiftool"

	// configDirPerm / configFilePerm follow the principle of least privilege:
	// the cache dir and config file are only accessible by the owning user.
	configDirPerm  = 0700
	configFilePerm = 0600
)

// Config stores global application settings.
type Config struct {
	ExiftoolPath           string            `json:"exiftoolPath"`
	Debug                  bool              `json:"debug"`
	Theme                  string            `json:"theme"` // "dark" or "light"
	DuplicateThreshold     float64           `json:"duplicateThreshold"`
	AutoRefresh            bool              `json:"autoRefresh"`
	AutoRefreshSeconds     int               `json:"autoRefreshSeconds"`
	AutoAdvance            bool              `json:"autoAdvance"`
	StartupSnapshotEnabled bool              `json:"startupSnapshotEnabled"`
	WindowWidth            int               `json:"windowWidth"`
	WindowHeight           int               `json:"windowHeight"`
	WindowX                int               `json:"windowX"`
	WindowY                int               `json:"windowY"`
	WindowIsMaximized      bool              `json:"windowIsMaximized"`
	WindowIsFullscreen     bool              `json:"windowIsFullscreen"`
	Shortcuts              map[string]string `json:"shortcuts"`
	BurstSeconds           int               `json:"burstSeconds"`
	BurstMaxFiles          int               `json:"burstMaxFiles"`
}

var (
	config     Config
	configOnce sync.Once
	configMu   sync.Mutex
)

// GetConfig returns the global configuration.
func GetConfig() Config {
	configOnce.Do(loadConfig)
	configMu.Lock()
	defer configMu.Unlock()
	return config
}

// UpdateConfig updates the global configuration and saves it to disk.
func UpdateConfig(newConfig Config) error {
	// Validate tool paths: if non-empty, they must be absolute paths to prevent
	// execution of unintended binaries via relative path confusion.
	if newConfig.ExiftoolPath != "" {
		newConfig.ExiftoolPath = filepath.Clean(newConfig.ExiftoolPath)
		if !filepath.IsAbs(newConfig.ExiftoolPath) {
			return ErrExiftoolPathMustBeAbs
		}
	}
	if newConfig.AutoRefreshSeconds <= 0 {
		newConfig.AutoRefreshSeconds = defaultAutoRefreshSeconds
	}

	configMu.Lock()
	config = newConfig
	if config.Debug {
		utils.LogLevel.Set(slog.LevelDebug)
	} else {
		utils.LogLevel.Set(slog.LevelInfo)
	}
	configMu.Unlock()
	return saveConfig()
}

func loadConfig() {
	cacheRoot, _ := os.UserCacheDir()
	path := filepath.Join(cacheRoot, AppName, configFile)

	data, err := os.ReadFile(path) // #nosec G304 -- config path is deterministic under user cache dir.
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Error("Failed to read config file", "path", path, "error", err)
		}
		applyNewConfigDefaults(&config)
		return
	}

	configMu.Lock()
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		slog.Warn("Failed to unmarshal config keys", "path", path, "error", err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		slog.Error("Failed to unmarshal config", "path", path, "error", err)
	}
	normalizeLoadedConfig(&config, raw)
	setConfigLogLevel(config.Debug)
	configMu.Unlock()
}

func applyNewConfigDefaults(cfg *Config) {
	cfg.WindowWidth = defaultWindowWidth
	cfg.WindowHeight = defaultWindowHeight
	cfg.DuplicateThreshold = defaultDuplicateThreshold
	cfg.AutoRefreshSeconds = defaultAutoRefreshSeconds
	cfg.BurstSeconds = defaultBurstSeconds
	cfg.BurstMaxFiles = defaultBurstMaxFiles
	cfg.StartupSnapshotEnabled = defaultStartupSnapshot
}

func normalizeLoadedConfig(cfg *Config, raw map[string]json.RawMessage) {
	if cfg.DuplicateThreshold <= 0 {
		cfg.DuplicateThreshold = defaultDuplicateThreshold
	}
	if cfg.WindowWidth < minWindowWidth {
		cfg.WindowWidth = defaultWindowWidth
	}
	if cfg.WindowHeight < minWindowHeight {
		cfg.WindowHeight = defaultWindowHeight
	}
	if cfg.AutoRefreshSeconds <= 0 {
		cfg.AutoRefreshSeconds = defaultAutoRefreshSeconds
	}
	if cfg.BurstSeconds <= 0 {
		cfg.BurstSeconds = defaultBurstSeconds
	}
	if cfg.BurstMaxFiles <= 0 {
		cfg.BurstMaxFiles = defaultBurstMaxFiles
	}
	if _, ok := raw["startupSnapshotEnabled"]; !ok {
		cfg.StartupSnapshotEnabled = defaultStartupSnapshot
	}
	if _, ok := raw["windowIsMaximized"]; !ok {
		cfg.WindowIsMaximized = false
	}
	if _, ok := raw["windowIsFullscreen"]; !ok {
		cfg.WindowIsFullscreen = false
	}
	if _, ok := raw["windowX"]; !ok {
		cfg.WindowX = -1
	}
	if _, ok := raw["windowY"]; !ok {
		cfg.WindowY = -1
	}
	if cfg.Shortcuts == nil {
		cfg.Shortcuts = make(map[string]string)
	}
}

func setConfigLogLevel(debug bool) {
	if debug {
		utils.LogLevel.Set(slog.LevelDebug)
	} else {
		utils.LogLevel.Set(slog.LevelInfo)
	}
}

func saveConfig() error {
	cacheRoot, _ := os.UserCacheDir()
	dir := filepath.Join(cacheRoot, AppName)
	if err := os.MkdirAll(dir, configDirPerm); err != nil {
		slog.Error("Failed to create config dir", "dir", dir, "error", err)
		return ErrConfigDirCreate
	}

	path := filepath.Join(dir, configFile)

	configMu.Lock()
	defer configMu.Unlock()

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return utils.AtomicWriteFileDurable(path, data, configFilePerm)
}

// ExiftoolPath returns configured exiftool path or searches common locations,
// falling back to the default binary name if not found.
func ExiftoolPath() string {
	if path, ok := validatedConfiguredExiftoolPath(GetConfig().ExiftoolPath); ok {
		return path
	}

	binaryName := exiftoolBinaryName()
	if p, err := exec.LookPath(binaryName); err == nil {
		return p
	}
	if path, ok := bundledExiftoolPath(binaryName); ok {
		return path
	}
	if path, ok := firstInstalledExiftoolPath(); ok {
		return path
	}
	return binaryName
}

func validatedConfiguredExiftoolPath(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	if !filepath.IsAbs(path) || strings.Contains(path, "..") {
		slog.Warn("configured exiftool path rejected: must be absolute and not contain '..', falling back", "path", path)
		return "", false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		slog.Warn("configured exiftool path rejected: not an executable file, falling back", "path", path, "error", err)
		return "", false
	}
	return path, true
}

func exiftoolBinaryName() string {
	if runtime.GOOS == "windows" {
		return "exiftool.exe"
	}
	return defaultExiftoolBinary
}

func bundledExiftoolPath(binaryName string) (string, bool) {
	executable, err := os.Executable()
	if err != nil {
		return "", false
	}
	target := filepath.Join(filepath.Dir(executable), binaryName)
	info, err := os.Stat(target)
	return target, err == nil && !info.IsDir()
}

func firstInstalledExiftoolPath() (string, bool) {
	paths := []string{"/opt/homebrew/bin/exiftool", "/usr/local/bin/exiftool", "/usr/bin/exiftool", "/bin/exiftool"}
	if runtime.GOOS == "windows" {
		paths = []string{`C:\Windows\exiftool.exe`, `C:\Program Files\exiftool\exiftool.exe`, `C:\Program Files (x86)\exiftool\exiftool.exe`}
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() && (runtime.GOOS == "windows" || info.Mode()&0o111 != 0) {
			return path, true
		}
	}
	return "", false
}
