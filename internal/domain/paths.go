package domain

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
)

// GetFolderID returns a unique ID for a given photo root based on its absolute path.
func GetFolderID(root string) string {
	absRoot, _ := filepath.Abs(root)
	h := sha256.Sum256([]byte(absRoot))
	return fmt.Sprintf("%x", h[:8])
}

// GetCacheDir returns the standard system cache directory for a given photo root.
func GetCacheDir(root string) string {
	appCache := GetAppCacheDir()
	folderID := GetFolderID(root)
	return filepath.Join(appCache, folderID)
}

// GetAppCacheDir returns the root cache directory of the application.
func GetAppCacheDir() string {
	if testDir := os.Getenv("QUICKCULL_TEST_CACHE_DIR"); testDir != "" {
		return testDir
	}
	userCache, err := os.UserCacheDir()
	if err != nil {
		// Fallback to home dir if UserCacheDir fails
		home, _ := os.UserHomeDir()
		if home != "" {
			userCache = filepath.Join(home, ".cache")
		} else {
			userCache = filepath.Join(os.TempDir(), AppName+"-cache")
		}
	}
	return filepath.Join(userCache, AppName)
}

// GetLogPath returns the absolute path to the application log file.
func GetLogPath() string {
	return filepath.Join(GetAppCacheDir(), AppName+".log")
}
