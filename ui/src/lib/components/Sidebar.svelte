<script lang="ts">
  import { appState } from "../appState.svelte";
  import { cameraLabel } from "../cameraLabel";
  import { i18n } from "../i18n.svelte";
  import { buildFolderTree, type TreeNode } from "../treeUtils";

  const { open } = $props<{ open: boolean }>();

  const tree = $derived(buildFolderTree(appState.folders));

  let defaultExpanded = $state(true);
  let expandedState = $state<Record<string, boolean>>({});

  function isExpanded(path: string) {
    return expandedState[path] ?? defaultExpanded;
  }

  function toggle(path: string) {
    expandedState[path] = !isExpanded(path);
  }

  function collapseAll() {
    defaultExpanded = false;
    expandedState = {};
  }

  function expandAll() {
    defaultExpanded = true;
    expandedState = {};
  }

  function handleKeydown(e: KeyboardEvent) {
    if (!open) return;
    
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      const focusable = Array.from(document.querySelectorAll('aside.open button:not(.toggle)')) as HTMLButtonElement[];
      const index = focusable.indexOf(document.activeElement as HTMLButtonElement);
      let nextIndex = 0;
      if (e.key === "ArrowDown") {
        nextIndex = (index + 1) % focusable.length;
      } else {
        nextIndex = (index - 1 + focusable.length) % focusable.length;
      }
      focusable[nextIndex]?.focus();
    }
  }
</script>

