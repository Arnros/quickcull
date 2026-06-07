import { domain, review } from '../../wailsjs/go/models';

/**
 * Wails bridge pattern:
 *
 * Wails v2 injects the Go backend methods into `window.go.<package>.<struct>`
 * at runtime, after the webview finishes loading. Because the injection is
 * asynchronous, we cannot call Go methods synchronously at module load time.
 *
 * `waitForBindings` polls until `window.go.review.App` is present (up to 5 s),
 * and `callGo` dispatches every outbound RPC through that handle.
 *
 * The `window` global is typed as a known shape below so that internal helpers
 * remain type-safe without leaking `any` into the public API surface.
 */

/** Minimal shape of the Wails-injected Go bindings on `window`. */
interface WailsWindow extends Window {
  go?: {
    review?: {
      App?: Record<string, (...args: unknown[]) => Promise<unknown>>;
    };
  };
  runtime?: {
    EventsOn?: (name: string, callback: (data: unknown) => void) => void;
  };
}

declare const window: WailsWindow;

/** Polls for the Wails Go bindings, retrying every 100 ms for up to 5 seconds. */
async function waitForBindings(): Promise<Record<string, (...args: unknown[]) => Promise<unknown>>> {
  let retries = 50;
  while (retries > 0) {
    if (window.go && window.go.review && window.go.review.App) {
      return window.go.review.App;
    }
    await new Promise(r => setTimeout(r, 100));
    retries--;
  }
  throw new Error('Wails bindings failed to load after 5 seconds');
}

let goBindingsPromise: Promise<Record<string, (...args: unknown[]) => Promise<unknown>>> | null = null;

/** Returns a singleton promise that resolves to the injected Go method map. */
async function getBindings(): Promise<Record<string, (...args: unknown[]) => Promise<unknown>>> {
  if (!goBindingsPromise) {
    goBindingsPromise = waitForBindings();
  }
  return goBindingsPromise;
}

/** Calls a Go backend method by name and casts the result to `T`. */
async function callGo<T>(method: string, ...args: unknown[]): Promise<T> {
  const go = await getBindings();
  return go[method](...args) as Promise<T>;
}

