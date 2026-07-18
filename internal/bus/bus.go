package bus

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

// EventType strongly types our system events.
type EventType string

// subscriberChannelBuffer is the capacity of each subscriber's event channel.
// A buffer of 100 absorbs short bursts (e.g. rapid label changes) without
// blocking the publisher, while staying well within memory budget (~3 KB/subscriber).
// subscriberChannelBuffer is the capacity of each subscriber's event channel.
// A buffer of 100 absorbs short bursts (e.g. rapid label changes) without
// blocking the publisher, while staying well within memory budget (~3 KB/subscriber).
const subscriberChannelBuffer = 100

// commandChannelBuffer is the buffer used for Command* (user intent) subscribers.
// Sized orders of magnitude larger than State channels so user actions are effectively
// never dropped under realistic interactive throughput (keynote-fast culling).
// If this buffer ever fills, DroppedEvents surfaces it as a diagnostic signal rather
// than silently losing user intent.
const commandChannelBuffer = 4096

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

	var err error
	switch e.Type {
	case TypeCommandToggleStar:
		e.Payload, err = decodePayload[CommandToggleStarPayload](aux.Payload)
	case TypeCommandTrashPhoto:
		e.Payload, err = decodePayload[CommandTrashPhotoPayload](aux.Payload)
	case TypeCommandLabelPhoto:
		e.Payload, err = decodePayload[CommandLabelPhotoPayload](aux.Payload)
	case TypeCommandRotatePhoto:
		e.Payload, err = decodePayload[CommandRotatePhotoPayload](aux.Payload)
	case TypeCommandUndo:
		e.Payload, err = decodePayload[CommandUndoPayload](aux.Payload)
	case TypeCommandResetMetadata:
		e.Payload, err = decodePayload[CommandResetMetadataPayload](aux.Payload)
	case TypeCommandBatch:
		e.Payload, err = decodeBatchPayload(aux.Payload)
	default:
		// For State events, we might not need to unmarshal them from history
		e.Payload = aux.Payload
	}
	return err
}

func decodePayload[T any](raw json.RawMessage) (T, error) {
	var payload T
	err := json.Unmarshal(raw, &payload)
	return payload, err
}

func decodeBatchPayload(raw json.RawMessage) (CommandBatchPayload, error) {
	payload, err := decodePayload[CommandBatchPayload](raw)
	if err != nil {
		return payload, err
	}
	for _, event := range payload.Events {
		if event.Type == TypeCommandBatch {
			return payload, fmt.Errorf("nested CommandBatch not allowed")
		}
	}
	return payload, nil
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
	PhotoID     string
	Direction   string // "left", "right", or "reset"
	OldRotation int    // previous rotation for exact undo
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
	dropped     sync.Map // map[EventType]*atomic.Int64 — dropped event counters
}

// New creates a new Event Bus.
func New() *Bus {
	return &Bus{
		subscribers: make(map[EventType][]chan Event),
	}
}

// Subscribe returns a channel that will receive events of the given type.
// Command* subscribers get a vastly larger buffer to guarantee user intent
// is never dropped under realistic interactive throughput.
func (b *Bus) Subscribe(t EventType) <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	buf := subscriberChannelBuffer
	if isCommandType(t) {
		buf = commandChannelBuffer
	}
	ch := make(chan Event, buf)
	b.subscribers[t] = append(b.subscribers[t], ch)
	return ch
}

// Publish broadcasts an event to all subscribers synchronously.
// Drops are tracked atomically; user-intent Commands drop much later (4096 buffer)
// and surface via DroppedEvents for diagnosis rather than silent loss.
func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if channels, ok := b.subscribers[e.Type]; ok {
		for _, ch := range channels {
			select {
			case ch <- e:
			default:
				// Channel full: track drop count for diagnostics.
				v, _ := b.dropped.LoadOrStore(e.Type, &atomic.Int64{})
				v.(*atomic.Int64).Add(1)
			}
		}
	}
}

// isCommandType reports whether the event type carries user intent and gets
// the larger command buffer to avoid silent loss under rapid culling.
func isCommandType(t EventType) bool {
	switch t {
	case TypeCommandToggleStar, TypeCommandTrashPhoto, TypeCommandLabelPhoto,
		TypeCommandRotatePhoto, TypeCommandUndo, TypeCommandResetMetadata, TypeCommandBatch:
		return true
	}
	return false
}

// DroppedEvents returns the number of events dropped per type since the
// last call. Calling this method resets the counters atomically.
func (b *Bus) DroppedEvents() map[EventType]int64 {
	result := make(map[EventType]int64)
	b.dropped.Range(func(key, value any) bool {
		t, tok := key.(EventType)
		counter, cok := value.(*atomic.Int64)
		if tok && cok {
			n := counter.Swap(0)
			if n > 0 {
				result[t] = n
			}
		}
		return true
	})
	return result
}
