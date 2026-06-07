//go:build !windows

package utils

func notifyCrashPlatform(content string) {
	// For now, non-Windows platforms just rely on stderr/logs.
	// We could add xdg-open/zenity for Linux in the future if needed.
}
