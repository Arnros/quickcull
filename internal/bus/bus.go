package bus

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
)

// EventType strongly types our system events.
type EventType string

// subscriberChannelBuffer is the capacity of each subscriber's event channel.
// A buffer of 100 absorbs short bursts (e.g. rapid label changes) without
// blocking the publisher, while staying well within memory budget (~3 KB/subscriber).
const subscriberChannelBuffer = 100

const (
	// Intention Commands (UI -> Core)
	TypeCommandToggleStar    EventType = "CommandToggleStar"
	TypeCommandTrashPhoto    EventType = "CommandTrashPhoto"
	TypeCommandLabelPhoto    EventType = "CommandLabelPhoto"
	TypeCommandRotatePhoto   EventType = "CommandRotatePhoto"
	TypeCommandUndo          EventType = "CommandUndo"
	TypeCommandResetMetadata EventType = "CommandResetMetadata"
	TypeCommandBatch         EventType = "CommandBatch"

	// State Events (Core -> UI)
	TypeStateUpdated EventType = "StateUpdated"
	TypeStateDelta   EventType = "StateDelta"
)

// Event is the envelope for all bus messages.
type Event struct {
	Type    EventType
	Payload any
}

// UnmarshalJSON implements custom unmarshaling for Event to handle polymorphic payloads.
func (e *Event) UnmarshalJSON(data []byte) error {
	type Alias Event
	aux := &struct {
		*Alias
		Payload json.RawMessage `json:"Payload"`
	}{
		Alias: (*Alias)(e),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	switch e.Type {
	case TypeCommandToggleStar:
		var p CommandToggleStarPayload
		if err := json.Unmarshal(aux.Payload, &p); err != nil {
			return err
		}
		e.Payload = p
	case TypeCommandTrashPhoto:
		var p CommandTrashPhotoPayload
		if err := json.Unmarshal(aux.Payload, &p); err != nil {
			return err
		}
		e.Payload = p
	case TypeCommandLabelPhoto:
		var p CommandLabelPhotoPayload
		if err := json.Unmarshal(aux.Payload, &p); err != nil {
			return err
		}
		e.Payload = p
	case TypeCommandRotatePhoto:
		var p CommandRotatePhotoPayload
		if err := json.Unmarshal(aux.Payload, &p); err != nil {
			return err
		}
		e.Payload = p
	case TypeCommandUndo:
		var p CommandUndoPayload
		if err := json.Unmarshal(aux.Payload, &p); err != nil {
			return err
		}
		e.Payload = p
	case TypeCommandResetMetadata:
		var p CommandResetMetadataPayload
		if err := json.Unmarshal(aux.Payload, &p); err != nil {
			return err
		}
		e.Payload = p
	case TypeCommandBatch:
		var p CommandBatchPayload
		if err := json.Unmarshal(aux.Payload, &p); err != nil {
			return err
		}
		for _, sub := range p.Events {
			if sub.Type == TypeCommandBatch {
				return fmt.Errorf("nested CommandBatch not allowed")
			}
		}
		e.Payload = p
	default:
		// For State events, we might not need to unmarshal them from history
		e.Payload = aux.Payload
	}

	return nil
}

// StateDeltaPayload carries only changes to the state
type StateDeltaPayload struct {
	PhotoID string
	Changes map[string]any // e.g. {"IsStarred": true, "IsTrashed": true}
}

// CommandToggleStarPayload is the payload for TypeCommandToggleStar.
// Starred holds the desired state (set direct); OldStarred enables exact undo.
type CommandToggleStarPayload struct {
	PhotoID    string
	Starred    bool
	OldStarred bool
}

// CommandTrashPhotoPayload is the payload for TypeCommandTrashPhoto.
// OldIsTrashed captures the pre-mutation state for exact undo (set-direct, not toggle).
// OriginalIndex stores the position in VisibleOrder at the time of trashing, used to
// restore the photo at its original position during undo.
type CommandTrashPhotoPayload struct {
	PhotoID       string
	OldIsTrashed  bool
	OriginalIndex int
}

// CommandLabelPhotoPayload is the payload for TypeCommandLabelPhoto
type CommandLabelPhotoPayload struct {
	PhotoID  string
	Label    int
	OldLabel int
}

// CommandRotatePhotoPayload is the payload for TypeCommandRotatePhoto
type CommandRotatePhotoPayload struct {
	PhotoID   string
	Direction string // "left" or "right"
}

// CommandBatchPayload wraps multiple events as a single undo-able unit.
// One U keystroke undoes the entire batch atomically.
type CommandBatchPayload struct {
	Events []Event
}

// CommandUndoPayload is the payload for TypeCommandUndo
type CommandUndoPayload struct{}

// CommandResetMetadataPayload is the payload for TypeCommandResetMetadata
type CommandResetMetadataPayload struct {
	Scope string // "stars", "labels", or "all"
}

// StateUpdatedPayload carries the new immutable state.
// We use 'any' here to avoid circular dependency with 'review',
// we will cast it back to review.AppState in the receiver.
type StateUpdatedPayload struct {
	State any
}

// Bus is a simple, thread-safe Event Bus.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[EventType][]chan Event
}

// New creates a new Event Bus.
func New() *Bus {
	return &Bus{
		subscribers: make(map[EventType][]chan Event),
	}
}

// Subscribe returns a channel that will receive events of the given type.
func (b *Bus) Subscribe(t EventType) <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, subscriberChannelBuffer)
	b.subscribers[t] = append(b.subscribers[t], ch)
	return ch
}

// Publish broadcasts an event to all subscribers synchronously.
// In a real high-throughput scenario, we might want this to be async,
// but for QuickCull's zero-ingestion desktop model, sync broadcast guarantees
// immediate predictability for UI updates.
func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if channels, ok := b.subscribers[e.Type]; ok {
		for _, ch := range channels {
			// Non-blocking send, drop if full (or use a large buffer)
			select {
			case ch <- e:
			default:
				slog.Warn("bus: subscriber channel full, event dropped", "type", e.Type)
			}
		}
	}
}
