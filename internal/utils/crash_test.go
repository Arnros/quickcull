package utils

import (
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type syncBuffer struct {
	buf       strings.Builder
	mu        sync.Mutex
	syncCalls int
}

func (b *syncBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) Sync() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.syncCalls++
	return nil
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestHandlePanicWritesCrashSinkAndSyncs(t *testing.T) {
	oldSink := crashSink.Load()
	oldNotify := crashNotify
	oldLogger := slog.Default()
	t.Cleanup(func() {
		crashSink.Store(oldSink)
		crashNotify = oldNotify
		slog.SetDefault(oldLogger)
	})

	logOut := &syncBuffer{}
	crashOut := &syncBuffer{}
	slog.SetDefault(slog.New(slog.NewTextHandler(logOut, nil)))
	crashSink.Store(newCrashSink(crashOut))

	notifyMsg := make(chan string, 1)
	crashNotify = func(content string) {
		notifyMsg <- content
	}

	func() {
		defer HandlePanic()
		panic("boom")
	}()

	// HandlePanic recovers, logs, and notifies but does NOT exit.
	// Exit is reserved for HandlePanicFatal.

	select {
	case msg := <-notifyMsg:
		if !strings.Contains(msg, "boom") {
			t.Fatalf("notify message %q does not contain panic value", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("crashNotify was not called")
	}

	if got := crashOut.String(); !strings.Contains(got, "CRITICAL PANIC DETECTED") {
		t.Fatalf("crash sink output = %q, want panic header", got)
	}
	if got := crashOut.String(); !strings.Contains(got, "boom") {
		t.Fatalf("crash sink output = %q, want panic value", got)
	}
	if crashOut.syncCalls == 0 {
		t.Fatal("crash sink Sync was not called")
	}
	if got := logOut.String(); !strings.Contains(got, "CRITICAL PANIC") {
		t.Fatalf("logger output = %q, want panic log", got)
	}
}

func TestHandlePanicFatalExits(t *testing.T) {
	oldSink := crashSink.Load()
	oldExit := crashExit
	oldNotify := crashNotify
	t.Cleanup(func() {
		crashSink.Store(oldSink)
		crashExit = oldExit
		crashNotify = oldNotify
	})

	crashOut := &syncBuffer{}
	crashSink.Store(newCrashSink(crashOut))
	crashNotify = func(string) {}

	exitCode := make(chan int, 1)
	crashExit = func(code int) {
		exitCode <- code
	}

	func() {
		defer HandlePanicFatal()
		panic("fatal boom")
	}()

	select {
	case code := <-exitCode:
		if code != 1 {
			t.Fatalf("crashExit code = %d, want 1", code)
		}
	case <-time.After(time.Second):
		t.Fatal("crashExit was not called")
	}

	if got := crashOut.String(); !strings.Contains(got, "CRITICAL PANIC DETECTED") {
		t.Fatalf("crash sink output = %q, want panic header", got)
	}
}

func TestSafeGoRecoversPanics(t *testing.T) {
	oldSink := crashSink.Load()
	oldNotify := crashNotify
	t.Cleanup(func() {
		crashSink.Store(oldSink)
		crashNotify = oldNotify
	})

	crashOut := &syncBuffer{}
	crashSink.Store(newCrashSink(crashOut))

	recovered := make(chan struct{}, 1)
	crashNotify = func(string) {
		recovered <- struct{}{}
	}

	SafeGo(func() {
		panic("goroutine boom")
	})

	// SafeGo uses HandlePanic (no exit). Wait for crashNotify to confirm recovery.
	select {
	case <-recovered:
		// goroutine recovered and notified
	case <-time.After(time.Second):
		t.Fatal("panic in SafeGo was not recovered")
	}

	if got := crashOut.String(); !strings.Contains(got, "goroutine boom") {
		t.Fatalf("crash sink output = %q, want goroutine panic", got)
	}
}
