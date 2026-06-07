# User Guide: quickcull V1.0

This guide explains how to get the most out of **quickcull** for high-speed photo selection.

## 🏁 Your First Launch

When you open `quickcull` for the first time, you are greeted with the **Picker**.
- **Open Folder**: Click the "Browse" button to select your photo directory.
- **Onboarding**: The app will automatically detect your keyboard layout (AZERTY or QWERTY) to provide the most intuitive navigation keys immediately.

## ⌨️ High-Speed Navigation

The core of quickcull is its **keyboard-first workflow**.

### Basic Keys
- **Next/Prev**: Use `Q`/`D` (QWERTY) or `A`/`D` (AZERTY). Arrow keys also work.
- **Toggle View**: `V` for Grid, `G` for Filmstrip, `I` for Info panel.
- **Zen Mode**: Press `H` to hide everything except the photo.

### Culling Actions
- **Star (`S`)**: Mark your best shots.
- **Labels (0 to 5)**: Categorize your photos by color.
- **Trash (`X` or `Delete`)**: Move unwanted photos to the local `.trash/` folder.
- **Undo (`U`)**: Instantly reverse your last action (Trash, Star, Label, Rotation). History is **persistent** across application sessions.

## ⚙️ Customizing Your Experience

Press `?` or click the Help button to see your current mappings. Go to **Settings** to customize them.

### Tabbed Settings
1. **General**: Manage language, theme, and duplicate detection sensitivity.
2. **Shortcuts**: Click any action, then press a key to remap it. Quickcull handles conflicts and saves them locally.
3. **Performance Debug**: (Visible in Settings) Monitor background analysis queue depth, worker count, and view-ready latency in real-time.

### Side-by-Side Comparison (`C`)
If you have similar photos, select one and press `C` to enter comparison mode. Navigate through similar groups and pick the sharpest one. You can collapse or expand duplicate groups using the dedicated buttons in the grid view.

### Smart Filters (`F`)
Press `F` to open the filter bar. This allows you to instantly isolate your best shots (`S`), specific labels (e.g., Green/5), or groups of duplicates.

### EXIF Rotation Write (`W`)
Visual rotation is fast, but sometimes you want to write it permanently to the file.
1. Rotate visually with `L` or `R`.
2. Press `W` to write the rotation to the EXIF data (supported for JPEG, HEIC, PNG).

### Smart Export
Quickcull does not alter your original files (except for EXIF rotation and moving to trash). When your selection is complete, use the **Export** feature (via the UI or shortcut) to copy or move your Starred or Labeled photos to a final destination for editing (e.g., Lightroom).

**Features:**
- **Live Progress**: A real-time counter shows exactly which file is being processed.
- **Cancellation**: You can stop a long export at any time without freezing the app.
- **Safety**: Quickcull ensures destination files are not overwritten by automatically adding a `_copy` suffix if a name conflict occurs.

## 🛡️ Privacy & Safety
- **Local Only**: No data is sent to any server.
- **Non-Destructive Trash**: Files moved to `.trash/` can be restored via the Maintenance tab or manually in your file explorer.
