package review

import (
	"fmt"
	"testing"

	"quickcull/internal/bus"
)

func benchmarkAppState(size int) *AppState {
	photos := make(map[string]Photo, size)
	order := make([]string, size)
	for i := range order {
		id := fmt.Sprintf("p%06d", i)
		order[i] = id
		photos[id] = Photo{ID: id}
	}
	return &AppState{VisibleOrder: order, photos: newPhotoStore(photos)}
}

func BenchmarkReduceToggleStarByLibrarySize(b *testing.B) {
	for _, size := range []int{1000, 30000, 100000} {
		b.Run(fmt.Sprintf("%d", size), func(b *testing.B) {
			state := benchmarkAppState(size)
			event := bus.Event{Payload: bus.CommandToggleStarPayload{PhotoID: state.VisibleOrder[0], Starred: true}}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				next, _, err := Reduce(state, event)
				if err != nil {
					b.Fatal(err)
				}
				state = next
			}
		})
	}
}
