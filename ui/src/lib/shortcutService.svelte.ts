import { appState } from './appState.svelte';
import { filterState } from './filterState.svelte';
import { viewState } from './viewState.svelte';
import { resolveShortcutContext } from './shortcutContext';
import { logger } from './logger';

export type ShortcutActionId =
  | 'NAV_PREV' | 'NAV_NEXT' | 'NAV_UP' | 'NAV_DOWN' | 'NAV_PAGE_UP' | 'NAV_PAGE_DOWN' | 'NAV_FIRST' | 'NAV_LAST' | 'NAV_SIDEBAR' | 'NAV_FILMSTRIP'
  | 'VIEW_GRID' | 'VIEW_INFO' | 'VIEW_FILTERS' | 'VIEW_ZOOM'
  | 'VIEW_ZEN' | 'VIEW_COMPARISON' | 'VIEW_TOGGLE_GRID'
  | 'ACTION_STAR' | 'ACTION_TRASH' | 'ACTION_UNDO'
  | 'ACTION_ROTATE_RIGHT' | 'ACTION_ROTATE_LEFT' | 'ACTION_ROTATE_RESET'
  | 'ACTION_APPLY_ROTATION' | 'ACTION_REFRESH' | 'ACTION_HELP'
  | 'ACTION_LABEL_0' | 'ACTION_LABEL_1' | 'ACTION_LABEL_2'
  | 'ACTION_LABEL_3' | 'ACTION_LABEL_4' | 'ACTION_LABEL_5'
  | 'ACTION_CLOSE';

interface ShortcutDefinition {
  id: ShortcutActionId;
  descriptionKey: string;
  category: 'navigation' | 'view' | 'action' | 'system';
  action: (e: KeyboardEvent) => void;
  defaultQwerty: string[];
  defaultAzerty: string[];
}

class ShortcutService {
  private definitions: ShortcutDefinition[] = [];
  private defMap: Map<ShortcutActionId, ShortcutDefinition> = new Map();
  isRecording = $state(false);
  lastTriggeredAction = $state<ShortcutActionId | null>(null);
  lastTriggeredAt = $state(0);

  // FSM State: Reactive context resolution
  currentContext = $derived(resolveShortcutContext({
    view: viewState.current,
    gridOpen: viewState.gridOpen,
    filterBarOpen: filterState.filterBarOpen,
    comparisonMode: viewState.comparisonMode,
    filterMode: filterState.filterMode as any,
    helpOpen: viewState.helpOpen,
    settingsOpen: viewState.settingsOpen,
  }));

  // Reactive mapping of Key -> ActionId
  private keyMap = $state<Record<string, ShortcutActionId>>({});

  // Mapping of ActionId -> Keys (for display/Help)
  activeMappings = $derived.by(() => {
    const map: Record<ShortcutActionId, string[]> = {} as any;
    const custom = viewState.config?.shortcuts || {};

    const layout = this.detectLayout();

    for (const def of this.definitions) {
      if (custom[def.id]) {
        map[def.id] = [custom[def.id]];
      } else {
        map[def.id] = layout === 'azerty' ? def.defaultAzerty : def.defaultQwerty;
      }
    }
    return map;
  });

  constructor() {
    this.initDefinitions();
    this.defMap = new Map(this.definitions.map(d => [d.id, d]));

    // Update the reverse keyMap whenever activeMappings changes
    $effect.root(() => {
      $effect(() => {
        const newKeyMap: Record<string, ShortcutActionId> = {};
        const mappings = this.activeMappings;
        for (const [id, keys] of Object.entries(mappings)) {
          for (const key of keys) {
            newKeyMap[key.toLowerCase()] = id as ShortcutActionId;
          }
        }
        this.keyMap = newKeyMap;
      });
    });
  }

  private detectLayout(): 'azerty' | 'qwerty' {
    const lang = navigator.language.toLowerCase();
    if (lang.startsWith('fr')) return 'azerty';
    return 'qwerty';
  }

