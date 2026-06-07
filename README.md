# quickcull

**Ultra-fast local photo culling for high-volume photographers.**

No ingest, no cloud, no heavy database. Just raw speed.

[![Release](https://img.shields.io/github/v/release/Arnros/quickcull)](https://github.com/Arnros/quickcull/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## ⚡ Why quickcull?

Most photo managers are editors first. `quickcull` is a **selection tool** first.
- **Zero-Ingestion**: Open a folder with 30,000 photos and start reviewing in seconds.
- **Authoritative Sync**: Unified `SyncState` protocol with ultra-optimized chunked delivery.
- **Keyboard-Centric**: AZERTY/QWERTY detection. Keep your hands on the keys.
- **Smart Tech**: Go 1.26 backend (Wails 2.12) + Svelte 5 (Node 26) frontend.
- **Modular Architecture**: Cleanly decoupled event-driven core for industrial reliability.

## ✨ Core Features

- **Instant Preview**: High-speed background analysis for RAW, HEIC, and JPEG.
- **Massive Collection Support**: Optimized for stable memory usage on 30k+ photo libraries.
- **Pro Triage**: Stars (`S`), Color Labels (`0-5`), and non-destructive Trash (`X`).
- **Duplicate Detection**: Find and compare similar photos side-by-side (`C`).
- **Industrial Persistence**: All metadata and undo history stored in a local high-performance BoltDB database.
- **Fully Internationalized**: English and French native support.

## 🚀 Quick Start

1. **Launch** the app.
2. **Select** a folder.
3. **Review**: Use `Q/D` (detected for your layout) or arrows to navigate.
4. **Cull**: Mark with `S` (Star), `1-5` (Label), or `X` (Trash).
5. **Finalize**: Review your `.trash/` folder before clearing.

## ⌨️ Keyboard Shortcuts (Default)

| Layout | Previous | Next | Star | Trash |
|---|---|---|---|---|
| **QWERTY** | `Q` / `←` | `D` / `→` | `S` | `X` |
| **AZERTY** | `A` / `←` | `D` / `→` | `S` | `X` |

*All keys are fully customizable in **Settings > Shortcuts**.*

## 🛡️ Reliability & Security

- **Thread-Safe Core**: Strict lock discipline and atomic operations for concurrent processing.
- **Atomic Saves**: metadata is saved using a temp-and-rename strategy.
- **Double Verification**: Backend validates filenames against indices for every action.
- **Memory Watchdog**: Proactive GC management ensures RAM stability on low-resource machines.

## 🏗️ Technical Foundation

- **Backend**: Go 1.26 (Wails V2.12).
- **Frontend**: Svelte 5 (Node 26), TypeScript.
- **Storage**: BoltDB (local metadata & cache).
- **Metadata**: ExifTool integration.

## 🛠️ Build From Source

```bash
# Clone and install dependencies
git clone https://github.com/Arnros/quickcull.git
cd quickcull

# Development mode
wails dev

# Production build (Linux)
wails build -tags webkit2gtk_4_1 -ldflags "-s -w"
```

## 📖 Documentation

- [User Guide](docs/usage.md): Full walkthrough of features and workflows.
- [Architecture](docs/architecture.md): Deep dive into the modular engine.
- [Benchmarks](docs/benchmarks.md): Speed results on massive datasets.
- [Development Mandates](GEMINI.md): Project engineering standards.

## 📄 License

MIT - Built with ❤️ for photographers.
