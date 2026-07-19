import { api } from './api';
import { toastService } from './toast.svelte';
import { backendErrorCode, localizeBackendError } from './backendError';
import { i18n } from './i18n.svelte';
import { domain, review } from '../../wailsjs/go/models';
import { viewState } from './viewState.svelte';
import { filterState } from './filterState.svelte';
import { watchService } from './watchService.svelte';
import { navigationService } from './navigationService.svelte';
import { syncService } from './syncService.svelte';
import { exportService } from './exportService.svelte';
import { trashService } from './trashService.svelte';
import { logger } from './logger';
import {
  ACTION_TRAIL_MAX_LENGTH,
  ANALYSIS_FLUSH_DEBOUNCE_MS,
  DEFAULT_MAX_LABEL,
  EVENTS,
  GRID_GAP,
  GRID_ITEM_SIZE,
  POLL_INTERVAL_MS,
  POLL_STALENESS_THRESHOLD_MS,
  UI_SETTLE_BEFORE_RELOAD_MS,
} from './constants';
import type { AppStateV2 } from './types';
import {
  focusFirstFilteredIfNeeded,
  getSelectedPaths,
  handleTournamentProgress,
  refreshAfterStateMutation,
  runExportFlow,
  setFilterMode
} from './appState.helpers';


type ActionTrailItem = {
  label: string;
  undoable: boolean;
  at: number;
};

class AppState {
  private static readonly exifWritableFormats = new Set(['JPEG', 'HEIC', 'PNG']);
  private initialized = false;
  lastNonUndoableAction = $state('');
  private progressPollTimer: ReturnType<typeof setInterval> | null = null;
  private lastStateUpdateAt = 0;
  private analysisFlushTimer: ReturnType<typeof setTimeout> | null = null;
  private pendingAnalysis: { current: number; total: number } | null = null;
  private pendingAnalysisAt = 0;
  private pendingAnalysisSource = 'none';
  
  isExporting = $state(false);

  /**
   * The authoritative, immutable application state snapshot pushed from the
   * backend. Named `v2` because it replaced an earlier mutable state model.
   * All structural photo data (VisibleOrder, Photos map, History) lives here.
   */
  v2 = $state<AppStateV2 | null>(null);

  get autoAdvance() { return viewState.config?.autoAdvance ?? false; }
  get maxLabel() { return this.stats?.maxLabel || DEFAULT_MAX_LABEL; }

  async toggleAutoAdvance(): Promise<void> {
    if (!viewState.config) return;
    const newConfig = { ...viewState.config, autoAdvance: !viewState.config.autoAdvance };
    await api.updateConfig(newConfig);
    viewState.config = newConfig;
  }

  get currentIndex() { return navigationService.currentIndex; }
  set currentIndex(v) { navigationService.currentIndex = v; }
  get comparisonIndex() { return navigationService.comparisonIndex; }
  set comparisonIndex(v) { navigationService.comparisonIndex = v; }
  get selectionPivot() { return navigationService.selectionPivot; }
  set selectionPivot(v) { navigationService.selectionPivot = v; }
  get currentFile() { return navigationService.currentFile; }
  set currentFile(v) { navigationService.currentFile = v; }
  get referenceFile() { return navigationService.referenceFile; }
  set referenceFile(v) { navigationService.referenceFile = v; }

  /** 
   * Returns the synchronous, authoritative target for actions based on current UI state.
   * Protects against race conditions from asynchronous currentFile updates.
   */
  get activeTarget(): { index: number; path: string } | null {
    const index = this.currentIndex;
    const path = this.v2?.VisibleOrder[index];
    if (!path) return null;
    return { index, path };
  }

  private filterContext() {
    const owner = this;
    return {
      get filterMode() { return filterState.filterMode; },
      set filterMode(value) { filterState.filterMode = value; },
      get activeFilters() { return filterState.activeFilters; },
      set activeFilters(value) { filterState.activeFilters = value; },
      get activeLabelFilter() { return filterState.activeLabelFilter; },
      set activeLabelFilter(value) { filterState.activeLabelFilter = value; },
      get duplicateGroups() { return filterState.duplicateGroups; },
      set duplicateGroups(value) { filterState.duplicateGroups = value; },
      get filteredIndices() { return filterState.filteredIndices; },
      set filteredIndices(value) { filterState.filteredIndices = value; },
      get gridOpen() { return viewState.gridOpen; },
      set gridOpen(value) { viewState.gridOpen = value; },
      get currentIndex() { return navigationService.currentIndex; },
      updateFilteredIndices: () => owner.updateFilteredIndices(),
      loadFile: (index: number, manual?: boolean) => owner.loadFile(index, manual),
    };
  }

