/**
 * Mirrors the Go `domain.Photo` struct.
 * Represents the per-photo metadata stored in the backend state map, keyed by
 * the photo's file path (the `ID` field).
 */
interface Photo {
	ID: string;
	IsStarred: boolean;
	Rotation: number;
	Label: number;
	IsTrashed: boolean;
}

/**
 * Mirrors the Go `review.AppState` (v2) payload pushed via the `SyncState`
 * Wails event. Carries the full authoritative state snapshot: the photo
 * metadata map, the display-ordered list of paths, and the undo history.
 *
 * `History` is typed as `unknown[]` because the backend emits opaque undo
 * records that the UI only inspects for `.length` (to enable the Undo button).
 */
export interface AppStateV2 {
	Root: string;
	CacheDir: string;
	IsPartial?: boolean;
	Photos: Record<string, Photo>;
	VisibleOrder: string[];
	TrashedCount: number;
	StarredCount: number;
	LabeledCount: number;
	RotatedCount: number;
	UndoLen: number;
}

/**
 * Mirrors the Go `review.StateDelta` payload pushed via the `SyncDelta` Wails
 * event. Carries a single-photo incremental update.
 *
 * `Changes` is typed as `Record<string, any>` because the backend sends a
 * Go `map[string]interface{}` whose values are heterogeneous (booleans,
 * numbers, and a nested `_stats` sub-object). Narrowing happens at the call
 * site in `syncService.svelte.ts`.
 *
 * @public
 */
export interface StateDelta {
	PhotoID: string;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	Changes: Record<string, any>;
}
