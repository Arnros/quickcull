<script lang="ts">
  import { onMount } from "svelte";
  import { appState } from "../appState.svelte";
  import { i18n } from "../i18n.svelte";
  import Icon from "./Icon.svelte";
  import { logger } from "../logger";
  import { GRID_GAP, GRID_ITEM_SIZE, GRID_PADDING } from "../constants";
  import { Virtualization } from "../virtualization.svelte";
  import { viewState } from "../viewState.svelte";
  import PhotoBadge from "./PhotoBadge.svelte";
  import Button from "./Button.svelte";
  import Kbd from "./Kbd.svelte";

  let container: HTMLDivElement;
  let loadedThumbs = $state<Record<string, boolean>>({});

  const virtual = new Virtualization({
    itemSize: GRID_ITEM_SIZE,
    gap: GRID_GAP,
    padding: GRID_PADDING,
    direction: 'vertical'
  });

  // Cache the full items array to avoid re-allocating for large folders
  let fullItemsArray = $state<number[]>([]);
  $effect(() => {
    const total = appState.v2?.VisibleOrder?.length || 0;
    if (fullItemsArray.length !== total) {
      // Optimized: only re-create the array if the count changed
      fullItemsArray = Array.from({ length: total }, (_, i) => i);
    }
  });

  /**
   * Items list with optional null spacers to force visual line-breaks between
   * folder groups. Spacers fill the remaining cells of each folder's last row
   * so the next folder always starts at column 0.
   *
   * - Active only in non-filtered mode with multiple folders.
   * - Falls back to plain index arrays when filters are on or cols is unknown.
   */
  let items = $derived.by((): (number | null)[] => {
    const hasFilters =
      appState.filterMode !== "none" ||
      Object.keys(appState.activeFilters).length > 0;
    if (hasFilters) return appState.filteredIndices;

    const cols = virtual.columns;
    const folders = appState.folders;
    if (!folders || folders.length <= 1 || cols <= 1) return fullItemsArray;

    // Build the items array with null spacers at each folder boundary
    const result: (number | null)[] = [];
    for (const f of folders) {
      for (let i = 0; i < f.count; i++) {
        result.push(f.startIndex + i);
      }
      // How many cells remain in this folder's last row?
      const remainder = f.count % cols;
      if (remainder !== 0) {
        const pad = cols - remainder;
        for (let i = 0; i < pad; i++) result.push(null);
      }
    }
    return result;
  });

  let totalHeight = $derived(virtual.getTotalSize(items.length));
  let visibleRange = $derived(virtual.getRange(items.length));

  // Attention Stream: Prioritize processing for visible indices
  let lastPrioritizeAt = 0;
  $effect(() => {
    const range = visibleRange;
    const now = Date.now();
    // Throttle to avoid bridge congestion
    if (now - lastPrioritizeAt < 150) return;

    // range is an array of {index, x, y}. obj.index is the offset in the 'items' array.
    // Filter out null spacers before sending to backend.
    const visibleIndices = range
      .map(obj => items[obj.index])
      .filter((idx): idx is number => idx !== null && idx !== undefined);
    if (visibleIndices.length > 0) {
      lastPrioritizeAt = now;
      appState.prioritizeIndices(visibleIndices);
    }
  });

  // Optimized O(1) lookups for render cycle
  let selectedSet = $derived(new Set(appState.selectedIndices));

  let didInitialAlign = $state(false);

  onMount(() => {
    logger.debug("Grid mounted", { itemCount: items.length });

    const ro = new ResizeObserver((entries) => {
      for (let entry of entries) {
        virtual.updateDimensions(entry.contentRect.width, entry.contentRect.height);
        appState.gridColumns = virtual.columns;
      }
    });

    if (container) {
      container.scrollTop = viewState.gridScrollTop;
      container.scrollLeft = viewState.gridScrollLeft;
      virtual.scrollTop = viewState.gridScrollTop;
      virtual.scrollLeft = viewState.gridScrollLeft;
      ro.observe(container);
    }

    return () => ro.disconnect();
  });

  function handleClick(e: MouseEvent, i: number) {
    if (e.detail > 1) {
      // Double click to open and close grid
      appState.select(i, false, false);
      if (appState.filterMode === "duplicates") {
        appState.comparisonMode = true;
      }
      appState.gridOpen = false;
      return;
    }
    appState.select(i, e.ctrlKey || e.metaKey, e.shiftKey);
  }

  function handleContextMenu(e: MouseEvent, i: number) {
    e.preventDefault();
    appState.revealInExplorer(i);
  }

  function photoKeyForIndex(id: number | null, arrayIdx: number) {
    if (id === null) return `spacer:${arrayIdx}`;
    return appState.v2?.VisibleOrder?.[id] ?? `idx:${id}`;
  }

  /**
   * Returns the display label for a folder path: the last 2 segments joined by '/'.
   * e.g. "Photos/2024/01" → "2024/01", "." or "" → '' (hidden)
   */
  function folderLabel(path: string): string {
    if (!path || path === '.') return '';
    const parts = path.split('/').filter((p) => p.length > 0 && p !== '.');
    if (parts.length === 0) return '';
    return parts.slice(-2).join('/');
  }

  /**
   * Maps the real photo index (startIndex) → folder label for the first photo of
   * each folder group. Only populated when there are multiple distinct folders,
   * so single-folder sessions have zero overhead.
   */
  let folderStartMap = $derived.by(() => {
    const map = new Map<number, string>();
    if ((appState.folders?.length ?? 0) <= 1) return map;
    for (const f of appState.folders ?? []) {
      const label = folderLabel(f.path);
      if (label) map.set(f.startIndex, label);
    }
    return map;
  });

  function onThumbLoad(photoKey: string) {
    if (loadedThumbs[photoKey]) return;
    loadedThumbs = { ...loadedThumbs, [photoKey]: true };
  }

  function isThumbLoaded(photoKey: string) {
    return !!loadedThumbs[photoKey];
  }

  $effect(() => {
    appState.sessionVersion;
    loadedThumbs = {};
  });

  let scrollPersistTimer: ReturnType<typeof setTimeout> | undefined;

  function handleViewportScroll(e: UIEvent) {
    virtual.handleScroll(e);
    clearTimeout(scrollPersistTimer);
    scrollPersistTimer = setTimeout(() => {
      viewState.gridScrollTop = virtual.scrollTop;
      viewState.gridScrollLeft = virtual.scrollLeft;
    }, 100);
  }

  // Sync scroll when currentIndex changes or when container dimensions become available
  $effect(() => {
    const idx = appState.currentIndex;
    const open = appState.gridOpen;
    const h = virtual.containerHeight;
    const w = virtual.containerWidth;
    const total = items.length;

    if (!open || !container || h <= 0 || w <= 0 || total <= 0) return;

    // Use 'auto' for initial align and 'smooth' for subsequent jumps
    const behavior = didInitialAlign ? "smooth" : "auto";
    didInitialAlign = true;

    void virtual.scrollTo(container, idx, items, behavior, "nearest");
  });

  // Ensure keyboard shortcuts bound on the overlay (e.g. Ctrl/Cmd+A) are received.
  $effect(() => {
    if (!appState.gridOpen) return;
    queueMicrotask(() => {
      const overlay = container?.closest('.grid-overlay') as HTMLDivElement | null;
      overlay?.focus();
    });
  });
