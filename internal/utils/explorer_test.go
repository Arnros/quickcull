package utils

import (
	"errors"
	"runtime"
	"testing"
)

func TestOpenFolderInExplorer(t *testing.T) {
	var capturedName string

	// Mock commandExecutor
	oldExecutor := commandExecutor
	defer func() { commandExecutor = oldExecutor }()
	commandExecutor = func(name string, args ...string) error {
		capturedName = name
		return nil
	}

	path := "/test/path with spaces"
	err := OpenFolderInExplorer(path)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	switch runtime.GOOS {
	case "windows":
		if capturedName != "explorer.exe" {
			t.Errorf("Expected explorer.exe, got %s", capturedName)
		}
	case "linux":
		// Note: linux test depends on lookPath results, 
		// but we mostly want to check if commandExecutor was called
		if capturedName == "" {
			t.Errorf("Expected a command to be executed on linux")
		}
	}
}

func TestRevealFileInExplorer(t *testing.T) {
	var capturedName string
	var capturedArgs []string

	// Mock commandExecutor
	oldExecutor := commandExecutor
	defer func() { commandExecutor = oldExecutor }()
	commandExecutor = func(name string, args ...string) error {
		capturedName = name
		capturedArgs = args
		return nil
	}

	// Mock lookPath for linux to simulate xdg-open availability
	oldLookPath := lookPath
	defer func() { lookPath = oldLookPath }()
	lookPath = func(file string) (string, error) {
		if file == "xdg-open" {
			return "/usr/bin/xdg-open", nil
		}
		return "", errors.New("not found")
	}

	path := "/test/file.jpg"
	_ = RevealFileInExplorer(path)

	switch runtime.GOOS {
	case "windows":
		if capturedName != "explorer.exe" {
			t.Errorf("Expected explorer.exe, got %s", capturedName)
		}
		if len(capturedArgs) != 2 || capturedArgs[0] != "/select," {
			t.Errorf("Unexpected args for windows: %v", capturedArgs)
		}
	case "linux":
		if capturedName != "xdg-open" {
			t.Errorf("Expected xdg-open, got %s", capturedName)
		}
	}
}
