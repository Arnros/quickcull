package bus

import (
	"encoding/json"
	"testing"
)

func TestEventUnmarshalTypedPayload(t *testing.T) {
	raw := []byte(`{"Type":"CommandLabelPhoto","Payload":{"PhotoID":"a.jpg","Label":3,"OldLabel":1}}`)
	var ev Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	p, ok := ev.Payload.(CommandLabelPhotoPayload)
	if !ok {
		t.Fatalf("expected CommandLabelPhotoPayload, got %T", ev.Payload)
	}
	if p.PhotoID != "a.jpg" || p.Label != 3 || p.OldLabel != 1 {
		t.Fatalf("unexpected payload: %+v", p)
	}
}

func TestEventUnmarshalUnknownTypeUsesRawPayload(t *testing.T) {
	raw := []byte(`{"Type":"StateUpdated","Payload":{"k":"v"}}`)
	var ev Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	payload, ok := ev.Payload.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage, got %T", ev.Payload)
	}
	if string(payload) != `{"k":"v"}` {
		t.Fatalf("unexpected raw payload: %s", string(payload))
	}
}

func TestBusSubscribeAndPublish(t *testing.T) {
	b := New()
	ch := b.Subscribe(TypeCommandToggleStar)

	want := Event{
		Type:    TypeCommandToggleStar,
		Payload: CommandToggleStarPayload{PhotoID: "a.jpg"},
	}
	b.Publish(want)

	select {
	case got := <-ch:
		if got.Type != want.Type {
			t.Fatalf("unexpected type: %v", got.Type)
		}
		p, ok := got.Payload.(CommandToggleStarPayload)
		if !ok {
			t.Fatalf("unexpected payload type: %T", got.Payload)
		}
		if p.PhotoID != "a.jpg" {
			t.Fatalf("unexpected photo id: %s", p.PhotoID)
		}
	default:
		t.Fatal("expected event to be delivered")
	}
}

func TestBusMultipleSubscribers(t *testing.T) {
	b := New()

	const numSubscribers = 5
	const numEvents = 10

	// Subscribe 5 channels to TypeCommandToggleStar.
	channels := make([]<-chan Event, numSubscribers)
	for i := 0; i < numSubscribers; i++ {
		channels[i] = b.Subscribe(TypeCommandToggleStar)
	}

	// Subscribe one channel to a different type — should receive nothing.
	otherCh := b.Subscribe(TypeCommandTrashPhoto)

	// Publish 10 events to TypeCommandToggleStar.
	for i := 0; i < numEvents; i++ {
		b.Publish(Event{
			Type:    TypeCommandToggleStar,
			Payload: CommandToggleStarPayload{PhotoID: "x.jpg"},
		})
	}

	// Each of the 5 subscribers must receive exactly 10 events.
	for i, ch := range channels {
		count := 0
		for {
			select {
			case <-ch:
				count++
			default:
				goto drained
			}
		}
	drained:
		if count != numEvents {
			t.Errorf("subscriber %d: got %d events, want %d", i, count, numEvents)
		}
	}

	// The subscriber on a different type must receive nothing.
	select {
	case ev := <-otherCh:
		t.Errorf("unexpected event on TypeCommandTrashPhoto channel: %v", ev)
	default:
		// correct: channel is empty
	}
}

func TestEventUnmarshalToggleStar(t *testing.T) {
	raw := []byte(`{"Type":"CommandToggleStar","Payload":{"PhotoID":"x.jpg","Starred":true,"OldStarred":false}}`)
	var ev Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	p, ok := ev.Payload.(CommandToggleStarPayload)
	if !ok {
		t.Fatalf("expected CommandToggleStarPayload, got %T", ev.Payload)
	}
	if p.PhotoID != "x.jpg" || !p.Starred || p.OldStarred {
		t.Fatalf("unexpected payload: %+v", p)
	}
}