</script>

<div
  class="grid-overlay"
  onkeydown={(e) => {
    if (e.key === "a" && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      appState.selectAll();
    }
  }}
  tabindex="-1"
  role="dialog"
  aria-label={i18n.t("grid")}
>
  <div class="grid-header">
    <div class="header-left">
      <h2>{i18n.t("grid")} <span class="count-badge">{items.length}</span></h2>
      
      <div class="header-filters">
        <Button
          variant="mini"
          icon="star"
          class="star-filter"
          active={appState.filterMode === "starred"}
          onclick={() => appState.toggleStarFilter()}
          title={i18n.t("starred")}
        />

        <div class="v-divider"></div>

        {#each Array.from({ length: appState.maxLabel }, (_, i) => i + 1) as i}
          <Button
            variant="mini"
            class="label-filter"
            active={appState.filterMode === "label" &&
              appState.activeLabelFilter === i}
            onclick={() => appState.setLabelFilter(i)}
            title={`${i18n.t("labeled")} ${i}`}
          >
            <div class="dot" style="background-color: var(--label-{i})"></div>
          </Button>
        {/each}

        <Button
          variant="mini"
          active={appState.filterMode === "label" &&
            appState.activeLabelFilter === 0}
          onclick={() => appState.setLabelFilter(0)}
          title={i18n.t("labeled")}
        >
          <div class="multi-dots">
            <span class="d1"></span>
            <span class="d2"></span>
          </div>
        </Button>

        <div class="v-divider"></div>

        <Button
          variant="mini"
          active={appState.filterMode === "duplicates"}
          onclick={() => appState.toggleDuplicatesFilter()}
          title={i18n.t("duplicates")}
        >👯</Button>
      </div>
    </div>
    <div class="actions-group">
      <button class="text-btn" onclick={() => appState.selectAll()}>
        {i18n.t("select_all")}
      </button>
      <span class="selection-count"
        >{appState.selectedIndices.length} {i18n.t("selected")}</span
      >
      <Button variant="primary" icon="close" onclick={() => (appState.gridOpen = false)}>
        {i18n.t("close")} <Kbd key="V" variant="outline" />
      </Button>
    </div>
  </div>

  <div bind:this={container} class="grid-viewport" onscroll={handleViewportScroll}>
    <div class="grid-content" style="height: {totalHeight}px">
      {#each visibleRange as item (photoKeyForIndex(items[item.index], item.index))}
        {@const id = items[item.index]}
        {#if id === null}
          <!-- Invisible spacer cell filling the remainder of a folder's last row -->
          <div
            class="grid-spacer"
            aria-hidden="true"
            style="width: {GRID_ITEM_SIZE}px; height: {GRID_ITEM_SIZE}px; transform: translate3d({item.x}px, {item.y}px, 0);"
          ></div>
        {:else}
          {@const photoKey = photoKeyForIndex(id, item.index)}
          <button
            class="grid-item"
            class:active={id === appState.currentIndex}
            class:selected={selectedSet.has(id)}
            class:loading={!isThumbLoaded(photoKey)}
            style="width: {GRID_ITEM_SIZE}px; height: {GRID_ITEM_SIZE}px; transform: translate3d({item.x}px, {item.y}px, 0);"
            onclick={(e) => handleClick(e, id)}
            oncontextmenu={(e) => handleContextMenu(e, id)}
          >
            {#if !isThumbLoaded(photoKey)}
              <div class="thumb-skeleton" aria-hidden="true"></div>
            {/if}
            <img
              src={appState.getThumbUrl(id)}
              alt="thumb"
              loading="lazy"
              decoding="async"
              class:visible={isThumbLoaded(photoKey)}
              onload={() => onThumbLoad(photoKey)}
              onerror={(e) => {
                const img = e.currentTarget as HTMLImageElement;
                if (img.dataset.retried) {
                  onThumbLoad(photoKey);
                  return;
                }
                img.dataset.retried = "true";
                setTimeout(() => {
                  const currentSrc = img.src;
                  img.src = currentSrc.includes('?') ? `${currentSrc}&retry=1` : `${currentSrc}?retry=1`;
                }, 500);
              }}
            />

            <PhotoBadge
              isStarred={appState.v2?.Photos[appState.v2.VisibleOrder[id]]?.IsStarred ?? false}
              label={appState.v2?.Photos[appState.v2.VisibleOrder[id]]?.Label ?? 0}
              isSelected={selectedSet.has(id)}
              burstIndex={appState.currentFile?.index === id ? appState.currentFile?.burst?.index : 0}
              burstCount={appState.currentFile?.index === id ? appState.currentFile?.burst?.count : 0}
              role={appState.comparisonMode ? (id === appState.comparisonIndex ? 'reference' : (id === appState.currentIndex ? 'active' : 'none')) : 'none'}
            />

            {#if folderStartMap.has(id)}
              <span class="folder-badge" aria-hidden="true">{folderStartMap.get(id)}</span>
            {/if}
          </button>
        {/if}
      {/each}
    </div>
  </div>

  {#if appState.selectedIndices.length > 0}
    <div class="selection-bar">
      <div class="selection-info">
        <span class="count">{appState.selectedIndices.length}</span>
        <span class="label">{i18n.t("selected")}</span>
      </div>

      <div class="selection-actions">
        <Button variant="action" icon="trash" onclick={() => appState.trash()} title={i18n.t("trash")}>
          {i18n.t("trash")}
        </Button>
        <Button variant="action" icon="star" onclick={() => appState.toggleStar()} title={i18n.t("star")}>
          {i18n.t("star")}
        </Button>

        <div class="v-divider"></div>

        <Button
          variant="primary"
          icon="export"
          disabled={appState.isExporting}
          onclick={() => appState.exportSelected(false)}
          title={i18n.t("copy")}
        >
          {i18n.t("copy")} ({appState.selectedIndices.length})
        </Button>

        <Button
          variant="secondary"
          class="move-btn"
          disabled={appState.isExporting}
          onclick={() => appState.exportSelected(true)}
          title={i18n.t("move")}
        >
          {i18n.t("move")} ({appState.selectedIndices.length})
        </Button>
      </div>

      <button
        class="clear-btn"
        onclick={() => (appState.selectedIndices = [])}
        title={i18n.t("clear_selection")}
      >
        {i18n.t("cancel")}
      </button>
    </div>
  {/if}
</div>

<style>
  .grid-overlay {
    position: absolute;
    inset: 0;
    background:
      radial-gradient(circle at 10% 10%, rgba(var(--accent-rgb), 0.07), transparent 32%),
      var(--bg-app);
    z-index: var(--z-grid);
    display: flex;
    flex-direction: column;
    animation: fade-in var(--transition-base) ease-out;
  }

  .grid-header {
    height: 64px;
    padding: 0 var(--space-6);
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-bottom: 1px solid var(--border-main);
    background:
      linear-gradient(180deg, rgba(var(--accent-rgb), 0.08), transparent 52%),
      var(--bg-surface);
    backdrop-filter: var(--blur-lg);
    z-index: var(--z-bar);
  }

  .header-left {
    display: flex;
    align-items: center;
    gap: var(--space-6);
  }
  .header-filters {
    display: flex;
    gap: var(--space-1-5);
    background: color-mix(in srgb, var(--bg-app) 86%, transparent);
    padding: var(--space-1);
    border-radius: var(--radius-lg);
    border: 1px solid var(--border-main);
  }

  :global(.grid-header .mini-filter) {
    width: 30px;
    height: 30px;
    padding: 0;
  }

  .dot {
    width: 10px;
    height: 10px;
    border-radius: var(--radius-full);
    border: 1px solid var(--dot-border);
  }

  .multi-dots {
    display: flex;
    gap: var(--space-1);
  }
  .multi-dots span {
    width: 5px;
    height: 5px;
    border-radius: var(--radius-full);
    background: var(--text-muted);
  }

  .v-divider {
    width: 1px;
    height: 16px;
    background: var(--border-main);
    margin: 0 var(--space-1);
  }

  .count-badge {
    font-size: var(--text-caption);
    background: var(--key-bg);
    padding: 2px var(--space-2);
    border-radius: var(--radius-lg);
    color: var(--text-muted);
    margin-left: var(--space-2);
  }

  .actions-group {
    display: flex;
    align-items: center;
    gap: var(--space-4);
  }
  .selection-count {
    font-size: var(--text-base);
    color: var(--text-muted);
  }

  .text-btn {
    background: transparent;
    border: none;
    color: var(--accent);
    font-size: var(--text-base);
    font-weight: 600;
    cursor: pointer;
    padding: var(--space-1) var(--space-2);
    border-radius: var(--radius-sm);
  }
  .text-btn:hover {
    background: var(--accent-dim);
  }

  .grid-viewport {
    flex: 1;
    overflow-y: auto;
    overflow-x: hidden;
    background: var(--bg-app);
    /* Performance hint for browser */
    contain: layout size style;
  }

  .grid-content {
    position: relative;
    width: 100%;
    will-change: contents;
  }

  /* Invisible placeholder cells filling the remainder of a folder's last row */
  .grid-spacer {
    position: absolute;
    top: 0;
    left: 0;
    will-change: transform;
    pointer-events: none;
  }

  .grid-item {
    position: absolute;
    top: 0;
    left: 0;
    background: color-mix(in srgb, var(--bg-surface) 82%, transparent);
    border: 2px solid transparent;
    border-radius: var(--radius-lg);
    overflow: hidden;
    padding: 0;
    cursor: pointer;
    transition: border-color var(--transition-base), transform var(--transition-fast) ease-out;
    box-shadow: var(--shadow-item), 0 0 0 1px rgba(var(--accent-rgb), 0.04);
    will-change: transform;
  }

  .grid-item:hover {
    transform: translateY(-2px);
    border-color: var(--text-muted);
    z-index: 5;
  }
  .grid-item.active {
    border-color: var(--accent);
    box-shadow: 0 0 0 2px var(--accent);
    z-index: 6;
  }
  .grid-item.selected {
    border-color: var(--accent);
  }
  .grid-item.selected::after {
    content: "";
    position: absolute;
    inset: 0;
    background: rgba(var(--accent-rgb), 0.15);
  }

  .grid-item img {
    position: relative;
    width: 100%;
    height: 100%;
    object-fit: contain;
    background: #000;
    opacity: 0;
    transition: opacity 0.22s ease;
    z-index: 2;
  }

  .grid-item img.visible {
    opacity: 1;
  }

  .thumb-skeleton {
    position: absolute;
    inset: 0;
    background:
      linear-gradient(100deg, rgba(255,255,255,0.03) 20%, rgba(255,255,255,0.12) 38%, rgba(255,255,255,0.03) 56%),
      radial-gradient(circle at 30% 25%, rgba(255,255,255,0.08), transparent 55%),
      #131313;
    background-size: 220% 100%, 100% 100%, 100% 100%;
    animation: skeleton-slide 1.2s linear infinite;
    z-index: 1;
  }

  @keyframes skeleton-slide {
    0% { background-position: 200% 0, 0 0, 0 0; }
    100% { background-position: -40% 0, 0 0, 0 0; }
  }

  .selection-bar {
    position: absolute;
    bottom: var(--space-8);
    left: 50%;
    transform: translateX(-50%);
    background: linear-gradient(180deg, var(--bg-surface), var(--bg-app));
    border: 1px solid rgba(var(--accent-rgb), 0.72);
    padding: var(--space-2) var(--space-6);
    border-radius: var(--radius-2xl);
    display: flex;
    align-items: center;
    gap: var(--space-8);
    box-shadow:
      0 12px 40px rgba(0, 0, 0, 0.6),
      0 0 0 1px rgba(var(--accent-rgb), 0.2);
    backdrop-filter: blur(24px);
    z-index: var(--z-selection);
    animation: slide-up var(--transition-slow) cubic-bezier(0.16, 1, 0.3, 1);
  }

  @keyframes slide-up {
    from {
      transform: translate(-50%, 20px);
      opacity: 0;
    }
    to {
      transform: translate(-50%, 0);
      opacity: 1;
    }
  }

  .selection-info {
    display: flex;
    align-items: baseline;
    gap: var(--space-1-5);
    color: var(--accent);
  }

  .selection-info .count {
    font-size: 20px;
    font-weight: 800;
    font-family: var(--mono);
  }

  .selection-info .label {
    font-size: var(--text-base);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    opacity: 0.8;
  }

  .selection-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .clear-btn {
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    font-size: var(--text-base);
    font-weight: 600;
    padding: var(--space-1) var(--space-2);
  }

  .clear-btn:hover {
    color: var(--danger);
  }

  .folder-badge {
    position: absolute;
    bottom: 5px;
    /* Flush with the left edge of the thumbnail for a "tab" look */
    left: 0;
    background: rgba(0, 0, 0, 0.78);
    backdrop-filter: blur(8px);
    color: rgba(255, 255, 255, 0.95);
    font-size: 10px;
    font-family: var(--mono);
    font-weight: 700;
    letter-spacing: 0.04em;
    /* Right side rounded, left flush */
    padding: 3px 8px 3px 5px;
    border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
    line-height: 1.3;
    max-width: calc(100% - 4px);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    /* Above the img (z-index 2) */
    z-index: 3;
    pointer-events: none;
    /* Accent left border — the main visual hook during scroll */
    border-left: 3px solid var(--accent);
    box-shadow: 0 2px 10px rgba(0, 0, 0, 0.5);
  }
</style>
