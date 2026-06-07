import { api } from './api';
import { appState } from './appState.svelte';
import { i18n } from './i18n.svelte';
import { toastService } from './toast.svelte';
import { logger } from './logger';
import { getSelectedPaths, hasActiveFiltering } from './appState.helpers';
import { backendErrorCode, localizeBackendError } from './backendError';
import { FILTER_MODES } from './constants';
import { review } from '../../wailsjs/go/models';
import type { translations } from './translations';

/** Delay (ms) before reloading the folder list after a trash/restore action. */
const FOLDER_RELOAD_DELAY_MS = 200;

/** Translation key lookup for each undoable action type returned by the backend. */
const UNDO_ACTION_KEY: Record<string, keyof (typeof translations)['en']> = {
  trash: 'action_undone_trash',
  star: 'action_undone_star',
  rotate: 'action_undone_rotate',
  label: 'action_undone_label',
};

class TrashService {
  loading = $state(false);
  restoring = $state(false);
  isTrashing = $state(false);
  items = $state<string[]>([]);
  selectedItems = $state<string[]>([]);
  pickerOpen = $state(false);

  /** Fetches the current trash contents and pre-selects all items. */
  async list(): Promise<review.TrashListResponse> {
    this.loading = true;
    try {
      const res = await api.listTrash();
      this.items = res.items || [];
      this.selectedItems = [...this.items];
      logger.debug('Trash items listed', { count: this.items.length });
      return res;
    } finally {
      this.loading = false;
    }
  }

  /**
   * Moves the active file (or current selection) to the system trash.
   * Handles single, multi-select, and duplicate-compare scenarios.
   */
  async trash(): Promise<void> {
    const target = appState.activeTarget;
    if (!target || this.isTrashing) return;

    this.isTrashing = true;
    const wasInDuplicateCompare = appState.comparisonMode && appState.filterMode === FILTER_MODES.DUPLICATES;

    try {
      const selectedPaths = getSelectedPaths(appState.selectedIndices, appState.v2?.VisibleOrder || []);
      const explicitSingleSelection =
        appState.selectedIndices.length === 1 &&
        appState.selectedIndices[0] !== target.index;
      const useSelectedPaths =
        selectedPaths.length > 0 &&
        (appState.selectedIndices.length > 1 || explicitSingleSelection);

      const effectiveIndex =
        explicitSingleSelection && selectedPaths.length === 1
          ? appState.selectedIndices[0]
          : target.index;
      const effectivePath =
        explicitSingleSelection && selectedPaths.length === 1
          ? selectedPaths[0]
          : target.path;

      const res = await api.trash(
        effectiveIndex,
        effectivePath,
        useSelectedPaths ? selectedPaths : undefined
      );
      appState.selectedIndices = [];
      appState.sessionVersion = Date.now();

      await appState.refreshAfterStateMutation(res.index ?? 0, {
        refreshFilters: hasActiveFiltering(appState.filterMode, appState.activeFilters),
        resetDuplicateGroups: wasInDuplicateCompare,
      });

      if (res && res.total === 0) {
        appState.view = 'picker';
      } else if (res && wasInDuplicateCompare) {
        const hasRemainingDuplicatePair = appState.duplicateGroups.some((group) => group.length > 1);
        if (hasRemainingDuplicatePair) {
          appState.gridOpen = false;
          appState.comparisonMode = true;
          await appState.next();
        } else {
          appState.comparisonMode = false;
          appState.gridOpen = true;
        }
      }

      appState.pushAction(i18n.t('action_trash'), true);
      appState.lastNonUndoableAction = '';
      setTimeout(() => appState.loadFolders(), FOLDER_RELOAD_DELAY_MS);
    } catch (e: unknown) {
      const rawMessage = e instanceof Error ? e.message : i18n.t('moved_to_trash');
      logger.error('Trash action failed', { error: rawMessage });
      toastService.error(localizeBackendError(rawMessage));
    } finally {
      this.isTrashing = false;
    }
  }