export const api = {
  async getConfig(): Promise<domain.Config> {
    return callGo<domain.Config>('GetConfig');
  },

  async getAnalysisProgress(): Promise<review.AnalysisProgressResponse> {
    return callGo<review.AnalysisProgressResponse>('GetAnalysisProgress');
  },

  async updateConfig(cfg: domain.Config): Promise<{ ok: true }> {
    await callGo<void>('UpdateConfig', cfg);
    return { ok: true };
  },

  async getFile(index: number): Promise<review.FileResponse> {
    return callGo<review.FileResponse>('GetFile', index, false);
  },

  async getStats(): Promise<review.AppStats> {
    return callGo<review.AppStats>('GetStats');
  },

  async getAppState(): Promise<review.AppState> {
    return callGo<review.AppState>('GetAppState');
  },

  async getFolders(): Promise<{ folders: review.FolderInfo[] }> {
    const folders = await callGo<review.FolderInfo[]>('GetFolders');
    return { folders: folders || [] };
  },

  async trash(index?: number, path?: string, paths?: string[]): Promise<review.ActionResponse> {
    return callGo<review.ActionResponse>('Trash', index || 0, path || '', paths || []);
  },

  async toggleStar(index?: number, path?: string, paths?: string[], starred?: boolean): Promise<review.ActionResponse> {
    return callGo<review.ActionResponse>('ToggleStar', index || 0, path || '', paths || [], !!starred);
  },

  async setLabel(index?: number, path?: string, paths?: string[], label?: number): Promise<review.ActionResponse> {
    return callGo<review.ActionResponse>('SetLabel', index || 0, path || '', paths || [], label || 0);
  },

  async resetStars(): Promise<{ ok: true }> {
    await callGo<void>('ResetStars');
    return { ok: true };
  },

  async resetLabels(): Promise<{ ok: true }> {
    await callGo<void>('ResetLabels');
    return { ok: true };
  },

  async resetAppCache(): Promise<{ ok: true }> {
    await callGo<void>('ResetAppCache');
    return { ok: true };
  },

  async undo(): Promise<review.UndoResponse> {
    return callGo<review.UndoResponse>('Undo');
  },

  async rotate(index: number, path: string, direction: 'right' | 'left'): Promise<review.ActionResponse> {
    return callGo<review.ActionResponse>('Rotate', index, path, direction);
  },

  async rotateReset(index: number, path: string): Promise<review.ActionResponse> {
    return callGo<review.ActionResponse>('RotateReset', index, path);
  },

  async applyRotation(index: number, path: string): Promise<{ success: true }> {
    await callGo<void>('ApplyRotation', index, path);
    return { success: true };
  },

  async reanalyzeMetadata(index: number, path: string): Promise<review.FileResponse> {
    return callGo<review.FileResponse>('ReanalyzeMetadata', index, path);
  },

  async refresh(index: number): Promise<review.ActionResponse> {
    return callGo<review.ActionResponse>('Refresh', index);
  },

  async getStarredIndices(): Promise<review.FilteredIndicesResponse> {
    return callGo<review.FilteredIndicesResponse>('GetStarredIndices');
  },

  async getLabelIndices(label: number): Promise<review.FilteredIndicesResponse> {
    return callGo<review.FilteredIndicesResponse>('GetLabelIndices', label);
  },

  async getFilteredIndices(filters: { camera?: string, iso?: string, dateFrom?: string, dateTo?: string, sizeMin?: string, sizeMax?: string }): Promise<review.FilteredIndicesResponse> {
    const f: Record<string, string> = {};
    if (filters.camera) f.camera = filters.camera;
    if (filters.iso) f.iso = filters.iso;
    if (filters.dateFrom) f.dateFrom = filters.dateFrom;
    if (filters.dateTo) f.dateTo = filters.dateTo;
    if (filters.sizeMin) f.sizeMin = filters.sizeMin;
    if (filters.sizeMax) f.sizeMax = filters.sizeMax;
    return callGo<review.FilteredIndicesResponse>('GetFilteredIndices', f);
  },

  async getFilters(): Promise<review.FilterValuesResponse> {
    return callGo<review.FilterValuesResponse>('GetFilters');
  },

  async revealInExplorer(index: number): Promise<void> {
    return callGo<void>('RevealInExplorer', index);
  },

  async browse(path: string): Promise<review.BrowseResponse> {
    return callGo<review.BrowseResponse>('Browse', path);
  },

  async getBookmarks(): Promise<review.BookmarkResponse> {
    return callGo<review.BookmarkResponse>('GetBookmarks');
  },

  async getRecent(): Promise<{ recent: string[] }> {
    const recent = await callGo<string[]>('GetHistory');
    return { recent: recent || [] };
  },

  async removeHistory(path: string): Promise<void> {
    await callGo<void>('RemoveHistory', path);
  },

  async getDuplicates(threshold: number = 90.0): Promise<{ groups: number[][] }> {
    const groups = await callGo<number[][]>('GetDuplicates', threshold);
    return { groups: groups || [] };
  },

  async listTrash(): Promise<review.TrashListResponse> {
    return callGo<review.TrashListResponse>('ListTrash');
  },

  async restoreFromTrash(paths: string[]): Promise<review.RestoreResponse> {
    return callGo<review.RestoreResponse>('RestoreFromTrash', paths || []);
  },

  async savePosition(index: number): Promise<void> {
    await callGo<void>('SavePosition', index);
  },

  async browseDialog(): Promise<review.PathResponse> {
    return callGo<review.PathResponse>('BrowseDialog');
  },

  async exiftoolDialog(): Promise<review.PathResponse> {
    return callGo<review.PathResponse>('ExiftoolDialog');
  },

  async openFolder(path: string): Promise<{ ok: true }> {
    await callGo<void>('OpenFolder', path);
    return { ok: true };
  },

  async sysCheck(): Promise<review.SysCheckResponse> {
    return callGo<review.SysCheckResponse>('SysCheck');
  },

  async openConfigFolder(): Promise<void> {
    return callGo<void>('OpenConfigFolder');
  },

  async exportLogs(): Promise<void> {
    return callGo<void>('ExportLogs');
  },

  async exportFiles(paths: string[], destDir: string, move: boolean): Promise<void> {
    return callGo<void>('ExportFiles', paths, destDir, move);
  },

  async setSortOrder(order: string): Promise<void> {
    return callGo<void>('SetSortOrder', order);
  },

  async exportSelection(criteria: string, label: number, destDir: string, move: boolean): Promise<void> {
    return callGo<void>('ExportSelection', criteria, label, destDir, move);
  },

  async cancelExport(): Promise<void> {
    return callGo<void>('CancelExport');
  },

  async searchStream(query: string): Promise<void> {
    return callGo<void>('SearchStream', query);
  },

  async prioritizeIndices(indices: number[]): Promise<void> {
    return callGo<void>('PrioritizeIndices', indices);
  },

  async quit(): Promise<void> {
    await callGo<void>('Quit');
  },

  /**
   * Subscribes to a Wails runtime event. The `callback` receives the raw
   * payload emitted by the Go backend; callers are responsible for narrowing
   * the `unknown` value to their expected shape.
   */
  onEvent<T = unknown>(name: string, callback: (data: T) => void) {
    if (window.runtime && window.runtime.EventsOn) {
      window.runtime.EventsOn(name, callback as (data: unknown) => void);
    }
  }
};
