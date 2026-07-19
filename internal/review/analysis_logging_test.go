package review

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLogAnalysisLifecycleSummaryIncludesOutcomeAndPolicy(t *testing.T) {
	var out bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })

	logAnalysisLifecycleSummary(analysisLifecycleSummary{
		Result: "cancelled", Duration: 1250 * time.Millisecond,
		Processed: 12, Total: 40, IOWorkers: 4,
		ComputeWorkers: 2, HashDeferred: true,
	})

	logs := out.String()
	for _, want := range []string{
		"result=cancelled", "duration_ms=1250", "processed=12", "total=40",
		"io_workers=4", "compute_workers=2", "hash_deferred=true",
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("missing %q in %q", want, logs)
		}
	}
}

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

func TestLogAnalysisSkipReason(t *testing.T) {
	for _, reason := range []string{"missing_state", "invalid_path", "missing_processor", "context_cancelled", "queue_exhausted", "urgent_retry"} {
		t.Run(reason, func(t *testing.T) {
			var out bytes.Buffer
			oldLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
			t.Cleanup(func() { slog.SetDefault(oldLogger) })

			logAnalysisDecision(reason, "index", 7)

			if !strings.Contains(out.String(), "reason="+reason) {
				t.Fatalf("missing reason in %q", out.String())
			}
		})
	}
}

func TestLogAnalysisHashPolicyTransitionIncludesPreviousAndCurrent(t *testing.T) {
	var out bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })

	logAnalysisHashPolicyTransition(false, true)

	logs := out.String()
	for _, want := range []string{"reason=hash_policy_changed", "previous=false", "current=true"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("missing %q in %q", want, logs)
		}
	}
}

func BenchmarkLogAnalysisDecisionDisabled(b *testing.B) {
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelInfo})))
	b.Cleanup(func() { slog.SetDefault(oldLogger) })
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logAnalysisDecision("urgent_retry", "index", i)
	}
}
