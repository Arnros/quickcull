<script lang="ts">
  import { appState } from "../appState.svelte";
  import { i18n } from "../i18n.svelte";
  import Logo from "./Logo.svelte";
  import Icon from "./Icon.svelte";
  import Button from "./Button.svelte";
  import Kbd from "./Kbd.svelte";
  import { APP_NAME } from "../constants";
  import { resolveContextShortcutHints } from "../shortcutContext";

  let dateFromInput = $state("");
  let dateToInput = $state("");
  let sizeMinInput = $state("");
  let sizeMaxInput = $state("");

  let hasCustomFilters = $derived(Object.keys(appState.activeFilters || {}).length > 0);
  let contextHints = $derived(resolveContextShortcutHints({
    view: appState.view,
    filterBarOpen: appState.filterBarOpen,
  }));

  $effect(() => {
    dateFromInput = appState.activeFilters?.dateFrom || "";
    dateToInput = appState.activeFilters?.dateTo || "";
    sizeMinInput = appState.activeFilters?.sizeMin || "";
    sizeMaxInput = appState.activeFilters?.sizeMax || "";
  });

  function clearAllFilters() {
    appState.clearAllFilters();
  }
</script>

<div class="topbar-shell">
  <div class="row row-main">
    <div class="left">
      <div class="brand">
        <span class="logo-icon"><Logo size="18px" /></span>
        <span class="brand-text">{APP_NAME}</span>
      </div>

      <Button
        variant="ghost"
        icon="folder"
        onclick={() => (appState.view = "picker")}
        title={i18n.t("open_another")}
      >
        {i18n.t("folder")}
      </Button>
    </div>

    <div class="center">
      {#if appState.isScanning || appState.analysis.current < appState.analysis.total}
        <div class="analysis-pill" title={i18n.t('background_analysis')}>
          <div class="spinner"></div>
          {#if appState.isScanning}
            <span class="pct">{i18n.t('scanning')}</span>
          {:else}
            <span class="pct">{Math.round((appState.analysis.current / appState.analysis.total) * 100)}%</span>
          {/if}
          <span class="counts">{appState.analysis.current}/{appState.analysis.total}</span>
        </div>
      {/if}

      <div class="counter">
        <span class="current">{appState.currentIndex + 1}</span>
        <span>/</span>
        <span>{appState.stats?.total}</span>
      </div>
      <div class="session-stats">
        <span class="stat star" title={i18n.t("starred")}>
          <Icon name="star" size={13} /> {appState.stats?.starredCount}
        </span>
        {#if appState.stats?.labeledCount}
          <span class="stat label" title={i18n.t("labeled")}>🏷 {appState.stats.labeledCount}</span>
        {/if}
        <span class="stat trash" title={i18n.t("trashed")}>
          <Icon name="trash" size={13} /> {appState.stats?.trashedCount}
        </span>
      </div>
      {#if appState.actionTrail.length > 0}
        <div class="action-trail" aria-label={i18n.t("recent_actions")}>
          {#each appState.actionTrail as item}
            <span class="trail-item" class:non-undoable={!item.undoable} title={item.undoable ? i18n.t("undoable") : i18n.t("non_undoable")}>
              {item.label}
            </span>
          {/each}
        </div>
      {/if}
    </div>

    <div class="right">
      <Button variant="ghost" icon="eye" onclick={() => (appState.zenMode = !appState.zenMode)} title={`${i18n.t("zen")} (H)`} ariaLabel={i18n.t("zen")} />
      <Button variant="ghost" icon="refresh" onclick={() => appState.refresh()} title={`${i18n.t("refresh")} (F5)`} ariaLabel={i18n.t("refresh")} />
      <Button 
        variant="ghost" 
        icon={appState.sortOrder === 'name' ? 'clock' : 'sort'} 
        onclick={() => appState.setSortOrder(appState.sortOrder === 'name' ? 'date' : 'name')} 
        title={`${i18n.t("sort")} (${appState.sortOrder === 'name' ? 'Date' : 'Name'})`} 
        ariaLabel={i18n.t("sort")}
      />
      <Button variant="ghost" icon={appState.config?.theme === "light" ? "moon" : "sun"} onclick={() => appState.toggleTheme()} title={i18n.t('toggle_theme')} ariaLabel={i18n.t('toggle_theme_desc')} />
      <Button variant="ghost" icon="settings" onclick={() => (appState.settingsOpen = !appState.settingsOpen)} title={i18n.t("settings")} />
      <Button variant="ghost" icon="help" onclick={() => (appState.helpOpen = !appState.helpOpen)} title={`${i18n.t("help")} (?)`} />
    </div>
  </div>

  <div class="row row-tools">
    <div class="left tools-wrap">
      <Button variant="secondary" icon="folder" active={appState.sidebarOpen} onclick={() => (appState.sidebarOpen = !appState.sidebarOpen)}>
        <Kbd key="Tab" variant="outline" />
        {i18n.t("folders")}
      </Button>
      <Button variant="secondary" icon="film" active={appState.filmstripOpen} onclick={() => (appState.filmstripOpen = !appState.filmstripOpen)}>
        <Kbd key="G" variant="outline" />
        {i18n.t("filmstrip")}
      </Button>
      <Button variant="secondary" icon="grid" active={appState.gridOpen} onclick={() => (appState.gridOpen = !appState.gridOpen)}>
        <Kbd key="V" variant="outline" />
        {i18n.t("grid")}
      </Button>
      <Button variant="secondary" icon="info" active={appState.infoOpen} onclick={() => (appState.infoOpen = !appState.infoOpen)}>
        <Kbd key="I" variant="outline" />
        {i18n.t("info")}
      </Button>
      <Button variant="secondary" icon="compare" active={appState.comparisonMode} onclick={() => appState.toggleComparisonMode()} title={`${i18n.t("comparison")} (C)`}>
        <Kbd key="C" variant="outline" />
        {i18n.t("comparison")}
      </Button>
      <Button
        variant="secondary"
        icon="compare"
        active={appState.filterMode === "duplicates"}
        onclick={() => appState.toggleDuplicatesFilter()}
        title={i18n.t("show_duplicates")}
      >
        {i18n.t("duplicates")}
      </Button>
      <Button
        variant="ghost"
        icon="filter"
        active={appState.filterBarOpen}
        class={appState.filterMode !== "none" || hasCustomFilters ? "filter-active" : ""}
        onclick={() => {
          appState.filterBarOpen = !appState.filterBarOpen;
          if (appState.filterBarOpen) {
            appState.loadFilters();
          }
        }}
        title={i18n.t("filters")}
      >
        <Kbd key="F" variant="outline" />
        {i18n.t("filters")}
        {#if appState.filterMode !== "none" || hasCustomFilters}
          <span class="active-dot"></span>
        {/if}
      </Button>

      {#if appState.currentFile}
        <Button
          variant="secondary"
          icon="check"
          disabled={!appState.canApplyRotation()}
          class={appState.canApplyRotation() ? "apply-active" : ""}
          onclick={() => appState.applyRotation()}
          title={i18n.t("apply")}
        >
          <Kbd key="W" variant="outline" />
          {i18n.t("apply")}
        </Button>
        <Button 
          variant="secondary" 
          icon="refresh" 
          disabled={!appState.hasPendingRotation()} 
          onclick={() => appState.rotateReset()}
          title={i18n.t("reset")}
        >
          <Kbd key="O" variant="outline" />
          {i18n.t("reset")}
        </Button>
      {/if}
    </div>
    <div class="right context-hints" aria-label={i18n.t('shortcut_hint_title')}>
      <span class="hint-chip"><Kbd key="S" variant="outline" /> {i18n.t(contextHints.starHintKey as any)}</span>
      <span class="hint-chip"><Kbd key={`0-${appState.maxLabel}`} variant="outline" /> {i18n.t(contextHints.labelHintKey as any)}</span>
    </div>
  </div>
</div>

{#if appState.filterBarOpen}
  <div class="filter-panel">
    <div class="filter-section">
      <div class="filter-label">{i18n.t("filters")}:</div>
      <div class="filter-options">
        <Button 
          variant="mini" 
          icon="star" 
          active={appState.filterMode === "starred"} 
          onclick={() => appState.toggleStarFilter()}
        >
          <Kbd key="S" variant="outline" />
          {i18n.t("starred")}
        </Button>

        <div class="v-divider"></div>

        {#each Array.from({ length: appState.maxLabel }, (_, i) => i + 1) as i}
          <Button
            variant="mini"
            class="label-{i}"
            active={appState.filterMode === "label" && appState.activeLabelFilter === i}
            onclick={() => appState.setLabelFilter(i)}
          >
            <Kbd key={i.toString()} variant="outline" />
            <div class="dot" style="background-color: var(--label-{i})"></div>
          </Button>
        {/each}

        <Button 
          variant="mini" 
          active={appState.filterMode === "label" && appState.activeLabelFilter === 0} 
          onclick={() => appState.setLabelFilter(0)} 
          title={i18n.t("labeled")}
        >
          <Kbd key="0" variant="outline" /> {i18n.t("labeled")}
        </Button>

      {#if appState.filterMode !== 'none'}
        <div class="v-divider"></div>
        <Button
            variant="mini"
            icon="export"
            disabled={appState.isExporting}
            onclick={() => appState.exportSelection(appState.filterMode as any, appState.activeLabelFilter)}
            title={i18n.t("export_selection")}
          >
            {i18n.t("export_selection")}
          </Button>
        {/if}
      </div>

      <button class="clear-all" onclick={clearAllFilters} aria-label={i18n.t("clear_filters")}>✕</button>
    </div>

    <div class="filter-section custom-filter-section">
      <div class="filter-label">{i18n.t("custom_filters")}:</div>
      <div class="custom-fields">
        <label class="cf-field">
          <span>{i18n.t("date_from")}</span>
          <input
            type="date"
            bind:value={dateFromInput}
            onchange={(e) => appState.setAdvancedFilter('dateFrom', (e.currentTarget as HTMLInputElement).value)}
          />
        </label>
        <label class="cf-field">
          <span>{i18n.t("date_to")}</span>
          <input
            type="date"
            bind:value={dateToInput}
            onchange={(e) => appState.setAdvancedFilter('dateTo', (e.currentTarget as HTMLInputElement).value)}
          />
        </label>
        <label class="cf-field">
          <span>{i18n.t("size_min_mb")}</span>
          <input
            type="number"
            min="0"
            step="0.1"
            inputmode="decimal"
            bind:value={sizeMinInput}
            placeholder="0"
            onchange={(e) => appState.setAdvancedFilter('sizeMin', (e.currentTarget as HTMLInputElement).value)}
          />
        </label>
        <label class="cf-field">
          <span>{i18n.t("size_max_mb")}</span>
          <input
            type="number"
            min="0"
            step="0.1"
            inputmode="decimal"
            bind:value={sizeMaxInput}
            placeholder="100"
            onchange={(e) => appState.setAdvancedFilter('sizeMax', (e.currentTarget as HTMLInputElement).value)}
          />
        </label>
      </div>
    </div>
  </div>
{/if}

<style>
  .topbar-shell {
    width: 100%;
    border-bottom: 1px solid var(--topbar-border);
    background:
      linear-gradient(180deg, rgba(var(--accent-rgb), 0.1), transparent 42%),
      var(--topbar-bg);
    backdrop-filter: var(--blur-md);
    -webkit-backdrop-filter: var(--blur-md);
    position: relative;
    z-index: var(--z-bar);
    box-shadow: 0 10px 24px rgba(0, 0, 0, 0.14);
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2-5);
    padding: var(--space-2) var(--space-3);
    overflow-x: auto;
  }

  .row-main {
    border-bottom: 1px solid var(--border-main);
    min-height: 52px;
  }

  .row-tools {
    min-height: 46px;
  }
  .context-hints {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-left: auto;
    flex-wrap: wrap;
    justify-content: flex-end;
  }
  .hint-chip {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1-5);
    padding: 4px 8px;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border-main);
    background: var(--bg-surface);
    color: var(--text-muted);
    font-size: var(--text-xs);
    white-space: nowrap;
  }

  .left,
  .center,
  .right,
  .tools-wrap {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: max-content;
  }

  .brand {
    font-size: var(--text-lg);
    font-weight: 800;
    color: var(--text-muted);
    user-select: none;
    letter-spacing: 0.01em;
    display: flex;
    align-items: center;
    gap: var(--space-1-5);
    padding-right: var(--space-2-5);
    margin-right: var(--space-1-5);
    border-right: 1px solid var(--border-main);
  }

  .brand .logo-icon {
    color: var(--accent);
    display: flex;
    align-items: center;
  }

  :global(.topbar-shell .btn) {
    height: 32px;
  }

  :global(.topbar-shell .btn.icon) {
    width: 32px;
    padding: 0;
  }

  :global(.topbar-shell .btn.apply-active) {
    border-color: var(--accent);
    color: var(--accent);
    background: var(--accent-dim);
  }

  :global(.topbar-shell .btn.filter-active) {
    border-color: var(--accent);
  }

  .active-dot {
    width: 6px;
    height: 6px;
    background: var(--accent);
    border-radius: 50%;
    position: absolute;
    top: 4px;
    right: 4px;
    box-shadow: 0 0 10px rgba(var(--accent-rgb), 0.8);
  }

  .counter {
    font-size: var(--text-base);
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
    font-family: var(--mono);
    min-width: 90px;
    text-align: center;
    background: color-mix(in srgb, var(--key-bg) 92%, transparent);
    padding: var(--space-1) var(--space-2-5);
    border-radius: var(--radius-md);
    border: 1px solid var(--border-main);
    display: flex;
    gap: var(--space-1-5);
    justify-content: center;
  }

  .counter .current {
    color: var(--text-main);
    font-weight: 700;
  }

  .analysis-pill {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    background: var(--accent-dim);
    padding: var(--space-1) var(--space-2-5);
    border-radius: var(--radius-full);
    border: 1px solid rgba(var(--accent-rgb), 0.7);
    font-family: var(--mono);
    font-size: var(--text-xs);
    box-shadow: 0 0 10px rgba(var(--accent-rgb), 0.15);
    animation: pulse 2.5s infinite ease-in-out;
  }

  @keyframes pulse {
    0% { opacity: 0.85; transform: scale(0.98); }
    50% { opacity: 1; transform: scale(1); }
    100% { opacity: 0.85; transform: scale(0.98); }
  }

  .analysis-pill .spinner {
    width: 10px;
    height: 10px;
    border: 1.5px solid var(--accent);
    border-top-color: transparent;
    border-radius: var(--radius-full);
    animation: spin 1s linear infinite;
  }

  .analysis-pill .pct {
    font-weight: 700;
    color: var(--accent);
  }

  .analysis-pill .counts {
    color: var(--text-muted);
    font-size: 10px;
  }

  .session-stats {
    display: flex;
    gap: var(--space-2-5);
    font-size: var(--text-caption);
    font-weight: 600;
  }

  .action-trail {
    display: flex;
    gap: var(--space-1-5);
    margin-top: var(--space-1-5);
    max-width: 560px;
    overflow-x: auto;
    padding-bottom: 2px;
  }

  .trail-item {
    font-size: var(--text-sm);
    color: var(--text-muted);
    background: color-mix(in srgb, var(--key-bg) 90%, transparent);
    border: 1px solid var(--border-main);
    border-radius: var(--radius-full);
    padding: 2px var(--space-2);
    white-space: nowrap;
  }

  .trail-item.non-undoable {
    border-color: rgba(var(--accent-rgb), 0.45);
    color: var(--accent);
  }

  .stat {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
  }

  .stat.star { color: var(--star); }
  .stat.trash { color: var(--danger); }
  .stat.label { color: var(--text-muted); }

  .filter-panel {
    position: relative;
    width: 100%;
    background: linear-gradient(180deg, var(--bg-surface), var(--bg-app));
    backdrop-filter: var(--blur-lg);
    border-top: 1px solid rgba(var(--accent-rgb), 0.35);
    border-bottom: 1px solid var(--border-main);
    z-index: var(--z-dropdown);
    display: flex;
    flex-direction: column;
    animation: slide-down 0.16s ease;
  }

  .filter-section {
    display: flex;
    align-items: center;
    min-height: 44px;
    padding: 6px var(--space-4);
    gap: var(--space-4);
  }

  .custom-filter-section {
    border-top: 1px dashed var(--border-main);
    gap: var(--space-3);
  }

  .custom-fields {
    display: flex;
    flex-wrap: wrap;
    align-items: end;
    gap: 8px 10px;
    flex: 1;
  }

  .cf-field {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 130px;
  }

  .cf-field span {
    font-size: var(--text-sm);
    color: var(--text-muted);
    font-weight: 600;
  }

  .cf-field input {
    background: var(--bg-app);
    border: 1px solid var(--border-main);
    color: var(--text-main);
    border-radius: var(--radius-md);
    height: 30px;
    padding: 0 var(--space-2);
    font-size: var(--text-caption);
    outline: none;
  }

  .cf-field input:focus {
    border-color: var(--accent);
  }

  @keyframes slide-down {
    from {
      transform: translateY(-6px);
      opacity: 0;
    }
    to {
      transform: translateY(0);
      opacity: 1;
    }
  }

  .filter-label {
    font-size: var(--text-sm);
    font-weight: 700;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .filter-options {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex: 1;
    min-width: 0;
    overflow-x: auto;
  }

  .dot {
    width: 10px;
    height: 10px;
    border-radius: var(--radius-full);
    border: 1px solid var(--dot-border);
  }

  .v-divider {
    width: 1px;
    height: 16px;
    background: var(--border-main);
    margin: 0 2px;
  }

  .clear-all {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    font-size: var(--text-lg);
    padding: var(--space-1);
  }

  .clear-all:hover {
    color: var(--danger);
  }

  @media (max-width: 1100px) {
    .row {
      padding: 8px;
    }

    .brand {
      font-size: var(--text-base);
      padding-right: var(--space-2);
      margin-right: 2px;
    }

    .filter-section {
      flex-wrap: wrap;
      align-items: flex-start;
      padding: var(--space-2) var(--space-3);
      gap: var(--space-2);
    }

    .clear-all {
      margin-left: auto;
    }
  }
</style>
