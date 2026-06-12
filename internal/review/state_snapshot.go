package review

// BuildSyncSnapshot creates an immutable AppState payload for SyncState emission.
// It deep-copies mutable map/slice fields so UI payloads cannot observe later mutations.
// If includePhotos is false, the Photos map is excluded to prevent OOM on large libraries.
func BuildSyncSnapshot(state AppState, includePhotos bool) AppState {
	snapshot := state.Clone(includePhotos)

	// Optimisation: The UI does not need InitialState or the full History log.
	// It only needs UndoLen for UI reactivity (e.g. enabling/disabling Undo button).
	snapshot.InitialState = nil
	snapshot.History = nil
	snapshot.UndoLen = len(state.History)

	return snapshot
}

// BuildSyncSnapshotSelective creates an AppState snapshot containing only the photos in affectedPhotos.
func BuildSyncSnapshotSelective(state AppState, isPartial bool, affectedPhotos []string) AppState {
	snapshot := state.Clone(false)
	snapshot.IsPartial = isPartial
	snapshot.InitialState = nil
	snapshot.History = nil
	snapshot.UndoLen = len(state.History)

	if len(affectedPhotos) > 0 && state.Photos != nil {
		snapshot.Photos = make(map[string]Photo, len(affectedPhotos))
		for _, path := range affectedPhotos {
			if p, ok := state.Photos[path]; ok {
				snapshot.Photos[path] = p
			}
		}
	}
	return snapshot
}
