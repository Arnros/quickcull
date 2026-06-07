import { api } from './api';
import { i18n } from './i18n.svelte';
import { toastService } from './toast.svelte';

type FilterMode = 'none' | 'starred' | 'label' | 'duplicates';

type FilterCtx = {
  filterMode: FilterMode;
  activeFilters: { camera?: string; iso?: string };
  activeLabelFilter: number;
  duplicateGroups: number[][];
  filteredIndices: number[];
  gridOpen: boolean;
  currentIndex: number;
  updateFilteredIndices: () => Promise<void>;
  loadFile: (index: number, manual?: boolean) => Promise<void>;
};

type RefreshCtx = {
  duplicateGroups: number[][];
  loadFile: (index: number, manual?: boolean) => Promise<void>;
  updateFilteredIndices: () => Promise<void>;
  filterMode: FilterMode;
  activeFilters: { camera?: string; iso?: string };
};

type ExportCtx = {
  isExporting: boolean;
};

/** Returns true when any filter mode or attribute filter is currently active. */
export function hasActiveFiltering(filterMode: FilterMode, activeFilters: { camera?: string; iso?: string }): boolean {
  return filterMode !== 'none' || Object.keys(activeFilters).length > 0;
}

/** Maps an array of selection indices to their corresponding file paths, dropping out-of-range indices. */
export function getSelectedPaths(selectedIndices: number[], visibleOrder: string[]): string[] {
  return selectedIndices
    .map(idx => visibleOrder[idx])
    .filter(path => !!path);
}

/** Navigates to the first filtered index when the current index is not included in the filtered set. */
export async function focusFirstFilteredIfNeeded(ctx: FilterCtx): Promise<void> {
  if (ctx.filteredIndices.length > 0 && !ctx.filteredIndices.includes(ctx.currentIndex)) {
    await ctx.loadFile(ctx.filteredIndices[0]);
  }
}

/** Applies the requested filter mode, resetting conflicting state and navigating to the first match. */
export async function setFilterMode(
  ctx: FilterCtx,
  mode: FilterMode,
  label?: number,
  options?: { keepGridOnNone?: boolean }
): Promise<void> {
  ctx.filterMode = mode;
  if (mode !== 'label') {
    ctx.activeLabelFilter = 0;
  }
  if (mode === 'duplicates') {
    ctx.activeFilters = {};
    ctx.gridOpen = true;
  }
  if (mode === 'none') {
    ctx.duplicateGroups = [];
    ctx.filteredIndices = [];
    ctx.gridOpen = options?.keepGridOnNone ?? false;
    return;
  }
  if (mode === 'label' && typeof label === 'number') {
    ctx.activeLabelFilter = label;
  }
  await ctx.updateFilteredIndices();
  await focusFirstFilteredIfNeeded(ctx);
}

/**
 * Refreshes the file view and optional filter state after a structural mutation
 * (trash, undo). Skips `loadFile` for negative sentinel indices returned by the backend.
 */
export async function refreshAfterStateMutation(
  ctx: RefreshCtx,
  targetIndex: number,
  options?: { refreshFilters?: boolean; resetDuplicateGroups?: boolean }
): Promise<void> {
  const refreshFilters = options?.refreshFilters ?? hasActiveFiltering(ctx.filterMode, ctx.activeFilters);
  if (options?.resetDuplicateGroups) {
    ctx.duplicateGroups = [];
  }
  
  // Local sync of starred indices is now handled synchronously in AppState.
  // Guard against invalid sentinel indices from backend responses.
  const updates: Promise<void>[] = [];
  if (Number.isFinite(targetIndex) && targetIndex >= 0) {
    updates.push(ctx.loadFile(targetIndex));
  }
  if (refreshFilters) {
    updates.push(ctx.updateFilteredIndices());
  }
  if (updates.length === 0) {
    return;
  }
  await Promise.all(updates);
}

/** Opens the destination folder picker, guards against concurrent exports, and runs the export. */
export async function runExportFlow(ctx: ExportCtx, exporter: (destPath: string) => Promise<void>): Promise<void> {
  if (ctx.isExporting) return;
  const res = await api.browseDialog();
  if (!res?.path) return;
  ctx.isExporting = true;
  try {
    toastService.show(i18n.t('export_started'), 'info');
    await exporter(res.path);
  } finally {
    ctx.isExporting = false;
  }
}

type TournamentCtx = {
  comparisonMode: boolean;
  comparisonIndex: number;
  autoAdvance: boolean;
  next: () => Promise<void>;
};

/** Advances to the next candidate in tournament/comparison mode after a star or label action. */
export async function handleTournamentProgress(ctx: TournamentCtx, targetIndex: number, isSelected: boolean): Promise<void> {
  // TOURNAMENT LOGIC: If we select (star/label) the ACTIVE photo (Right) in comparison mode
  if (ctx.comparisonMode && isSelected && targetIndex !== ctx.comparisonIndex) {
    ctx.comparisonIndex = targetIndex; // It becomes the new reference
    await ctx.next(); // And we move to the next candidate
  } else if (ctx.autoAdvance) {
    await ctx.next();
  }
}
