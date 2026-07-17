import { describe, expect, it, vi, beforeEach } from 'vitest';
import { FILTER_MODES } from './constants';

const { handlers, updateFilteredIndices } = vi.hoisted(() => ({
  handlers: {} as Record<string, (data: any) => void | Promise<void>>,
  updateFilteredIndices: vi.fn(async () => undefined),
}));

vi.mock('./api', () => ({
  api: {
    onEvent: vi.fn((name: string, cb: (data: any) => void) => {
      handlers[name] = cb;
    }),
  },
}));

vi.mock('./filterState.svelte', () => ({
  filterState: {
    filterMode: FILTER_MODES.NONE,
    activeFilters: {},
    updateFilteredIndices,
  },
}));

vi.mock('./logger', () => ({
  logger: {
    debug: vi.fn(),
    info: vi.fn(),
  },
}));

import { syncService } from './syncService.svelte';
import { filterState } from './filterState.svelte';

function makeSyncAppState(photos: Record<string, any>, visibleOrder: string[]): any {
  return {
    v2: {
      Root: '/media',
      CacheDir: '',
      VisibleOrder: visibleOrder,
      Photos: photos,
      TrashedCount: 0,
      StarredCount: 0,
      LabeledCount: 0,
      RotatedCount: 0,
      UndoLen: 0,
    },
    stats: { total: visibleOrder.length, trashedCount: 0, starredCount: 0, labeledCount: 0, undoLen: 0 },
    currentFile: null,
    currentIndex: 0,
    selectionPivot: 0,
    lastNonUndoableAction: '',
    selectedIndices: [],
    sessionVersion: 1,
    updateStarredIndices: vi.fn(),
    validateSelection: vi.fn(),
    starredIndices: [],
  };
}