  private tournamentContext() {
    const owner = this;
    return {
      get comparisonMode() { return viewState.comparisonMode; },
      get comparisonIndex() { return navigationService.comparisonIndex; },
      set comparisonIndex(value: number) { navigationService.comparisonIndex = value; },
      get autoAdvance() { return owner.autoAdvance; },
      next: () => owner.next(),
    };
  }

  // Legacy stats (will be gradually replaced by v2 properties)
  stats = $state<review.AppStats>({
    total: 0,
    initialTotal: 0,
    trashedCount: 0,
    starredCount: 0,
    labeledCount: 0,
    rotatedCount: 0,
    undoLen: 0,
    savedPosition: 0,
    maxLabel: DEFAULT_MAX_LABEL,
    heicSupported: false,
    version: '',
    ioWorkers: 0,
    hashDeferred: false,
    cacheMetaGc: 0,
    cacheHashGc: 0,
    cacheDerivedGc: 0
  });

  folders = $state<review.FolderInfo[]>([]);
  loading = $state(false);
  isScanning = $state(false);
  gridColumns = $state(1);
  analysis = $state({ current: 0, total: 0 });
  
  exportStatus = $state<{
    active: boolean;
    current: number;
    total: number;
    file: string;
    error: string | null;
  }>({
    active: false,
    current: 0,
    total: 0,
    file: '',
    error: null,
  });

  searchResults = $state<number[]>([]);
  searchActive = $state(false);
  searchQuery = $state('');
  perf = $state({
    progressEvents: 0,
    stateEvents: 0,
    completeEvents: 0,
    pollRequests: 0,
    analysisFlushes: 0,
    avgFlushDelayMs: 0,
    lastFlushDelayMs: 0,
    lastSource: 'none',
    schedulerMode: 'unknown',
    navPromotionTotal: 0,
    viewReadyLatencyMs: 0,
    viewReadyP50Ms: 0,
    activeModeMs: 0,
    idleModeMs: 0,
  });
  sessionVersion = $state(Date.now());
  sortOrder = $state<'name' | 'date'>('name');
  actionTrail = $state<ActionTrailItem[]>([]);
  runtimeCapabilities = $state<review.RuntimeCapabilities | null>(null);
  
  starredIndices = $derived.by<number[]>(() => {
    if (!this.v2 || !this.v2.VisibleOrder) {
      return [];
    }
    const photos = this.v2.Photos;
    return this.v2.VisibleOrder
      .map((id, index) => photos[id]?.IsStarred ? index : -1)
      .filter(idx => idx !== -1);
  });

  public updateStarredIndices(): void {
    // No-op: starredIndices is now a derived property.
  }

  selectedIndices = $state<number[]>([]);

  public pushAction(label: string, undoable: boolean): void {
    this.actionTrail = [{ label, undoable, at: Date.now() }, ...this.actionTrail].slice(0, ACTION_TRAIL_MAX_LENGTH);
    logger.debug('Action performed', { label, undoable });
  }

  private async runAction(action: () => Promise<unknown>, errorMsg: string): Promise<void> {
    this.loading = true;
    try {
      await action();
    } catch (e) {
      const rawMessage = e instanceof Error ? e.message : errorMsg;
      logger.error('UI Action failed', { error: rawMessage, msg: errorMsg });
      toastService.error(localizeBackendError(rawMessage));
    } finally {
      this.loading = false;
    }
  }

  async init(): Promise<void> {
    if (this.initialized) return;
    this.initialized = true;

    logger.info('AppState initialization started');
    watchService.start();
    this.ensureProgressPolling();

    // Listen for backend events
    api.onEvent(EVENTS.FOLDER_CHANGED, () => {
      logger.info('Backend signaled folder change, refreshing...');
      void this.refresh();
    });

    api.onEvent(EVENTS.STATE_UPDATE, (data: unknown) => {
      this.onStateUpdate(data as Parameters<typeof this.onStateUpdate>[0]);
    });

    api.onEvent(EVENTS.PROGRESS, (data: unknown) => {
      this.onAnalysisProgress(data as Parameters<typeof this.onAnalysisProgress>[0]);
    });

    api.onEvent(EVENTS.ANALYSIS_COMPLETE, (data: unknown) => {
      this.onAnalysisComplete(data as Parameters<typeof this.onAnalysisComplete>[0]);
    });

    api.onEvent(EVENTS.DUPLICATES_FOUND, (data: unknown) => {
      if (data) this.onDuplicatesFound(data as Parameters<typeof this.onDuplicatesFound>[0]);
    });

    api.onEvent(EVENTS.SEARCH_RESULTS, (data: unknown) => {
      if (data) this.onSearchResults(data as Parameters<typeof this.onSearchResults>[0]);
    });

    api.onEvent(EVENTS.SEARCH_COMPLETE, (data: unknown) => {
      if (data) this.onSearchComplete(data as Parameters<typeof this.onSearchComplete>[0]);
    });

    api.onEvent(EVENTS.EXPORT_PROGRESS, (data: unknown) => {
      if (data) this.onExportProgress(data as Parameters<typeof this.onExportProgress>[0]);
    });

    api.onEvent(EVENTS.EXPORT_COMPLETE, (data: unknown) => {
      this.onExportComplete(data as Parameters<typeof this.onExportComplete>[0]);
    });

    api.onEvent(EVENTS.EXPORT_ERROR, (data: unknown) => {
      if (data) this.onExportError(data as Parameters<typeof this.onExportError>[0]);
    });

    api.onEvent(EVENTS.EXPORT_CANCELLED, (data: unknown) => {
      this.onExportCancelled(data as Parameters<typeof this.onExportCancelled>[0]);
    });

    // Delegated to SyncService
    syncService.init(this);

    await this._doInit();
  }

