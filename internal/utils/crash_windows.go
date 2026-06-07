//go:build windows

package utils

import (
	"syscall"
	"unsafe"
)

func notifyCrashPlatform(content string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBox := user32.NewProc("MessageBoxW")

	titlePtr, _ := syscall.UTF16PtrFromString("QuickCull - Critical Crash")
	contentPtr, _ := syscall.UTF16PtrFromString("The application has crashed unexpectedly.\n\n" + content + "\n\nLogs can be found in your local AppData/Cache folder.")

	// MB_OK | MB_ICONERROR = 0x00000000 | 0x00000010
	_, _, _ = messageBox.Call(
		0,
		uintptr(unsafe.Pointer(contentPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		0x00000010,
	)
}
