package utils

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
)

type syncer interface {
	Sync() error
}

type panicSink struct {
	mu      sync.Mutex
	writers []io.Writer
	syncers []syncer
}

func newCrashSink(writers ...io.Writer) *panicSink {
	if len(writers) == 0 {
		writers = []io.Writer{os.Stderr}
	}

	sink := &panicSink{}
	for _, w := range writers {
		if w == nil {
			continue
		}
		sink.writers = append(sink.writers, w)
		if s, ok := w.(syncer); ok {
			sink.syncers = append(sink.syncers, s)
		}
	}
	if len(sink.writers) == 0 {
		sink.writers = []io.Writer{os.Stderr}
		sink.syncers = append(sink.syncers, os.Stderr)
	}
	return sink
}

func (s *panicSink) WriteString(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, w := range s.writers {
		_, _ = io.WriteString(w, content)
	}
	for _, syncer := range s.syncers {
		_ = syncer.Sync()
	}
}

var (
	crashSink   atomic.Pointer[panicSink]
	crashExit   = os.Exit
	crashNotify = notifyCrashPlatform
)

func init() {
	crashSink.Store(newCrashSink(os.Stderr))
}

// SetCrashOutputs configures the dedicated crash sink used when recovering a panic.
func SetCrashOutputs(writers ...io.Writer) {
	crashSink.Store(newCrashSink(writers...))
}

func reportPanic(r any) {
	stack := debug.Stack()
	content := fmt.Sprintf("\n\nCRITICAL PANIC DETECTED:\n%v\n\nStack Trace:\n%s\n", r, string(stack))
	content = strings.ReplaceAll(content, GetHomeDir(), "~")

	slog.Error("CRITICAL PANIC", "error", r, "stack", string(stack))
	crashSink.Load().WriteString(content)
	crashNotify(content)
}

// HandlePanic captures a panic, logs it with a stack trace, and performs
// platform-specific notification if necessary.
func HandlePanic() {
	if r := recover(); r != nil {
		reportPanic(r)
		crashExit(1)
	}
}

// SafeGo runs the given function in a new goroutine and captures any panics.
func SafeGo(fn func()) {
	go func() {
		defer HandlePanic()
		fn()
	}()
}
