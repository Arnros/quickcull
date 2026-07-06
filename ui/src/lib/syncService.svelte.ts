import { api } from './api';
import { filterState } from './filterState.svelte';
import { logger } from './logger';
import { review } from '../../wailsjs/go/models';
import type { AppStateV2, StateDelta } from './types';

/** Wails event names for v2 state synchronisation. */
const SYNC_EVENTS = {
  STATE: 'SyncState',
  STATE_BASE: 'sync:state:base',
  STATE_PHOTOS: 'sync:state:photos',
  DELTA: 'SyncDelta',
} as const;

// We import the type only to avoid circular dependency if we needed the instance,
// but we'll pass the appState instance to the init method.
interface IAppState {
  v2: AppStateV2 | null;
  stats: review.AppStats;
  sessionVersion: number;
  currentFile: review.FileResponse | null;
  currentIndex: number;
  selectionPivot: number;
  lastNonUndoableAction: string;
  selectedIndices: number[];
  validateSelection: () => void;
  starredIndices: number[];
  updateStarredIndices: () => void;
}

function remapIndexByPath(
  oldVisibleOrder: string[] | undefined,
  newVisibleOrder: string[] | undefined,
  index: number,
  fallback: number
): number {
  if (!oldVisibleOrder || !newVisibleOrder) return fallback;
  const path = oldVisibleOrder[index];
  if (!path) return fallback;
  const remapped = newVisibleOrder.indexOf(path);
  return remapped !== -1 ? remapped : fallback;
}

/** Shape of the inline stats sub-object that may be embedded in a SyncDelta changes map. */
interface DeltaStats {
  starred: number;
  labeled: number;
  trashed: number;
  undoLen: number;
}

function remapSelectedIndices(
  oldVisibleOrder: string[] | undefined,
  newVisibleOrder: string[] | undefined,
  selectedIndices: number[]
): number[] {
  if (!oldVisibleOrder || !newVisibleOrder || selectedIndices.length === 0) {
    return selectedIndices;
  }

  const selectedPaths = selectedIndices
    .map((idx) => oldVisibleOrder[idx])
    .filter((p): p is string => !!p);
  if (selectedPaths.length === 0) {
    return [];
  }

  const pos = new Map<string, number>();
  for (let i = 0; i < newVisibleOrder.length; i++) {
    pos.set(newVisibleOrder[i], i);
  }

  const remapped: number[] = [];
  for (const p of selectedPaths) {
    const idx = pos.get(p);
    if (idx !== undefined) remapped.push(idx);
  }
  return remapped;
}

function hasActiveFiltering(): boolean {
  return filterState.filterMode !== 'none' || Object.keys(filterState.activeFilters).length > 0;
}

async function refreshFilteredIndicesIfNeeded(): Promise<void> {
  if (!hasActiveFiltering()) return;
  await filterState.updateFilteredIndices();
}

/**
 * Updates the legacy stats fields on appState from any payload that carries
 * the standard count fields (VisibleOrder, TrashedCount, StarredCount,
 * LabeledCount, History).  Called from SyncState handlers.
 */
function updateStatsFromPayload(
  data: AppStateV2,
  appState: IAppState
): void {
  appState.stats.total = data.VisibleOrder?.length || 0;
  appState.stats.trashedCount = data.TrashedCount || 0;
  appState.stats.starredCount = data.StarredCount || 0;
  appState.stats.labeledCount = data.LabeledCount || 0;
  appState.stats.undoLen = data.UndoLen || 0;
  appState.sessionVersion = Date.now();
}

