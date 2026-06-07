//go:build !windows

package utils

import "os/exec"

// ConfigureSilentCommand is a no-op on non-Windows platforms.
func ConfigureSilentCommand(cmd *exec.Cmd) {}
