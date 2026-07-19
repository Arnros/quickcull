package review

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLogForcedGCSummaryReportsReclaimedMemory(t *testing.T) {
	var out bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })

	logForcedGCSummary(2300, 1700, 2000, 15*time.Millisecond)

	logs := out.String()
	for _, want := range []string{"heap_before_mb=2300", "heap_after_mb=1700", "reclaimed_mb=600", "threshold_mb=2000", "duration_ms=15"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("missing %q in %q", want, logs)
		}
	}
}

func TestForceGCIfNeededLogsOnlyWhenThresholdExceeded(t *testing.T) {
	var out bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })

	gcCalls := 0
	collect := func() uint64 {
		gcCalls++
		return 1700
	}

	if forceGCIfNeeded(1900, 2000, collect, func() time.Duration { return 5 * time.Millisecond }) {
		t.Fatal("GC triggered below threshold")
	}
	if out.Len() != 0 {
		t.Fatalf("unexpected log below threshold: %q", out.String())
	}
	if !forceGCIfNeeded(2300, 2000, collect, func() time.Duration { return 15 * time.Millisecond }) {
		t.Fatal("GC not triggered above threshold")
	}
	if gcCalls != 1 {
		t.Fatalf("expected one forced GC, got %d", gcCalls)
	}
	if !strings.Contains(out.String(), "reclaimed_mb=600") {
		t.Fatalf("missing forced-GC summary in %q", out.String())
	}
}
