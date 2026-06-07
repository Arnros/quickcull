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

func TestBusDropsEventWhenChannelFull(t *testing.T) {
	b := New()
	ch := b.Subscribe(TypeCommandToggleStar)

	// Publish more events than the channel buffer can hold.
	total := subscriberChannelBuffer + 5
	for i := 0; i < total; i++ {
		// Publish must return immediately (non-blocking) even when the channel is full.
		b.Publish(Event{
			Type:    TypeCommandToggleStar,
			Payload: CommandToggleStarPayload{PhotoID: "y.jpg"},
		})
	}

	// Exactly subscriberChannelBuffer events should be buffered; extras are dropped.
	if got := len(ch); got != subscriberChannelBuffer {
		t.Errorf("channel length = %d, want %d (extras should be dropped)", got, subscriberChannelBuffer)
	}
}
