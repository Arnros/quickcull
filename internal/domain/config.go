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

// ExiftoolPath returns configured exiftool path or searches common locations,
// falling back to the default binary name if not found.
func ExiftoolPath() string {
	path := GetConfig().ExiftoolPath
	if path != "" {
		// Security: reject configured paths that are relative or contain traversal.
		if !filepath.IsAbs(path) || strings.Contains(path, "..") {
			slog.Warn("configured exiftool path rejected: must be absolute and not contain '..', falling back", "path", path)
		} else if info, err := os.Stat(path); err != nil || info.IsDir() {
			slog.Warn("configured exiftool path rejected: not an executable file, falling back", "path", path, "error", err)
		} else {
			return path
		}
	}

	// 1. Try LookPath first (checks system PATH)
	binaryName := defaultExiftoolBinary
	if runtime.GOOS == "windows" {
		binaryName = "exiftool.exe"
	}
	if p, err := exec.LookPath(binaryName); err == nil {
		return p
	}

	// 2. Check the directory of the running executable (useful for portable setups)
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		var target string
		if runtime.GOOS == "windows" {
			target = filepath.Join(exeDir, "exiftool.exe")
		} else {
			target = filepath.Join(exeDir, "exiftool")
		}
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			return target
		}
	}

	// 3. Search common absolute installation paths on Unix-like and Windows systems
	var commonPaths []string
	if runtime.GOOS == "windows" {
		commonPaths = []string{
			`C:\Windows\exiftool.exe`,
			`C:\Program Files\exiftool\exiftool.exe`,
			`C:\Program Files (x86)\exiftool\exiftool.exe`,
		}
	} else {
		commonPaths = []string{
			"/opt/homebrew/bin/exiftool",
			"/usr/local/bin/exiftool",
			"/usr/bin/exiftool",
			"/bin/exiftool",
		}
	}

	for _, cp := range commonPaths {
		if info, err := os.Stat(cp); err == nil && !info.IsDir() {
			if runtime.GOOS == "windows" {
				return cp
			}
			// Check if executable on Unix
			if info.Mode()&0111 != 0 {
				return cp
			}
		}
	}

	return binaryName
}
