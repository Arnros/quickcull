package review

import (
	"fmt"
	"testing"
)

func benchmarkRefreshServer(size int) *Server {
	appState := benchmarkAppState(size)
	state := NewState("", appState.VisibleOrder)
	server := NewServer()
	server.state = state
	server.appState = appState
	return server
}

func TestRefreshVisibleOrderKeepsPhotoStoreShared(t *testing.T) {
	server := benchmarkRefreshServer(30_000)
	before := server.appState.photos

	server.RefreshVisibleOrder()

	if server.appState.photos != before {
		t.Fatal("refresh must retain the immutable photo store")
	}
	if server.appState.Photos != nil {
		t.Fatal("refresh must not materialize photos")
	}
	if got := server.appState.photoCount(); got != 30_000 {
		t.Fatalf("photo count = %d, want 30000", got)
	}
}

func BenchmarkRefreshVisibleOrderByLibrarySize(b *testing.B) {
	for _, size := range []int{1_000, 30_000, 100_000} {
		b.Run(fmt.Sprintf("%d", size), func(b *testing.B) {
			server := benchmarkRefreshServer(size)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				server.RefreshVisibleOrder()
			}
		})
	}
}