  /** Undoes the most recent undoable action and navigates to the affected file. */
  async undo(): Promise<void> {
    appState.loading = true;
    appState.lastNonUndoableAction = 'undo';
    try {
      const res = await api.undo();
      if (res) {
        logger.info('Undo action successful', { type: res.actionType });
        appState.sessionVersion = Date.now();

        const targetIndex = (typeof res.index === 'number' && res.index >= 0)
          ? res.index
          : appState.currentIndex;

        await appState.refreshAfterStateMutation(targetIndex, {
          refreshFilters: hasActiveFiltering(appState.filterMode, appState.activeFilters)
        });

        const key = UNDO_ACTION_KEY[res.actionType] || 'action_undone';
        toastService.success(i18n.t(key));
        appState.pushAction(i18n.t(key), false);
      }
    } catch (e: unknown) {
      const rawMessage = e instanceof Error ? e.message : i18n.t('nothing_to_undo');
      logger.error('Undo action failed', { error: rawMessage });
      const code = backendErrorCode(rawMessage);
      if (code === 'nothing_to_undo' && appState.lastNonUndoableAction === 'apply_rotation') {
        toastService.error(i18n.t('nothing_to_undo_after_apply_rotation'));
      } else {
        toastService.error(localizeBackendError(rawMessage));
      }
    } finally {
      appState.loading = false;
      appState.lastNonUndoableAction = '';
    }
  }

  /** Toggles the selection state of a single trash item by path. */
  toggleItem(path: string): void {
    if (this.selectedItems.includes(path)) {
      this.selectedItems = this.selectedItems.filter((p) => p !== path);
    } else {
      this.selectedItems = [...this.selectedItems, path];
    }
  }

  /** Selects all items currently in the trash list. */
  selectAll(): void {
    this.selectedItems = [...this.items];
  }

  /** Deselects all items in the trash list. */
  clearSelection(): void {
    this.selectedItems = [];
  }

  /** Restores the given paths (defaults to `selectedItems`) from trash back to their original locations. */
  async restore(relPaths: string[] = this.selectedItems): Promise<void> {
    if (relPaths.length === 0) return;
    this.restoring = true;
    logger.info('Restoring items from trash', { count: relPaths.length });
    try {
      const res = await api.restoreFromTrash(relPaths);
      appState.sessionVersion = Date.now();
      appState.selectedIndices = [];

      const updates: Promise<unknown>[] = [
        appState.loadFolders(),
        appState.loadFilters(),
      ];

      if (appState.filterMode === FILTER_MODES.DUPLICATES) {
        appState.duplicateGroups = [];
      }

      updates.push(appState.updateFilteredIndices());

      if (!res || res.total <= 0) {
        await Promise.all(updates);
        await appState.init();
        return;
      }

      const targetIndex = res.index;
      const isFiltered = (appState.filterMode !== FILTER_MODES.NONE || Object.keys(appState.activeFilters).length > 0);

      updates.push(appState.loadFile(targetIndex));

      await Promise.all(updates);

      if (isFiltered) {
        const list = appState.filteredIndices || [];
        if (list.length === 0) {
          appState.currentFile = null;
        } else if (!list.includes(targetIndex)) {
          await appState.loadFile(list[0]);
        }
      }

      toastService.success(i18n.t('restored_from_trash'));
      this.pickerOpen = false;
    } catch (e: unknown) {
      const rawMessage = e instanceof Error ? e.message : String(e);
      logger.error('Restore from trash failed', { error: rawMessage });
      toastService.error(i18n.t('restore_from_trash_failed'));
    } finally {
      this.restoring = false;
    }
  }

  /** Fetches the full trash list and restores every item. Shows a toast if the trash is already empty. */
  async restoreAll(): Promise<void> {
    const res = await this.list();
    if (!res.items.length) {
      toastService.success(i18n.t('trash_empty'));
      return;
    }
    await this.restore(res.items);
  }
}

export const trashService = new TrashService();