  private async _doInit(): Promise<void> {
    this.loading = true;
    this.selectedIndices = [];
    viewState.gridScrollTop = 0;
    viewState.gridScrollLeft = 0;
    try {
      viewState.config = await api.getConfig();
      await this.refreshRuntimeCapabilities();
      const s = await api.getStats();
      if (s) this.stats = s;

      // v2 Initial Load
      const v2State = await api.getAppState() as unknown as AppStateV2;
      if (v2State && v2State.Root) {
        this.v2 = v2State;
        this.updateStarredIndices();
      }

      const hasPhotos = this.stats.total > 0 || (this.v2?.VisibleOrder?.length || 0) > 0;
      if (hasPhotos && viewState.current === 'picker') {
        logger.info('Photos detected (stats or v2), switching to review');
        viewState.current = 'review';
        const targetPos = this.stats.savedPosition || 0;
        await navigationService.loadFile(targetPos);
        // Load auxiliary data in parallel after initial file is visible
        void Promise.all([
          this.loadFolders(),
          this.updateFilteredIndices(),
          this.loadFilters()
        ]);
      }
    } catch (e) {
      logger.error('Init failed', { error: e });
    } finally {
      this.loading = false;
    }
  }

  async refreshRuntimeCapabilities(): Promise<void> {
    try {
      const info = await api.sysCheck();
      this.runtimeCapabilities = info?.capabilities || null;
    } catch (e) {
      logger.warn('Runtime capabilities check failed', { error: e });
    }
  }

  async loadFilters(): Promise<void> { await filterState.loadFilters(); }
  async updateFilteredIndices(): Promise<void> {
    await filterState.updateFilteredIndices();
    // Security: purge selection of indices that are no longer visible after filter change
    if (filterState.filterMode !== 'none' || Object.keys(filterState.activeFilters).length > 0) {
      const visibleSet = new Set(filterState.filteredIndices);
      const prevCount = this.selectedIndices.length;
      this.selectedIndices = this.selectedIndices.filter(idx => visibleSet.has(idx));
      if (this.selectedIndices.length !== prevCount) {
        logger.debug('Selection purged after filter update', {
          removed: prevCount - this.selectedIndices.length
        });
      }
    }
  }
  async setFilter(type: 'camera' | 'iso', value: string): Promise<void> {
    await filterState.setFilter(type, value);
    await focusFirstFilteredIfNeeded(this.filterContext());
  }
  async setAdvancedFilter(type: 'dateFrom' | 'dateTo' | 'sizeMin' | 'sizeMax', value: string): Promise<void> {
    await filterState.setAdvancedFilter(type, value);
    if (filterState.filteredIndices.length > 0 && !filterState.filteredIndices.includes(this.currentIndex)) {
      await this.loadFile(filterState.filteredIndices[0]);
    }
  }
  async clearFilters(): Promise<void> { await filterState.clearFilters(); }

  clearAllFilters(): void {
    filterState.filterMode = 'none';
    filterState.duplicateGroups = [];
    filterState.filteredIndices = [];
    void this.clearFilters();
  }

  exitDuplicatesMode(options?: { keepGrid?: boolean }): void {
    const keepGrid = options?.keepGrid ?? false;
    if (viewState.comparisonMode) {
      viewState.comparisonMode = false;
      this.referenceFile = null;
    }
    filterState.filterMode = 'none';
    filterState.filteredIndices = [];
    filterState.duplicateGroups = [];
    viewState.gridOpen = keepGrid;
  }

  toggleTheme(): void { viewState.toggleTheme(); }

  async toggleComparisonMode(): Promise<void> {
    const turningOn = !viewState.comparisonMode;
    if (turningOn && this.stats.total > 1) {
      // Current becomes reference, next becomes active
      const ref = this.currentIndex;
      let active = this.currentIndex + 1;
      if (active >= this.stats.total) active = this.currentIndex - 1;
      
      this.comparisonIndex = ref;
      viewState.comparisonMode = true; // Set mode BEFORE loadFile
      await this.loadFile(active, true);
    } else {
      viewState.comparisonMode = false;
      this.referenceFile = null;
    }
  }

