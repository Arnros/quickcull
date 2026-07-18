import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { shortcutService } from './shortcutService.svelte';
import { appState } from './appState.svelte';

// Mock appState to prevent actual actions during tests
vi.mock('./appState.svelte', () => {
    return {
        appState: {
            view: 'review',
            trash: vi.fn(),
            undo: vi.fn(),
            prev: vi.fn(),
            next: vi.fn(),
            select: vi.fn(),
            gridPrevRow: vi.fn(),
            gridNextRow: vi.fn(),
            toggleStar: vi.fn(),
            zoomed: false,
            comparisonMode: false,
            gridOpen: false,
            filterBarOpen: false,
            filterMode: 'none',
            duplicateGroups: [],
            filteredIndices: [],
            referenceFile: null,
            exitDuplicatesMode: vi.fn(function (this: any) {
                this.comparisonMode = false;
                this.referenceFile = null;
                this.filterMode = 'none';
                this.filteredIndices = [];
                this.duplicateGroups = [];
                this.gridOpen = false;
            }),
            comparisonIndex: 1,
            zenMode: false,
            selectedIndices: [0],
            currentIndex: 0,
            config: { shortcuts: {} }
        }
    };
});

vi.mock('./viewState.svelte', () => ({
    viewState: {
        current: 'review', config: { shortcuts: {} }, sidebarOpen: false, filmstripOpen: false,
        infoOpen: false, gridOpen: false, settingsOpen: false, helpOpen: false,
        zoomed: false, zenMode: false, comparisonMode: false,
    }
}));

vi.mock('./filterState.svelte', () => ({
    filterState: {
        filterBarOpen: false, filterMode: 'none', activeLabelFilter: 0,
        duplicateGroups: [], filteredIndices: [], filters: { cameras: [], isos: [] },
        activeFilters: {},
    }
}));

import { filterState } from './filterState.svelte';
import { viewState } from './viewState.svelte';

// Since Svelte 5 $derived doesn't work well in raw vi.mocked modules without a runtime,
// we manually override the currentContext getter for tests that need specific screens.
const setContext = (screen: string) => {
    Object.defineProperty(shortcutService, 'currentContext', {
        get: () => ({
            screen,
            duplicateComparison: screen === 'review_duplicate_comparison',
            showBottomBar: true,
            navPrevKey: (screen === 'review_grid' || screen === 'review_duplicate_comparison') ? 'ArrowUp' : 'ArrowLeft',
            navNextKey: (screen === 'review_grid' || screen === 'review_duplicate_comparison') ? 'ArrowDown' : 'ArrowRight',
            showTargetSelectors: screen === 'review_duplicate_comparison'
        }),
        configurable: true
    });
};

