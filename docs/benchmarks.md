# Performance Benchmarks

Quickcull is tested on massive, realistic datasets to ensure a lag-free experience.

## Target Environment
- **CPU**: AMD Ryzen 7 / Intel Core i7 (8 cores)
- **RAM**: 16GB
- **Storage**: NVMe SSD (Recommended)
- **Dataset**: 30,000 Mixed Photos (RAW, JPEG, HEIC)

## Measured Metrics (v1.0 - Optimized)

| Action | Latency | Performance Note |
|---|---|---|
| **App Startup** | < 500ms | Native binary load time. |
| **Folder Open (30k items)** | < 1s | Immediate UI availability (Fast-Reopen Snapshot). |
| **Grid Scroll** | 0 dropped frames | Full virtualization (60 FPS). |
| **Shortcut Action** | < 16ms | Sub-frame latency for immediate feedback. |
| **Undo Response** | < 50ms | Constant-time O(1) state transitions. |

## Memory Usage
- **Idle**: ~120MB
- **Heavy Scan (30k photos)**: ~2.5GB (Capped by Watchdog)
- **Optimization**: Chunked state delivery eliminates the 10GB RAM spikes previously caused by massive JSON serialization on the Wails bridge.

## Background Analysis
Background workers are capped at **50% of logical CPU cores**. This ensures:
1. The main UI thread always remains responsive.
2. RAM usage remains stable during metadata extraction.
3. Thermal throttling is minimized during long triage sessions.
