import { api } from './api';
import { logger } from './logger';
import { viewState } from './viewState.svelte';
import { FILTER_MODES, type FilterMode } from './constants';

/** Default perceptual-hash similarity threshold (0–100) used for duplicate detection. */
const DEFAULT_DUPLICATE_THRESHOLD = 90;

class FilterState {
  filterBarOpen = $state(false);
  filterMode = $state<FilterMode>(FILTER_MODES.NONE);
  activeLabelFilter = $state(0);
  duplicateGroups = $state<number[][]>([]);
  filteredIndices = $state<number[]>([]);
  filters = $state<{ cameras: string[], isos: string[] }>({ cameras: [], isos: [] });
  activeFilters = $state<{
    camera?: string,
    iso?: string,
    dateFrom?: string,
    dateTo?: string,
    sizeMin?: string,
    sizeMax?: string
  }>({});

  async updateFilteredIndices(): Promise<void> {
    try {
      if (this.filterMode === FILTER_MODES.DUPLICATES) {
        if (this.duplicateGroups.length === 0) {
          const threshold = viewState.config?.duplicateThreshold || DEFAULT_DUPLICATE_THRESHOLD;
          const res = await api.getDuplicates(threshold);
          this.duplicateGroups = res?.groups || [];
        }
        this.filteredIndices = this.duplicateGroups.flat();
      } else if (this.filterMode === FILTER_MODES.STARRED) {
        const res = await api.getStarredIndices();
        this.filteredIndices = res?.indices || [];
      } else if (this.filterMode === FILTER_MODES.LABEL) {
        const res = await api.getLabelIndices(this.activeLabelFilter);
        this.filteredIndices = res?.indices || [];
      } else if (Object.keys(this.activeFilters).length > 0) {
        const res = await api.getFilteredIndices(this.activeFilters);
        this.filteredIndices = res?.indices || [];
      } else {
        this.filteredIndices = [];
      }
    } catch (e) {
      logger.error('Failed to update filtered indices', { mode: this.filterMode, error: e });
    }
  }

  async loadFilters(): Promise<void> {
    try {
      const res = await api.getFilters();
      this.filters = {
        cameras: res?.cameras || [],
        isos: res?.isos || []
      };
    } catch (e) {
      logger.error('Failed to load filters', { error: e });
    }
  }

  async loadDuplicates(): Promise<void> {
    try {
      const threshold = viewState.config?.duplicateThreshold || DEFAULT_DUPLICATE_THRESHOLD;
      const res = await api.getDuplicates(threshold);
      this.duplicateGroups = res?.groups || [];
      this.filteredIndices = this.duplicateGroups.flat();
    } catch (e) {
      logger.error('Failed to load duplicates', { error: e });
    }
  }

  async setFilter(type: 'camera' | 'iso', value: string): Promise<void> {
    logger.debug('Setting metadata filter', { type, value });
    if (this.activeFilters[type] === value) {
      const { [type]: _, ...rest } = this.activeFilters;
      this.activeFilters = rest;
    } else {
      this.activeFilters = { ...this.activeFilters, [type]: value };
    }
    await this.updateFilteredIndices();
  }

  async setAdvancedFilter(type: 'dateFrom' | 'dateTo' | 'sizeMin' | 'sizeMax', value: string): Promise<void> {
    const trimmed = value.trim();
    if (trimmed === '') {
      const { [type]: _, ...rest } = this.activeFilters;
      this.activeFilters = rest;
    } else {
      this.activeFilters = { ...this.activeFilters, [type]: trimmed };
    }
    await this.updateFilteredIndices();
  }

  async clearFilters(): Promise<void> {
    logger.debug('Clearing all filters');
    this.activeFilters = {};
    await this.updateFilteredIndices();
  }
}

export const filterState = new FilterState();
