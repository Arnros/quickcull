package review

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLogSyncSummaryContainsMetricsWithoutStatePayloads(t *testing.T) {
	var out bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })

	logSyncSummary(syncSummary{
		Result:            "completed",
		Duration:          20 * time.Millisecond,
		Photos:            1000,
		Chunks:            4,
		FullMapSuppressed: true,
	})

	logs := out.String()
	for _, forbidden := range []string{"History", "InitialState"} {
		if strings.Contains(logs, forbidden) {
			t.Fatalf("forbidden field %q in %q", forbidden, logs)
		}
	}
	for _, want := range []string{"result=completed", "duration_ms=20", "photos=1000", "chunks=4", "full_map_suppressed=true"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("missing %q in %q", want, logs)
		}
	}
}
