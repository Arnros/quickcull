# QuickCull — Claude Instructions

## Architecture

Native desktop app: **Go 1.26+ backend** (Wails v2.12+) + **Svelte 5 frontend** (Node 26).

- **Event sourcing**: immutable `AppState`, pure `Reduce()` function, persistent undo stack (BoltDB, capped 100 entries).
- **Zero-ingestion**: no catalog, on-the-fly scan + in-memory index with O(1) lookups.
- **Authoritative Sync**: unified `SyncState` (full or partial/structural) + `SyncDelta` (metadata). Ultra-optimized chunking for large sets.
- **Background workers**: capped at 50% CPU cores, viewport-priority queue for thumbnails/EXIF.
- **Memory Safety**: automatic `runtime.GC()` watchdog (2GB threshold) + on-demand chunk building.
- **Pure Photo Focus**: image formats only; videos are strictly ignored.

## Key files

| File | Role |
|---|---|
| `internal/review/state_immutable.go` | Pure reducer + undo logic |
| `internal/review/sync_delivery.go` | Optimized chunked delivery + `RefreshVisibleOrder` |
| `internal/review/state_deltas.go` | Incremental metadata updates + persistence |
| `internal/review/event_engine.go` | Event bus subscriber loop |
| `internal/review/analysis_policy.go` | Concurrency & prioritization logic |
| `internal/review/analysis_workers.go` | Task processing loop |
| `internal/review/analysis_telemetry.go` | GC watchdog + progress reporting |
| `internal/review/app.go` | Wails-bound API methods |
| `internal/utils/logging.go` | Unified categorized logging system |
| `ui/src/lib/appState.svelte.ts` | Frontend state + actions |

## Standards

- **Logging**: use `internal/utils/logging.go` helpers (`LogNav`, `LogAnalysis`, `LogCore`).
- **Atomic saves**: metadata writes = write temp → rename.
- **Lock Discipline**: strictly follow `stateMu` → `appStateMu` → `perfMu`.
- **Concurrency**: use `atomic.Bool` for shared policy flags.
- **Payload efficiency**: never transmit `History` or `InitialState` to the UI; use `UndoLen`.
- **i18n**: all UI text via `ui/src/lib/i18n.svelte.ts`.
- **Shortcuts**: use `ShortcutService`; never hardcode keys in components.

## Validation mandatory

Before any commit: `./scripts/test-all.sh`
(Go tests + Race detector + Vitest + svelte-check + build)
