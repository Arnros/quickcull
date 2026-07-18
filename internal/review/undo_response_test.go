package review

import (
	"testing"

	"quickcull/internal/bus"
)

func TestUndoResponseTarget(t *testing.T) {
	tests := []struct {
		name       string
		event      bus.Event
		actionType string
		photoID    string
		index      int
	}{
		{
			name:       "trash",
			event:      bus.Event{Payload: bus.CommandTrashPhotoPayload{PhotoID: "trash.jpg", OriginalIndex: 7}},
			actionType: "trash",
			photoID:    "trash.jpg",
			index:      7,
		},
		{
			name:       "star",
			event:      bus.Event{Payload: bus.CommandToggleStarPayload{PhotoID: "star.jpg"}},
			actionType: "star",
			photoID:    "star.jpg",
			index:      -1,
		},
		{
			name:       "label",
			event:      bus.Event{Payload: bus.CommandLabelPhotoPayload{PhotoID: "label.jpg"}},
			actionType: "label",
			photoID:    "label.jpg",
			index:      -1,
		},
		{
			name:       "rotate",
			event:      bus.Event{Payload: bus.CommandRotatePhotoPayload{PhotoID: "rotate.jpg"}},
			actionType: "rotate",
			photoID:    "rotate.jpg",
			index:      -1,
		},
		{
			name: "batch action from first event",
			event: bus.Event{Payload: bus.CommandBatchPayload{Events: []bus.Event{
				{Payload: bus.CommandLabelPhotoPayload{PhotoID: "first.jpg"}},
				{Payload: bus.CommandTrashPhotoPayload{PhotoID: "second.jpg", OriginalIndex: 3}},
			}}},
			actionType: "label",
			index:      0,
		},
		{
			name:  "empty batch",
			event: bus.Event{Payload: bus.CommandBatchPayload{}},
		},
		{
			name:  "unknown payload",
			event: bus.Event{Payload: struct{}{}},
			index: -1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actionType, photoID, index := undoResponseTarget(test.event)
			if actionType != test.actionType || photoID != test.photoID || index != test.index {
				t.Fatalf("undo target = (%q, %q, %d), want (%q, %q, %d)", actionType, photoID, index, test.actionType, test.photoID, test.index)
			}
		})
	}
}
