package review

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLogLoadSummaryReportsRestoredStateAndSync(t *testing.T) {
	var out bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })

	logLoadSummary(loadSummary{
		Result: "completed", Duration: time.Second, Discovered: 25,
		SnapshotHit: true, Reconciled: 2, MetadataRestored: 7,
		UndoRestored: 3, SyncEmitted: true,
	})

	logs := out.String()
	for _, want := range []string{
		"result=completed", "duration_ms=1000", "discovered=25",
		"metadata_restored=7", "undo_restored=3", "sync_emitted=true",
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("missing %q in %q", want, logs)
		}
	}
}