  async toggleDuplicatesFilter(): Promise<void> {
    logger.info('Toggling duplicates filter', { active: filterState.filterMode !== 'duplicates' });
    if (filterState.filterMode === 'duplicates') {
      this.exitDuplicatesMode({ keepGrid: viewState.gridOpen && !viewState.comparisonMode });
    } else {
      this.loading = true;
      try {
        await setFilterMode(this.filterContext(), 'duplicates');
        if (this.stats.total > 1) {
          viewState.comparisonMode = true; // Explicit side-effect here, better than in loadFile
        }
      } finally {
        this.loading = false;
      }
    }
  }

  async toggleStarFilter(): Promise<void> {
    logger.info('Toggling star filter');
    if (filterState.filterMode === 'starred') {
      await setFilterMode(this.filterContext(), 'none', undefined, {
        keepGridOnNone: viewState.gridOpen && !viewState.comparisonMode,
      });
    } else {
      await setFilterMode(this.filterContext(), 'starred');
    }
  }

  async setLabelFilter(label: number): Promise<void> {
    logger.info('Setting label filter', { label });
    if (filterState.filterMode === 'label' && filterState.activeLabelFilter === label) {
      await setFilterMode(this.filterContext(), 'none', undefined, {
        keepGridOnNone: viewState.gridOpen && !viewState.comparisonMode,
      });
    } else {
      await setFilterMode(this.filterContext(), 'label', label);
    }
  }

  async selectFolder(path: string): Promise<void> {
    logger.info('Selecting new folder', { path });
    await this.runAction(async () => {
      await api.openFolder(path);
      this.sessionVersion = Date.now();
      await this._doInit();
    }, 'Failed to open folder');
  }

  async loadFolders(): Promise<void> {
    try {
      const res = await api.getFolders();
      this.folders = res?.folders || [];
    } catch (e) {
      logger.warn('Failed to load folders', { error: e });
    }
  }

  async revealInExplorer(index: number): Promise<void> {
    try {
      await api.revealInExplorer(index);
    } catch (e) {
      const message = e instanceof Error ? e.message : String(e);
      logger.error('Failed to reveal in explorer', { index, error: message });
      toastService.error(localizeBackendError(message));
    }
  }

  async loadFile(index: number, manual: boolean = false): Promise<void> { await navigationService.loadFile(index, manual); }
  select(index: number, multi: boolean = false, shift: boolean = false): void { navigationService.select(index, multi, shift); }
  persistPositionNow(): void { navigationService.persistPositionNow(); }

  async loadDuplicates(): Promise<void> {
    await filterState.loadDuplicates();
  }

  async prioritizeIndices(indices: number[]): Promise<void> {
    if (indices.length === 0) return;
    await api.prioritizeIndices(indices);
  }

  private async runBatchOrSingleMetadata(
    label: string,
    batchFn: (paths: string[]) => Promise<unknown>,
    singleFn: (target: { index: number; path: string }) => Promise<unknown>
  ): Promise<void> {
    const target = this.activeTarget;
    if (!target) return;

    await this.runAction(async () => {
      if (this.selectedIndices.length > 1) {
        const selectedPaths = getSelectedPaths(this.selectedIndices, this.v2?.VisibleOrder || []);
        const res = await batchFn(selectedPaths);
        this.pushAction(label, true);
        return res;
      } else {
        const res = await singleFn(target);
        if (res) {
          this.pushAction(label, true);
          return res;
        }
      }
    }, `Action failed: ${label}`);
  }

  private applyLabelSnapshot(paths: string[], label: number): void {
    if (!this.v2 || paths.length === 0) return;

    const changedPaths = new Set(paths);
    const photos = { ...this.v2.Photos };
    for (const path of changedPaths) {
      const photo = photos[path];
      if (photo) photos[path] = { ...photo, Label: label };
    }
    this.v2 = { ...this.v2, Photos: photos };

    if (this.currentFile && changedPaths.has(this.currentFile.filename)) {
      this.currentFile = { ...this.currentFile, label } as review.FileResponse;
    }
  }

