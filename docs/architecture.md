# Technical Architecture

`quickcull` is built for low-latency triage of massive photo collections.

## Core Pillars

### 0. RAW/HEIC/TIFF Thumbnail Pipeline
All non-native formats (DNG, CR2, ARW, HEIF, TIFF…) are routed through a single unified function `getThumbnailFromConverted()` in `thumbnail.go`:
1. If a processed JPEG already exists in cache, use it directly.
2. Otherwise call `ConvertRAW`/`ConvertHEIC`/`ConvertTIFF` (exiftool-based) to produce a JPEG.
3. Extract thumbnail from the produced JPEG.

### 1. Zero-Ingestion Pipeline
- **On-the-fly Scanning**: Files are discovered in background workers.
- **Ephemeral Indexing**: The backend maintains a fast in-memory index with $O(1)$ lookups.
- **BoltDB Persistence**: Stars, labels, and undo history are stored in a local `_metadata.db` for portability.
- **Position Caching**: Last viewed indices are cached in memory (with `sync.RWMutex`) to eliminate redundant DB reads during state updates.

### 2. Modular Backend (Go)
The server logic is decomposed into focused modules to ensure maintainability (Go 1.26 / Wails 2.12):
- **`sync_delivery`**: authoritative state synchronization using an ultra-optimized chunked protocol and the `broadcastAppState` helper.
- **`state_deltas`**: lightweight, incremental metadata updates.
- **`event_engine`**: reactive core based on a `Reduce(state, event)` pattern.
- **`analysis_policy`**: dynamic concurrency and prioritization logic based on UI activity.
- **`analysis_workers`**: parallel task execution with CPU resource capping (50% cores).
- **`analysis_telemetry`**: real-time performance monitoring and GC watchdog.

All UI-facing responses use the `App.snapshotStats` helper to provide a consistent, thread-safe view of application metrics.

### 3. Reactive Frontend (Svelte 5)
- **Runes**: Leverage Svelte 5's `$state` and `$derived` (Node 26).
- **Virtualization**: Handle 30,000+ items with constant memory usage.
- **Authoritative Sync**: The UI state is a reflection of the backend's immutable state snapshot.

## Data Flow

1. **User opens folder** -> Backend hydrates from `FolderSnapshot` for instant UI availability.
2. **Scanner** -> Streams new files into the authoritative state.
3. **SyncState** -> Authoritative update sent to UI. Supports `IsPartial: true` for structural-only changes (`RefreshVisibleOrder`). 
4. **SyncDelta** -> Incremental update for metadata changes (Star/Label).
5. **Double Verification** -> Every destructive action validates the filename against the current index to prevent desync errors.

## Memory Optimizations

To handle 30k+ photo libraries on 8GB/16GB machines:
1. **On-Demand Chunking**: `SyncFullState` builds chunks dynamically under short locks instead of duplicating the entire `Photos` map, halving the memory spike during synchronization.
2. **Payload Reduction**: Backend-only fields (`History`, `InitialState`) are omitted from UI snapshots. Only `UndoLen` is transmitted.
3. **GC Watchdog**: A dedicated background process triggers `runtime.GC()` if heap usage exceeds 2GB.
4. **Strict Media Focus**: Video files are ignored during discovery to avoid overhead from heavy containers.
5. **Lock Discipline**: Consistent `stateMu` → `appStateMu` → `perfMu` ordering prevents deadlocks and ensures stable snapshots.
