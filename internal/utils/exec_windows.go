//go:build windows

package utils

import (
	"os/exec"
	"syscall"
)

// ConfigureSilentCommand prevents child process windows from flashing on Windows.
func ConfigureSilentCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
