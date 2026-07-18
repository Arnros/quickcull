import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';

const { appEventHandlers } = vi.hoisted(() => ({
  appEventHandlers: {} as Record<string, (data?: unknown) => void | Promise<void>>,
}));

vi.mock('./api', () => ({
  api: {
	  onEvent: vi.fn((name: string, handler: (data?: unknown) => void | Promise<void>) => {
	    appEventHandlers[name] = handler;
	  }),
    trash: vi.fn(),
    undo: vi.fn(),
    toggleStar: vi.fn(),
    setLabel: vi.fn(),
    rotate: vi.fn(),
    rotateReset: vi.fn(),
    applyRotation: vi.fn(),
    reanalyzeMetadata: vi.fn(),
    updateConfig: vi.fn(),
    getStarredIndices: vi.fn(),
    getFile: vi.fn(),
    getFolders: vi.fn(),
    getFilters: vi.fn(),
    refresh: vi.fn(),
	  browseDialog: vi.fn(),
	  exportFiles: vi.fn(),
	  exportSelection: vi.fn(),
	  getConfig: vi.fn(),
	  getStats: vi.fn(),
	  getAppState: vi.fn(),
	  sysCheck: vi.fn(),
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
    show: vi.fn(),
    info: vi.fn(),
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
import { filterState } from './filterState.svelte';
import { viewState } from './viewState.svelte';

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
    filterState.filterMode = 'none';
    filterState.activeFilters = {};
    viewState.comparisonMode = false;
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

  it('does not load a file after refresh empties the folder', async () => {
    (api.refresh as any).mockResolvedValue({ total: 0, index: -1 });

    await appState.refresh();

    expect(navigationService.loadFile).not.toHaveBeenCalled();
    expect(appState.currentFile).toBeNull();
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
    viewState.comparisonMode = true;
    appState.referenceFile = { filename: 'ref.jpg' } as any;

    await appState.toggleComparisonMode();

    expect(viewState.comparisonMode).toBe(false);
    expect(appState.referenceFile).toBeNull();
  });

  it('exits duplicates mode consistently', () => {
    filterState.filterMode = 'duplicates' as any;
    viewState.gridOpen = true;
    viewState.comparisonMode = true;
    appState.referenceFile = { filename: 'ref.jpg' } as any;
    filterState.filteredIndices = [1, 2];
    filterState.duplicateGroups = [[1, 2]];

    appState.exitDuplicatesMode();

    expect(filterState.filterMode).toBe('none');
    expect(viewState.gridOpen).toBe(false);
    expect(viewState.comparisonMode).toBe(false);
    expect(appState.referenceFile).toBeNull();
    expect(filterState.filteredIndices).toEqual([]);
    expect(filterState.duplicateGroups).toEqual([]);
  });

  it('keeps grid open when disabling duplicates from grid view', async () => {
    filterState.filterMode = 'duplicates' as any;
    viewState.gridOpen = true;
    viewState.comparisonMode = false;
    filterState.filteredIndices = [1, 2];
    filterState.duplicateGroups = [[1, 2]];

    await appState.toggleDuplicatesFilter();

    expect(filterState.filterMode).toBe('none');
    expect(viewState.gridOpen).toBe(true);
    expect(filterState.filteredIndices).toEqual([]);
    expect(filterState.duplicateGroups).toEqual([]);
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
    filterState.filterMode = 'starred' as any;
    viewState.gridOpen = true;
    viewState.comparisonMode = false;
    filterState.filteredIndices = [1, 2];

    await appState.toggleStarFilter();

    expect(filterState.filterMode).toBe('none');
    expect(viewState.gridOpen).toBe(true);
    expect(filterState.filteredIndices).toEqual([]);
  });

  it('keeps grid open when disabling label filter from grid view', async () => {
    filterState.filterMode = 'label' as any;
    filterState.activeLabelFilter = 3 as any;
    viewState.gridOpen = true;
    viewState.comparisonMode = false;
    filterState.filteredIndices = [1, 2];

    await appState.setLabelFilter(3);

    expect(filterState.filterMode).toBe('none');
    expect(viewState.gridOpen).toBe(true);
    expect(filterState.filteredIndices).toEqual([]);
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

  it('updates selected photo labels immediately after a successful batch action', async () => {
    (api.setLabel as any).mockResolvedValue({ ok: true });
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

    expect(appState.v2?.Photos['a.jpg'].Label).toBe(3);
    expect(appState.v2?.Photos['b.jpg'].Label).toBe(3);
  });

  it('applies the requested label to every photo in a mixed batch selection', async () => {
    (api.setLabel as any).mockResolvedValue({ ok: true });
    appState.selectedIndices = [0, 1];
    appState.v2 = {
      ...appState.v2!,
      Photos: {
        'a.jpg': { ...appState.v2!.Photos['a.jpg'], Label: 3 },
        'b.jpg': { ...appState.v2!.Photos['b.jpg'], Label: 0 },
      },
    };

    await appState.setLabel(3);

    expect(api.setLabel).toHaveBeenCalledWith(undefined, undefined, ['a.jpg', 'b.jpg'], 3);
    expect(appState.v2?.Photos['a.jpg'].Label).toBe(3);
    expect(appState.v2?.Photos['b.jpg'].Label).toBe(3);
  });

  it('clears the requested label from every photo when the whole batch already has it', async () => {
    (api.setLabel as any).mockResolvedValue({ ok: true });
    appState.selectedIndices = [0, 1];
    appState.currentFile = { ...appState.currentFile!, label: 3 } as any;
    appState.v2 = {
      ...appState.v2!,
      Photos: {
        'a.jpg': { ...appState.v2!.Photos['a.jpg'], Label: 3 },
        'b.jpg': { ...appState.v2!.Photos['b.jpg'], Label: 3 },
      },
    };

    await appState.setLabel(3);

    expect(api.setLabel).toHaveBeenCalledWith(undefined, undefined, ['a.jpg', 'b.jpg'], 0);
    expect(appState.currentFile?.label).toBe(0);
    expect(appState.v2?.Photos['a.jpg'].Label).toBe(0);
    expect(appState.v2?.Photos['b.jpg'].Label).toBe(0);
  });

  it('refreshes photo, grid, and filmstrip labels after a successful single action', async () => {
    const { default: LabelViews } = await import('./components/LabelViews.test.svelte');
    (api.setLabel as any).mockResolvedValue({ ok: true });
    render(LabelViews);

    expect(screen.queryByTestId('grid-label')).toBeNull();
    expect(screen.queryByTestId('filmstrip-label')).toBeNull();

    await appState.setLabel(2);

    expect(appState.currentFile?.label).toBe(2);
    await waitFor(() => {
      expect(screen.getByTestId('grid-label').textContent).toBe('2');
      expect(screen.getByTestId('filmstrip-label').textContent).toBe('2');
    });
  });

  it('clears photo and grid labels when applying the active label again', async () => {
    (api.setLabel as any).mockResolvedValue({ ok: true });
    appState.currentFile = { ...appState.currentFile!, label: 2 } as any;
    appState.v2 = {
      ...appState.v2!,
      Photos: {
        ...appState.v2!.Photos,
        'a.jpg': { ...appState.v2!.Photos['a.jpg'], Label: 2 },
      },
    };

    await appState.setLabel(2);

    expect(api.setLabel).toHaveBeenCalledWith(0, 'a.jpg', undefined, 0);
    expect(appState.currentFile?.label).toBe(0);
    expect(appState.v2?.Photos['a.jpg'].Label).toBe(0);
  });

  it('keeps photo and grid labels unchanged when the backend rejects the action', async () => {
    (api.setLabel as any).mockRejectedValue(new Error('write failed'));

    await appState.setLabel(4);

    expect(appState.currentFile?.label).toBe(0);
    expect(appState.v2?.Photos['a.jpg'].Label).toBe(0);
  });

  it('does not expose moved filtered photos when filters are reset before structural sync', async () => {
    (api.browseDialog as any).mockResolvedValue({ path: '/remote' });
    (api.exportFiles as any).mockResolvedValue(undefined);
    filterState.filterMode = 'label';
    filterState.activeLabelFilter = 1;
    filterState.filteredIndices = [0, 1, 2];
    appState.selectedIndices = [0, 1, 2];
    appState.v2 = {
      ...appState.v2!,
      VisibleOrder: ['red-1.jpg', 'red-2.jpg', 'red-3.jpg', 'keep.jpg'],
      Photos: {
        'red-1.jpg': { ID: 'red-1.jpg', IsStarred: false, Rotation: 0, Label: 1, IsTrashed: false },
        'red-2.jpg': { ID: 'red-2.jpg', IsStarred: false, Rotation: 0, Label: 1, IsTrashed: false },
        'red-3.jpg': { ID: 'red-3.jpg', IsStarred: false, Rotation: 0, Label: 1, IsTrashed: false },
        'keep.jpg': { ID: 'keep.jpg', IsStarred: false, Rotation: 0, Label: 0, IsTrashed: false },
      },
    } as any;

    await appState.exportSelected(true);

    expect(appState.v2?.VisibleOrder).toEqual(['red-1.jpg', 'red-2.jpg', 'red-3.jpg', 'keep.jpg']);
    (appState as any).onExportComplete({
      root: '',
      movedPaths: ['red-1.jpg', 'red-2.jpg', 'red-3.jpg'],
    });
    appState.clearAllFilters();

    expect(api.exportFiles).toHaveBeenCalledWith(
      ['red-1.jpg', 'red-2.jpg', 'red-3.jpg'],
      '/remote',
      true,
    );
    expect(appState.v2?.VisibleOrder).toEqual(['keep.jpg']);
    expect(Object.keys(appState.v2?.Photos ?? {})).toEqual(['keep.jpg']);
  });

  it('removes moved label matches before resetting the label filter', async () => {
    (api.browseDialog as any).mockResolvedValue({ path: '/remote' });
    (api.exportSelection as any).mockResolvedValue(undefined);
    filterState.filterMode = 'label';
    filterState.activeLabelFilter = 1;
    filterState.filteredIndices = [0, 1, 2];
    appState.selectedIndices = [];
    appState.v2 = {
      ...appState.v2!,
      VisibleOrder: ['red-1.jpg', 'red-2.jpg', 'red-3.jpg', 'keep.jpg'],
      Photos: {
        'red-1.jpg': { ID: 'red-1.jpg', IsStarred: false, Rotation: 0, Label: 1, IsTrashed: false },
        'red-2.jpg': { ID: 'red-2.jpg', IsStarred: false, Rotation: 0, Label: 1, IsTrashed: false },
        'red-3.jpg': { ID: 'red-3.jpg', IsStarred: false, Rotation: 0, Label: 1, IsTrashed: false },
        'keep.jpg': { ID: 'keep.jpg', IsStarred: false, Rotation: 0, Label: 0, IsTrashed: false },
      },
    } as any;

    await appState.exportSelection('label', 1, true);

    expect(appState.v2?.VisibleOrder).toEqual(['red-1.jpg', 'red-2.jpg', 'red-3.jpg', 'keep.jpg']);
    (appState as any).onExportComplete({
      root: '',
      movedPaths: ['red-1.jpg', 'red-2.jpg', 'red-3.jpg'],
    });
    appState.clearAllFilters();

    expect(api.exportSelection).toHaveBeenCalledWith('label', 1, '/remote', true);
    expect(appState.v2?.VisibleOrder).toEqual(['keep.jpg']);
    expect(Object.keys(appState.v2?.Photos ?? {})).toEqual(['keep.jpg']);
  });

  it('reconciles only paths confirmed by a partially successful move', () => {
    appState.v2 = {
      ...appState.v2!,
      Root: '/source',
      VisibleOrder: ['moved.jpg', 'failed.jpg', 'keep.jpg'],
      Photos: {
        'moved.jpg': { ID: 'moved.jpg', IsStarred: false, Rotation: 0, Label: 1, IsTrashed: false },
        'failed.jpg': { ID: 'failed.jpg', IsStarred: false, Rotation: 0, Label: 1, IsTrashed: false },
        'keep.jpg': { ID: 'keep.jpg', IsStarred: false, Rotation: 0, Label: 0, IsTrashed: false },
      },
    } as any;

    (appState as any).onExportComplete({ root: '/source', movedPaths: ['moved.jpg'] });

    expect(appState.v2?.VisibleOrder).toEqual(['failed.jpg', 'keep.jpg']);
  });

  it('reconciles confirmed paths when a move is cancelled after partial success', () => {
    appState.v2 = {
      ...appState.v2!,
      Root: '/source',
      VisibleOrder: ['moved.jpg', 'pending.jpg'],
      Photos: {
        'moved.jpg': { ID: 'moved.jpg', IsStarred: false, Rotation: 0, Label: 1, IsTrashed: false },
        'pending.jpg': { ID: 'pending.jpg', IsStarred: false, Rotation: 0, Label: 1, IsTrashed: false },
      },
    } as any;

    (appState as any).onExportCancelled({ root: '/source', movedPaths: ['moved.jpg'] });

    expect(appState.v2?.VisibleOrder).toEqual(['pending.jpg']);
  });

  it('ignores a late export event from a previously open folder', () => {
    appState.v2 = {
      ...appState.v2!,
      Root: '/new-folder',
      VisibleOrder: ['same-name.jpg'],
      Photos: {
        'same-name.jpg': { ID: 'same-name.jpg', IsStarred: false, Rotation: 0, Label: 0, IsTrashed: false },
      },
    } as any;

    (appState as any).onExportComplete({ root: '/old-folder', movedPaths: ['same-name.jpg'] });

    expect(appState.v2?.VisibleOrder).toEqual(['same-name.jpg']);
  });

  it('accepts a confirmed move event after authoritative refresh already removed the paths', () => {
    appState.v2 = {
      ...appState.v2!,
      Root: '/source',
      VisibleOrder: ['keep.jpg'],
      Photos: {
        'keep.jpg': { ID: 'keep.jpg', IsStarred: false, Rotation: 0, Label: 0, IsTrashed: false },
      },
    } as any;

    (appState as any).onExportComplete({ root: '/source', movedPaths: ['already-gone.jpg'] });

    expect(appState.v2?.VisibleOrder).toEqual(['keep.jpg']);
    expect(Object.keys(appState.v2?.Photos ?? {})).toEqual(['keep.jpg']);
  });

  it('produces an empty coherent grid when every photo is confirmed moved', () => {
    appState.v2 = {
      ...appState.v2!,
      Root: '/source',
      VisibleOrder: ['a.jpg', 'b.jpg'],
    } as any;
    filterState.filteredIndices = [0, 1];
    appState.selectedIndices = [0, 1];

    (appState as any).onExportComplete({ root: '/source', movedPaths: ['a.jpg', 'b.jpg'] });

    expect(appState.v2?.VisibleOrder).toEqual([]);
    expect(appState.v2?.Photos).toEqual({});
    expect(filterState.filteredIndices).toEqual([]);
    expect(appState.selectedIndices).toEqual([]);
  });

  it('clears the current photo when its move is confirmed', () => {
    appState.v2 = {
      ...appState.v2!,
      Root: '/source',
      VisibleOrder: ['a.jpg', 'b.jpg'],
    } as any;
    appState.currentIndex = 0;
    appState.currentFile = { ...appState.currentFile!, filename: 'a.jpg', index: 0 } as any;

    (appState as any).onExportComplete({ root: '/source', movedPaths: ['a.jpg'] });

    expect(appState.currentFile).toBeNull();
    expect(appState.currentIndex).toBe(0);
    expect(appState.v2?.VisibleOrder).toEqual(['b.jpg']);
  });

  it('does not reconcile photos when the backend reports an export error', () => {
    appState.v2 = { ...appState.v2!, Root: '/source' } as any;

    appState.onExportError({ error: 'destination unavailable' });

    expect(appState.v2?.VisibleOrder).toEqual(['a.jpg', 'b.jpg']);
    expect(Object.keys(appState.v2?.Photos ?? {}).sort()).toEqual(['a.jpg', 'b.jpg']);
  });

  it('keeps photos visible after a copy export', async () => {
    (api.browseDialog as any).mockResolvedValue({ path: '/remote' });
    (api.exportFiles as any).mockResolvedValue(undefined);
    appState.selectedIndices = [0];

    await appState.exportSelected(false);

    expect(appState.v2?.VisibleOrder).toEqual(['a.jpg', 'b.jpg']);
    expect(Object.keys(appState.v2?.Photos ?? {}).sort()).toEqual(['a.jpg', 'b.jpg']);
  });

  it('keeps photos visible when a move export fails', async () => {
    (api.browseDialog as any).mockResolvedValue({ path: '/remote' });
    (api.exportFiles as any).mockRejectedValue(new Error('move failed'));
    appState.selectedIndices = [0];

    await appState.exportSelected(true);

    expect(appState.v2?.VisibleOrder).toEqual(['a.jpg', 'b.jpg']);
    expect(Object.keys(appState.v2?.Photos ?? {}).sort()).toEqual(['a.jpg', 'b.jpg']);
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

  it('delegates rotate reset for the active photo', async () => {
    (api.rotateReset as any).mockResolvedValue({ ok: true });

    await appState.rotateReset();

    expect(api.rotateReset).toHaveBeenCalledWith(0, 'a.jpg');
    expect(appState.lastNonUndoableAction).toBe('');
  });

  it('applies physical rotation then reloads the active photo', async () => {
    (api.applyRotation as any).mockResolvedValue(undefined);
    appState.currentFile = { ...appState.currentFile!, rotation: 90, format: 'JPEG' } as any;
    appState.runtimeCapabilities = { exifWrite: true } as any;
    const previousVersion = appState.sessionVersion;

    await appState.applyRotation();

    expect(api.applyRotation).toHaveBeenCalledWith(0, 'a.jpg');
    expect(navigationService.loadFile).toHaveBeenCalledWith(0, false);
    expect(appState.sessionVersion).not.toBe(previousVersion);
    expect(appState.lastNonUndoableAction).toBe('apply_rotation');
  });

  it('replaces currentFile with reanalyzed metadata', async () => {
    const refreshed = { ...appState.currentFile!, camera: 'Updated Camera' } as any;
    (api.reanalyzeMetadata as any).mockResolvedValue(refreshed);

    await appState.reanalyzeMetadata();

    expect(api.reanalyzeMetadata).toHaveBeenCalledWith(0, 'a.jpg');
    expect(appState.currentFile).toEqual(refreshed);
  });

  it('persists auto-advance before updating local config', async () => {
    (api.updateConfig as any).mockResolvedValue({ ok: true });
    viewState.config = { autoAdvance: false } as any;

    await appState.toggleAutoAdvance();

    expect(api.updateConfig).toHaveBeenCalledWith(expect.objectContaining({ autoAdvance: true }));
    expect(viewState.config?.autoAdvance).toBe(true);
  });

  it('refreshes exactly once when the backend emits folder:changed', async () => {
	  (api.getConfig as any).mockResolvedValue({ autoRefresh: false });
	  (api.getStats as any).mockResolvedValue({ total: 0 });
	  (api.getAppState as any).mockResolvedValue({ Root: '', VisibleOrder: [], Photos: {} });
	  (api.sysCheck as any).mockResolvedValue({ capabilities: {} });
	  (api.refresh as any).mockResolvedValue({ total: 0, index: -1 });

	  await appState.init();
	  expect(appEventHandlers['folder:changed']).toBeTypeOf('function');

	  await appEventHandlers['folder:changed']();
	  await vi.waitFor(() => expect(api.refresh).toHaveBeenCalledTimes(1));
  });
});
