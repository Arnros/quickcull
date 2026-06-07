package review

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"quickcull/internal/domain"
	"quickcull/internal/utils"
)

const (
	defaultScanWorkers    = 32
	defaultDirQueueSize   = 10000
	defaultEventQueueSize = 1024
)

func scanWorkerCount() int {
	return max(4, min(runtime.NumCPU(), defaultScanWorkers))
}

type DiscoveryEventType string

const (
	DiscoveryEventFound DiscoveryEventType = "found"
	DiscoveryEventError DiscoveryEventType = "error"
)

type DiscoveryEvent struct {
	Type    DiscoveryEventType
	RelPath string
	Dir     string
	Err     error
}

type DiscoveryEngineOptions struct {
	WorkerCount    int
	DirQueueSize   int
	EventQueueSize int
	IgnoreDirs     map[string]bool
	ReadDir        func(string) ([]os.DirEntry, error)
}

type DiscoveryEngine struct {
	root           string
	workerCount    int
	dirQueueSize   int
	eventQueueSize int
	ignoreDirs     map[string]bool
	readDir        func(string) ([]os.DirEntry, error)
}

func NewDiscoveryEngine(root string, opts DiscoveryEngineOptions) *DiscoveryEngine {
	workerCount := opts.WorkerCount
	if workerCount <= 0 {
		workerCount = scanWorkerCount()
	}
	if workerCount < 2 {
		workerCount = 2
	}

	dirQueueSize := opts.DirQueueSize
	if dirQueueSize <= 0 {
		dirQueueSize = defaultDirQueueSize
	}

	eventQueueSize := opts.EventQueueSize
	if eventQueueSize <= 0 {
		eventQueueSize = defaultEventQueueSize
	}

	readDir := opts.ReadDir
	if readDir == nil {
		readDir = os.ReadDir
	}

	ignoreDirs := opts.IgnoreDirs
	if ignoreDirs == nil {
		ignoreDirs = map[string]bool{}
	}

	return &DiscoveryEngine{
		root:           root,
		workerCount:    workerCount,
		dirQueueSize:   dirQueueSize,
		eventQueueSize: eventQueueSize,
		ignoreDirs:     ignoreDirs,
		readDir:        readDir,
	}
}

func (e *DiscoveryEngine) EventQueueSize() int {
	return e.eventQueueSize
}

func (e *DiscoveryEngine) Run(ctx context.Context, events chan<- DiscoveryEvent) error {
	defer close(events)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	dirCh := make(chan string, e.dirQueueSize)
	doneCh := make(chan struct{})
	var doneOnce sync.Once
	signalDone := func() {
		doneOnce.Do(func() { close(doneCh) })
	}

	var pending atomic.Int64
	pending.Store(1)

	var errMu sync.Mutex
	var firstErr error
	recordError := func(dir string, err error) {
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		errMu.Unlock()

		select {
		case events <- DiscoveryEvent{Type: DiscoveryEventError, Dir: dir, Err: err}:
		case <-ctx.Done():
		}
		cancel()
	}

	emitFound := func(relPath string) {
		select {
		case events <- DiscoveryEvent{Type: DiscoveryEventFound, RelPath: relPath}:
		case <-ctx.Done():
		}
	}

	dirCh <- e.root

	slog.Info("DiscoveryEngine: starting worker pool", "workers", e.workerCount, "root", e.root)

	var workerWg sync.WaitGroup
	for i := 0; i < e.workerCount; i++ {
		workerWg.Add(1)
		utils.SafeGo(func() {
			defer workerWg.Done()

			for {
				select {
				case <-doneCh:
					return
				case dir, ok := <-dirCh:
					if !ok {
						return
					}

					stack := []string{dir}
					for len(stack) > 0 {
						currentDir := stack[len(stack)-1]
						stack = stack[:len(stack)-1]

						if ctx.Err() != nil {
							if pending.Add(-1) == 0 {
								signalDone()
							}
							continue
						}

						entries, err := e.readDir(currentDir)
						if err != nil {
							recordError(currentDir, err)
							if pending.Add(-1) == 0 {
								signalDone()
							}
							continue
						}

						for _, d := range entries {
							if d.Type()&os.ModeSymlink != 0 {
								continue
							}

							name := d.Name()
							if strings.HasPrefix(name, ".") && name != "." {
								continue
							}

							if d.IsDir() {
								if e.ignoreDirs[name] {
									continue
								}

								childDir := filepath.Join(currentDir, name)
								pending.Add(1)
								select {
								case dirCh <- childDir:
								case <-ctx.Done():
									if pending.Add(-1) == 0 {
										signalDone()
									}
								default:
									// Saturated queue: process inline in this worker to stay bounded.
									stack = append(stack, childDir)
								}
								continue
							}

							ext := strings.ToLower(filepath.Ext(name))
							if !domain.IsSupportedExtension(ext) {
								continue
							}

							fullPath := filepath.Join(currentDir, name)
							rel, err := filepath.Rel(e.root, fullPath)
							if err != nil {
								slog.Debug("DiscoveryEngine: filepath.Rel error", "path", fullPath, "error", err)
								continue
							}

							emitFound(filepath.ToSlash(rel))
						}

						if pending.Add(-1) == 0 {
							signalDone()
						}
					}
				}
			}
		})
	}

	<-doneCh
	close(dirCh)
	workerWg.Wait()
	slog.Info("DiscoveryEngine: all workers done", "root", e.root)

	errMu.Lock()
	err := firstErr
	errMu.Unlock()

	return err
}
