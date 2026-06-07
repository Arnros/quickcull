package domain

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"quickcull/internal/utils"
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
		// Default values for new config
		config.WindowWidth = defaultWindowWidth
		config.WindowHeight = defaultWindowHeight
		config.DuplicateThreshold = defaultDuplicateThreshold
		config.AutoRefreshSeconds = defaultAutoRefreshSeconds
		config.BurstSeconds = defaultBurstSeconds
		config.BurstMaxFiles = defaultBurstMaxFiles
		config.StartupSnapshotEnabled = defaultStartupSnapshot
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
	if config.DuplicateThreshold <= 0 {
		config.DuplicateThreshold = defaultDuplicateThreshold
	}
	if config.WindowWidth < minWindowWidth {
		config.WindowWidth = defaultWindowWidth
	}
	if config.WindowHeight < minWindowHeight {
		config.WindowHeight = defaultWindowHeight
	}
	if config.AutoRefreshSeconds <= 0 {
		config.AutoRefreshSeconds = defaultAutoRefreshSeconds
	}
	if config.BurstSeconds <= 0 {
		config.BurstSeconds = defaultBurstSeconds
	}
	if config.BurstMaxFiles <= 0 {
		config.BurstMaxFiles = defaultBurstMaxFiles
	}
	if _, ok := raw["startupSnapshotEnabled"]; !ok {
		config.StartupSnapshotEnabled = defaultStartupSnapshot
	}
	// Initial state for new flags if not present
	if _, ok := raw["windowIsMaximized"]; !ok {
		config.WindowIsMaximized = false
	}
	if _, ok := raw["windowIsFullscreen"]; !ok {
		config.WindowIsFullscreen = false
	}
	// Initial position (negative means let OS decide)
	if _, ok := raw["windowX"]; !ok {
		config.WindowX = -1
	}
	if _, ok := raw["windowY"]; !ok {
		config.WindowY = -1
	}
if config.Debug {
		utils.LogLevel.Set(slog.LevelDebug)
	} else {
		utils.LogLevel.Set(slog.LevelInfo)
	}
	if config.Shortcuts == nil {
		config.Shortcuts = make(map[string]string)
	}
	configMu.Unlock()
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

// ExiftoolPath returns configured exiftool path or the default binary name.
func ExiftoolPath() string {
	path := GetConfig().ExiftoolPath
	if path == "" {
		return defaultExiftoolBinary
	}
	return path
}
