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
	run := newDiscoveryRun(e, ctx, cancel, events)
	run.dirCh <- e.root

	slog.Info("DiscoveryEngine: starting worker pool", "workers", e.workerCount, "root", e.root)
	var workerWg sync.WaitGroup
	for i := 0; i < e.workerCount; i++ {
		workerWg.Add(1)
		utils.SafeGo(func() {
			defer workerWg.Done()
			run.worker()
		})
	}

	<-run.doneCh
	close(run.dirCh)
	workerWg.Wait()
	slog.Info("DiscoveryEngine: all workers done", "root", e.root)
	return run.err()
}

type discoveryRun struct {
	engine   *DiscoveryEngine
	ctx      context.Context
	cancel   context.CancelFunc
	events   chan<- DiscoveryEvent
	dirCh    chan string
	doneCh   chan struct{}
	done     sync.Once
	pending  atomic.Int64
	errMu    sync.Mutex
	firstErr error
}

func newDiscoveryRun(engine *DiscoveryEngine, ctx context.Context, cancel context.CancelFunc, events chan<- DiscoveryEvent) *discoveryRun {
	run := &discoveryRun{engine: engine, ctx: ctx, cancel: cancel, events: events, dirCh: make(chan string, engine.dirQueueSize), doneCh: make(chan struct{})}
	run.pending.Store(1)
	return run
}

func (r *discoveryRun) worker() {
	for {
		select {
		case <-r.doneCh:
			return
		case dir, ok := <-r.dirCh:
			if !ok {
				return
			}
			r.processStack(dir)
		}
	}
}

func (r *discoveryRun) processStack(initial string) {
	stack := []string{initial}
	for len(stack) > 0 {
		last := len(stack) - 1
		currentDir := stack[last]
		stack = stack[:last]
		if r.ctx.Err() == nil {
			r.processDirectory(currentDir, &stack)
		}
		r.completeDirectory()
	}
}

func (r *discoveryRun) processDirectory(dir string, stack *[]string) {
	entries, err := r.engine.readDir(dir)
	if err != nil {
		r.recordError(dir, err)
		return
	}
	for _, entry := range entries {
		r.processEntry(dir, entry, stack)
	}
}

func (r *discoveryRun) processEntry(dir string, entry os.DirEntry, stack *[]string) {
	name := entry.Name()
	if entry.Type()&os.ModeSymlink != 0 || (strings.HasPrefix(name, ".") && name != ".") {
		return
	}
	if entry.IsDir() {
		if !r.engine.ignoreDirs[name] {
			r.queueDirectory(filepath.Join(dir, name), stack)
		}
		return
	}
	if !domain.IsSupportedExtension(strings.ToLower(filepath.Ext(name))) {
		return
	}
	fullPath := filepath.Join(dir, name)
	relPath, err := filepath.Rel(r.engine.root, fullPath)
	if err != nil {
		slog.Debug("DiscoveryEngine: filepath.Rel error", "path", fullPath, "error", err)
		return
	}
	r.emit(DiscoveryEvent{Type: DiscoveryEventFound, RelPath: filepath.ToSlash(relPath)})
}

func (r *discoveryRun) queueDirectory(dir string, stack *[]string) {
	r.pending.Add(1)
	select {
	case r.dirCh <- dir:
	case <-r.ctx.Done():
		r.completeDirectory()
	default:
		*stack = append(*stack, dir)
	}
}

func (r *discoveryRun) completeDirectory() {
	if r.pending.Add(-1) == 0 {
		r.done.Do(func() { close(r.doneCh) })
	}
}

func (r *discoveryRun) recordError(dir string, err error) {
	r.errMu.Lock()
	if r.firstErr == nil {
		r.firstErr = err
	}
	r.errMu.Unlock()
	r.emit(DiscoveryEvent{Type: DiscoveryEventError, Dir: dir, Err: err})
	r.cancel()
}

func (r *discoveryRun) emit(event DiscoveryEvent) {
	select {
	case r.events <- event:
	case <-r.ctx.Done():
	}
}

func (r *discoveryRun) err() error {
	r.errMu.Lock()
	defer r.errMu.Unlock()
	return r.firstErr
}