  async toggleStar(): Promise<void> {
    const target = this.activeTarget;
    if (!target) return;

    const isCurrentlyStarred = this.v2?.Photos[target.path]?.IsStarred ?? false;
    
    await this.runBatchOrSingleMetadata(
      isCurrentlyStarred ? i18n.t('action_unstar') : i18n.t('action_star'),
      async (paths) => {
        let allStarred = true;
        for (const p of paths) {
          if (!this.v2?.Photos[p]?.IsStarred) {
            allStarred = false;
            break;
          }
        }
        const shouldStar = !allStarred;
        await api.toggleStar(undefined, undefined, paths, shouldStar);
      },
      async (t) => {
        const res = await api.toggleStar(t.index, t.path, undefined, !isCurrentlyStarred);
        if (res) await handleTournamentProgress(this.tournamentContext(), t.index, !isCurrentlyStarred);
        return res;
      }
    );
    this.lastNonUndoableAction = '';
  }

  async setLabel(label: number): Promise<void> {
    const target = this.activeTarget;
    if (!target) return;

    await this.runBatchOrSingleMetadata(
      i18n.t('action_label_set'),
      async (paths) => {
        let allHaveLabel = true;
        for (const p of paths) {
          if (this.v2?.Photos[p]?.Label !== label) {
            allHaveLabel = false;
            break;
          }
        }
        const finalLabel = allHaveLabel ? 0 : label;
        const res = await api.setLabel(undefined, undefined, paths, finalLabel);
        this.applyLabelSnapshot(paths, finalLabel);
        return res;
      },
      async (t) => {
        const currentLabel = this.v2?.Photos[t.path]?.Label ?? 0;
        const finalLabel = (currentLabel === label) ? 0 : label;
        const res = await api.setLabel(t.index, t.path, undefined, finalLabel);
        if (res) {
          this.applyLabelSnapshot([t.path], finalLabel);
          await handleTournamentProgress(this.tournamentContext(), t.index, finalLabel > 0);
        }
        return res;
      }
    );

    if (filterState.filterMode === 'label') await this.updateFilteredIndices();
    this.lastNonUndoableAction = '';
  }

  /** Shared helper: calls `resetFn`, then re-initialises app state. */
  private async runResetAction(label: string, resetFn: () => Promise<unknown>): Promise<void> {
    logger.warn(`Resetting all ${label}`);
    await this.runAction(async () => {
      await resetFn();
      await this._doInit();
    }, 'Reset failed');
  }

  async resetStars(): Promise<void> {
    await this.runResetAction('stars', () => api.resetStars());
  }

  async resetLabels(): Promise<void> {
    await this.runResetAction('labels', () => api.resetLabels());
  }

  async resetAppCache(): Promise<void> {
    logger.warn('RESET APP CACHE TRIGGERED');
    await this.runAction(async () => {
      await api.resetAppCache();
      viewState.current = 'picker';
      // Wait for UI state to settle before full reload
      setTimeout(() => {
        window.location.reload();
      }, UI_SETTLE_BEFORE_RELOAD_MS);
    }, i18n.t('reset_app_cache_failed'));
  }

  async refreshAfterStateMutation(targetIndex: number, options?: { refreshFilters?: boolean; resetDuplicateGroups?: boolean }): Promise<void> {
    // Structural changes (Trash/Undo) trigger a SyncState (IsPartial=true) from backend.
    // Reactive $derived properties handle the rest.
    this.validateSelection();
    await refreshAfterStateMutation(this.filterContext(), targetIndex, options);
  }

  async trash(): Promise<void> {
    await trashService.trash();
  }

  async undo(): Promise<void> {
    await trashService.undo();
  }

  async rotate(direction: 'right' | 'left'): Promise<void> {
    const target = this.activeTarget;
    if (!target) return;

    const res = await api.rotate(target.index, target.path, direction);
    if (res) {
      this.lastNonUndoableAction = '';
      this.pushAction(direction === 'left' ? i18n.t('action_rotate_left') : i18n.t('action_rotate_right'), true);
    }
  }

  async rotateReset(): Promise<void> {
    const target = this.activeTarget;
    if (!target) return;

    const res = await api.rotateReset(target.index, target.path);
    if (res) {
      this.lastNonUndoableAction = '';
      this.pushAction(i18n.t('action_rotate_reset'), true);
    }
  }

  async applyRotation(): Promise<void> {
    if (!this.canApplyRotation()) return;
    const target = this.activeTarget;
    if (!target) return;

    logger.info('Applying EXIF rotation to file', { filename: target.path });
    await this.runAction(async () => {
      await api.applyRotation(target.index, target.path);
      this.sessionVersion = Date.now();
      await this.loadFile(target.index);
      toastService.success(i18n.t('apply_success'));
      this.lastNonUndoableAction = 'apply_rotation';
      this.pushAction(i18n.t('action_apply_rotation'), false);
    }, i18n.t('apply_failed'));
  }

