import { describe, it, expect, beforeEach, vi } from 'vitest';
import { navigationService } from './navigationService.svelte';
import { appState } from './appState.svelte';
import { filterState } from './filterState.svelte';
import { viewState } from './viewState.svelte';

// Mock the API and logger to avoid actual calls during unit tests
vi.mock('./api', () => ({
  api: {
    getFile: vi.fn(),
    savePosition: vi.fn(),
  },
}));

vi.mock('./logger', () => ({
  logger: {
    debug: vi.fn(),
    error: vi.fn(),
  },
}));

describe('NavigationService', () => {
  beforeEach(() => {
    // Reset state before each test
    navigationService.currentIndex = 0;
    navigationService.comparisonIndex = 0;
    appState.selectedIndices = [];
    appState.stats = { total: 10 } as any;
    filterState.filterMode = 'none' as any;
    filterState.duplicateGroups = [];
    filterState.filteredIndices = [];
    filterState.activeFilters = {};
    viewState.comparisonMode = false;
  });

  it('should select a single item', () => {
    navigationService.select(5);
    expect(appState.selectedIndices).toEqual([5]);
    expect(navigationService.currentIndex).toBe(5);
  });

  it('should toggle multiple items with multi-select', () => {
    navigationService.select(1, true); // Ctrl+Click 1
    navigationService.select(3, true); // Ctrl+Click 3
    expect(appState.selectedIndices).toEqual([1, 3]);
    
    navigationService.select(1, true); // Ctrl+Click 1 again (toggle off)
    expect(appState.selectedIndices).toEqual([3]);
  });

  it('should select range with shift-select', () => {
    navigationService.select(2); // Select 2
    navigationService.select(5, false, true); // Shift+Click 5
    
    // Range 2 to 5 should be [2, 3, 4, 5]
    expect(appState.selectedIndices).toEqual([2, 3, 4, 5]);
  });

  it('should select range backwards with shift-select', () => {
    navigationService.select(8); // Select 8
    navigationService.select(6, false, true); // Shift+Click 6
    
    // Range 8 to 6 should include exactly {6,7,8}; order is implementation detail
    expect(appState.selectedIndices).toEqual(expect.arrayContaining([6, 7, 8]));
    expect(appState.selectedIndices.length).toBe(3);
  });

  it('should add range with ctrl/cmd+shift semantics', () => {
    navigationService.select(1); // anchor at 1
    navigationService.select(6, true); // ctrl/cmd click -> add and move anchor to 6
    navigationService.select(8, true, true); // ctrl/cmd+shift -> add range [6..8]

    expect(appState.selectedIndices).toEqual([1, 6, 7, 8]);
    expect(navigationService.currentIndex).toBe(8);
  });

  it('should fallback to first duplicate group when current index is out of sync', async () => {
    filterState.filterMode = 'duplicates' as any;
    filterState.duplicateGroups = [[4, 5], [7, 8]];
    navigationService.currentIndex = 99;
    const loadSpy = vi.spyOn(navigationService, 'loadFile').mockResolvedValue(undefined);

    await navigationService.next();

    expect(loadSpy).toHaveBeenCalledWith(4);
    loadSpy.mockRestore();
  });

  it('should step again in duplicates to avoid comparison index conflict', async () => {
    filterState.filterMode = 'duplicates' as any;
    filterState.duplicateGroups = [[0, 1], [2, 3]];
    navigationService.currentIndex = 0;
    navigationService.comparisonIndex = 1;
    viewState.comparisonMode = true;
    const loadSpy = vi.spyOn(navigationService, 'loadFile').mockResolvedValue(undefined);

    await navigationService.next();

    expect(loadSpy).toHaveBeenCalledWith(2);
    loadSpy.mockRestore();
  });

  it('should fallback to first filtered index when current index is not visible', async () => {
    filterState.filteredIndices = [6, 7];
    filterState.activeFilters = { camera: 'x' };
    navigationService.currentIndex = 42;
    const loadSpy = vi.spyOn(navigationService, 'loadFile').mockResolvedValue(undefined);

    await navigationService.next();

    expect(loadSpy).toHaveBeenCalledWith(6);
    loadSpy.mockRestore();
  });

  it('should skip comparison index in non-filtered navigation', async () => {
    appState.stats = { total: 5 } as any;
    navigationService.currentIndex = 0;
    navigationService.comparisonIndex = 1;
    viewState.comparisonMode = true;
    const loadSpy = vi.spyOn(navigationService, 'loadFile').mockResolvedValue(undefined);

    await navigationService.next();

    expect(loadSpy).toHaveBeenCalledWith(2);
    loadSpy.mockRestore();
  });

  it('should stop at first photo on shift+prev without wrapping', async () => {
    appState.stats = { total: 6 } as any;
    navigationService.select(3);

    await navigationService.prev(true);
    await navigationService.prev(true);
    await navigationService.prev(true);
    await navigationService.prev(true);

    expect(navigationService.currentIndex).toBe(0);
    expect(appState.selectedIndices).toEqual([0, 1, 2, 3]);
  });

  it('should stop at last photo on shift+next without wrapping', async () => {
    appState.stats = { total: 6 } as any;
    navigationService.select(2);

    await navigationService.next(true);
    await navigationService.next(true);
    await navigationService.next(true);
    await navigationService.next(true);

    expect(navigationService.currentIndex).toBe(5);
    expect(appState.selectedIndices).toEqual([2, 3, 4, 5]);
  });
});
