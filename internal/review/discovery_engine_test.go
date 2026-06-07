package review

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"
)

func TestDiscoveryEngine_QueueSaturationSafety(t *testing.T) {
	root := t.TempDir()
	const dirCount = 180

	for i := 0; i < dirCount; i++ {
		dir := filepath.Join(root, fmt.Sprintf("d-%03d", i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "img.jpg"), []byte("x"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	engine := NewDiscoveryEngine(root, DiscoveryEngineOptions{
		WorkerCount:    1,
		DirQueueSize:   1,
		EventQueueSize: 8,
	})

	before := runtime.NumGoroutine()
	events := make(chan DiscoveryEvent, 8)
	errCh := make(chan error, 1)
	go func() {
		errCh <- engine.Run(context.Background(), events)
	}()

	var got []string
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				if err := <-errCh; err != nil {
					t.Fatalf("engine returned error: %v", err)
				}
				after := runtime.NumGoroutine()
				if delta := after - before; delta > 25 {
					t.Fatalf("unexpected goroutine growth under queue pressure: before=%d after=%d", before, after)
				}
				sort.Strings(got)
				if len(got) != dirCount {
					t.Fatalf("expected %d discovered files, got %d", dirCount, len(got))
				}
				return
			}
			if ev.Type == DiscoveryEventFound {
				got = append(got, ev.RelPath)
			}
		case <-timeout:
			t.Fatal("timed out waiting for discovery to complete")
		}
	}
}

func TestDiscoveryEngine_PropagatesReadDirError(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "ok")
	bad := filepath.Join(root, "bad")
	if err := os.MkdirAll(good, 0o755); err != nil {
		t.Fatalf("mkdir good: %v", err)
	}
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatalf("mkdir bad: %v", err)
	}
	if err := os.WriteFile(filepath.Join(good, "img.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write good file: %v", err)
	}

	wantErr := errors.New("boom readdir")
	engine := NewDiscoveryEngine(root, DiscoveryEngineOptions{
		WorkerCount:    2,
		DirQueueSize:   8,
		EventQueueSize: 8,
		ReadDir: func(path string) ([]os.DirEntry, error) {
			if filepath.Clean(path) == filepath.Clean(bad) {
				return nil, wantErr
			}
			return os.ReadDir(path)
		},
	})

	events := make(chan DiscoveryEvent, 8)
	errCh := make(chan error, 1)
	go func() {
		errCh <- engine.Run(context.Background(), events)
	}()

	var gotErrEvent bool
	for ev := range events {
		if ev.Type == DiscoveryEventError {
			gotErrEvent = true
			if !errors.Is(ev.Err, wantErr) {
				t.Fatalf("error event mismatch: got %v want %v", ev.Err, wantErr)
			}
		}
	}

	err := <-errCh
	if !errors.Is(err, wantErr) {
		t.Fatalf("engine error mismatch: got %v want %v", err, wantErr)
	}
	if !gotErrEvent {
		t.Fatal("expected discovery error event")
	}
}