  async reanalyzeMetadata(): Promise<void> {
    if (!this.currentFile) return;
    const targetIndex = this.currentFile.index;
    const targetPath = this.currentFile.filename;
    await this.runAction(async () => {
      const file = await api.reanalyzeMetadata(targetIndex, targetPath);
      if (file) {
        this.currentFile = file;
      } else {
        await this.loadFile(targetIndex);
      }
      toastService.success(i18n.t('metadata_reanalyzed'));
      this.pushAction(i18n.t('action_reanalyze_metadata'), false);
    }, i18n.t('metadata_reanalyze_failed'));
  }

  canApplyExifWrite(): boolean {
    const format = this.currentFile?.format;
    if (!format) return false;
    if (!this.runtimeCapabilities) return AppState.exifWritableFormats.has(format.toUpperCase());
    if (this.runtimeCapabilities.exifWrite === false) return false;
    return AppState.exifWritableFormats.has(format.toUpperCase());
  }

  hasPendingRotation(): boolean { return !!this.currentFile?.rotation; }
  canApplyRotation(): boolean { return this.hasPendingRotation() && this.canApplyExifWrite(); }
  canUndo(): boolean { return (this.v2?.UndoLen || 0) > 0; }

  async refresh(): Promise<void> {
    logger.info('Manual folder refresh triggered');
    await this.runAction(async () => {
      const res = await api.refresh(this.currentIndex);
      this.sessionVersion = Date.now(); // Cache buster

      const refreshedIndex = res?.index ?? -1;
      if ((res?.total ?? 0) === 0 || refreshedIndex < 0) {
        this.currentFile = null;
        this.currentIndex = 0;
      } else {
        await this.loadFile(refreshedIndex);
      }
      this.pushAction(i18n.t('action_refresh'), false);
    }, 'Refresh failed');
  }

  async setSortOrder(order: 'name' | 'date'): Promise<void> {
    logger.info('Changing sort order', { order });
    await this.runAction(async () => {
      await api.setSortOrder(order);
      this.sortOrder = order;
      this.sessionVersion = Date.now();
      await this.init();
      this.pushAction(`${i18n.t('action_sort')} ${order}`, false);
    }, 'Sort failed');
  }

  async exportSelection(criteria: 'starred' | 'label', label: number = 0, move: boolean = false): Promise<void> {
    await exportService.exportSelection(this, criteria, label, move);
  }

  reconcileMovedPhotos(paths: string[]): void {
    if (!this.v2 || paths.length === 0) return;

    const removed = new Set(paths);
    const oldOrder = this.v2.VisibleOrder;
    const currentPath = this.currentFile?.filename;
    const filteredPaths = filterState.filteredIndices
      .map((index) => oldOrder[index])
      .filter((path): path is string => Boolean(path) && !removed.has(path));
    const visibleOrder = oldOrder.filter((path) => !removed.has(path));
    const photos = Object.fromEntries(
      visibleOrder.flatMap((path) => {
        const photo = this.v2?.Photos[path];
        return photo ? [[path, photo] as const] : [];
      }),
    );
	const indexByPath = new Map<string, number>();
	for (let index = 0; index < visibleOrder.length; index++) {
		indexByPath.set(visibleOrder[index], index);
	}

    this.v2 = { ...this.v2, VisibleOrder: visibleOrder, Photos: photos };
    filterState.filteredIndices = filteredPaths
		.map((path) => indexByPath.get(path) ?? -1)
      .filter((index) => index >= 0);
    if (currentPath && removed.has(currentPath)) {
      this.currentFile = null;
      this.currentIndex = Math.min(this.currentIndex, Math.max(visibleOrder.length - 1, 0));
    } else if (currentPath) {
		const currentIndex = indexByPath.get(currentPath) ?? -1;
      if (currentIndex >= 0) {
        this.currentIndex = currentIndex;
        if (this.currentFile) {
          this.currentFile = { ...this.currentFile, index: currentIndex } as review.FileResponse;
        }
      }
    }
    this.validateSelection();
  }

  async exportSelected(move: boolean = false): Promise<void> {
    await exportService.exportSelected(this, move);
  }

  selectAll(): void {
    if (filterState.filterMode !== 'none' || Object.keys(filterState.activeFilters).length > 0) {
      this.selectedIndices = [...filterState.filteredIndices];
    } else {
      const total = this.v2?.VisibleOrder?.length || 0;
      this.selectedIndices = Array.from({ length: total }, (_, i) => i);
    }
    logger.debug('Selected all items', { count: this.selectedIndices.length });
  }

  getThumbUrl(index: number): string {
    return `/raw-media/thumb/${index}?v=${this.sessionVersion}`;
  }

  getFullUrl(index: number): string {
    return `/raw-media/full/${index}?v=${this.sessionVersion}`;
  }