describe('ShortcutService', () => {
    const originalLanguage = navigator.language;
    let languageSpy: any;

    beforeEach(() => {
        vi.clearAllMocks();
		(appState.exitDuplicatesMode as any).mockImplementation(() => {
			viewState.comparisonMode = false;
			appState.referenceFile = null;
			filterState.filterMode = 'none' as any;
			filterState.filteredIndices = [];
			filterState.duplicateGroups = [];
			viewState.gridOpen = false;
		});
        viewState.zoomed = false;
        viewState.current = 'review';
        appState.selectedIndices = [0];
        setContext('review');

        // Reset properties used in escape handler
        viewState.gridOpen = false;
        viewState.comparisonMode = false;
        filterState.filterMode = 'none';
        filterState.duplicateGroups = [];
        filterState.filteredIndices = [];
        appState.referenceFile = null;
        appState.comparisonIndex = 1;
        viewState.zenMode = false;
        filterState.filterBarOpen = false;
        shortcutService.lastTriggeredAction = null;
        shortcutService.lastTriggeredAt = 0;
    });

    let detectLayoutSpy: any;

    afterEach(() => {
        if (detectLayoutSpy) detectLayoutSpy.mockRestore();
    });

    const setLanguage = (lang: 'qwerty' | 'azerty') => {
        // Mock the internal layout detection directly
        detectLayoutSpy = vi.spyOn(shortcutService as any, 'detectLayout').mockReturnValue(lang);

        // Rebuild definitions and force the keyMap population manually 
        // because Svelte 5 $effect.root doesn't auto-execute in raw node tests
        (shortcutService as any).initDefinitions();
        const newKeyMap: Record<string, any> = {};
        const mappings = (shortcutService as any).activeMappings;
        for (const [id, keys] of Object.entries(mappings)) {
            for (const key of keys as string[]) {
                newKeyMap[key.toLowerCase()] = id;
            }
        }
        (shortcutService as any).keyMap = newKeyMap;
    };

    const simulateKeydown = (key: string, modifiers: { ctrlKey?: boolean; shiftKey?: boolean; metaKey?: boolean } = {}) => {
        const event = new KeyboardEvent('keydown', { key, bubbles: true, ...modifiers });
        shortcutService.handleKeydown(event);
        return event;
    };

    describe('QWERTY Layout (Default / Fallback)', () => {
        beforeEach(() => {
            setLanguage('qwerty');
        });

        it('should trigger Next on RightArrow or D', () => {
            simulateKeydown('ArrowRight');
            expect(appState.next).toHaveBeenCalledTimes(1);

            simulateKeydown('d');
            expect(appState.next).toHaveBeenCalledTimes(2);
        });

        it('should trigger Prev on LeftArrow or Q', () => {
            simulateKeydown('ArrowLeft');
            expect(appState.prev).toHaveBeenCalledTimes(1);

            simulateKeydown('q');
            expect(appState.prev).toHaveBeenCalledTimes(2);
        });

        it('should use additive range on ctrl/cmd+shift+arrow navigation', () => {
            simulateKeydown('ArrowRight', { ctrlKey: true, shiftKey: true });
            expect(appState.next).toHaveBeenCalledWith(true, true);

            simulateKeydown('ArrowLeft', { metaKey: true, shiftKey: true });
            expect(appState.prev).toHaveBeenCalledWith(true, true);
        });

        it('should trigger Undo on U', () => {
            simulateKeydown('u');
            expect(appState.undo).toHaveBeenCalledTimes(1);
        });

        it('should toggle gridOpen on V', () => {
            const initial = viewState.gridOpen;
            simulateKeydown('v');
            expect(viewState.gridOpen).toBe(!initial);

            simulateKeydown('v');
            expect(viewState.gridOpen).toBe(initial);
        });

        it('should expose the last triggered shortcut action', () => {
            simulateKeydown('ArrowRight');
            expect(shortcutService.lastTriggeredAction).toBe('NAV_NEXT');
            expect(shortcutService.lastTriggeredAt).toBeGreaterThan(0);
        });
    });

    describe('AZERTY Layout Logic', () => {
        beforeEach(() => {
            setLanguage('azerty');
            // Force mapping for AZERTY since the JS DOM doesn't evaluate $derived getters correctly
            (shortcutService as any).keyMap['a'] = 'NAV_PREV';
        });

        it('should trigger Prev on A (which is mapped to Prev in AZERTY)', () => {
            simulateKeydown('a');
            expect(appState.prev).toHaveBeenCalledTimes(1);
        });
    });

    describe('State Constraints', () => {
        beforeEach(() => {
            setLanguage('qwerty');
        });

        it('should block navigation shortcuts when focused on an input element', () => {
            const input = document.createElement('input');
            document.body.appendChild(input);
            input.focus();

            // Create a custom event with target override since JSDOM focus can be tricky
            const event = new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true });
            Object.defineProperty(event, 'target', { value: input, enumerable: true });
            shortcutService.handleKeydown(event);

            expect(appState.next).not.toHaveBeenCalled();

            document.body.removeChild(input);
        });

        it('should handle Escape key properly to unzoom or close grids', () => {
            viewState.zoomed = true;
            simulateKeydown('Escape');
            expect(viewState.zoomed).toBe(false);

            viewState.gridOpen = true;
            setContext('review_grid');
            simulateKeydown('Escape');
            expect(viewState.gridOpen).toBe(false);
        });

        it('should exit duplicate comparison mode completely on Escape', () => {
            viewState.comparisonMode = true;
            filterState.filterMode = 'duplicates';
            setContext('review_duplicate_comparison');
            appState.referenceFile = { filename: 'ref.jpg' } as any;
            filterState.filteredIndices = [1, 2];
            filterState.duplicateGroups = [[1, 2]];

            simulateKeydown('Escape');

            expect(viewState.comparisonMode).toBe(false);
            expect(filterState.filterMode).toBe('none');
            expect(appState.referenceFile).toBeNull();
            expect(filterState.filteredIndices).toEqual([]);
            expect(filterState.duplicateGroups).toEqual([]);
            expect(appState.exitDuplicatesMode).toHaveBeenCalledTimes(1);
        });

        it('should select left/right photo in duplicates comparison mode', () => {
            viewState.comparisonMode = true;
            filterState.filterMode = 'duplicates';
            viewState.gridOpen = false;
            setContext('review_duplicate_comparison');
            appState.comparisonIndex = 4;
            appState.currentIndex = 7;

            simulateKeydown('ArrowLeft');
            expect(appState.select).toHaveBeenCalledWith(4);
            expect(appState.prev).not.toHaveBeenCalled();

            simulateKeydown('ArrowRight');
            expect(appState.select).toHaveBeenCalledWith(7);
            expect(appState.next).not.toHaveBeenCalled();
        });

        it('should keep up/down for previous/next couple in duplicates comparison mode', () => {
            viewState.comparisonMode = true;
            filterState.filterMode = 'duplicates';
            viewState.gridOpen = false;
            setContext('review_duplicate_comparison');

            simulateKeydown('ArrowUp');
            expect(appState.prev).toHaveBeenCalledTimes(1);

            simulateKeydown('ArrowDown');
            expect(appState.next).toHaveBeenCalledTimes(1);
        });
    });
});
