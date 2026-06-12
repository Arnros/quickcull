import { api } from './api';
import { logger } from './logger';
import { review } from '../../wailsjs/go/models';
import { filterState } from './filterState.svelte';
import { viewState } from './viewState.svelte';
import { appState } from './appState.svelte';
import { FILTER_MODES, SAVE_POSITION_DEBOUNCE_MS } from './constants';

// ---------------------------------------------------------------------------
// Navigation constants
// ---------------------------------------------------------------------------

/** Minimum list/total size required to run comparison-slot collision avoidance. */
const MIN_ITEMS_FOR_COLLISION_AVOIDANCE = 3;

/** Index of the first member within a duplicate group (the stable reference). */
const FIRST_MEMBER = 0;

/** Index of the second member within a duplicate group. */
const SECOND_MEMBER = 1;

class NavigationService {
  currentIndex = $state(0);
  comparisonIndex = $state(0);
  selectionPivot = $state(0);
  currentFile = $state<review.FileResponse | null>(null);
  referenceFile = $state<review.FileResponse | null>(null);
  lastLoadedTxID = 0;

  private savePositionTimer: ReturnType<typeof setTimeout> | null = null;

  private findDuplicateGroup(index: number): number[] | null {
    return filterState.duplicateGroups.find((g) => g.includes(index)) || null;
  }

  private resolveComparisonIndices(
    index: number,
    isManualClick: boolean,
    oldCurrent: number,
    oldComparison: number
  ): { currentIndex: number; comparisonIndex: number } {
    const statsTotal = appState.stats.total;
    const filterMode = filterState.filterMode;
    const inComparison = viewState.comparisonMode;
    let nextCurrent = index;
    let nextComparison = this.comparisonIndex;

    if (filterMode === FILTER_MODES.DUPLICATES) {
      const group = this.findDuplicateGroup(index);
      if (group && group.length > 1) {
        const stableRef = group[FIRST_MEMBER];
        if (inComparison) {
          if (group.includes(oldComparison) && oldComparison !== index) {
            nextComparison = oldComparison;
          } else {
            nextComparison = stableRef;
          }
        } else {
          nextComparison = stableRef;
        }
      }
    } else if (inComparison) {
      if (isManualClick) {
        nextComparison = (index === oldComparison) ? oldCurrent : oldComparison;
      }
    } else {
      nextComparison = (index > 0) ? index - 1 : (statsTotal || 1) - 1;
    }

    if (inComparison && nextComparison === nextCurrent && statsTotal > 1) {
      if (filterMode === FILTER_MODES.DUPLICATES) {
        const group = this.findDuplicateGroup(index);
        if (group && group.length > 1) {
          if (nextCurrent === group[FIRST_MEMBER]) {
            nextCurrent = group[SECOND_MEMBER];
            nextComparison = group[FIRST_MEMBER];
          } else {
            nextComparison = group[FIRST_MEMBER];
          }
        }
      } else {
        nextComparison = (nextCurrent > 0) ? nextCurrent - 1 : nextCurrent + 1;
      }
    }

    return { currentIndex: nextCurrent, comparisonIndex: nextComparison };
  }

  private async fetchCurrentAndReference(requestedIndex: number, isManualClick: boolean): Promise<void> {
    try {
      const filePromise = api.getFile(this.currentIndex);
      const refPromise = viewState.comparisonMode
        ? api.getFile(this.comparisonIndex)
        : Promise.resolve(null);

      const [file, refFile] = await Promise.all([filePromise, refPromise]);
      if (file && file.txID >= this.lastLoadedTxID) {
        this.lastLoadedTxID = file.txID;
        this.currentFile = file;
        this.referenceFile = refFile;

        if (!isManualClick) {
          appState.selectedIndices = [this.currentIndex];
        }
      }
    } catch (e) {
      logger.error('Failed to load files', { index: requestedIndex, error: e });
    }
  }

  /**
   * Navigates to `index`, resolving the comparison slot and fetching both
   * files. When `isManualClick` is false (arrow navigation) the selection
   * pivot is updated so the next Shift+Arrow starts from the new position.
   */
  async loadFile(index: number, isManualClick: boolean = false): Promise<void> {
    const oldCurrent = this.currentIndex;
    const oldComparison = this.comparisonIndex;
    const resolved = this.resolveComparisonIndices(index, isManualClick, oldCurrent, oldComparison);

    this.currentIndex = resolved.currentIndex;
    this.comparisonIndex = resolved.comparisonIndex;

    if (!isManualClick) {
      this.selectionPivot = this.currentIndex;
    }

    this.scheduleSavePosition();

    await this.fetchCurrentAndReference(index, isManualClick);
  }

  /**
   * Selects `index` with optional multi-select (Ctrl) or range-select (Shift)
   * semantics, then loads the file at that index.
   */
  select(index: number, multi: boolean = false, shift: boolean = false): void {
    if (shift) {
      const start = Math.min(this.selectionPivot, index);
      const end = Math.max(this.selectionPivot, index);
      const range = Array.from({ length: end - start + 1 }, (_, i) => start + i);
      if (multi) {
        const merged = new Set<number>([...appState.selectedIndices, ...range]);
        appState.selectedIndices = Array.from(merged).sort((a, b) => a - b);
      } else {
        appState.selectedIndices = range;
      }
    } else if (multi) {
      this.selectionPivot = index;
      if (appState.selectedIndices.includes(index)) {
        appState.selectedIndices = appState.selectedIndices.filter(i => i !== index);
      } else {
        appState.selectedIndices.push(index);
      }
    } else {
      this.selectionPivot = index;
      appState.selectedIndices = [index];
    }

    void this.loadFile(index, true);
  }