  private queueAnalysisUpdate(current: number, total: number, force: boolean = false, source: string = 'unknown'): void {
    if (!this.pendingAnalysis) {
      this.pendingAnalysisAt = Date.now();
      this.pendingAnalysisSource = source;
    } else if (source !== 'poll') {
      this.pendingAnalysisSource = source;
    }
    this.pendingAnalysis = {
      current: Math.max(0, current),
      total: Math.max(0, total),
    };
    if (force) {
      this.flushAnalysis();
      return;
    }
    if (this.analysisFlushTimer) return;
    this.analysisFlushTimer = setTimeout(() => {
      this.flushAnalysis();
      this.analysisFlushTimer = null;
    }, ANALYSIS_FLUSH_DEBOUNCE_MS);
  }

  private flushAnalysis(): void {
    if (!this.pendingAnalysis) return;
    this.analysis = this.pendingAnalysis;
    this.lastStateUpdateAt = Date.now();
    const delay = this.pendingAnalysisAt > 0 ? Date.now() - this.pendingAnalysisAt : 0;
    this.perf.analysisFlushes += 1;
    this.perf.lastFlushDelayMs = delay;
    this.perf.avgFlushDelayMs = Math.round(((this.perf.avgFlushDelayMs * (this.perf.analysisFlushes - 1)) + delay) / this.perf.analysisFlushes);
    this.perf.lastSource = this.pendingAnalysisSource;
    this.pendingAnalysisAt = 0;
    this.pendingAnalysisSource = 'none';
    this.pendingAnalysis = null;
  }

  onAnalysisProgress(data: {
    current?: number;
    total?: number;
    scanning?: boolean;
    scheduler_mode?: string;
    nav_promotion_total?: number;
    view_ready_latency?: number;
    view_ready_latency_p50?: number;
    active_mode_ms?: number;
    idle_mode_ms?: number;
  } = {}): void {
    this.perf.progressEvents += 1;
    const total = Number(data?.total || 0);
    const current = Number(data?.current || 0);
    this.isScanning = !!data?.scanning;
    if (data?.scheduler_mode) {
      this.perf.schedulerMode = String(data.scheduler_mode);
    }
    if (data?.nav_promotion_total !== undefined) {
      this.perf.navPromotionTotal = Number(data.nav_promotion_total || 0);
    }
    if (data?.view_ready_latency !== undefined) {
      this.perf.viewReadyLatencyMs = Number(data.view_ready_latency || 0);
    }
    if (data?.view_ready_latency_p50 !== undefined) {
      this.perf.viewReadyP50Ms = Number(data.view_ready_latency_p50 || 0);
    }
    if (data?.active_mode_ms !== undefined) {
      this.perf.activeModeMs = Number(data.active_mode_ms || 0);
    }
    if (data?.idle_mode_ms !== undefined) {
      this.perf.idleModeMs = Number(data.idle_mode_ms || 0);
    }
    this.queueAnalysisUpdate(current, total, false, 'progress');
  }

  onAnalysisComplete(data: { total?: number } = {}): void {
    this.perf.completeEvents += 1;
    logger.info('Analysis completed', { total: data.total });
    this.isScanning = false;
    const done = Number(data?.total || this.analysis.total || 0);
    this.queueAnalysisUpdate(done, done, true, 'analysis:complete');
    this.loadFilters();
    if (filterState.filterMode === 'duplicates') {
      filterState.duplicateGroups = [];
      this.loadDuplicates();
    }
  }

  onExportProgress(data: { current: number; total: number; file: string }): void {
    this.exportStatus.active = true;
    this.exportStatus.current = data.current;
    this.exportStatus.total = data.total;
    this.exportStatus.file = data.file;
    this.exportStatus.error = null;
  }

  onExportComplete(data?: { root?: string; movedPaths?: string[] }): void {
    const movedPaths = data?.movedPaths;
    if (data?.root === this.v2?.Root && movedPaths?.length) {
      this.reconcileMovedPhotos(movedPaths);
    }
    this.exportStatus.active = false;
    toastService.success(i18n.t('export_started')); // The notification text might need updating to "Export finished"
  }

  onExportError(data: { error: string }): void {
    this.exportStatus.active = false;
    this.exportStatus.error = data.error;
    toastService.error(i18n.t('export_failed') + ': ' + data.error);
  }

  onExportCancelled(data?: { root?: string; movedPaths?: string[] }): void {
    const movedPaths = data?.movedPaths;
    if (data?.root === this.v2?.Root && movedPaths?.length) {
      this.reconcileMovedPhotos(movedPaths);
    }
    this.exportStatus.active = false;
    toastService.info(i18n.t('cancel'));
  }

  onDuplicatesFound(data: { count: number }): void {
    if (filterState.filterMode === 'duplicates') {
      this.loadDuplicates();
    }
  }

  onSearchResults(data: { indices: number[]; query: string; append: boolean }): void {
    if (data.query !== this.searchQuery) return;
    this.searchActive = true;
    if (data.append) {
      this.searchResults = [...this.searchResults, ...data.indices];
    } else {
      this.searchResults = data.indices;
    }
  }

