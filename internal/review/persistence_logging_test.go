package review

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLogPersistenceSummaryReportsBatchMetrics(t *testing.T) {
	var out bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })

	logPersistenceSummary("flush", 12, 2, 125*time.Millisecond)

	logs := out.String()
	for _, want := range []string{
		"operation=flush", "affected=12", "failures=2", "duration_ms=125",
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("missing %q in %q", want, logs)
		}
	}
	for _, forbidden := range []string{"metadata=", "paths="} {
		if strings.Contains(logs, forbidden) {
			t.Fatalf("forbidden payload %q in %q", forbidden, logs)
		}
	}
}

func TestPersistenceSummaryWindowAggregatesAndRateLimits(t *testing.T) {
	start := time.Unix(100, 0)
	var window persistenceSummaryWindow

	first, ok := window.record(start, 3, 1, 10*time.Millisecond, false)
	if !ok || first.Affected != 3 || first.Failures != 1 {
		t.Fatalf("expected immediate first summary, got %+v, emitted=%v", first, ok)
	}
	if _, ok := window.record(start.Add(100*time.Millisecond), 4, 0, 20*time.Millisecond, false); ok {
		t.Fatal("expected summary inside one-second window to be aggregated")
	}
	aggregated, ok := window.record(start.Add(time.Second), 5, 2, 30*time.Millisecond, false)
	if !ok {
		t.Fatal("expected aggregate at the one-second boundary")
	}
	if aggregated.Affected != 9 || aggregated.Failures != 2 || aggregated.Duration != 50*time.Millisecond {
		t.Fatalf("unexpected aggregate: %+v", aggregated)
	}
}

func TestPersistenceSummaryWindowFlushesPendingTail(t *testing.T) {
	start := time.Unix(100, 0)
	var window persistenceSummaryWindow
	window.record(start, 1, 0, time.Millisecond, false)
	window.record(start.Add(100*time.Millisecond), 2, 1, 2*time.Millisecond, false)

	tail, ok := window.record(start.Add(200*time.Millisecond), 0, 0, 0, true)
	if !ok || tail.Affected != 2 || tail.Failures != 1 || tail.Duration != 2*time.Millisecond {
		t.Fatalf("expected pending tail, got %+v, emitted=%v", tail, ok)
	}
}
