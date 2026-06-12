import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('./api', () => ({
  api: {
    trash: vi.fn(),
    undo: vi.fn(),
    toggleStar: vi.fn(),
    setLabel: vi.fn(),
    getStarredIndices: vi.fn(),
    getFile: vi.fn(),
    getFolders: vi.fn(),
    getFilters: vi.fn(),
  },
}));

vi.mock('./i18n.svelte', () => ({
  i18n: {
    t: (key: string) => key,
  },
}));

vi.mock('./logger', () => ({
  logger: {
    info: vi.fn(),
    warn: vi.fn(),
    debug: vi.fn(),
    error: vi.fn(),
  },
}));

vi.mock('./toast.svelte', () => ({
  toastService: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

vi.mock('./watchService.svelte', () => ({
  watchService: {
    start: vi.fn(),
  },
}));

vi.mock('./viewState.svelte', () => ({
  viewState: {
    current: 'review',
    config: {},
    sidebarOpen: false,
    filmstripOpen: false,
    infoOpen: false,
    gridOpen: false,
    settingsOpen: false,
    helpOpen: false,
    zoomed: false,
    zenMode: false,
    comparisonMode: false,
    toggleTheme: vi.fn(),
  },
}));

vi.mock('./filterState.svelte', () => ({
  filterState: {
    filterBarOpen: false,
    filterMode: 'none',
    activeLabelFilter: 0,
    duplicateGroups: [],
    filteredIndices: [],
    filters: { cameras: [], isos: [] },
    activeFilters: {},
    loadFilters: vi.fn(),
    updateFilteredIndices: vi.fn(),
    setFilter: vi.fn(),
    clearFilters: vi.fn(),
  },
}));

vi.mock('./navigationService.svelte', () => ({
  navigationService: {
    currentIndex: 0,
    comparisonIndex: 0,
    selectionPivot: 0,
    currentFile: null,
    referenceFile: null,
    loadFile: vi.fn(async (index: number) => index),
    select: vi.fn(),
    next: vi.fn(),
    prev: vi.fn(),
    persistPositionNow: vi.fn(),
  },
}));

import { api } from './api';
import { appState } from './appState.svelte';
import { navigationService } from './navigationService.svelte';

describe('AppState cache-busting on media-changing actions', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (api.getStarredIndices as any).mockResolvedValue({ indices: [] });
    (api.getFile as any).mockImplementation(async (index: number) => ({
      index,
      filename: index === 1 ? 'b.jpg' : 'a.jpg',
      type: 'image',
      format: 'JPEG',
      total: 2,
      folder: '.',
      starred: false,
      rotation: 0,
      label: 0,
    }));
    appState.sessionVersion = 1;
    appState.stats = {
      total: 2,
      initialTotal: 3,
      trashedCount: 1,
      starredCount: 0,
      labeledCount: 0,
      rotatedCount: 0,
      undoLen: 1,
      savedPosition: 0,
      heicSupported: false,
    } as any;
    appState.currentFile = {
      filename: 'a.jpg',
      type: 'image',
      format: 'JPEG',
      index: 0,
      total: 2,
      folder: '.',
      starred: false,
      rotation: 0,
      label: 0,
    } as any;
    appState.currentIndex = 0;
    appState.selectedIndices = [0];
    appState.filterMode = 'none';
    appState.activeFilters = {};
    appState.comparisonMode = false;
    appState.gridColumns = 5;
    appState.v2 = {
      VisibleOrder: ['a.jpg', 'b.jpg'],
      Photos: {
        'a.jpg': { ID: 'a.jpg', IsStarred: false, Rotation: 0, Label: 0, IsTrashed: false },
        'b.jpg': { ID: 'b.jpg', IsStarred: false, Rotation: 0, Label: 0, IsTrashed: false },
      },
      Root: '',
      CacheDir: '',
      TrashedCount: 0,
      StarredCount: 0,
      LabeledCount: 0,
      RotatedCount: 0,
      UndoLen: 0,
    };

  });

  it('increments sessionVersion after trash', async () => {
    (api.trash as any).mockResolvedValue({
      index: 0,
      total: 1,
      stats: {
        total: 1,
        initialTotal: 3,
        trashedCount: 2,
        starredCount: 0,
        labeledCount: 0,
        rotatedCount: 0,
        undoLen: 2,
        savedPosition: 0,
        heicSupported: false,
      },
    });

    appState.selectedIndices = [];
    const before = appState.sessionVersion;
    await appState.trash();

    await vi.waitFor(() => {
      expect(appState.sessionVersion).toBeGreaterThan(before);
    });
  });

  it('increments sessionVersion after undo', async () => {
    (api.undo as any).mockResolvedValue({
      index: 0,
      actionType: 'trash',
      stats: {
        total: 2,
        initialTotal: 3,
        trashedCount: 1,
        starredCount: 0,
        labeledCount: 0,
        rotatedCount: 0,
        undoLen: 1,
        savedPosition: 0,
        heicSupported: false,
      },
    });

    const before = appState.sessionVersion;
    await appState.undo();

    await vi.waitFor(() => {
      expect(navigationService.loadFile).toHaveBeenCalled();
    });
    expect(appState.sessionVersion).toBeGreaterThan(before);
  });

  it('loads folders and filters when undoing a trash action', async () => {
    const loadFoldersSpy = vi.spyOn(appState, 'loadFolders').mockResolvedValue(undefined);
    const loadFiltersSpy = vi.spyOn(appState, 'loadFilters').mockResolvedValue(undefined);

    (api.undo as any).mockResolvedValue({
      index: 0,
      actionType: 'trash',
      stats: {
        total: 2,
        initialTotal: 3,
        trashedCount: 1,
        starredCount: 0,
        labeledCount: 0,
        rotatedCount: 0,
        undoLen: 1,
        savedPosition: 0,
        heicSupported: false,
      },
    });

    await appState.undo();

    await vi.waitFor(() => {
      expect(loadFoldersSpy).toHaveBeenCalled();
      expect(loadFiltersSpy).toHaveBeenCalled();
    });
  });

  it('trashes explicitly selected single file when different from current', async () => {
    (api.trash as any).mockResolvedValue({
      index: 0,
      total: 1,
      stats: {
        total: 1,
        initialTotal: 3,
        trashedCount: 2,
        starredCount: 0,
        labeledCount: 0,
        rotatedCount: 0,
        undoLen: 2,
        savedPosition: 0,
        heicSupported: false,
      },
    });

    appState.selectedIndices = [1];
    await appState.trash();

    await vi.waitFor(() => {
      expect(api.trash).toHaveBeenCalled();
    });
    expect(api.trash).toHaveBeenCalledWith(1, 'b.jpg', ['b.jpg']);
  });

  it('clears reference file when leaving comparison mode', async () => {
    appState.comparisonMode = true;
    appState.referenceFile = { filename: 'ref.jpg' } as any;

    await appState.toggleComparisonMode();

    expect(appState.comparisonMode).toBe(false);
    expect(appState.referenceFile).toBeNull();
  });

  it('exits duplicates mode consistently', () => {
    appState.filterMode = 'duplicates' as any;
    appState.gridOpen = true;
    appState.comparisonMode = true;
    appState.referenceFile = { filename: 'ref.jpg' } as any;
    appState.filteredIndices = [1, 2];
    appState.duplicateGroups = [[1, 2]];

    appState.exitDuplicatesMode();

    expect(appState.filterMode).toBe('none');
    expect(appState.gridOpen).toBe(false);
    expect(appState.comparisonMode).toBe(false);
    expect(appState.referenceFile).toBeNull();
    expect(appState.filteredIndices).toEqual([]);
    expect(appState.duplicateGroups).toEqual([]);
  });

  it('keeps grid open when disabling duplicates from grid view', async () => {
    appState.filterMode = 'duplicates' as any;
    appState.gridOpen = true;
    appState.comparisonMode = false;
    appState.filteredIndices = [1, 2];
    appState.duplicateGroups = [[1, 2]];

    await appState.toggleDuplicatesFilter();

    expect(appState.filterMode).toBe('none');
    expect(appState.gridOpen).toBe(true);
    expect(appState.filteredIndices).toEqual([]);
    expect(appState.duplicateGroups).toEqual([]);
  });

  it('captures scheduler telemetry from progress payload', () => {
    appState.onAnalysisProgress({
      current: 42,
      total: 100,
      scanning: true,
      scheduler_mode: 'active',
      nav_promotion_total: 1234,
      view_ready_latency: 27,
      view_ready_latency_p50: 41,
      active_mode_ms: 5000,
      idle_mode_ms: 1200,
    });

    expect(appState.perf.schedulerMode).toBe('active');
    expect(appState.perf.navPromotionTotal).toBe(1234);
    expect(appState.perf.viewReadyLatencyMs).toBe(27);
    expect(appState.perf.viewReadyP50Ms).toBe(41);
    expect(appState.perf.activeModeMs).toBe(5000);
    expect(appState.perf.idleModeMs).toBe(1200);
    expect(appState.isScanning).toBe(true);
  });

  it('keeps grid open when disabling starred filter from grid view', async () => {
    appState.filterMode = 'starred' as any;
    appState.gridOpen = true;
    appState.comparisonMode = false;
    appState.filteredIndices = [1, 2];

    await appState.toggleStarFilter();

    expect(appState.filterMode).toBe('none');
    expect(appState.gridOpen).toBe(true);
    expect(appState.filteredIndices).toEqual([]);
  });

  it('keeps grid open when disabling label filter from grid view', async () => {
    appState.filterMode = 'label' as any;
    appState.activeLabelFilter = 3 as any;
    appState.gridOpen = true;
    appState.comparisonMode = false;
    appState.filteredIndices = [1, 2];

    await appState.setLabelFilter(3);

    expect(appState.filterMode).toBe('none');
    expect(appState.gridOpen).toBe(true);
    expect(appState.filteredIndices).toEqual([]);
  });

  it('disables EXIF apply when runtime capability exifWrite is false', () => {
    appState.currentFile = {
      filename: 'a.jpg',
      type: 'image',
      format: 'JPEG',
      index: 0,
      total: 2,
      folder: '.',
      starred: false,
      rotation: 90,
      label: 0,
    } as any;
    appState.runtimeCapabilities = {
      rawPreview: true,
      rawMetadata: true,
      heicDecode: true,
      exifWrite: false,
    } as any;

    expect(appState.canApplyExifWrite()).toBe(false);
    expect(appState.canApplyRotation()).toBe(false);
  });

  it('allows EXIF apply for writable format when capability is true', () => {
    appState.currentFile = {
      filename: 'a.heic',
      type: 'image',
      format: 'HEIC',
      index: 0,
      total: 2,
      folder: '.',
      starred: false,
      rotation: 90,
      label: 0,
    } as any;
    appState.runtimeCapabilities = {
      rawPreview: true,
      rawMetadata: true,
      heicDecode: true,
      exifWrite: true,
    } as any;

    expect(appState.canApplyExifWrite()).toBe(true);
    expect(appState.canApplyRotation()).toBe(true);
  });

  it('uses format fallback while runtime capabilities are not loaded yet', () => {
    appState.currentFile = {
      filename: 'a.jpg',
      type: 'image',
      format: 'JPEG',
      index: 0,
      total: 2,
      folder: '.',
      starred: false,
      rotation: 90,
      label: 0,
    } as any;
    appState.runtimeCapabilities = null;

    expect(appState.canApplyExifWrite()).toBe(true);
    expect(appState.canApplyRotation()).toBe(true);
  });

  it('keeps multi-selection visible after batch star action', async () => {
    (api.toggleStar as any).mockResolvedValue({});
    appState.selectedIndices = [0, 1];
    appState.currentIndex = 0;
    appState.v2 = {
      ...appState.v2!,
      VisibleOrder: ['a.jpg', 'b.jpg'],
      Photos: {
        'a.jpg': { ID: 'a.jpg', IsStarred: false, Rotation: 0, Label: 0, IsTrashed: false },
        'b.jpg': { ID: 'b.jpg', IsStarred: false, Rotation: 0, Label: 0, IsTrashed: false },
      },
    } as any;

    await appState.toggleStar();

    expect(api.toggleStar).toHaveBeenCalledWith(undefined, undefined, ['a.jpg', 'b.jpg'], true);
    expect(appState.selectedIndices).toEqual([0, 1]);
  });

  it('keeps multi-selection visible after batch label action', async () => {
    (api.setLabel as any).mockResolvedValue({});
    appState.selectedIndices = [0, 1];
    appState.currentIndex = 0;
    appState.v2 = {
      ...appState.v2!,
      VisibleOrder: ['a.jpg', 'b.jpg'],
      Photos: {
        'a.jpg': { ID: 'a.jpg', IsStarred: false, Rotation: 0, Label: 0, IsTrashed: false },
        'b.jpg': { ID: 'b.jpg', IsStarred: false, Rotation: 0, Label: 0, IsTrashed: false },
      },
    } as any;

    await appState.setLabel(3);

    expect(api.setLabel).toHaveBeenCalledWith(undefined, undefined, ['a.jpg', 'b.jpg'], 3);
    expect(appState.selectedIndices).toEqual([0, 1]);
  });

  it('disables EXIF apply for non-writable format even when capability is true', () => {
    appState.currentFile = {
      filename: 'a.gif',
      type: 'image',
      format: 'GIF',
      index: 0,
      total: 2,
      folder: '.',
      starred: false,
      rotation: 90,
      label: 0,
    } as any;
    appState.runtimeCapabilities = {
      rawPreview: true,
      rawMetadata: true,
      heicDecode: true,
      exifWrite: true,
    } as any;

    expect(appState.canApplyExifWrite()).toBe(false);
    expect(appState.canApplyRotation()).toBe(false);
  });
});
