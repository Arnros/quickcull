package review

import (
	"context"
	"quickcull/internal/bus"
	"quickcull/internal/utils"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// UI event names emitted via broadcast.
const (
	// eventSyncState is the full-state broadcast event name sent on initial load.
	eventSyncState = "SyncState"
	// eventSyncDelta is the lightweight delta broadcast event name for metadata-only changes.
	eventSyncDelta = "SyncDelta"
)

func (s *Server) broadcast(name string, data any) {
	s.onBroadcastMu.RLock()
	hook := s.onBroadcast
	s.onBroadcastMu.RUnlock()
	if hook != nil {
		hook(name, data)
	}
	if s.ctx != nil {
		wailsruntime.EventsEmit(s.ctx, name, data)
	}
}

// StartEventEngine starts the v2 Reducer loop that listens to Commands and emits StateUpdated
func (s *Server) StartEventEngine(ctx context.Context) {
	chStar := s.Bus.Subscribe(bus.TypeCommandToggleStar)
	chTrash := s.Bus.Subscribe(bus.TypeCommandTrashPhoto)
	chLabel := s.Bus.Subscribe(bus.TypeCommandLabelPhoto)
	chRotate := s.Bus.Subscribe(bus.TypeCommandRotatePhoto)
	chUndo := s.Bus.Subscribe(bus.TypeCommandUndo)
	chBatch := s.Bus.Subscribe(bus.TypeCommandBatch)
	chReset := s.Bus.Subscribe(bus.TypeCommandResetMetadata)

	utils.SafeGo(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-chStar:
				s.applyBusEvent(ev)
			case ev := <-chTrash:
				s.applyBusEvent(ev)
			case ev := <-chLabel:
				s.applyBusEvent(ev)
			case ev := <-chRotate:
				s.applyBusEvent(ev)
			case ev := <-chUndo:
				s.applyBusEvent(ev)
			case ev := <-chBatch:
				s.applyBusEvent(ev)
			case ev := <-chReset:
				s.applyBusEvent(ev)
			}
		}
	})
}

func (s *Server) applyBusEvent(event bus.Event) {
	if _, _, err := s.applyEvent(event); err != nil {
		utils.LogError("Event engine failed to apply event", "type", event.Type, "error", err)
	}
}
