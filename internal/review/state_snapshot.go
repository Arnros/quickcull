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
