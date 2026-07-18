<script lang="ts">
    import { onDestroy } from "svelte";
    import { appState } from "../appState.svelte";
    import { filterState } from "../filterState.svelte";
    import { viewState } from "../viewState.svelte";
    import { i18n } from "../i18n.svelte";
    import { resolveShortcutContext } from "../shortcutContext";
    import { shortcutService, type ShortcutActionId } from "../shortcutService.svelte";
    import Button from "./Button.svelte";
    import Kbd from "./Kbd.svelte";

    let shortcutCtx = $derived(resolveShortcutContext({
        view: viewState.current,
        gridOpen: viewState.gridOpen,
        filterBarOpen: filterState.filterBarOpen,
        comparisonMode: viewState.comparisonMode,
        filterMode: filterState.filterMode as 'none' | 'starred' | 'label' | 'duplicates',
        helpOpen: viewState.helpOpen,
        settingsOpen: viewState.settingsOpen,
    }));
    let prevTitleKey = $derived(shortcutCtx.navPrevKey);
    let nextTitleKey = $derived(shortcutCtx.navNextKey);
    let prevKeyHint = $derived(shortcutCtx.navPrevKey === 'ArrowUp' ? 'ArrowUp' : 'ArrowLeft');
    let nextKeyHint = $derived(shortcutCtx.navNextKey === 'ArrowDown' ? 'ArrowDown' : 'ArrowRight');

    type ButtonFlashId = 'target-left' | 'target-right' | 'prev' | 'next' | 'star' | 'rotate-left' | 'rotate-right' | 'trash' | 'undo';
    const FLASH_MS = 150;
    let flashes = $state<Record<ButtonFlashId, boolean>>({
        'target-left': false,
        'target-right': false,
        prev: false,
        next: false,
        star: false,
        'rotate-left': false,
        'rotate-right': false,
        trash: false,
        undo: false,
    });
    const timers: Partial<Record<ButtonFlashId, ReturnType<typeof setTimeout>>> = {};

    function flashButton(id: ButtonFlashId) {
        if (timers[id]) clearTimeout(timers[id]);
        flashes[id] = true;
        timers[id] = setTimeout(() => {
            flashes[id] = false;
        }, FLASH_MS);
    }

    function mapActionToButton(actionId: ShortcutActionId): ButtonFlashId | null {
        if (shortcutCtx.duplicateComparison) {
            if (actionId === 'NAV_PREV') return 'target-left';
            if (actionId === 'NAV_NEXT') return 'target-right';
            if (actionId === 'NAV_UP') return 'prev';
            if (actionId === 'NAV_DOWN') return 'next';
        }

        if (actionId === 'NAV_PREV' || actionId === 'NAV_UP') return 'prev';
        if (actionId === 'NAV_NEXT' || actionId === 'NAV_DOWN') return 'next';
        if (actionId === 'ACTION_STAR') return 'star';
        if (actionId === 'ACTION_ROTATE_LEFT') return 'rotate-left';
        if (actionId === 'ACTION_ROTATE_RIGHT') return 'rotate-right';
        if (actionId === 'ACTION_TRASH') return 'trash';
        if (actionId === 'ACTION_UNDO') return 'undo';
        return null;
    }

    $effect(() => {
        const token = shortcutService.lastTriggeredAt;
        const actionId = shortcutService.lastTriggeredAction;
        if (!token || !actionId || !shortcutCtx.showBottomBar) return;

        const targetButton = mapActionToButton(actionId);
        if (targetButton) flashButton(targetButton);
    });

    onDestroy(() => {
        for (const timer of Object.values(timers)) {
            if (timer) clearTimeout(timer);
        }
    });
</script>