  onSearchComplete(data: { indices: number[]; query: string }): void {
    if (data.query !== this.searchQuery) return;
    this.searchResults = [...this.searchResults, ...data.indices];
    this.searchActive = false;
  }

  async cancelExport(): Promise<void> {
    await api.cancelExport();
    this.exportStatus.active = false;
  }

  search(query: string): void {
    this.searchQuery = query;
    this.searchResults = [];
    if (!query) {
      this.searchActive = false;
      return;
    }
    this.searchActive = true;
    api.searchStream(query);
  }


  onStateUpdate(data: { stats?: review.AppStats; analysis?: { current?: number; total?: number }; thumbnails?: { current?: number; total?: number } } = {}): void {
    this.perf.stateEvents += 1;
    if (data?.stats) {
      if (data.stats.total !== this.stats.total || data.stats.trashedCount !== this.stats.trashedCount) {
        this.sessionVersion = Date.now(); // List structure changed, invalidate cache
      }
      this.stats = data.stats;

      // Auto-transition to review if photos appear while on picker
      if (this.stats.total > 0 && viewState.current === "picker") {
        logger.info("Photos detected via state update, switching to review");
        viewState.current = "review";
        void navigationService.loadFile(this.stats.savedPosition || 0);
        
        // CRITICAL: Ensure side data is loaded during auto-transition
        void Promise.all([
          this.loadFolders(),
          this.loadFilters()
        ]);
      }
    }
    if (data?.analysis) {
      this.queueAnalysisUpdate(Number(data.analysis.current || 0), Number(data.analysis.total || 0), false, "state:update");
    } else {
      this.lastStateUpdateAt = Date.now();
    }
  }

  private ensureProgressPolling(): void {
    if (this.progressPollTimer) return;
    this.progressPollTimer = setInterval(async () => {
      if (viewState.current !== 'review') return;
      if (Date.now() - this.lastStateUpdateAt < POLL_STALENESS_THRESHOLD_MS) return;
      try {
        this.perf.pollRequests += 1;
        const p = await api.getAnalysisProgress();
        if (p.total > 0) {
          this.queueAnalysisUpdate(p.current, p.total, false, 'poll');
        }
      } catch (e) {
        logger.warn('Progress poll failed', { error: e });
      }
    }, POLL_INTERVAL_MS);
  }

  async next(shift: boolean = false, multi: boolean = false): Promise<void> { await navigationService.next(shift, multi); }
  async prev(shift: boolean = false, multi: boolean = false): Promise<void> { await navigationService.prev(shift, multi); }
  first(shift: boolean = false, multi: boolean = false): void {
    if (shift) {
      this.select(0, multi, true);
      return;
    }
    navigationService.loadFile(0);
  }
  last(shift: boolean = false, multi: boolean = false): void {
    if (this.stats.total <= 0) return;
    const target = this.stats.total - 1;
    if (shift) {
      this.select(target, multi, true);
      return;
    }
    navigationService.loadFile(target);
  }

  /** Ensures selectedIndices don't point to non-existent items after structural changes */
  validateSelection(): void {
    if (this.selectedIndices.length === 0) return;
    const max = (this.v2?.VisibleOrder?.length || 0) - 1;
    this.selectedIndices = this.selectedIndices.filter(idx => idx >= 0 && idx <= max);
  }

  gridNextRow(shift: boolean = false, multi: boolean = false): void {
    const columns = this.gridColumns;
    const target = Math.min((this.v2?.VisibleOrder?.length || 1) - 1, this.currentIndex + columns);
    if (shift) this.select(target, multi, true);
    else this.loadFile(target, false); // isManualClick = false so loadFile updates pivot
  }

  gridPrevRow(shift: boolean = false, multi: boolean = false): void {
    const columns = this.gridColumns;
    const target = Math.max(0, this.currentIndex - columns);
    if (shift) this.select(target, multi, true);
    else this.loadFile(target, false); // isManualClick = false so loadFile updates pivot
  }

  gridPageUp(shift: boolean = false, multi: boolean = false): void {
    const columns = this.gridColumns || 4;
    const rows = 4; // Jump 4 rows
    const target = Math.max(0, this.currentIndex - (columns * rows));
    if (shift) this.select(target, multi, true);
    else this.loadFile(target, false);
  }

  gridPageDown(shift: boolean = false, multi: boolean = false): void {
    const columns = this.gridColumns || 4;
    const rows = 4; // Jump 4 rows
    const target = Math.min((this.v2?.VisibleOrder?.length || 1) - 1, this.currentIndex + (columns * rows));
    if (shift) this.select(target, multi, true);
    else this.loadFile(target, false);
  }
}

export const appState = new AppState();