{#snippet folderNode({ node, level }: { node: TreeNode; level: number })}
  <div class="folder-node">
    <div class="row">
      <div class="indent" style="width: {level * 16 + 8}px"></div>
      {#if node.childrenArr.length > 0}
        <button class="toggle" onclick={() => toggle(node.fullPath)} tabindex="-1">
          {isExpanded(node.fullPath) ? "▼" : "▶"}
        </button>
      {:else}
        <span class="spacer"></span>
      {/if}

      <button
        class="name-btn"
        class:active={appState.currentFile?.folder === node.fullPath}
        class:clickable={!!node.folderInfo}
        onclick={() => {
          if (node.folderInfo) {
            appState.loadFile(node.folderInfo.startIndex);
          }
          if (node.childrenArr.length > 0) {
            toggle(node.fullPath);
          }
        }}
      >
        <span class="label" title={node.name}
          >{node.name === "/" ? i18n.t("folder") : node.name}</span
        >
        {#if node.folderInfo}
          <span class="count">{node.folderInfo.count}</span>
        {/if}
      </button>
    </div>

    {#if isExpanded(node.fullPath) && node.childrenArr.length > 0}
      <div class="children">
        {#each node.childrenArr as child}
          {@render folderNode({ node: child, level: level + 1 })}
        {/each}
      </div>
    {/if}
  </div>
{/snippet}

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<aside class:open onkeydown={handleKeydown}>
  <div class="header">
    <div class="header-left">
      <span>{i18n.t("folders")}</span>
      <span class="total-count">{appState.folders.length}</span>
    </div>
    <div class="header-actions">
      <button class="icon-btn-small" onclick={expandAll} title={i18n.t('expand_all')}
        >⇊</button
      >
      <button class="icon-btn-small" onclick={collapseAll} title={i18n.t('collapse_all')}
        >⇈</button
      >
    </div>
  </div>
  <div class="list">
    {#each tree as node}
      {@render folderNode({ node, level: 0 })}
    {/each}
  </div>

  <div class="header filters-header">
    <div class="header-left">
      <span>{i18n.t("filters")}</span>
    </div>
  </div>
  <div class="filters-list">
    {#if appState.filters.cameras.length > 0}
      <div class="filter-group">
        <div class="filter-title">{i18n.t("camera")}</div>
        {#each appState.filters.cameras as camera}
          <button 
            class="filter-btn" 
            class:active={appState.activeFilters.camera === camera}
            onclick={() => appState.setFilter('camera', camera)}
          >
            {cameraLabel(camera)}
          </button>
        {/each}
      </div>
    {/if}

    {#if appState.filters.isos.length > 0}
      <div class="filter-group">
        <div class="filter-title">{i18n.t('iso_label')}</div>
        <div class="filter-chips">
          {#each appState.filters.isos as iso}
            <button 
              class="chip" 
              class:active={appState.activeFilters.iso === iso}
              onclick={() => appState.setFilter('iso', iso)}
            >
              {iso}
            </button>
          {/each}
        </div>
      </div>
    {/if}

    {#if appState.activeFilters.camera || appState.activeFilters.iso}
      <button class="clear-filters" onclick={() => appState.clearFilters()}>
        {i18n.t("clear_filters")}
      </button>
    {/if}
  </div>
</aside>

<style>
  aside {
    width: 280px;
    height: 100%;
    background:
      linear-gradient(180deg, rgba(var(--accent-rgb), 0.08), transparent 24%),
      var(--bg-surface);
    border-right: 1px solid var(--border-main);
    display: flex;
    flex-direction: column;
    position: relative;
    z-index: var(--z-raised);
    margin-left: -280px;
    transition: margin-left var(--transition-base) ease-in-out;
    box-shadow: 8px 0 26px rgba(0, 0, 0, 0.15);
  }
  aside.open {
    margin-left: 0;
  }
  .header {
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--border-main);
    font-size: var(--text-xs);
    color: var(--text-muted);
    text-transform: uppercase;
    font-weight: 800;
    letter-spacing: 0.12em;
    display: flex;
    justify-content: space-between;
    align-items: center;
    height: 48px;
    flex-shrink: 0;
  }
  .filters-header {
    border-top: 1px solid var(--border-main);
    margin-top: auto;
  }
  .header-left {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .total-count {
    background: var(--key-bg);
    padding: 2px var(--space-1-5);
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
    color: var(--accent);
  }
  .header-actions {
    display: flex;
    gap: var(--space-1);
  }
  .icon-btn-small {
    background: transparent;
    border: 1px solid transparent;
    color: var(--text-muted);
    padding: var(--space-1);
    border-radius: var(--radius-sm);
    cursor: pointer;
    font-size: var(--text-base);
    line-height: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: all var(--transition-base);
  }
  .icon-btn-small:hover {
    background: var(--key-bg);
    color: var(--text-main);
    border-color: var(--border-main);
  }
  .list {
    flex: 1;
    overflow-y: auto;
    padding: var(--space-2) 0;
  }

  .filters-list {
    max-height: 40%;
    overflow-y: auto;
    padding: var(--space-3);
    background: color-mix(in srgb, var(--bg-inset) 82%, transparent);
  }

  .filter-group {
    margin-bottom: var(--space-4);
  }

  .filter-title {
    font-size: var(--text-xs);
    color: var(--text-muted);
    margin-bottom: var(--space-2);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .filter-btn {
    display: block;
    width: 100%;
    text-align: left;
    background: transparent;
    border: none;
    color: var(--text-main);
    padding: var(--space-1) var(--space-2);
    font-size: var(--text-caption);
    border-radius: var(--radius-sm);
    cursor: pointer;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .filter-btn:hover {
    background: var(--bg-hover-subtle);
  }

  .filter-btn.active {
    background: var(--accent-dim);
    color: var(--accent);
  }

  .filter-chips {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }

  .chip {
    background: var(--key-bg);
    border: 1px solid var(--border-main);
    color: var(--text-muted);
    padding: 2px var(--space-2);
    border-radius: var(--radius-xl);
    font-size: var(--text-xs);
    cursor: pointer;
  }

  .chip:hover {
    border-color: var(--accent);
    color: var(--text-main);
  }

  .chip.active {
    background: var(--accent);
    color: var(--on-accent);
    border-color: var(--accent);
    font-weight: bold;
  }

  .clear-filters {
    width: 100%;
    background: transparent;
    border: 1px dashed var(--border-main);
    color: var(--text-muted);
    font-size: var(--text-sm);
    padding: var(--space-1-5);
    border-radius: var(--radius-sm);
    cursor: pointer;
    margin-top: var(--space-2);
  }

  .clear-filters:hover {
    border-color: var(--danger);
    color: var(--danger);
  }

  .row {
    display: flex;
    align-items: center;
    width: 100%;
  }

  .toggle {
    background: transparent;
    border: none;
    color: var(--text-muted);
    font-size: var(--text-xs);
    width: 20px;
    height: 28px;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    flex-shrink: 0;
  }
  .toggle:hover {
    color: var(--text-main);
  }

  .spacer {
    width: 20px;
    flex-shrink: 0;
  }

  .name-btn {
    flex: 1;
    display: flex;
    align-items: center;
    padding: var(--space-1-5) var(--space-3) var(--space-1-5) var(--space-1);
    background: transparent;
    border: none;
    border-radius: var(--radius-sm);
    color: var(--text-main);
    cursor: default;
    text-align: left;
    gap: var(--space-2);
    min-width: 0;
  }

  .name-btn.clickable {
    cursor: pointer;
  }
  .name-btn.clickable:hover {
    background: var(--bg-hover-subtle);
  }

  .name-btn:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: -2px;
    background: var(--accent-dim);
  }

  .name-btn.active {
    background: var(--accent-dim);
    color: var(--accent);
  }

  .label {
    flex: 1;
    font-size: var(--text-base);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .count {
    font-size: var(--text-xs);
    color: var(--text-muted);
    background: var(--key-bg);
    padding: 2px var(--space-1-5);
    border-radius: var(--radius-lg);
  }
  .name-btn.active .count {
    background: var(--accent);
    color: var(--on-star);
    font-weight: bold;
  }
</style>