  /**
   * Returns the next index when stepping through duplicate groups.
   * Wraps around at both group and list boundaries.
   */
  private stepInDuplicates(
    fromIndex: number,
    direction: 1 | -1,
    groups: number[][],
    posByIndex: Map<number, { groupIdx: number; memberIdx: number }>,
    wrap: boolean
  ): number {
    const pos = posByIndex.get(fromIndex);
    if (!pos) return groups[FIRST_MEMBER][FIRST_MEMBER];
    const group = groups[pos.groupIdx];

    if (direction === 1) {
      if (pos.memberIdx < group.length - 1) {
        return group[pos.memberIdx + 1];
      }
      if (!wrap && pos.groupIdx >= groups.length - 1) {
        return fromIndex;
      }
      const nextGroupIdx = (pos.groupIdx + 1) % groups.length;
      return groups[nextGroupIdx][FIRST_MEMBER];
    }

    if (pos.memberIdx > 0) {
      return group[pos.memberIdx - 1];
    }
    if (!wrap && pos.groupIdx <= 0) {
      return fromIndex;
    }
    const prevGroupIdx = (pos.groupIdx - 1 + groups.length) % groups.length;
    const prevGroup = groups[prevGroupIdx];
    return prevGroup[prevGroup.length - 1];
  }

  /**
   * Returns the candidate index after stepping in `direction` within a
   * filtered list, falling back to the first item when not found.
   */
  private stepInList(fromIndex: number, direction: 1 | -1, list: number[], wrap: boolean): number {
    const from = list.indexOf(fromIndex);
    if (from === -1) return list[FIRST_MEMBER];
    if (!wrap) {
      const next = Math.max(0, Math.min(list.length - 1, from + direction));
      return list[next];
    }
    return list[(from + direction + list.length) % list.length];
  }

  private getTargetIndex(direction: 1 | -1, options: { wrap?: boolean } = {}): number {
    const wrap = options.wrap ?? true;
    const total = appState.stats.total;
    if (total === 0) return 0;

    const isFiltered = (filterState.filterMode !== FILTER_MODES.NONE || Object.keys(filterState.activeFilters).length > 0);
    const list = isFiltered ? (filterState.filteredIndices || []) : null;

    /**
     * Avoids landing on the comparison slot when stepping.
     * If the candidate collides, tries one more step; gives up if still colliding.
     */
    const avoidComparisonConflict = (candidate: number, step: (from: number) => number): number => {
      if (!viewState.comparisonMode || candidate !== this.comparisonIndex) {
        return candidate;
      }
      const alt = step(candidate);
      return alt !== this.comparisonIndex ? alt : candidate;
    };

    if (filterState.filterMode === FILTER_MODES.DUPLICATES && filterState.duplicateGroups.length > 0) {
      const groups = filterState.duplicateGroups;
      const posByIndex = new Map<number, { groupIdx: number; memberIdx: number }>();
      for (let groupIdx = 0; groupIdx < groups.length; groupIdx++) {
        const group = groups[groupIdx];
        for (let memberIdx = 0; memberIdx < group.length; memberIdx++) {
          posByIndex.set(group[memberIdx], { groupIdx, memberIdx });
        }
      }

      if (!posByIndex.has(this.currentIndex)) {
        return groups[FIRST_MEMBER][FIRST_MEMBER];
      }

      const step = (from: number): number => this.stepInDuplicates(from, direction, groups, posByIndex, wrap);
      return avoidComparisonConflict(step(this.currentIndex), step);
    }

    if (list && list.length > 0) {
      const idx = list.indexOf(this.currentIndex);
      if (idx === -1) return list[FIRST_MEMBER];

      const step = (from: number): number => this.stepInList(from, direction, list, wrap);
      const target = step(this.currentIndex);
      if (list.length < MIN_ITEMS_FOR_COLLISION_AVOIDANCE) return target;
      return avoidComparisonConflict(target, step);
    }

    const stepInAll = (fromIndex: number): number => {
      if (!wrap) return Math.max(0, Math.min(total - 1, fromIndex + direction));
      return (fromIndex + direction + total) % total;
    };
    const target = stepInAll(this.currentIndex);
    if (total < MIN_ITEMS_FOR_COLLISION_AVOIDANCE) return target;
    return avoidComparisonConflict(target, stepInAll);
  }

  /** Moves to the next photo, extending the selection if `shift` is held. */
  async next(shift: boolean = false, multi: boolean = false): Promise<void> {
    const target = this.getTargetIndex(1, { wrap: !shift });
    if (shift) this.select(target, multi, true);
    else await this.loadFile(target);
  }

  /** Moves to the previous photo, extending the selection if `shift` is held. */
  async prev(shift: boolean = false, multi: boolean = false): Promise<void> {
    const target = this.getTargetIndex(-1, { wrap: !shift });
    if (shift) this.select(target, multi, true);
    else await this.loadFile(target);
  }

  /**
   * Cancels any pending debounced save and immediately persists the current
   * position to the backend (used on app close / folder change).
   */
  persistPositionNow(): void {
    if (this.savePositionTimer) {
      clearTimeout(this.savePositionTimer);
      this.savePositionTimer = null;
    }
    api.savePosition(this.currentIndex).catch((e) => {
      logger.error('Failed to persist position', { index: this.currentIndex, error: e });
    });
  }

  private scheduleSavePosition(): void {
    if (this.savePositionTimer) {
      clearTimeout(this.savePositionTimer);
    }
    const index = this.currentIndex;
    this.savePositionTimer = setTimeout(() => {
      api.savePosition(index).catch((e) => {
        logger.error('Failed to save position', { index, error: e });
      });
      this.savePositionTimer = null;
    }, SAVE_POSITION_DEBOUNCE_MS);
  }
}

export const navigationService = new NavigationService();
