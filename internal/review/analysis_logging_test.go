package review

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestAnalysisIssueCollectorLogsAggregatedSummaryAndSampledDebugDetails(t *testing.T) {
	var out bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })

	c := newAnalysisIssueCollector(2)
	c.Record(analysisIssueHash, "/photos/a.jpg", errors.New("decode failed"))
	c.Record(analysisIssueHash, "/photos/b.jpg", errors.New("decode failed"))
	c.Record(analysisIssueHash, "/photos/c.jpg", errors.New("decode failed"))
	c.Record(analysisIssueEXIF, "/photos/d.raw", errors.New("exiftool json"))
	c.Record(analysisIssueThumbnail, "/photos/e.heic", errors.New("no preview"))

	c.LogSummary()

	logs := out.String()
	if !strings.Contains(logs, "BackgroundAnalysis: issue summary") {
		t.Fatalf("expected aggregated summary log, got %q", logs)
	}
	if !strings.Contains(logs, "hash_failures=3") {
		t.Fatalf("expected hash count in summary, got %q", logs)
	}
	if !strings.Contains(logs, "exif_failures=1") {
		t.Fatalf("expected exif count in summary, got %q", logs)
	}
	if !strings.Contains(logs, "thumbnail_failures=1") {
		t.Fatalf("expected thumbnail count in summary, got %q", logs)
	}
	if got := strings.Count(logs, "BackgroundAnalysis: sampled issue"); got != 4 {
		t.Fatalf("expected 4 sampled detail logs, got %d in %q", got, logs)
	}
	if strings.Contains(logs, "/photos/c.jpg") {
		t.Fatalf("expected sampling limit to suppress third hash detail, got %q", logs)
	}
}
