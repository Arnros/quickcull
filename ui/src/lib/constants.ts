/** Application Identity */
export const APP_NAME = 'quickcull';
export const DEFAULT_MAX_LABEL = 5;

/** Grid Layout Constants — used by Grid.svelte and Virtualization. */
export const GRID_ITEM_SIZE = 160;
export const GRID_GAP = 12;
export const GRID_PADDING = 24;

/**
 * Timing Constants — delays and intervals used for debouncing and polling.
 * Kept here so they are easy to tune without hunting through service files.
 */
/** Milliseconds to debounce position-save calls in navigationService. */
export const SAVE_POSITION_DEBOUNCE_MS = 250;
/** Milliseconds to debounce batched analysis-progress UI updates in appState. */
export const ANALYSIS_FLUSH_DEBOUNCE_MS = 80;
/** Milliseconds of silence from the backend before the progress-poll fallback fires in appState. */
export const POLL_STALENESS_THRESHOLD_MS = 1500;
/** Milliseconds between each progress-poll attempt in appState. */
export const POLL_INTERVAL_MS = 700;
/** Milliseconds to wait for UI state to settle before triggering a hard page reload in appState. */
export const UI_SETTLE_BEFORE_RELOAD_MS = 200;
/** Maximum number of entries kept in the visible action trail in appState. */
export const ACTION_TRAIL_MAX_LENGTH = 8;

/** Wails runtime event names used by both the root component and appState. */
export const EVENTS = {
  PROGRESS: 'progress',
  ANALYSIS_COMPLETE: 'analysis:complete',
  STATE_UPDATE: 'state:update',
  FOLDER_CHANGED: 'folder:changed',
  EXPORT_PROGRESS: 'export:progress',
  EXPORT_COMPLETE: 'export:complete',
  EXPORT_ERROR: 'export:error',
  EXPORT_CANCELLED: 'export:cancelled',
  DUPLICATES_FOUND: 'duplicates:found',
  SEARCH_RESULTS: 'search:results',
  SEARCH_COMPLETE: 'search:complete',
} as const;

/** Filter mode identifiers used across appState, filterState, and UI components. */
export const FILTER_MODES = {
  NONE: 'none',
  STARRED: 'starred',
  LABEL: 'label',
  DUPLICATES: 'duplicates',
} as const;

export type FilterMode = (typeof FILTER_MODES)[keyof typeof FILTER_MODES];
