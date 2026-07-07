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

Quickcull automatically adapts to QWERTY or AZERTY keyboards. You can remap these at any time in **Settings > Shortcuts**.

### Navigation
| Action | QWERTY | AZERTY |
|---|---|---|
| **Previous Photo** | `Q` / `←` / `Backspace` | `A` / `←` / `Backspace` |
| **Next Photo** | `D` / `→` / `Space` | `D` / `→` / `Space` |
| **Up / Down (Grid)**| `↑` / `↓` | `↑` / `↓` |
| **First / Last** | `Home` / `End` | `Home` / `End` |

### Culling & Actions
| Action | Key | Description |
|---|---|---|
| **Star** | `S` | Toggle star rating |
| **Labels 1-5** | `1`-`5` | Set color label (e.g. 1=Red, 5=Purple) |
| **Clear Label** | `0` | Remove label |
| **Trash** | `X` / `Del` | Move to local `.trash/` folder |
| **Undo** | `U` | Instantly revert last action |

### View & Interface
| Action | Key | Description |
|---|---|---|
| **Toggle Grid** | `V` / `Enter` | Switch between single photo and grid view |
| **Side-by-Side** | `C` | Compare multiple photos simultaneously |
| **Zoom** | `Z` | Toggle 100% zoom |
| **Zen Mode** | `H` | Hide all UI elements |
| **Toggle Sidebar**| `Tab` | Show/hide left folder tree |
| **Toggle Info** | `I` | Show/hide EXIF metadata panel |
| **Filter Bar** | `F` | Open smart filter bar |
| **Refresh / Sync**| `F5` | Manually rescan folder |
| **Close / Exit** | `Escape` | Close modals, clear selection, or exit views |

### Image Rotation
| Action | Key | Description |
|---|---|---|
| **Rotate Left** | `L` | Visual rotation (-90°) |
| **Rotate Right** | `R` | Visual rotation (+90°) |
| **Reset Rotation**| `O` | Reset visual rotation to original |
| **Write EXIF** | `W` | Permanently write rotation to image file |

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
- [Development Mandates](CLAUDE.md): Project engineering standards.

## 📄 License

MIT - Built with ❤️ for photographers.

---

## 🔮 Planned Improvements (Photographer Perspective)

Quickcull is a **culling tool first** — fast triage before you move to your editor. Areas where the pipeline can evolve for professional workflows:

### Color Management (ICC Profiles)
- **Current**: Thumbnails and converted previews are decoded without ICC profile awareness. Images in Adobe RGB or ProPhoto RGB color spaces may appear desaturated or with wrong tones.
- **Plan**: Embed output-referred profiles (sRGB) in generated thumbnails and adopt a color-aware decoding pipeline for wide-gamut originals.

### EXIF & Metadata Completeness
- **Missing fields**: GPS coordinates (latitude, longitude, altitude), lens model (`LensModel`), and full camera make+model are not extracted.
- **Missing formats**: IPTC/XMP sidecar files (`.xmp`, Photo Mechanic ratings/labels) are not read.
- **Plan**: Extend `EXIFInfo` to include GPS, lens, and XMP sidecar support.

### RAW Format Coverage
- **Current**: Supports 13 RAW extensions but missing Sony `.sr2`, Phase One `.iiq`, Hasselblad `.3fr/.fff`, Sigma `.x3f`, Canon `.crw`, GoPro `.gpr`, Epson `.erf`, and modern formats AVIF (`.avif`) and JPEG XL (`.jxl`).
- **Plan**: Add missing extensions to the scanner and handle them through the existing exiftool-backed preview pipeline.

### Rotation Safety
- **Current**: `Apply Rotation` (`W` key) writes EXIF orientation directly via `exiftool -overwrite_original`, modifying the original file with no backup. Tags could be lost if exiftool encounters an unsupported EXIF structure.
- **Plan**: Add an optional backup step (`_original` copy) before writing, and warn the user when a rotation commit is about to modify originals.

### Thumbnail & Preview Quality
- **Current**: Thumbnails use the `Box` filter (fastest, lowest quality) at 240px fixed. No sharpening pass. No 2× variant for HiDPI displays.
- **Plan**: Offer `Lanczos` resampling, optional unsharp mask, and 2× thumbnail variants for Retina screens.

### Culling Metrics
- **Current**: No culling-rate counter (images/hour) visible in the UI.
- **Plan**: Add a session-timer and rate indicator to help photographers pace long culling sessions.

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines.
