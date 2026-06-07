package utils

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	logBuffer *bufio.Writer
	logFile   *os.File
	mu        sync.Mutex

	// LogLevel allows changing the global log level at runtime.
	LogLevel = &slog.LevelVar{}

	// Categorized levels for fine-grained control
	// Set LevelNavigation to LevelDebug to keep the app silent during fast scrolling
	LevelNavigation = slog.LevelDebug
	LevelAnalysis   = slog.LevelInfo
	LevelCore       = slog.LevelInfo
)

// LogNav logs a navigation-related message (high frequency)
func LogNav(msg string, args ...any) {
	slog.Log(context.TODO(), LevelNavigation, msg, args...)
}

// LogAnalysis logs background analysis events
func LogAnalysis(msg string, args ...any) {
	slog.Log(context.TODO(), LevelAnalysis, msg, args...)
}

// LogCore logs application lifecycle events
func LogCore(msg string, args ...any) {
	slog.Log(context.TODO(), LevelCore, msg, args...)
}

// LogWarn logs a warning message
func LogWarn(msg string, args ...any) {
	slog.Warn(msg, args...)
}

// LogError logs an error message
func LogError(msg string, args ...any) {
	slog.Error(msg, args...)
}

// SetupGlobalLogging initializes the structured logger with buffering and rotation.
func SetupGlobalLogging(debugMode bool, logPath string) {
	mu.Lock()
	defer mu.Unlock()

	if debugMode {
		LogLevel.Set(slog.LevelDebug)
	} else {
		LogLevel.Set(slog.LevelInfo)
	}

	logDir := filepath.Dir(logPath)
	_ = os.MkdirAll(logDir, 0700)

	// Rotate log if it gets too big (> 5MB)
	rotateLog(logPath)

	var err error
	logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "CRITICAL: Failed to open log file: %v\n", err)
		return
	}

	// Use a 64KB buffer to minimize disk I/O impact on performance
	logBuffer = bufio.NewWriterSize(logFile, 64*1024)

	// Anonymize home directory in logs
	home := GetHomeDir()
	opts := &slog.HandlerOptions{
		Level: LogLevel, // Use the dynamic LevelVar
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if home != "" && a.Value.Kind() == slog.KindString {
				val := a.Value.String()
				if strings.Contains(val, home) {
					return slog.String(a.Key, strings.ReplaceAll(val, home, "{{USER_HOME}}"))
				}
			}
			return a
		},
	}

	var handler slog.Handler
	if runtime.GOOS == "windows" {
		// On Windows, we primarily log to file as stderr might be hidden in GUI mode
		handler = slog.NewTextHandler(logBuffer, opts)
		os.Stderr = logFile
	} else {
		// On Linux/Mac, log to both for easier debugging
		multi := io.MultiWriter(os.Stderr, logBuffer)
		handler = slog.NewTextHandler(multi, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Set crash output to include the log file
	SetCrashOutputs(os.Stderr, logBuffer)

	// Background flusher
	go func() {
		for {
			time.Sleep(2 * time.Second)
			FlushLogs()
		}
	}()

	slog.Info("Logging initialized", "path", logPath, "level", LogLevel.Level())
}

// FlushLogs forces any buffered log entries to be written to disk.
func FlushLogs() {
	mu.Lock()
	defer mu.Unlock()
	if logBuffer != nil {
		_ = logBuffer.Flush()
	}
	if logFile != nil {
		_ = logFile.Sync()
	}
}

func rotateLog(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}

	// 5 MB limit
	if info.Size() > 5*1024*1024 {
		oldPath := path + ".old"
		_ = os.Remove(oldPath)
		_ = os.Rename(path, oldPath)
	}
}