describe('syncService cache busting on structural sync', () => {
  beforeEach(() => {
    Object.keys(handlers).forEach((k) => delete handlers[k]);
    updateFilteredIndices.mockClear();
    filterState.filterMode = FILTER_MODES.NONE as any;
    filterState.activeFilters = {};
  });

  it('replaces photo and map references when applying a label delta', () => {
    const photo = { ID: 'a.jpg', IsStarred: false, Label: 0, Rotation: 0, IsTrashed: false };
    const photos = { 'a.jpg': photo };
    const appState = makeSyncAppState(photos, ['a.jpg']);
    appState.currentFile = { filename: 'a.jpg', index: 0, label: 0, starred: false, rotation: 0 };
    syncService.init(appState);

    handlers.SyncDelta?.({ PhotoID: 'a.jpg', Changes: { Label: 1 } });

    expect(appState.v2.Photos).not.toBe(photos);
    expect(appState.v2.Photos['a.jpg']).not.toBe(photo);
    expect(appState.v2.Photos['a.jpg'].Label).toBe(1);
    expect(appState.currentFile.label).toBe(1);
    expect(appState.v2.Photos['a.jpg']).not.toHaveProperty('_stats');
  });

  it('prunes photos absent from partial structural order', async () => {
    const appState = makeSyncAppState({
      'a.jpg': { ID: 'a.jpg', IsStarred: false, Label: 0, Rotation: 0, IsTrashed: false },
      'b.jpg': { ID: 'b.jpg', IsStarred: false, Label: 0, Rotation: 0, IsTrashed: false },
      'c.jpg': { ID: 'c.jpg', IsStarred: false, Label: 1, Rotation: 0, IsTrashed: false },
    }, ['a.jpg', 'b.jpg', 'c.jpg']);
    appState.currentFile = { filename: 'a.jpg', index: 0, label: 0, starred: false, rotation: 0 };
    appState.selectedIndices = [1, 2];
    appState.selectionPivot = 1;
    syncService.init(appState);

    await handlers.SyncState?.({
      Root: '/media', CacheDir: '', IsPartial: true,
      VisibleOrder: ['a.jpg', 'c.jpg'], Photos: {},
      TrashedCount: 0, StarredCount: 0, LabeledCount: 1, RotatedCount: 0, UndoLen: 0,
    });

    expect(Object.keys(appState.v2.Photos).sort()).toEqual(['a.jpg', 'c.jpg']);
    expect(appState.v2.Photos['c.jpg'].Label).toBe(1);
    expect(appState.selectedIndices).toEqual([1]);
    expect(appState.selectionPivot).toBe(appState.currentIndex);
  });

  it('fully replaces photos on non-partial authoritative sync', async () => {
    const appState = makeSyncAppState({
      'stale.jpg': { ID: 'stale.jpg', IsStarred: true, Label: 1, Rotation: 0, IsTrashed: false },
    }, ['stale.jpg']);
    syncService.init(appState);

    await handlers.SyncState?.({
      Root: '/media', CacheDir: '', IsPartial: false,
      VisibleOrder: ['fresh.jpg'],
      Photos: { 'fresh.jpg': { ID: 'fresh.jpg', IsStarred: false, Label: 0, Rotation: 0, IsTrashed: false } },
      TrashedCount: 0, StarredCount: 0, LabeledCount: 0, RotatedCount: 0, UndoLen: 0,
    });

    expect(Object.keys(appState.v2.Photos)).toEqual(['fresh.jpg']);
  });

  it('increments sessionVersion on structural SyncState', async () => {
    const appState: any = {
      v2: {
        Root: '/media',
        VisibleOrder: ['a.jpg', 'b.jpg'],
        Photos: {
          'a.jpg': { IsStarred: false, Label: 0, Rotation: 0 },
          'b.jpg': { IsStarred: false, Label: 0, Rotation: 0 },
          'c.jpg': { IsStarred: false, Label: 0, Rotation: 0 },
        },
        TrashedCount: 0,
        StarredCount: 0,
        LabeledCount: 0,
        History: [],
      },
      stats: { total: 2, trashedCount: 0, starredCount: 0, labeledCount: 0, undoLen: 0 },
      currentFile: { filename: 'a.jpg', index: 0, starred: false, label: 0, rotation: 0 },
      currentIndex: 0,
      selectionPivot: 0,
      lastNonUndoableAction: '',
      selectedIndices: [],
      sessionVersion: 1,
      updateStarredIndices: vi.fn(),
      validateSelection: vi.fn(),
      starredIndices: [],
    };

    syncService.init(appState);

    await handlers.SyncState?.({
      Root: '/media',
      VisibleOrder: ['a.jpg', 'b.jpg', 'c.jpg'],
      Photos: {},
      IsPartial: true,
      TrashedCount: 0,
      StarredCount: 0,
      LabeledCount: 0,
      History: [],
    });

    expect(appState.sessionVersion).toBeGreaterThan(1);
  });

  it('remaps selected indices to keep same photos on structural reorder', async () => {
    const appState: any = {
      v2: {
        Root: '/media',
        VisibleOrder: ['a.jpg', 'b.jpg', 'c.jpg'],
        Photos: {
          'a.jpg': { ID: 'a.jpg' },
          'b.jpg': { ID: 'b.jpg' },
          'c.jpg': { ID: 'c.jpg' },
        },
        TrashedCount: 0,
        StarredCount: 0,
        LabeledCount: 0,
        History: [],
      },
      stats: { total: 3, trashedCount: 0, starredCount: 0, labeledCount: 0, undoLen: 0 },
      currentFile: { filename: 'a.jpg', index: 0, starred: false, label: 0, rotation: 0 },
      currentIndex: 0,
      selectionPivot: 2,
      lastNonUndoableAction: '',
      selectedIndices: [0, 2], // a.jpg + c.jpg
      sessionVersion: 1,
      updateStarredIndices: vi.fn(),
      validateSelection: vi.fn(),
      starredIndices: [],
    };

    syncService.init(appState);

    await handlers.SyncState?.({
      Root: '/media',
      VisibleOrder: ['c.jpg', 'a.jpg', 'b.jpg'],
      Photos: {},
      IsPartial: true,
      TrashedCount: 0,
      StarredCount: 0,
      LabeledCount: 0,
      History: [],
    });

    expect(appState.selectedIndices).toEqual([1, 0]); // a.jpg -> 1, c.jpg -> 0
    expect(appState.selectionPivot).toBe(0); // c.jpg (old pivot=2) -> new index 0
  });

  it('remaps selected indices on SyncState full update', async () => {
    const appState: any = {
      v2: {
        Root: '/media',
        VisibleOrder: ['x.jpg', 'y.jpg', 'z.jpg'],
        Photos: {},
        TrashedCount: 0,
        StarredCount: 0,
        LabeledCount: 0,
        History: [],
      },
      stats: { total: 3, trashedCount: 0, starredCount: 0, labeledCount: 0, undoLen: 0 },
      currentFile: { filename: 'x.jpg', index: 0, starred: false, label: 0, rotation: 0 },
      currentIndex: 0,
      selectionPivot: 1,
      lastNonUndoableAction: '',
      selectedIndices: [1, 2], // y + z
      sessionVersion: 1,
      updateStarredIndices: vi.fn(),
      validateSelection: vi.fn(),
      starredIndices: [],
    };

    syncService.init(appState);

    await handlers.SyncState?.({
      Root: '/media',
      CacheDir: '',
      Photos: {},
      VisibleOrder: ['z.jpg', 'x.jpg', 'y.jpg'],
      TrashedCount: 0,
      StarredCount: 0,
      LabeledCount: 0,
      History: [],
    });

    expect(appState.selectedIndices).toEqual([2, 0]); // y -> 2, z -> 0
    expect(appState.selectionPivot).toBe(2); // y.jpg -> index 2
  });

  it('falls back pivot to currentIndex when anchored photo disappears', async () => {
    const appState: any = {
      v2: {
        Root: '/media',
        VisibleOrder: ['a.jpg', 'b.jpg', 'c.jpg'],
        Photos: {
          'a.jpg': { ID: 'a.jpg' },
          'b.jpg': { ID: 'b.jpg' },
          'c.jpg': { ID: 'c.jpg' },
        },
        TrashedCount: 0,
        StarredCount: 0,
        LabeledCount: 0,
        History: [],
      },
      stats: { total: 3, trashedCount: 0, starredCount: 0, labeledCount: 0, undoLen: 0 },
      currentFile: { filename: 'a.jpg', index: 0, starred: false, label: 0, rotation: 0 },
      currentIndex: 0,
      selectionPivot: 2, // c.jpg
      lastNonUndoableAction: '',
      selectedIndices: [2],
      sessionVersion: 1,
      updateStarredIndices: vi.fn(),
      validateSelection: vi.fn(),
      starredIndices: [],
    };

    syncService.init(appState);

    await handlers.SyncState?.({
      Root: '/media',
      VisibleOrder: ['a.jpg', 'b.jpg'],
      Photos: {},
      IsPartial: true,
      TrashedCount: 1,
      StarredCount: 0,
      LabeledCount: 0,
      History: [],
    });

    expect(appState.selectionPivot).toBe(appState.currentIndex);
  });

  it('refreshes label-filtered indices after structural sync removes photos from visible order', async () => {
    const appState: any = {
      v2: {
        Root: '/media',
        VisibleOrder: ['a.jpg', 'b.jpg', 'c.jpg'],
        Photos: {
          'a.jpg': { ID: 'a.jpg' },
          'b.jpg': { ID: 'b.jpg' },
          'c.jpg': { ID: 'c.jpg' },
        },
        TrashedCount: 0,
        StarredCount: 0,
        LabeledCount: 2,
        History: [],
      },
      stats: { total: 3, trashedCount: 0, starredCount: 0, labeledCount: 2, undoLen: 0 },
      currentFile: { filename: 'a.jpg', index: 0, starred: false, label: 0, rotation: 0 },
      currentIndex: 0,
      selectionPivot: 1,
      lastNonUndoableAction: '',
      selectedIndices: [1],
      sessionVersion: 1,
      updateStarredIndices: vi.fn(),
      validateSelection: vi.fn(),
      starredIndices: [],
    };

    filterState.filterMode = FILTER_MODES.LABEL as any;

    syncService.init(appState);

    await handlers.SyncState?.({
      Root: '/media',
      VisibleOrder: ['a.jpg'],
      Photos: {},
      IsPartial: true,
      TrashedCount: 2,
      StarredCount: 0,
      LabeledCount: 2,
      History: [],
    });

    expect(updateFilteredIndices).toHaveBeenCalledTimes(1);
  });

  it('refreshes metadata-filtered indices after SyncState reorders the grid', async () => {
    const appState: any = {
      v2: {
        Root: '/media',
        VisibleOrder: ['a.jpg', 'b.jpg', 'c.jpg'],
        Photos: {},
        TrashedCount: 0,
        StarredCount: 0,
        LabeledCount: 0,
        History: [],
      },
      stats: { total: 3, trashedCount: 0, starredCount: 0, labeledCount: 0, undoLen: 0 },
      currentFile: { filename: 'a.jpg', index: 0, starred: false, label: 0, rotation: 0 },
      currentIndex: 0,
      selectionPivot: 0,
      lastNonUndoableAction: '',
      selectedIndices: [0],
      sessionVersion: 1,
      updateStarredIndices: vi.fn(),
      validateSelection: vi.fn(),
      starredIndices: [],
    };

    filterState.activeFilters = { camera: 'Sony' } as any;

    syncService.init(appState);

    await handlers.SyncState?.({
      Root: '/media',
      CacheDir: '',
      Photos: {},
      VisibleOrder: ['c.jpg', 'a.jpg', 'b.jpg'],
      TrashedCount: 0,
      StarredCount: 0,
      LabeledCount: 0,
      History: [],
    });

    expect(updateFilteredIndices).toHaveBeenCalledTimes(1);
  });

  it('preserves existing photos on structural-only SyncState (large library optimization)', async () => {
    const existingPhotos = {
      'a.jpg': { ID: 'a.jpg', IsStarred: true, Label: 1, Rotation: 0, IsTrashed: false },
    };
    const appState: any = {
      v2: {
        Root: '/media',
        VisibleOrder: ['a.jpg'],
        Photos: { ...existingPhotos },
        TrashedCount: 0,
        StarredCount: 1,
        LabeledCount: 1,
        History: [],
      },
      stats: { total: 1, trashedCount: 0, starredCount: 1, labeledCount: 1, undoLen: 0 },
      currentFile: { filename: 'a.jpg', index: 0, starred: true, label: 1, rotation: 0 },
      currentIndex: 0,
      selectionPivot: 0,
      selectedIndices: [0],
      sessionVersion: 1,
      updateStarredIndices: vi.fn(),
      validateSelection: vi.fn(),
    };

    syncService.init(appState);

    // Receive SyncState with EMPTY photos but IsPartial: true (large library structural refresh)
    await handlers.SyncState?.({
      Root: '/media',
      CacheDir: '/cache',
      VisibleOrder: ['a.jpg'],
      Photos: {}, // Empty!
      IsPartial: true,
      TrashedCount: 0,
      StarredCount: 1,
      LabeledCount: 1,
      History: [],
    });

    expect(appState.v2.Photos).toEqual(existingPhotos);
    expect(appState.currentFile.starred).toBe(true);
  });

  it('does NOT preserve existing photos if the root folder changes', async () => {
    const existingPhotos = {
      'a.jpg': { ID: 'a.jpg', IsStarred: true, Label: 1, Rotation: 0, IsTrashed: false },
    };
    const appState: any = {
      v2: {
        Root: '/old/media',
        VisibleOrder: ['a.jpg'],
        Photos: { ...existingPhotos },
      },
      stats: { total: 1, trashedCount: 0, starredCount: 1, labeledCount: 1, undoLen: 0 },
      currentFile: null,
      selectedIndices: [0],
      updateStarredIndices: vi.fn(),
      validateSelection: vi.fn(),
    };

    syncService.init(appState);

    // Receive SyncState for a NEW folder with empty photos (IsPartial: true doesn't matter here)
    await handlers.SyncState?.({
      Root: '/new/empty/folder',
      CacheDir: '/cache',
      VisibleOrder: [],
      Photos: {}, 
      IsPartial: true,
      TrashedCount: 0,
      StarredCount: 0,
      LabeledCount: 0,
      History: [],
    });

    expect(appState.v2.Photos).toEqual({});
  });

  it('clears photos if VisibleOrder is empty and IsPartial is false', async () => {
    const existingPhotos = {
      'a.jpg': { ID: 'a.jpg', IsStarred: true, Label: 1, Rotation: 0, IsTrashed: false },
    };
    const appState: any = {
      v2: {
        Root: '/media',
        VisibleOrder: ['a.jpg'],
        Photos: { ...existingPhotos },
      },
      stats: { total: 1, trashedCount: 0, starredCount: 1, labeledCount: 1, undoLen: 0 },
      currentFile: null,
      selectedIndices: [0],
      updateStarredIndices: vi.fn(),
      validateSelection: vi.fn(),
    };

    syncService.init(appState);

    // Receive SyncState for the SAME folder but NOT partial and with empty order
    await handlers.SyncState?.({
      Root: '/media',
      CacheDir: '/cache',
      VisibleOrder: [], // Empty!
      Photos: {},       // Empty
      IsPartial: false,
      TrashedCount: 0,
      StarredCount: 0,
      LabeledCount: 0,
      History: [],
    });

    expect(appState.v2.Photos).toEqual({});
  });
});
