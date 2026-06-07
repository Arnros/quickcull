package review

import (
	"log/slog"
	"strings"

	"quickcull/internal/bus"
)

// executeBatchAction performs an action on multiple paths using current app state for metadata.
func (a *App) executeBatchAction(paths []string, actionType bus.EventType, payloadCreator func(string, Photo) any) (any, error) {
	a.server.appStateMu.RLock()
	currentPhotos := make(map[string]Photo, len(paths))
	if a.server.appState != nil {
		for _, rel := range paths {
			normalized := strings.ReplaceAll(rel, "\\", "/")
			if p, ok := a.server.appState.Photos[normalized]; ok {
				currentPhotos[normalized] = p
			}
		}
	}
	a.server.appStateMu.RUnlock()

	var events []bus.Event
	for _, rel := range paths {
		normalizedRel := strings.ReplaceAll(rel, "\\", "/")
		photo := currentPhotos[normalizedRel]
		payload := payloadCreator(normalizedRel, photo)
		if payload == nil {
			continue
		}
		events = append(events, bus.Event{Type: actionType, Payload: payload})
	}

	switch len(events) {
	case 0:
		// nothing to do
	case 1:
		a.server.Bus.Publish(events[0])
	default:
		// Wrap in a single batch event so one Undo reverts all changes at once.
		a.server.Bus.Publish(bus.Event{
			Type:    bus.TypeCommandBatch,
			Payload: bus.CommandBatchPayload{Events: events},
		})
	}
	slog.Debug("ExecuteBatchAction: batch action completed", "action", actionType, "count", len(events))
	return &ActionResponse{Stats: a.snapshotStats(), Ok: true}, nil
}

// executePhotoActionVerified is a generic helper for simple metadata actions when state is already verified.
func (a *App) executePhotoActionVerified(state *State, eventType bus.EventType, payload any) (ActionResponse, error) {
	a.server.Bus.Publish(bus.Event{
		Type:    eventType,
		Payload: payload,
	})

	return ActionResponse{Stats: a.snapshotStats(), Ok: true}, nil
}
