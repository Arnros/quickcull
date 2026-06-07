package utils

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type syncBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	syncCalls int
}

func (b *syncBuffer) Write(p []byte) (int, error) {
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
	oldExit := crashExit
	oldNotify := crashNotify
	oldLogger := slog.Default()
	t.Cleanup(func() {
		crashSink.Store(oldSink)
		crashExit = oldExit
		crashNotify = oldNotify
		slog.SetDefault(oldLogger)
	})

	logOut := &syncBuffer{}
	crashOut := &syncBuffer{}
	slog.SetDefault(slog.New(slog.NewTextHandler(logOut, nil)))
	crashSink.Store(newCrashSink(crashOut))

	exitCode := make(chan int, 1)
	crashExit = func(code int) {
		exitCode <- code
	}

	notifyMsg := make(chan string, 1)
	crashNotify = func(content string) {
		notifyMsg <- content
	}

	func() {
		defer HandlePanic()
		panic("boom")
	}()

	select {
	case code := <-exitCode:
		if code != 1 {
			t.Fatalf("crashExit code = %d, want 1", code)
		}
	case <-time.After(time.Second):
		t.Fatal("crashExit was not called")
	}

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

func TestSafeGoRecoversPanics(t *testing.T) {
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

	done := make(chan int, 1)
	crashExit = func(code int) {
		done <- code
	}

	SafeGo(func() {
		panic("goroutine boom")
	})

	select {
	case code := <-done:
		if code != 1 {
			t.Fatalf("crashExit code = %d, want 1", code)
		}
	case <-time.After(time.Second):
		t.Fatal("panic in SafeGo was not recovered")
	}

	if got := crashOut.String(); !strings.Contains(got, "goroutine boom") {
		t.Fatalf("crash sink output = %q, want goroutine panic", got)
	}
}
