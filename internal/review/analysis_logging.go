package review

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"quickcull/internal/utils"
)

type analysisLifecycleSummary struct {
	Result         string
	Duration       time.Duration
	Processed      int
	Total          int
	IOWorkers      int
	ComputeWorkers int
	HashDeferred   bool
}

func logAnalysisLifecycleSummary(v analysisLifecycleSummary) {
	utils.LogAnalysis("BackgroundAnalysis: lifecycle summary",
		"result", v.Result,
		"duration_ms", v.Duration.Milliseconds(),
		"processed", v.Processed,
		"total", v.Total,
		"io_workers", v.IOWorkers,
		"compute_workers", v.ComputeWorkers,
		"hash_deferred", v.HashDeferred,
	)
}

type analysisIssueKind string

const (
	analysisIssueHash      analysisIssueKind = "hash"
	analysisIssueEXIF      analysisIssueKind = "exif"
	analysisIssueThumbnail analysisIssueKind = "thumbnail"
	analysisIssueProcessor analysisIssueKind = "processor"
)

type analysisIssueCollector struct {
	mu          sync.Mutex
	sampleLimit int
	counts      map[analysisIssueKind]int
	samples     map[analysisIssueKind][]string
}

func newAnalysisIssueCollector(sampleLimit int) *analysisIssueCollector {
	if sampleLimit < 0 {
		sampleLimit = 0
	}
	return &analysisIssueCollector{
		sampleLimit: sampleLimit,
		counts:      make(map[analysisIssueKind]int, 4),
		samples:     make(map[analysisIssueKind][]string, 4),
	}
}

func (c *analysisIssueCollector) Record(kind analysisIssueKind, path string, err error) {
	if c == nil || err == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.counts[kind]++
	if len(c.samples[kind]) >= c.sampleLimit {
		return
	}
	c.samples[kind] = append(c.samples[kind], fmt.Sprintf("kind=%s path=%s error=%v", kind, path, err))
}

func (c *analysisIssueCollector) LogSummary() {
	if c == nil {
		return
	}

	c.mu.Lock()
	hashFailures := c.counts[analysisIssueHash]
	exifFailures := c.counts[analysisIssueEXIF]
	thumbnailFailures := c.counts[analysisIssueThumbnail]
	processorFailures := c.counts[analysisIssueProcessor]
	samples := make([]string, 0, len(c.samples[analysisIssueHash])+len(c.samples[analysisIssueEXIF])+len(c.samples[analysisIssueThumbnail])+len(c.samples[analysisIssueProcessor]))
	for _, kind := range []analysisIssueKind{
		analysisIssueHash,
		analysisIssueEXIF,
		analysisIssueThumbnail,
		analysisIssueProcessor,
	} {
		samples = append(samples, c.samples[kind]...)
	}
	c.mu.Unlock()

	if hashFailures == 0 && exifFailures == 0 && thumbnailFailures == 0 && processorFailures == 0 {
		return
	}

	slog.Info("BackgroundAnalysis: issue summary",
		"hash_failures", hashFailures,
		"exif_failures", exifFailures,
		"thumbnail_failures", thumbnailFailures,
		"processor_failures", processorFailures,
	)
	for _, sample := range samples {
		slog.Debug("BackgroundAnalysis: sampled issue", "detail", sample)
	}
}

var activeAnalysisIssueCollector atomic.Pointer[analysisIssueCollector]

func setActiveAnalysisIssueCollector(c *analysisIssueCollector) func() {
	activeAnalysisIssueCollector.Store(c)
	return func() {
		activeAnalysisIssueCollector.CompareAndSwap(c, nil)
	}
}

func recordAnalysisIssue(kind analysisIssueKind, path string, err error) bool {
	collector := activeAnalysisIssueCollector.Load()
	if collector == nil {
		return false
	}
	collector.Record(kind, path, err)
	return true
}
