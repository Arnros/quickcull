type FilterMode = 'none' | 'starred' | 'label' | 'duplicates';

type ShortcutScreen =
  | 'picker'
  | 'review'
  | 'review_grid'
  | 'review_filters'
  | 'review_comparison'
  | 'review_duplicate_comparison'
  | 'modal';

type ShortcutContext = {
  screen: ShortcutScreen;
  duplicateComparison: boolean;
  showBottomBar: boolean;
  navPrevKey: 'ArrowLeft' | 'ArrowUp';
  navNextKey: 'ArrowRight' | 'ArrowDown';
  showTargetSelectors: boolean;
};

type ContextShortcutHints = {
  starHintKey: 'shortcut_hint_s_review' | 'shortcut_hint_s_filters';
  labelHintKey: 'shortcut_hint_label_review' | 'shortcut_hint_label_filters';
};

export function resolveShortcutContext(state: {
  view: 'picker' | 'review';
  gridOpen: boolean;
  filterBarOpen: boolean;
  comparisonMode: boolean;
  filterMode: FilterMode;
  helpOpen: boolean;
  settingsOpen: boolean;
}): ShortcutContext {
  const modal = state.helpOpen || state.settingsOpen;
  const duplicateComparison = state.comparisonMode && state.filterMode === 'duplicates' && !state.gridOpen;

  if (modal) {
    return {
      screen: 'modal',
      duplicateComparison: false,
      showBottomBar: false,
      navPrevKey: 'ArrowLeft',
      navNextKey: 'ArrowRight',
      showTargetSelectors: false,
    };
  }

  if (state.view === 'picker') {
    return {
      screen: 'picker',
      duplicateComparison: false,
      showBottomBar: false,
      navPrevKey: 'ArrowLeft',
      navNextKey: 'ArrowRight',
      showTargetSelectors: false,
    };
  }

  if (state.gridOpen) {
    return {
      screen: 'review_grid',
      duplicateComparison: false,
      showBottomBar: false,
      navPrevKey: 'ArrowUp',
      navNextKey: 'ArrowDown',
      showTargetSelectors: false,
    };
  }

  if (duplicateComparison) {
    return {
      screen: 'review_duplicate_comparison',
      duplicateComparison: true,
      showBottomBar: true,
      navPrevKey: 'ArrowUp',
      navNextKey: 'ArrowDown',
      showTargetSelectors: true,
    };
  }

  if (state.filterBarOpen) {
    return {
      screen: 'review_filters',
      duplicateComparison: false,
      showBottomBar: true,
      navPrevKey: 'ArrowLeft',
      navNextKey: 'ArrowRight',
      showTargetSelectors: false,
    };
  }

  if (state.comparisonMode) {
    return {
      screen: 'review_comparison',
      duplicateComparison: false,
      showBottomBar: true,
      navPrevKey: 'ArrowLeft',
      navNextKey: 'ArrowRight',
      showTargetSelectors: false,
    };
  }

  return {
    screen: 'review',
    duplicateComparison: false,
    showBottomBar: true,
    navPrevKey: 'ArrowLeft',
    navNextKey: 'ArrowRight',
    showTargetSelectors: false,
  };
}

export function resolveContextShortcutHints(state: {
  view: 'picker' | 'review';
  filterBarOpen: boolean;
}): ContextShortcutHints {
  const inFilterContext = state.view === 'review' && state.filterBarOpen;
  return {
    starHintKey: inFilterContext ? 'shortcut_hint_s_filters' : 'shortcut_hint_s_review',
    labelHintKey: inFilterContext ? 'shortcut_hint_label_filters' : 'shortcut_hint_label_review',
  };
}