  private initDefinitions() {
    this.definitions = [
      {
        id: 'NAV_PREV', descriptionKey: 'help_nav_prev', category: 'navigation', action: (e) => {
          if (this.currentContext.screen === 'review_duplicate_comparison') {
            appState.select(appState.comparisonIndex);
            return;
          }
          appState.prev(e.shiftKey, e.shiftKey && (e.ctrlKey || e.metaKey));
        }, defaultQwerty: ['arrowleft', 'q', 'backspace'], defaultAzerty: ['arrowleft', 'a', 'backspace']
      },
      {
        id: 'NAV_NEXT', descriptionKey: 'help_nav_next', category: 'navigation', action: (e) => {
          if (this.currentContext.screen === 'review_duplicate_comparison') {
            appState.select(appState.currentIndex);
            return;
          }
          appState.next(e.shiftKey, e.shiftKey && (e.ctrlKey || e.metaKey));
        }, defaultQwerty: ['arrowright', 'd', ' '], defaultAzerty: ['arrowright', 'd', ' ']
      },
      {
        id: 'NAV_UP', descriptionKey: 'help_nav_up', category: 'navigation', action: (e) => {
          if (this.currentContext.screen === 'review_grid') appState.gridPrevRow(e.shiftKey, e.shiftKey && (e.ctrlKey || e.metaKey));
          else appState.prev(e.shiftKey, e.shiftKey && (e.ctrlKey || e.metaKey));
        }, defaultQwerty: ['arrowup'], defaultAzerty: ['arrowup']
      },
      {
        id: 'NAV_DOWN', descriptionKey: 'help_nav_down', category: 'navigation', action: (e) => {
          if (this.currentContext.screen === 'review_grid') appState.gridNextRow(e.shiftKey, e.shiftKey && (e.ctrlKey || e.metaKey));
          else appState.next(e.shiftKey, e.shiftKey && (e.ctrlKey || e.metaKey));
        }, defaultQwerty: ['arrowdown'], defaultAzerty: ['arrowdown']
      },
      {
        id: 'NAV_PAGE_UP', descriptionKey: 'help_nav_page_up', category: 'navigation', action: (e) => {
          if (this.currentContext.screen === 'review_grid') appState.gridPageUp(e.shiftKey, e.shiftKey && (e.ctrlKey || e.metaKey));
          else appState.prev(e.shiftKey, e.shiftKey && (e.ctrlKey || e.metaKey));
        }, defaultQwerty: ['pageup'], defaultAzerty: ['pageup']
      },
      {
        id: 'NAV_PAGE_DOWN', descriptionKey: 'help_nav_page_down', category: 'navigation', action: (e) => {
          if (this.currentContext.screen === 'review_grid') appState.gridPageDown(e.shiftKey, e.shiftKey && (e.ctrlKey || e.metaKey));
          else appState.next(e.shiftKey, e.shiftKey && (e.ctrlKey || e.metaKey));
        }, defaultQwerty: ['pagedown'], defaultAzerty: ['pagedown']
      },
      { id: 'NAV_FIRST', descriptionKey: 'help_nav_first', category: 'navigation', action: (e) => appState.first(e.shiftKey, e.shiftKey && (e.ctrlKey || e.metaKey)), defaultQwerty: ['home'], defaultAzerty: ['home'] },
      { id: 'NAV_LAST', descriptionKey: 'help_nav_last', category: 'navigation', action: (e) => appState.last(e.shiftKey, e.shiftKey && (e.ctrlKey || e.metaKey)), defaultQwerty: ['end'], defaultAzerty: ['end'] },
      { id: 'NAV_SIDEBAR', descriptionKey: 'help_nav_sidebar', category: 'navigation', action: (e) => { e.preventDefault(); viewState.sidebarOpen = !viewState.sidebarOpen; }, defaultQwerty: ['tab'], defaultAzerty: ['tab'] },
      { id: 'NAV_FILMSTRIP', descriptionKey: 'help_nav_filmstrip', category: 'navigation', action: () => viewState.filmstripOpen = !viewState.filmstripOpen, defaultQwerty: ['g'], defaultAzerty: ['g'] },

      {
        id: 'VIEW_GRID', descriptionKey: 'help_view_grid', category: 'view', action: () => {
          if (this.currentContext.screen === 'review_grid' && filterState.filterMode === 'duplicates') {
            appState.exitDuplicatesMode();
            return;
          }
          viewState.gridOpen = !viewState.gridOpen;
        }, defaultQwerty: ['v'], defaultAzerty: ['v']
      },
      {
        id: 'VIEW_TOGGLE_GRID', descriptionKey: 'help_view_grid', category: 'view', action: () => {
          if (this.currentContext.screen === 'review_grid' && filterState.filterMode === 'duplicates') {
            appState.exitDuplicatesMode();
            return;
          }
          viewState.gridOpen = !viewState.gridOpen;
        }, defaultQwerty: ['enter'], defaultAzerty: ['enter']
      },
      { id: 'VIEW_INFO', descriptionKey: 'help_view_info', category: 'view', action: () => viewState.infoOpen = !viewState.infoOpen, defaultQwerty: ['i'], defaultAzerty: ['i'] },
      {
        id: 'VIEW_FILTERS', descriptionKey: 'help_view_filters', category: 'view', action: () => {
          filterState.filterBarOpen = !filterState.filterBarOpen;
          if (filterState.filterBarOpen) void appState.loadFilters();
        }, defaultQwerty: ['f'], defaultAzerty: ['f']
      },
      { id: 'VIEW_ZOOM', descriptionKey: 'help_view_zoom', category: 'view', action: () => viewState.zoomed = !viewState.zoomed, defaultQwerty: ['z'], defaultAzerty: ['z'] },
      { id: 'VIEW_ZEN', descriptionKey: 'help_view_zen', category: 'view', action: () => viewState.zenMode = !viewState.zenMode, defaultQwerty: ['h'], defaultAzerty: ['h'] },
      { id: 'VIEW_COMPARISON', descriptionKey: 'help_view_comparison', category: 'view', action: () => appState.toggleComparisonMode(), defaultQwerty: ['c'], defaultAzerty: ['c'] },

      {
        id: 'ACTION_STAR', descriptionKey: 'help_action_star', category: 'action', action: () => {
          if (this.currentContext.screen === 'review_filters') appState.toggleStarFilter();
          else appState.toggleStar();
        }, defaultQwerty: ['s'], defaultAzerty: ['s']
      },
      { id: 'ACTION_TRASH', descriptionKey: 'help_action_trash', category: 'action', action: () => appState.trash(), defaultQwerty: ['x', 'delete'], defaultAzerty: ['x', 'delete'] },
      { id: 'ACTION_UNDO', descriptionKey: 'help_action_undo', category: 'action', action: () => appState.undo(), defaultQwerty: ['u'], defaultAzerty: ['u'] },
      { id: 'ACTION_ROTATE_RIGHT', descriptionKey: 'help_action_rotate_right', category: 'action', action: () => appState.rotate('right'), defaultQwerty: ['r'], defaultAzerty: ['r'] },
      { id: 'ACTION_ROTATE_LEFT', descriptionKey: 'help_action_rotate_left', category: 'action', action: () => appState.rotate('left'), defaultQwerty: ['l'], defaultAzerty: ['l'] },
      { id: 'ACTION_ROTATE_RESET', descriptionKey: 'help_action_rotate_reset', category: 'action', action: () => appState.rotateReset(), defaultQwerty: ['o'], defaultAzerty: ['o'] },
      { id: 'ACTION_APPLY_ROTATION', descriptionKey: 'help_action_apply_rotation', category: 'action', action: () => { if (appState.canApplyRotation()) appState.applyRotation(); }, defaultQwerty: ['w'], defaultAzerty: ['w'] },
      { id: 'ACTION_REFRESH', descriptionKey: 'help_sys_refresh', category: 'system', action: (e) => { e.preventDefault(); appState.refresh(); }, defaultQwerty: ['f5'], defaultAzerty: ['f5'] },
      { id: 'ACTION_HELP', descriptionKey: 'help', category: 'system', action: () => viewState.helpOpen = !viewState.helpOpen, defaultQwerty: ['?'], defaultAzerty: ['?'] },
      { id: 'ACTION_CLOSE', descriptionKey: 'help_sys_close', category: 'system', action: () => this.handleEscape(), defaultQwerty: ['escape'], defaultAzerty: ['escape'] },
    ];

    // Labels 0 - maxLabel
    const azertyLabels: Record<number, string> = {
      0: 'à',
      1: '&',
      2: 'é',
      3: '"',
      4: "'",
      5: '(',
    };

    for (let i = 0; i <= appState.maxLabel; i++) {
      const id = `ACTION_LABEL_${i}` as ShortcutActionId;
      const azertyKeys = [i.toString()];
      if (azertyLabels[i]) azertyKeys.push(azertyLabels[i]);

      this.definitions.push({
        id,
        descriptionKey: 'help_action_label',
        category: 'action',
        action: () => {
          if (this.currentContext.screen === 'review_filters') appState.setLabelFilter(i);
          else appState.setLabel(i);
        },
        defaultQwerty: [i.toString()],
        defaultAzerty: azertyKeys
      });
    }
  }

