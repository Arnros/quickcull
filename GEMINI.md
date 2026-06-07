# Development Mandates for quickcull

This document defines the foundational standards for **quickcull**. These instructions take absolute precedence over general workflows.

## Skill Reference (Global)
This project follows the standards defined in:
1.  **`go` Skill**: Go expert profile (Idioms, Performance, Clean Code).
2.  **`sysec.md` Skill**: System and application security standards.
3.  **`ux.md` Skill**: User experience and interface design principles.

### Fundamental Principles
- **Native Desktop App**: Go 1.26+ backend (Wails v2.12+) + Svelte 5 frontend (Node 26).
- **High Performance**: 
    - **Zero-Ingestion**: Fast scanning + BoltDB caching.
    - **Memory Safety**: `sync.Pool` for buffers, 2GB GC watchdog, on-demand chunked state delivery.
    - **Capped Analysis**: Background workers limited to 50% CPU cores.
- **Authoritative State**:
    - **Event Sourcing**: Immutable `AppState` transitioned via `Reduce(state, event)`.
    - **Unified Sync**: Standardize on `SyncState` (supports `IsPartial: true` for structural updates) and `SyncDelta`.
    - **Payload Efficiency**: Never transmit internal backend state (`History`, `InitialState`) to the UI.
- **Modularity**:
    - Avoid "God files". Server logic split into `sync_delivery`, `state_deltas`, `event_engine`, and `analysis_*` modules.
    - Strictly follow Lock Discipline: `stateMu` → `appStateMu` → `perfMu`.
- **Safety**:
    - **Atomic Writes**: Temp file then rename for all metadata saves.
    - **Double Verification**: Validate both index and filename for all destructive actions.
- **Interface & UX**:
    - Reflect exact backend state.
    - i18n via `ui/src/lib/i18n.svelte.ts`.
    - Dynamic shortcuts via `ShortcutService`.

---

## Workspace-Specific Instructions
- **Build Tags**: Linux builds MUST include `-tags webkit2gtk_4_1`.
- **README Maintenance**: Update systematically after core changes.
- **Validation**: You MUST run `./scripts/test-all.sh` (includes Go race detector) before considering a task complete.

---
*This document is a foundational mandate for Gemini CLI.*
