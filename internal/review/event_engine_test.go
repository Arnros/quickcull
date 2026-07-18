package review

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"quickcull/internal/bus"
)

func TestApplyBusEventLogsRejectedEvent(t *testing.T) {
	previous := slog.Default()
	var output bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	server := NewServer()
	server.applyBusEvent(bus.Event{Type: bus.TypeCommandToggleStar, Payload: bus.CommandToggleStarPayload{PhotoID: "missing.jpg"}})

	logged := output.String()
	if !strings.Contains(logged, "Event engine failed to apply event") || !strings.Contains(logged, "folder_not_found") {
		t.Fatalf("rejected event was not logged with its error: %q", logged)
	}
}
