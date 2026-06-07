import { describe, expect, it } from 'vitest';
import { resolveContextShortcutHints, resolveShortcutContext } from './shortcutContext';

describe('resolveShortcutContext', () => {
  it('returns picker context in picker view', () => {
    const ctx = resolveShortcutContext({
      view: 'picker',
      gridOpen: false,
      filterBarOpen: false,
      comparisonMode: false,
      filterMode: 'none',
      helpOpen: false,
      settingsOpen: false,
    });
    expect(ctx.screen).toBe('picker');
    expect(ctx.showBottomBar).toBe(false);
  });

  it('returns duplicate comparison context with up/down navigation and target selectors', () => {
    const ctx = resolveShortcutContext({
      view: 'review',
      gridOpen: false,
      filterBarOpen: false,
      comparisonMode: true,
      filterMode: 'duplicates',
      helpOpen: false,
      settingsOpen: false,
    });
    expect(ctx.screen).toBe('review_duplicate_comparison');
    expect(ctx.navPrevKey).toBe('ArrowUp');
    expect(ctx.navNextKey).toBe('ArrowDown');
    expect(ctx.showTargetSelectors).toBe(true);
  });

  it('returns modal context when settings/help is open', () => {
    const ctx = resolveShortcutContext({
      view: 'review',
      gridOpen: false,
      filterBarOpen: false,
      comparisonMode: true,
      filterMode: 'duplicates',
      helpOpen: true,
      settingsOpen: false,
    });
    expect(ctx.screen).toBe('modal');
    expect(ctx.showBottomBar).toBe(false);
  });
});

describe('resolveContextShortcutHints', () => {
  it('returns review actions when filter bar is closed', () => {
    const hints = resolveContextShortcutHints({
      view: 'review',
      filterBarOpen: false,
    });
    expect(hints.starHintKey).toBe('shortcut_hint_s_review');
    expect(hints.labelHintKey).toBe('shortcut_hint_label_review');
  });

  it('returns filter actions when filter bar is open', () => {
    const hints = resolveContextShortcutHints({
      view: 'review',
      filterBarOpen: true,
    });
    expect(hints.starHintKey).toBe('shortcut_hint_s_filters');
    expect(hints.labelHintKey).toBe('shortcut_hint_label_filters');
  });
});
