package domain

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

const historyFile = "history.json"
const maxHistory = 10

var historyMu sync.Mutex

// AddToHistory adds a folder to the recent folders list (descending order).
func AddToHistory(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	historyMu.Lock()
	defer historyMu.Unlock()

	history, _ := getHistoryLocked()

	// Remove if already present to move it to the top
	newHistory := []string{absPath}
	for _, h := range history {
		if h != absPath {
			newHistory = append(newHistory, h)
		}
	}

	// Limit history size
	if len(newHistory) > maxHistory {
		newHistory = newHistory[:maxHistory]
	}

	appCache := GetAppCacheDir()
	if err := os.MkdirAll(appCache, 0o700); err != nil {
		slog.Error("History cache directory creation failed", "path", appCache, "error", err)
		return err
	}

	data, err := json.MarshalIndent(newHistory, "", "  ")
	if err != nil {
		return err
	}

	target := filepath.Join(appCache, historyFile)
	err = os.WriteFile(target, data, 0o600)
	if err != nil {
		slog.Error("History write failed", "file", target, "error", err)
	}
	return err
}

// GetHistory returns the list of recently opened folders.
func GetHistory() ([]string, error) {
	historyMu.Lock()
	defer historyMu.Unlock()
	return getHistoryLocked()
}

func getHistoryLocked() ([]string, error) {
	appCache := GetAppCacheDir()
	target := filepath.Join(appCache, historyFile)
	data, err := os.ReadFile(target) // #nosec G304 -- app-owned cache file path.
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Error("History read failed", "file", target, "error", err)
		}
		return nil, err
	}

	var history []string
	if err := json.Unmarshal(data, &history); err != nil {
		slog.Error("History decoding failed", "file", target, "error", err)
		return nil, err
	}

	return history, nil
}

// RemoveFromHistory removes a specific folder from the history.
func RemoveFromHistory(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	historyMu.Lock()
	defer historyMu.Unlock()

	history, _ := getHistoryLocked()
	newHistory := make([]string, 0, len(history))
	for _, h := range history {
		if h != absPath {
			newHistory = append(newHistory, h)
		}
	}

	appCache := GetAppCacheDir()
	data, err := json.MarshalIndent(newHistory, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(appCache, historyFile), data, 0o600)
}
