package review

import (
	"context"
	"log/slog"

	"quickcull/internal/domain"
	"quickcull/internal/utils"
)

type scanFilesFunc func(root string, filesChan chan<- string) error

var scanFiles scanFilesFunc = ScanFiles

// ScanFiles recursively scans the directory for supported media files concurrently.
// It streams discovered file paths to filesChan and closes it when finished.
func ScanFiles(root string, filesChan chan<- string) error {
	slog.Info("ScanFiles: Starting recursive scan", "root", root)
	defer close(filesChan)

	ignore := map[string]bool{
		domain.DirTrash:      true,
		domain.DirEvents:     true,
		domain.DirDuplicates: true,
	}

	engine := NewDiscoveryEngine(root, DiscoveryEngineOptions{
		WorkerCount:  scanWorkerCount(),
		DirQueueSize: defaultDirQueueSize,
		IgnoreDirs:   ignore,
	})
	engineEvents := make(chan DiscoveryEvent, engine.EventQueueSize())
	errCh := make(chan error, 1)

	utils.SafeGo(func() {
		errCh <- engine.Run(context.Background(), engineEvents)
	})

	for ev := range engineEvents {
		switch ev.Type {
		case DiscoveryEventFound:
			filesChan <- ev.RelPath
		case DiscoveryEventError:
			slog.Error("ScanFiles: ReadDir failed", "dir", ev.Dir, "error", ev.Err)
		}
	}

	if err := <-errCh; err != nil {
		slog.Error("ScanFiles: Scan failed", "root", root, "error", err)
		return err
	}

	slog.Info("ScanFiles: Scan completed successfully", "root", root)
	return nil
}