  private handleEscape() {
    const screen = this.currentContext.screen;

    if (screen === 'modal') {
      viewState.helpOpen = false;
      viewState.settingsOpen = false;
      return;
    }

    // Contextual hierarchy
    if (appState.selectedIndices.length > 1) {
      appState.selectedIndices = [appState.currentIndex];
      return;
    }

    if (screen === 'review_grid') {
      if (filterState.filterMode === 'duplicates') {
        appState.exitDuplicatesMode();
        return;
      }
      viewState.gridOpen = false;
      filterState.filterBarOpen = false; // Close bar too if open
      return;
    }

    if (screen === 'review_filters') {
      filterState.filterBarOpen = false;
      return;
    }

    if (viewState.zoomed) {
      viewState.zoomed = false;
      return;
    }

    if (screen === 'review_duplicate_comparison') {
      appState.exitDuplicatesMode();
      return;
    }

    if (screen === 'review_comparison') {
      void appState.toggleComparisonMode();
      return;
    }

    if (filterState.filterMode !== 'none') {
      filterState.filterMode = 'none';
      filterState.filteredIndices = [];
      filterState.filterBarOpen = false;
      return;
    }

    if (viewState.zenMode) {
      viewState.zenMode = false;
      return;
    }
  }

  handleKeydown(e: KeyboardEvent) {
    if (this.isRecording) return;
    if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement || e.target instanceof HTMLSelectElement) {
      return;
    }
    const rawKey = e.key.toLowerCase();
    const isArrowNavKey = rawKey === 'arrowleft' || rawKey === 'arrowright' || rawKey === 'arrowup' || rawKey === 'arrowdown';
    const isBoundaryNavKey = rawKey === 'home' || rawKey === 'end';
    const allowMetaOnNavigation = e.metaKey && e.shiftKey && (isArrowNavKey || isBoundaryNavKey);
    if ((e.metaKey && !allowMetaOnNavigation) || e.altKey) return;

    let key = rawKey;
    if (e.ctrlKey) key = `ctrl+${key}`;

    let actionId = this.keyMap[key];
    if (!actionId && (isArrowNavKey || isBoundaryNavKey) && (e.ctrlKey || allowMetaOnNavigation)) {
      actionId = this.keyMap[rawKey];
    }
    const screen = this.currentContext.screen;

    if (actionId) {
      const def = this.defMap.get(actionId);
      if (def) {
        logger.debug('Shortcut triggered', { actionId, screen, key });
        def.action(e);
        this.lastTriggeredAction = actionId;
        this.lastTriggeredAt = Date.now();
      }
    }
  }

  getDefinitions() {
    return this.definitions;
  }
}

export const shortcutService = new ShortcutService();
