import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('./api', () => ({
  api: {
    browseDialog: vi.fn(),
  },
}));

vi.mock('./i18n.svelte', () => ({
  i18n: {
    t: (key: string) => `tr:${key}`,
  },
}));

vi.mock('./toast.svelte', () => ({
  toastService: {
    show: vi.fn(),
  },
}));

import { api } from './api';
import {
  focusFirstFilteredIfNeeded,
  getSelectedPaths,
  handleTournamentProgress,
  hasActiveFiltering,
  refreshAfterStateMutation,
  runExportFlow,
  setFilterMode,
} from './appState.helpers';
import { toastService } from './toast.svelte';

describe('appState.helpers', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('detects active filtering', () => {
    expect(hasActiveFiltering('none', {})).toBe(false);
    expect(hasActiveFiltering('starred', {})).toBe(true);
    expect(hasActiveFiltering('none', { camera: 'x' })).toBe(true);
  });

  it('maps selected indices to paths safely', () => {
    expect(getSelectedPaths([0, 2], ['a.jpg', 'b.jpg'])).toEqual(['a.jpg']);
  });

  it('focuses first filtered index when current index is outside filter', async () => {
    const loadFile = vi.fn(async () => undefined);
    await focusFirstFilteredIfNeeded({
      filterMode: 'starred',
      activeFilters: {},
      activeLabelFilter: 0,
      duplicateGroups: [],
      filteredIndices: [5, 7],
      gridOpen: false,
      currentIndex: 0,
      updateFilteredIndices: vi.fn(async () => undefined),
      loadFile,
    });
    expect(loadFile).toHaveBeenCalledWith(5);
  });

  it('setFilterMode toggles duplicates mode and resets none mode', async () => {
    const ctx: any = {
      filterMode: 'none',
      activeFilters: { camera: 'x' },
      activeLabelFilter: 2,
      duplicateGroups: [[1, 2]],
      filteredIndices: [1, 2],
      gridOpen: false,
      currentIndex: 3,
      updateFilteredIndices: vi.fn(async () => undefined),
      loadFile: vi.fn(async () => undefined),
    };

    await setFilterMode(ctx, 'duplicates');
    expect(ctx.filterMode).toBe('duplicates');
    expect(ctx.activeFilters).toEqual({});
    expect(ctx.gridOpen).toBe(true);
    expect(ctx.updateFilteredIndices).toHaveBeenCalled();

    await setFilterMode(ctx, 'none', undefined, { keepGridOnNone: true });
    expect(ctx.duplicateGroups).toEqual([]);
    expect(ctx.filteredIndices).toEqual([]);
    expect(ctx.gridOpen).toBe(true);
  });

  it('refreshAfterStateMutation refreshes filters when active', async () => {
    const ctx: any = {
      duplicateGroups: [[1, 2]],
      loadFile: vi.fn(async () => undefined),
      updateFilteredIndices: vi.fn(async () => undefined),
      filterMode: 'none',
      activeFilters: { iso: '100' },
    };
    await refreshAfterStateMutation(ctx, 4, { resetDuplicateGroups: true });
    expect(ctx.duplicateGroups).toEqual([]);
    expect(ctx.loadFile).toHaveBeenCalledWith(4);
    expect(ctx.updateFilteredIndices).toHaveBeenCalled();
  });

  it('refreshAfterStateMutation skips loadFile for negative index but still refreshes filters', async () => {
    const ctx: any = {
      duplicateGroups: [],
      loadFile: vi.fn(async () => undefined),
      updateFilteredIndices: vi.fn(async () => undefined),
      filterMode: 'none',
      activeFilters: { camera: 'x' },
    };
    await refreshAfterStateMutation(ctx, -1);
    expect(ctx.loadFile).not.toHaveBeenCalled();
    expect(ctx.updateFilteredIndices).toHaveBeenCalled();
  });

  it('runExportFlow guards concurrency and restores flag', async () => {
    (api.browseDialog as any).mockResolvedValue({ path: '/tmp/out' });
    const exporter = vi.fn(async () => undefined);
    const ctx = { isExporting: false };

    await runExportFlow(ctx, exporter);

    expect(toastService.show).toHaveBeenCalledWith('tr:export_started', 'info');
    expect(exporter).toHaveBeenCalledWith('/tmp/out');
    expect(ctx.isExporting).toBe(false);
  });

  it('handleTournamentProgress advances based on comparison or auto-advance', async () => {
    const next = vi.fn(async () => undefined);
    const ctx = {
      comparisonMode: true,
      comparisonIndex: 2,
      autoAdvance: false,
      next,
    };

    await handleTournamentProgress(ctx, 5, true);
    expect(ctx.comparisonIndex).toBe(5);
    expect(next).toHaveBeenCalledTimes(1);

    next.mockClear();
    ctx.comparisonMode = false;
    ctx.autoAdvance = true;
    await handleTournamentProgress(ctx, 5, false);
    expect(next).toHaveBeenCalledTimes(1);
  });
});