class SyncService {
  init(appState: IAppState): void {
    // v2: Chunked Delivery - Base State
    api.onEvent(SYNC_EVENTS.STATE_BASE, async (data: AppStateV2) => {
      logger.info('v2 SyncState: Base received', { orderLen: data.VisibleOrder?.length });
      const oldVisibleOrder = appState.v2?.VisibleOrder;
      const oldSelectedIndices = [...appState.selectedIndices];

      appState.v2 = {
        ...data,
        Photos: appState.v2?.Photos || {} // Keep existing photos if any, or start empty
      };

      appState.selectedIndices = remapSelectedIndices(oldVisibleOrder, data.VisibleOrder, oldSelectedIndices);
      updateStatsFromPayload(data, appState);
      appState.validateSelection();
      await refreshFilteredIndicesIfNeeded();
    });

    // v2: Chunked Delivery - Photos Chunks
    api.onEvent(SYNC_EVENTS.STATE_PHOTOS, (data: { photos: Record<string, any>; index: number; total: number; isLast?: boolean }) => {
      if (!appState.v2) return;
      
      // Merge chunk into state
      Object.assign(appState.v2.Photos, data.photos);
      
      const count = Object.keys(data.photos).length;
      if (data.isLast || data.index + count >= data.total) {
        logger.info('v2 SyncState: All photos received', { total: data.total });
        appState.updateStarredIndices();
        
        // Final metadata sync for current file
        if (appState.currentFile && appState.v2.Photos[appState.currentFile.filename]) {
          const p = appState.v2.Photos[appState.currentFile.filename];
          appState.currentFile = {
            ...appState.currentFile,
            starred: p.IsStarred,
            label: p.Label,
            rotation: p.Rotation,
          } as review.FileResponse;
        }
      }
    });

    // v2: Listen for SyncState events (Full - Fallback/Legacy)
    api.onEvent(SYNC_EVENTS.STATE, async (data: AppStateV2) => {
      const incomingPhotos = data.Photos || {};
      logger.debug('v2 SyncState received', { 
        photoCount: Object.keys(incomingPhotos).length, 
        isPartial: data.IsPartial 
      });

      const oldFilename = appState.currentFile?.filename;
      const oldVisibleOrder = appState.v2?.VisibleOrder;
      const oldPhotos = appState.v2?.Photos;
      const oldRoot = appState.v2?.Root;
      const oldSelectedIndices = [...appState.selectedIndices];
      const oldSelectionPivot = appState.selectionPivot;
      const isUndoOperation = appState.lastNonUndoableAction === 'undo';

      const isSameFolder = oldRoot === data.Root;
      let preservedPhotos: Record<string, any>;
      if (data.IsPartial && isSameFolder && oldPhotos && Object.keys(oldPhotos).length > 0) {
        preservedPhotos = { ...oldPhotos };
        if (incomingPhotos) {
          Object.assign(preservedPhotos, incomingPhotos);
        }
      } else {
        preservedPhotos = incomingPhotos;
      }

      appState.v2 = {
        ...data,
        Photos: preservedPhotos
      };
      
      appState.selectedIndices = remapSelectedIndices(oldVisibleOrder, data.VisibleOrder, oldSelectedIndices);
      appState.updateStarredIndices();
      // Sync legacy stats for backward compatibility with UI components
      updateStatsFromPayload(data, appState);

      appState.validateSelection();

      // Re-sync metadata for current file if it was changed and photos are present
      const photos = appState.v2.Photos;
      if (appState.currentFile && photos[appState.currentFile.filename]) {
        const p = photos[appState.currentFile.filename];
        appState.currentFile = {
          ...appState.currentFile,
          starred: p.IsStarred,
          label: p.Label,
          rotation: p.Rotation,
        } as review.FileResponse;
      }

      // If the order changed (e.g. Sort), try to maintain current photo selection
      // But SKIP this during Undo to let the Undo logic handle the focus on the restored item
      if (!isUndoOperation && oldFilename && data.VisibleOrder) {
        const newIndex = data.VisibleOrder.indexOf(oldFilename);
        if (newIndex !== -1 && newIndex !== appState.currentIndex) {
          logger.debug('Maintaining current photo at new index', { oldIndex: appState.currentIndex, newIndex });
          appState.currentIndex = newIndex;
          if (appState.currentFile) {
            appState.currentFile = { ...appState.currentFile, index: newIndex } as review.FileResponse;
          }
        }
      }
      appState.selectionPivot = remapIndexByPath(
        oldVisibleOrder,
        data.VisibleOrder,
        oldSelectionPivot,
        appState.currentIndex
      );

      await refreshFilteredIndicesIfNeeded();
    });

    // v2: Listen for SyncDelta events (Incremental)
    api.onEvent(SYNC_EVENTS.DELTA, (delta: StateDelta) => {
      if (!appState.v2) return;
      const photoID = delta.PhotoID;
      const changes = delta.Changes;

      if (appState.v2.Photos[photoID]) {
        const p = appState.v2.Photos[photoID];
        const starredChanged = changes.IsStarred !== undefined && changes.IsStarred !== p.IsStarred;

        // Targeted mutation: only this object changes
        Object.assign(p, changes);

        // Sync stats if present in delta
        if (changes._stats) {
          const s = changes._stats as DeltaStats;
          appState.stats.starredCount = s.starred;
          appState.stats.labeledCount = s.labeled;
          appState.stats.trashedCount = s.trashed;
          appState.stats.undoLen = s.undoLen;
        }

        if (starredChanged) {
          appState.updateStarredIndices();
        }

        // Re-sync current file if it's the one that changed.
        // Create a new object to reliably trigger $state reactivity through
        // the getter chain (AppState.currentFile → NavigationService.currentFile).
        if (appState.currentFile && appState.currentFile.filename === photoID) {
          let changed = false;
          const updated = { ...appState.currentFile };
          if (changes.IsStarred !== undefined) { updated.starred = changes.IsStarred; changed = true; }
          if (changes.Label !== undefined) { updated.label = changes.Label; changed = true; }
          if (changes.Rotation !== undefined) { updated.rotation = changes.Rotation; changed = true; }
          if (changed) appState.currentFile = updated as review.FileResponse;
        }
      }
    });
  }
}

export const syncService = new SyncService();
