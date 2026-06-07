package utils

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
)

var (
	// commandExecutor allows overriding for tests
	commandExecutor = func(name string, args ...string) error {
		return exec.Command(name, args...).Start()
	}
	// lookPath allows overriding for tests
	lookPath = exec.LookPath
)

// OpenFolderInExplorer opens the system file explorer at the given directory path.
func OpenFolderInExplorer(dirPath string) error {
	dirPath = filepath.Clean(dirPath)

	switch runtime.GOOS {
	case "windows":
		return commandExecutor("explorer.exe", dirPath)
	case "darwin":
		return commandExecutor("open", dirPath)
	case "linux":
		if _, err := lookPath("xdg-open"); err == nil {
			return commandExecutor("xdg-open", dirPath)
		} else if _, err := lookPath("gio"); err == nil {
			return commandExecutor("gio", "open", dirPath)
		} else {
			return fmt.Errorf("no file explorer found (xdg-open or gio)")
		}
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// RevealFileInExplorer opens the system file explorer and selects the given file.
func RevealFileInExplorer(filePath string) error {
	filePath = filepath.Clean(filePath)

	switch runtime.GOOS {
	case "windows":
		return commandExecutor("explorer.exe", "/select,", filePath)
	case "darwin":
		return commandExecutor("open", "-R", filePath)
	case "linux":
		// Linux doesn't have a universal way to "select" a file,
		// so we just open the containing folder.
		if _, err := lookPath("xdg-open"); err == nil {
			return commandExecutor("xdg-open", filepath.Dir(filePath))
		} else if _, err := lookPath("gio"); err == nil {
			return commandExecutor("gio", "open", filepath.Dir(filePath))
		} else {
			return fmt.Errorf("no file explorer found (xdg-open or gio)")
		}
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}