<div class="bottombar">
    <div class="status-group">
        {#if appState.currentFile}
            {@const fp = appState.currentFile.filename}
            {@const slash = fp.lastIndexOf('/')}
            <div class="filepath" title={fp}>
                {#if slash >= 0}
                    <span class="filepath-dir">{fp.slice(0, slash + 1)}</span><span class="filepath-name">{fp.slice(slash + 1)}</span>
                {:else}
                    {fp}
                {/if}
            </div>
        {/if}

        {#if viewState.config?.debug}
        <div class="perf-status" title={i18n.t('perf_metrics')}>
            <span>evt p:{appState.perf.progressEvents} s:{appState.perf.stateEvents} c:{appState.perf.completeEvents}</span>
            <span>poll:{appState.perf.pollRequests}</span>
            <span>flush:{appState.perf.analysisFlushes}</span>
            <span>avg:{appState.perf.avgFlushDelayMs}{i18n.t('unit_seconds') === 's' ? 'ms' : 'ms'}</span>
            <span>last:{appState.perf.lastSource}/{appState.perf.lastFlushDelayMs}ms</span>
        </div>
        {/if}
    </div>
    <div class="controls">
        {#if shortcutCtx.showTargetSelectors}
            <Button
                variant="action"
                class={appState.selectedIndices.length === 1 && appState.selectedIndices[0] === appState.comparisonIndex ? "active-target" : ""}
                active={flashes['target-left']}
                onclick={() => appState.select(appState.comparisonIndex)}
                title={`${i18n.t('reference')} (ArrowLeft)`}
            >
                <Kbd key="ArrowLeft" variant="outline" />
                {i18n.t("reference")}
            </Button>
            <Button
                variant="action"
                class={appState.selectedIndices.length === 1 && appState.selectedIndices[0] === appState.currentIndex ? "active-target" : ""}
                active={flashes['target-right']}
                onclick={() => appState.select(appState.currentIndex)}
                title={`${i18n.t('active')} (ArrowRight)`}
            >
                {i18n.t("active")}
                <Kbd key="ArrowRight" variant="outline" />
            </Button>
        {/if}
        <Button
            variant="action"
            active={flashes.prev}
            onclick={() => appState.prev()}
            title={`${i18n.t('prev')} (${prevTitleKey})`}
        >
            <Kbd key={prevKeyHint} variant="outline" />
            {i18n.t("prev")}
        </Button>
        <Button
            variant="action"
            class="star-btn"
            active={flashes.star || appState.currentFile?.starred}
            onclick={() => appState.toggleStar()}
            title={`${i18n.t('star')} (S)`}
        >
            <span>{appState.currentFile?.starred ? "★" : "☆"}</span>
            {i18n.t("star")} <Kbd key="S" variant="outline" />
        </Button>
        <Button
            variant="action"
            active={flashes['rotate-left']}
            onclick={() => appState.rotate("left")}
            title={`${i18n.t('action_rotate_left')} (L)`}
            ariaLabel={i18n.t('action_rotate_left')}
        >
            <Kbd key="L" variant="outline" /> ↺
        </Button>
        <Button
            variant="action"
            active={flashes['rotate-right']}
            onclick={() => appState.rotate("right")}
            title={`${i18n.t('action_rotate_right')} (R)`}
            ariaLabel={i18n.t('action_rotate_right')}
        >
            <Kbd key="R" variant="outline" /> ↻
        </Button>
        <Button
            variant="danger"
            active={flashes.trash}
            onclick={() => appState.trash()}
            title={`${i18n.t('trash')} (X)`}
        >
            🗑 {i18n.t("trash")} <Kbd key="X" variant="outline" />
        </Button>
        <Button 
            variant="action" 
            active={flashes.undo} 
            onclick={() => appState.undo()} 
            title={`${i18n.t('undo')} (U)`}
        >
            <Kbd key="U" variant="outline" />
            {i18n.t("undo")}
        </Button>
        <Button 
            variant="action" 
            active={flashes.next} 
            onclick={() => appState.next()} 
            title={`${i18n.t('next')} (${nextTitleKey})`}
        >
            {i18n.t("next")} <Kbd key={nextKeyHint} variant="outline" />
        </Button>
    </div>
</div>

<style>
    .bottombar {
        width: 100%;
        min-height: 60px;
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-2-5);
        padding: 0 var(--space-4);
        background:
          linear-gradient(180deg, rgba(var(--accent-rgb), 0.08), transparent 48%),
          var(--bg-surface);
        backdrop-filter: var(--blur-md);
        -webkit-backdrop-filter: var(--blur-md);
        border-top: 1px solid var(--border-main);
        flex-shrink: 0;
        position: relative;
        z-index: var(--z-bar);
        box-shadow: 0 -8px 24px rgba(0, 0, 0, 0.14);
    }

    .status-group {
        display: flex;
        align-items: center;
        gap: var(--space-4);
        flex: 1 1 auto;
        min-width: 0;
        max-width: none;
    }

    .perf-status {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        padding: 5px var(--space-2);
        border-radius: var(--radius-md);
        border: 1px dashed rgba(var(--accent-rgb), 0.35);
        background: var(--bg-app);
        color: var(--text-muted);
        font-family: var(--mono);
        font-size: var(--text-xs);
        white-space: nowrap;
    }

    .filepath {
        display: flex;
        align-items: baseline;
        gap: 0;
        flex: 1 1 auto;
        min-width: 0;
        font-size: var(--text-caption);
        font-family: var(--mono);
        color: var(--text-muted);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        max-width: none;
        background: color-mix(in srgb, var(--key-bg) 90%, transparent);
        padding: var(--space-1) var(--space-2);
        border-radius: var(--radius-sm);
        border: 1px solid var(--border-main);
    }

    .filepath-dir {
        opacity: 0.5;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        flex: 1 1 auto;
        min-width: 0;
    }

    .filepath-name {
        color: var(--text-main);
        font-weight: 500;
        flex: 0 1 auto;
        max-width: 60%;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .controls {
        display: flex;
        align-items: center;
        justify-content: flex-end;
        flex-wrap: wrap;
        gap: var(--space-2-5);
        flex: 0 1 auto;
    }

    :global(.bottombar .btn) {
        padding: var(--space-2) var(--space-5);
        font-weight: 500;
    }

    :global(.bottombar .btn.active) {
        animation: shortcut-flash 150ms ease-out;
        border-color: var(--accent);
        background: var(--accent-dim);
        color: var(--accent);
    }

    @keyframes shortcut-flash {
        0% { transform: translateY(0); }
        50% { transform: translateY(-1px) scale(1.01); }
        100% { transform: translateY(0); }
    }

    :global(.bottombar .btn.star-btn.active) {
        color: var(--star);
        border-color: var(--star);
        background: var(--star-dim);
    }

    :global(.bottombar .btn.active-target) {
        border-color: var(--accent);
        color: var(--accent);
        background: var(--accent-dim);
    }

    @media (prefers-reduced-motion: reduce) {
        :global(.bottombar .btn.active) {
            animation: none;
        }
    }

    @media (max-width: 1200px) {
        .bottombar {
            padding: var(--space-2) var(--space-3);
            align-items: stretch;
            flex-wrap: wrap;
        }

        .status-group {
            width: 100%;
            order: 1;
        }

        .controls {
            width: 100%;
            order: 2;
            justify-content: flex-start;
        }

        :global(.bottombar .btn) {
            padding: var(--space-2) var(--space-3);
        }
    }

    @media (max-width: 820px) {
        .perf-status {
            display: none;
        }
    }

</style>