func TestEventUnmarshalTrashPhoto(t *testing.T) {
	raw := []byte(`{"Type":"CommandTrashPhoto","Payload":{"PhotoID":"t.jpg","OldIsTrashed":false}}`)
	var ev Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	p, ok := ev.Payload.(CommandTrashPhotoPayload)
	if !ok {
		t.Fatalf("expected CommandTrashPhotoPayload, got %T", ev.Payload)
	}
	if p.PhotoID != "t.jpg" || p.OldIsTrashed {
		t.Fatalf("unexpected payload: %+v", p)
	}
}

func TestEventUnmarshalRotatePhoto(t *testing.T) {
	raw := []byte(`{"Type":"CommandRotatePhoto","Payload":{"PhotoID":"r.jpg","Direction":"right"}}`)
	var ev Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	p, ok := ev.Payload.(CommandRotatePhotoPayload)
	if !ok {
		t.Fatalf("expected CommandRotatePhotoPayload, got %T", ev.Payload)
	}
	if p.PhotoID != "r.jpg" || p.Direction != "right" {
		t.Fatalf("unexpected payload: %+v", p)
	}
}

func TestEventUnmarshalUndo(t *testing.T) {
	raw := []byte(`{"Type":"CommandUndo","Payload":{}}`)
	var ev Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := ev.Payload.(CommandUndoPayload); !ok {
		t.Fatalf("expected CommandUndoPayload, got %T", ev.Payload)
	}
}

func TestEventUnmarshalResetMetadata(t *testing.T) {
	raw := []byte(`{"Type":"CommandResetMetadata","Payload":{"Scope":"stars"}}`)
	var ev Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	p, ok := ev.Payload.(CommandResetMetadataPayload)
	if !ok {
		t.Fatalf("expected CommandResetMetadataPayload, got %T", ev.Payload)
	}
	if p.Scope != "stars" {
		t.Fatalf("unexpected scope: %q", p.Scope)
	}
}

func TestEventUnmarshalBatch(t *testing.T) {
	raw := []byte(`{"Type":"CommandBatch","Payload":{"Events":[
		{"Type":"CommandToggleStar","Payload":{"PhotoID":"a.jpg","Starred":true,"OldStarred":false}},
		{"Type":"CommandLabelPhoto","Payload":{"PhotoID":"b.jpg","Label":2,"OldLabel":0}}
	]}}`)
	var ev Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal batch: %v", err)
	}
	p, ok := ev.Payload.(CommandBatchPayload)
	if !ok {
		t.Fatalf("expected CommandBatchPayload, got %T", ev.Payload)
	}
	if len(p.Events) != 2 {
		t.Fatalf("expected 2 sub-events, got %d", len(p.Events))
	}
	if _, ok := p.Events[0].Payload.(CommandToggleStarPayload); !ok {
		t.Fatalf("sub-event[0] wrong type: %T", p.Events[0].Payload)
	}
	if _, ok := p.Events[1].Payload.(CommandLabelPhotoPayload); !ok {
		t.Fatalf("sub-event[1] wrong type: %T", p.Events[1].Payload)
	}
}

func TestEventUnmarshalNestedBatchRejected(t *testing.T) {
	raw := []byte(`{"Type":"CommandBatch","Payload":{"Events":[
		{"Type":"CommandBatch","Payload":{"Events":[]}}
	]}}`)
	var ev Event
	if err := json.Unmarshal(raw, &ev); err == nil {
		t.Fatal("expected error for nested CommandBatch, got nil")
	}
}

// TestBusDropsStateEventWhenChannelFull verifies that State events are dropped
// (and tracked) when the small State subscriber channel saturates, keeping the
// analyzer loop non-blocking. Command events use a much larger buffer and are
// exercised separately.
func TestBusDropsStateEventWhenChannelFull(t *testing.T) {
	b := New()
	ch := b.Subscribe(TypeStateUpdated)

	// Publish more events than the channel buffer can hold.
	total := subscriberChannelBuffer + 5
	for i := 0; i < total; i++ {
		// Publish must return immediately (non-blocking) even when the channel is full.
		b.Publish(Event{
			Type:    TypeStateUpdated,
			Payload: nil,
		})
	}

	// Exactly subscriberChannelBuffer events should be buffered; extras are dropped.
	if got := len(ch); got != subscriberChannelBuffer {
		t.Errorf("channel length = %d, want %d (extras should be dropped)", got, subscriberChannelBuffer)
	}

	dropped := b.DroppedEvents()
	if dropped[TypeStateUpdated] != 5 {
		t.Errorf("dropped StateUpdated = %d, want 5", dropped[TypeStateUpdated])
	}
}

