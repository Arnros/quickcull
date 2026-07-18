<script lang="ts">
  import { onMount } from "svelte";
  import { appState } from "../appState.svelte";
  import { filterState } from "../filterState.svelte";
  import { viewState } from "../viewState.svelte";
  import { Virtualization } from "../virtualization.svelte";
  import PhotoBadge from "./PhotoBadge.svelte";

  let container: HTMLDivElement;

  const virtual = new Virtualization({
    itemSize: 56,
    gap: 4, // var(--space-1) is 4px
    padding: 4,
    direction: "horizontal",
    buffer: 5,
  });

  // Cache the full items array to avoid re-allocating for large folders
  let visibleCount = $derived(appState.v2?.VisibleOrder?.length || 0);
  let fullItemsArray = $derived(Array.from({ length: visibleCount }, (_, i) => i));

  let items = $derived.by(() => {
    if (
      filterState.filterMode !== "none" ||
      Object.keys(filterState.activeFilters).length > 0
    ) {
      return filterState.filteredIndices;
    }
    return fullItemsArray;
  });

  let totalWidth = $derived(virtual.getTotalSize(items.length));
  let visibleRange = $derived(virtual.getRange(items.length));

  onMount(() => {
    const ro = new ResizeObserver((entries) => {
      for (let entry of entries) {
        virtual.updateDimensions(entry.contentRect.width, entry.contentRect.height);
      }
    });

    if (container) {
      ro.observe(container);
    }

    return () => ro.disconnect();
  });

  // Sync scroll when currentIndex changes or when container dimensions become available
  // (containerWidth starts at 0 until ResizeObserver fires, so we must re-scroll once dimensions are known)
  $effect(() => {
    const idx = appState.currentIndex;
    const width = virtual.containerWidth; // tracked so the effect re-runs when dimensions arrive
    if (container && width > 0) {
      void virtual.scrollTo(container, idx, items, "smooth");
    }
  });
</script>

<div
  bind:this={container}
  class="filmstrip"
  onscroll={(e) => virtual.handleScroll(e)}
>
  <div class="filmstrip-content" style="width: {totalWidth}px; height: 100%;">
    {#each visibleRange as item (appState.v2?.VisibleOrder?.[items[item.index]] ?? `idx:${items[item.index]}`)}
      {@const i = items[item.index]}
      <button
        class="thumb"
        class:active={i === appState.currentIndex}
        class:is-reference={i === appState.comparisonIndex &&
          viewState.comparisonMode}
        class:burst={appState.currentFile?.index === i &&
          appState.currentFile?.burst}
        style="transform: translate3d({item.x}px, 0, 0);"
        onclick={() => appState.select(i)}
      >
        <img src={appState.getThumbUrl(i)} alt="thumbnail" loading="lazy" />

        <PhotoBadge
          isStarred={appState.v2?.Photos[appState.v2.VisibleOrder[i]]?.IsStarred ?? false}
          label={appState.v2?.Photos[appState.v2.VisibleOrder[i]]?.Label ?? 0}
          isSelected={appState.selectedIndices.includes(i)}
          burstIndex={appState.currentFile?.index === i ? appState.currentFile?.burst?.index : 0}
          burstCount={appState.currentFile?.index === i ? appState.currentFile?.burst?.count : 0}
          role={viewState.comparisonMode ? (i === appState.comparisonIndex ? 'reference' : (i === appState.currentIndex ? 'active' : 'none')) : 'none'}
        />
      </button>
    {/each}
  </div>
</div>

<style>
  .filmstrip {
    width: 100%;
    height: 72px;
    background:
      linear-gradient(180deg, rgba(var(--accent-rgb), 0.07), transparent 52%),
      var(--bg-surface);
    border-top: 1px solid var(--border-main);
    overflow-x: auto;
    overflow-y: hidden;
    flex-shrink: 0;
    contain: layout size style;
  }

  .filmstrip-content {
    position: relative;
    will-change: contents;
  }

  .thumb {
    position: absolute;
    top: 6px;
    left: 0;
    width: 56px;
    height: 56px;
    border-radius: var(--radius-md);
    overflow: hidden;
    border: 2px solid transparent;
    padding: 0;
    background: color-mix(in srgb, var(--bg-app) 92%, transparent);
    cursor: pointer;
    opacity: 0.6;
    transition:
      opacity var(--transition-fast),
      border-color var(--transition-fast);
    will-change: transform;
  }
  .thumb:hover {
    opacity: 1;
  }
  .thumb.active {
    opacity: 1;
    border-color: var(--accent);
  }
  .thumb.is-reference {
    border-color: var(--text-muted);
    opacity: 0.9;
  }
  .thumb.burst {
    border-bottom-color: var(--success);
  }

  img {
    width: 100%;
    height: 100%;
    object-fit: contain;
    background: #000;
  }
</style>
