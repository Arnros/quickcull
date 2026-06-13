package review

// AppStateDTO is the data transfer object sent to the frontend.
// It explicitly hides internal domain state like History and InitialState.
type AppStateDTO struct {
	Root         string
	CacheDir     string
	IsPartial    bool             `json:"is_partial,omitempty"`
	Photos       map[string]Photo `json:"Photos,omitempty"`
	VisibleOrder []string         `json:"VisibleOrder,omitempty"`
	TrashedCount int
	StarredCount int
	LabeledCount int
	RotatedCount int
	UndoLen      int              `json:"undoLen"`
}

// BuildSyncSnapshot creates an immutable AppStateDTO payload for SyncState emission.
// It deep-copies mutable map/slice fields so UI payloads cannot observe later mutations.
// If includePhotos is false, the Photos map is excluded to prevent OOM on large libraries.
func BuildSyncSnapshot(state AppState, includePhotos bool) AppStateDTO {
	dto := AppStateDTO{
		Root:         state.Root,
		CacheDir:     state.CacheDir,
		IsPartial:    state.IsPartial,
		TrashedCount: state.TrashedCount,
		StarredCount: state.StarredCount,
		LabeledCount: state.LabeledCount,
		RotatedCount: state.RotatedCount,
		UndoLen:      len(state.History),
	}

	if state.VisibleOrder != nil {
		dto.VisibleOrder = make([]string, len(state.VisibleOrder))
		copy(dto.VisibleOrder, state.VisibleOrder)
	}

	if includePhotos && state.Photos != nil {
		dto.Photos = make(map[string]Photo, len(state.Photos))
		for k, v := range state.Photos {
			dto.Photos[k] = v
		}
	}
	return dto
}

// BuildSyncSnapshotSelective creates an AppStateDTO snapshot containing only the photos in affectedPhotos.
func BuildSyncSnapshotSelective(state AppState, isPartial bool, affectedPhotos []string) AppStateDTO {
	dto := AppStateDTO{
		Root:         state.Root,
		CacheDir:     state.CacheDir,
		IsPartial:    isPartial,
		TrashedCount: state.TrashedCount,
		StarredCount: state.StarredCount,
		LabeledCount: state.LabeledCount,
		RotatedCount: state.RotatedCount,
		UndoLen:      len(state.History),
	}

	if state.VisibleOrder != nil {
		dto.VisibleOrder = make([]string, len(state.VisibleOrder))
		copy(dto.VisibleOrder, state.VisibleOrder)
	}

	if len(affectedPhotos) > 0 && state.Photos != nil {
		dto.Photos = make(map[string]Photo, len(affectedPhotos))
		for _, path := range affectedPhotos {
			if p, ok := state.Photos[path]; ok {
				dto.Photos[path] = p
			}
		}
	}
	return dto
}
