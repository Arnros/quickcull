import { beforeEach, describe, expect, it, vi } from 'vitest';
import { FILTER_MODES } from './constants';

vi.mock('./api', () => ({
  api: {
    getFilteredIndices: vi.fn(),
    getDuplicates: vi.fn(),
    getStarredIndices: vi.fn(),
    getLabelIndices: vi.fn(),
    getFilters: vi.fn(),
  },
}));

vi.mock('./logger', () => ({
  logger: {
    debug: vi.fn(),
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
  },
}));

vi.mock('./viewState.svelte', () => ({
  viewState: {
    config: null,
  },
}));

import { api } from './api';
import { filterState } from './filterState.svelte';

describe('FilterState', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Reset to initial state via public methods + direct assignment
    filterState.filterMode = FILTER_MODES.NONE as any;
    filterState.activeFilters = {};
    filterState.filteredIndices = [];
    filterState.duplicateGroups = [];
    filterState.activeLabelFilter = 0;
    filterState.filters = { cameras: [], isos: [] };
  });

  it('initialise avec NONE', () => {
    expect(filterState.filterMode).toBe(FILTER_MODES.NONE);
    expect(filterState.filteredIndices).toEqual([]);
    expect(filterState.activeFilters).toEqual({});
  });

  it('charge les filtres disponibles depuis le backend', async () => {
    (api.getFilters as any).mockResolvedValue({ cameras: ['Canon', 'Sony'], isos: ['100', '800'] });

    await filterState.loadFilters();

    expect(api.getFilters).toHaveBeenCalledOnce();
    expect(filterState.filters).toEqual({ cameras: ['Canon', 'Sony'], isos: ['100', '800'] });
  });

  it('updateFilteredIndices en mode NONE retourne tableau vide', async () => {
    filterState.filterMode = FILTER_MODES.NONE as any;
    filterState.activeFilters = {};

    await filterState.updateFilteredIndices();

    expect(api.getFilteredIndices).not.toHaveBeenCalled();
    expect(filterState.filteredIndices).toEqual([]);
  });

  it('setFilter camera active les filtres avancés', async () => {
    (api.getFilteredIndices as any).mockResolvedValue({ indices: [0, 2, 4] });

    await filterState.setFilter('camera', 'Sony A7');

    expect(filterState.activeFilters.camera).toBe('Sony A7');
    expect(api.getFilteredIndices).toHaveBeenCalled();
    expect(filterState.filteredIndices).toEqual([0, 2, 4]);
  });

  it('clearFilters remet tout à zéro', async () => {
    // Setup: put filterState in LABEL mode with active filters
    filterState.filterMode = FILTER_MODES.LABEL as any;
    filterState.activeFilters = { camera: 'Canon EOS', iso: '400' };
    (api.getLabelIndices as any).mockResolvedValue({ indices: [] });

    await filterState.clearFilters();

    expect(filterState.activeFilters).toEqual({});
    expect(filterState.filteredIndices).toEqual([]);
  });

  it('setAdvancedFilter avec dateFrom et dateTo', async () => {
    (api.getFilteredIndices as any).mockResolvedValue({ indices: [1, 3] });

    await filterState.setAdvancedFilter('dateFrom', '2023-01-01');
    await filterState.setAdvancedFilter('dateTo', '2023-12-31');

    expect(api.getFilteredIndices).toHaveBeenCalledWith(
      expect.objectContaining({ dateFrom: '2023-01-01', dateTo: '2023-12-31' })
    );
    expect(filterState.filteredIndices).toEqual([1, 3]);
  });
});