// TestBusCommandBufferLargerThanState ensures Command subscribers get the
// larger command buffer so user intent does not get dropped under burst.
func TestBusCommandBufferLargerThanState(t *testing.T) {
	b := New()
	cmdCh := b.Subscribe(TypeCommandToggleStar)
	stateCh := b.Subscribe(TypeStateUpdated)

	if cap(cmdCh) != commandChannelBuffer {
		t.Errorf("command channel cap = %d, want %d", cap(cmdCh), commandChannelBuffer)
	}
	if cap(stateCh) != subscriberChannelBuffer {
		t.Errorf("state channel cap = %d, want %d", cap(stateCh), subscriberChannelBuffer)
	}
}

// TestBus_PublishCommandSurvivesBurst locks in the P0-1 fix behavior: Command*
// events are NEVER dropped in realistic interactive bursts. Without this
// assertion, someone can shrink commandChannelBuffer back to 100 and the only
// failure would be a `cap(ch)` mismatch — not a behavioral signal.
//
// Scenario: subscribe to one Command type, never drain it, publish 50 more
// events than the State buffer can hold (which would have dropped them under
// the old single-buffer scheme). Assert: every event is buffered (no drop) and
// DroppedEvents[CommandToggleStar] == 0.
func TestBus_PublishCommandSurvivesBurst(t *testing.T) {
	b := New()
	ch := b.Subscribe(TypeCommandToggleStar)

	burst := subscriberChannelBuffer + 50 // would have dropped at least 50 with the old buffer.
	for i := 0; i < burst; i++ {
		b.Publish(Event{
			Type:    TypeCommandToggleStar,
			Payload: CommandToggleStarPayload{PhotoID: "p.jpg"},
		})
	}

	if got := len(ch); got != burst {
		t.Errorf("command channel buffered %d events, want %d (user intent must not be dropped)", got, burst)
	}

	dropped := b.DroppedEvents()
	if dropped[TypeCommandToggleStar] != 0 {
		t.Errorf("CommandToggleStar dropped = %d, want 0 (commands never drop)", dropped[TypeCommandToggleStar])
	}
}

// TestBus_DroppedEventsAtomicReset ensures DroppedEvents works under concurrent
// publish and reset (the P2-6 atomic-counter fix).
func TestBus_DroppedEventsAtomicReset(t *testing.T) {
	b := New()
	ch := b.Subscribe(TypeStateUpdated)

	const droppedCount = 25
	for i := 0; i < subscriberChannelBuffer+droppedCount; i++ {
		b.Publish(Event{Type: TypeStateUpdated})
	}

	got := b.DroppedEvents()
	if got[TypeStateUpdated] != droppedCount {
		t.Fatalf("first call: dropped = %d, want %d", got[TypeStateUpdated], droppedCount)
	}

	// Drain channel, no new drops expected because channel is now being drained by the second batch.
	// (Drain then publish a small overflow of 3 and confirm we read back exactly 3.)
	for {
		select {
		case <-ch:
		default:
			goto overflow
		}
	}
overflow:
	for i := 0; i < subscriberChannelBuffer+3; i++ {
		b.Publish(Event{Type: TypeStateUpdated})
	}

	got2 := b.DroppedEvents()
	if got2[TypeStateUpdated] != 3 {
		t.Errorf("after reset: dropped = %d, want 3", got2[TypeStateUpdated])
	}
	if got3 := b.DroppedEvents(); got3[TypeStateUpdated] != 0 {
		t.Errorf("after second call: dropped = %d, want 0 (counters reset)", got3[TypeStateUpdated])
	}
}
