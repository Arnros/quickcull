<script lang="ts">
  import { appState } from "../appState.svelte";
  import { cameraLabel } from "../cameraLabel";
  import { i18n } from "../i18n.svelte";
  import { logger } from "../logger";
  import Sidebar from "./Sidebar.svelte";
  import Filmstrip from "./Filmstrip.svelte";
  import Grid from "./Grid.svelte";
  import Settings from "./Settings.svelte";
  import Help from "./Help.svelte";
  import ShortcutOnboarding from "./ShortcutOnboarding.svelte";

  import Topbar from "./Topbar.svelte";
  import Bottombar from "./Bottombar.svelte";
  import { resolveShortcutContext } from "../shortcutContext";

  let mediaRetry = $state(0);
  let mediaUrl = $derived(
    appState.currentFile
      ? `${appState.getFullUrl(appState.currentFile.index)}&tx=${appState.currentFile.txID}&r=${mediaRetry}`
      : ''
  );
  $effect(() => {
    // Reset retry counter when the selected file changes.
    appState.currentFile?.filename;
    mediaRetry = 0;
  });
  let shortcutCtx = $derived(resolveShortcutContext({
    view: appState.view,
    gridOpen: appState.gridOpen,
    filterBarOpen: appState.filterBarOpen,
    comparisonMode: appState.comparisonMode,
    filterMode: appState.filterMode as 'none' | 'starred' | 'label' | 'duplicates',
    helpOpen: appState.helpOpen,
    settingsOpen: appState.settingsOpen,
  }));
</script>

<div class="review-layout" class:zen={appState.zenMode} class:grid-mode={appState.gridOpen}>
  {#if !appState.gridOpen && !appState.comparisonMode}
    <Topbar />

    <button
      class="progress-bar"
      aria-label={i18n.t('progress_label')}
      onclick={(e) => {
        const rect = e.currentTarget.getBoundingClientRect();
        const x = e.clientX - rect.left;
        const pct = x / rect.width;
        appState.select(Math.floor(pct * (appState.stats.total || 1)));
      }}
    >
      <div
        class="progress-fill"
        style="width: {((appState.currentIndex + 1) /
          (appState.stats.total || 1)) *
          100}%"
      ></div>
    </button>
  {/if}

  {#if !appState.gridOpen && appState.comparisonMode}
    <Topbar />
  {/if}

  <div class="main-container">
    <Sidebar open={appState.sidebarOpen && !appState.zenMode} />

    <div
      class="viewer"
      class:zoomed={appState.zoomed}
      class:split={appState.comparisonMode}
    >
      {#if appState.gridOpen}
        <Grid />
      {/if}

      {#if !appState.gridOpen}
        {#if appState.loading}
          <div class="spinner"></div>
        {/if}

        <div class="action-flash" class:visible={appState.loading}></div>

        {#if appState.comparisonMode && appState.stats && appState.stats.total > 1}
          {@const leftIdx = appState.comparisonIndex}
          {@const rightIdx = appState.currentIndex}
          
          <button 
            class="comparison-pane" 
            class:selected={appState.selectedIndices.includes(leftIdx)}
            class:active-compare={false}
            onclick={(e) => appState.select(leftIdx, e.ctrlKey || e.metaKey, e.shiftKey)}
            title={`${i18n.t('reference')} - ${i18n.t('click_to_select_target')}`}
          >
            {#if appState.selectedIndices.includes(leftIdx)}
              <div class="selection-badge">✓</div>
              <div class="target-pill">{i18n.t('target_desc')}</div>
            {/if}
            <div class="img-zoom-wrapper">
              <img
                src={appState.getFullUrl(leftIdx)}
                alt="comparison-reference"
                class="comparison-img loaded"
                style="transform: rotate({appState.referenceFile?.rotation || 0}deg)"
              />
            </div>
            <div class="comparison-label">
              {i18n.t('reference')}
            </div>
          </button>

          <button 
            class="main-pane" 
            class:selected={appState.selectedIndices.includes(rightIdx)}
            class:active-compare={true}
            class:zoomed={appState.zoomed}
            onclick={(e) => appState.select(rightIdx, e.ctrlKey || e.metaKey, e.shiftKey)}
            title={`${i18n.t('active')} - ${i18n.t('click_to_select_target')}`}
          >
            {#if appState.selectedIndices.includes(rightIdx)}
              <div class="selection-badge">✓</div>
              <div class="target-pill">{i18n.t('target_desc')}</div>
            {/if}
            <div class="img-zoom-wrapper" class:zoom-cursor={!appState.zoomed}>
              <img
                src={appState.getFullUrl(rightIdx)}
                alt="comparison-active"
                class="comparison-img loaded"
                style="transform: rotate({appState.currentFile?.rotation || 0}deg)"
              />
            </div>
            <div class="comparison-label active">
              {i18n.t('active')}
            </div>
          </button>
        {:else}
          <button 
            class="main-pane" 
            onclick={() => (appState.zoomed = !appState.zoomed)}
          >
            {#if appState.currentFile}
              {#if appState.currentFile.starred}
                <div class="favorite-chip">★ {i18n.t('favorite_badge')}</div>
              {/if}
              
              {#if appState.currentFile.label}
              <div class="label-badge" style="background-color: var(--label-{appState.currentFile.label})"></div>
              {/if}

              {#if appState.currentFile.type === "image"}
                <div
                  class="img-zoom-wrapper"
                  class:zoom-cursor={!appState.zoomed}
                >
                  <img
                    src={mediaUrl}
                    alt={appState.currentFile.filename}
                    class:loaded={!appState.loading}
                    style="transform: rotate({appState.currentFile.rotation}deg)"
                    onerror={() => {
                      if (mediaRetry < 1) {
                        mediaRetry += 1;
                        return;
                      }
                      logger.error("Main image failed to load after retry", {
                        index: appState.currentFile?.index,
                        filename: appState.currentFile?.filename,
                        mediaUrl
                      });
                    }}
                  />
                </div>
              {/if}
            {/if}
          </button>
        {/if}

        {#if appState.infoOpen && !appState.zenMode}
          {#if appState.comparisonMode && appState.referenceFile && appState.currentFile}
            <div class="info-overlay comparison-info-left">
              {@render infoContent(appState.referenceFile, appState.currentFile)}
            </div>
            <div class="info-overlay comparison-info-right">
              {@render infoContent(appState.currentFile, appState.referenceFile)}
            </div>
          {:else if appState.currentFile}
            <div class="info-overlay">
              {@render infoContent(appState.currentFile)}
            </div>
          {/if}
        {/if}
        
        <!-- Smart Prefetch: Preload next 2 images -->
        {#if appState.stats && appState.stats.total > 0 && appState.currentFile && appState.currentFile.type === 'image' && appState.v2}
          {@const next1Idx = (appState.currentIndex + 1) % appState.stats.total}
          {@const next2Idx = (appState.currentIndex + 2) % appState.stats.total}

          <img class="prefetch" src={appState.getFullUrl(next1Idx)} alt="prefetch-1" />
          {#if appState.stats.total > 2}
            <img class="prefetch" src={appState.getFullUrl(next2Idx)} alt="prefetch-2" />
          {/if}
        {/if}
      {/if}
    </div>
  </div>

  {#if !appState.gridOpen && appState.filmstripOpen && !appState.zenMode}
    <Filmstrip />
  {/if}

  {#if shortcutCtx.showBottomBar}
    <Bottombar />
  {/if}

  {#if appState.zenMode}
    <div class="zen-hint">{i18n.t('zen_hint')}</div>
  {/if}
  

  {#if appState.settingsOpen}
    <Settings />
  {/if}

  {#if appState.helpOpen}
    <Help />
  {/if}

  <ShortcutOnboarding />
</div>

{#snippet infoContent(f: import("../../../wailsjs/go/models").review.FileResponse, other?: import("../../../wailsjs/go/models").review.FileResponse | null)}
  <div class="info-actions">
    <button class="info-action-btn" onclick={() => appState.reanalyzeMetadata()}>
      {i18n.t("reanalyze_metadata")}
    </button>
  </div>
  {#if f.camera}
    <div class="info-row">
      <span class="label">{i18n.t("camera")}:</span>
      <span class="val" class:diff={other && f.camera !== other.camera}>{cameraLabel(f.camera)}</span>
    </div>
  {/if}
  {#if f.iso || f.aperture || f.shutter}
    <div class="info-row">
      <span class="label">{i18n.t('exif_label')}</span>
      <span class="val">
        <span class:diff={other && f.iso !== other.iso}>{f.iso ? `ISO ${f.iso}` : ''}</span>
        <span class:diff={other && f.aperture !== other.aperture}>{f.aperture || ''}</span>
        <span class:diff={other && f.shutter !== other.shutter}>{f.shutter || ''}</span>
        <span class:diff={other && f.focal !== other.focal}>{f.focal || ''}</span>
      </span>
    </div>
  {/if}
  {#if f.date}
    <div class="info-row">
      <span class="label">{i18n.t('date_label')}</span>
      <span class="val" class:diff={other && f.date !== other.date}>{f.date}</span>
    </div>
  {/if}
  <div class="divider-small"></div>
  <div class="info-row">
    <span class="label">{i18n.t("file")}:</span>
    <span class="val">{f.filename}</span>
  </div>
  <div class="info-row">
    <span class="label">{i18n.t("format")}:</span>
    <span class="val">{f.format}</span>
  </div>
  {#if f.width && f.height}
    <div class="info-row">
      <span class="label">{i18n.t("resolution")}:</span>
      <span class="val" class:diff={other && (f.width !== other.width || f.height !== other.height)}>
        {f.width} × {f.height} px
      </span>
    </div>
  {/if}
  {#if f.size}
    <div class="info-row">
      <span class="label">{i18n.t("size")}:</span>
      <span class="val" class:diff={other && f.size !== other.size}>
        {(f.size / 1024 / 1024).toFixed(2)} {i18n.t('unit_mb')}
      </span>
    </div>
  {/if}
  {#if f.similarity !== undefined && f.similarity >= 0}
    <div class="info-row">
      <span class="label">{i18n.t("similarity")}:</span>
      <span class="val">{f.similarity.toFixed(1)}%</span>
    </div>
  {/if}
{/snippet}

<style>
  .review-layout {
    display: flex;
    flex-direction: column;
    height: 100vh;
    width: 100%;
    background:
      radial-gradient(circle at 8% 6%, rgba(var(--accent-rgb), 0.06), transparent 34%),
      var(--bg-app);
    color: var(--text-main);
    font-family: var(--sans);
    position: relative;
  }

  :global(.review-layout.zen .topbar),
  .review-layout.zen .progress-bar,
  :global(.review-layout.zen .bottombar) {
    display: none;
  }
  .review-layout.zen .viewer {
    background: black;
  }

  .zen-hint {
    position: fixed;
    bottom: var(--space-4);
    left: 50%;
    transform: translateX(-50%);
    background: var(--overlay-tinted);
    color: var(--text-muted);
    font-size: var(--text-sm);
    font-family: var(--mono);
    padding: 5px var(--space-3);
    border-radius: var(--radius-full);
    border: 1px solid rgba(255, 255, 255, 0.1);
    pointer-events: none;
    opacity: 0;
    transition: opacity var(--transition-slow) ease;
    z-index: var(--z-raised);
    white-space: nowrap;
  }
  .review-layout.zen:hover .zen-hint {
    opacity: 1;
  }

  .progress-bar {
    display: block;
    width: 100%;
    padding: 0;
    border: none;
    height: 4px;
    background: color-mix(in srgb, var(--border-main) 78%, transparent);
    cursor: pointer;
    z-index: 100;
    flex-shrink: 0;
  }
  .progress-bar:hover {
    height: 7px;
  }
  .progress-fill {
    height: 100%;
    background: linear-gradient(90deg, color-mix(in srgb, var(--accent) 82%, #fff), var(--accent));
    transition: width var(--transition-base);
  }

  .main-container {
    flex: 1;
    display: flex;
    overflow: hidden;
    position: relative;
  }

  .viewer {
    flex: 1;
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--viewer-bg);
    overflow: hidden;
    gap: 3px;
  }
  .viewer.zoomed {
    overflow: auto;
    display: block;
  }
  .viewer.split {
    display: grid;
    grid-template-columns: 1fr 1fr;
  }

  .comparison-pane,
  .main-pane {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
    width: 100%;
    overflow: hidden;
    background: none;
    border: none;
    padding: 0;
    cursor: default;
  }
  .comparison-pane {
    cursor: pointer;
    border-right: 1px solid var(--border-main);
  }
  .comparison-pane.selected,
  .main-pane.selected {
    background: rgba(var(--accent-rgb), 0.08);
  }
  .comparison-pane.selected img,
  .main-pane.selected img {
    outline: 4px solid var(--accent);
    outline-offset: -4px;
  }
  .active-compare {
    z-index: 10;
  }
  .active-compare img {
    outline: 2px solid var(--accent);
    outline-offset: -2px;
  }
  .comparison-img,
  .main-pane img {
    width: 100%;
    height: 100%;
    object-fit: contain;
    opacity: 0;
    transition: opacity var(--transition-base) ease-in-out;
    /* Trigger GPU acceleration */
    will-change: transform;
    backface-visibility: hidden;
    transform-style: preserve-3d;
  }
  .loaded {
    opacity: 1 !important;
  }

  .viewer.zoomed .main-pane:not(.comparison-mode) img,
  .main-pane.zoomed img {
    max-width: none;
    max-height: none;
    object-fit: none;
  }
  .viewer.zoomed,
  .main-pane.zoomed {
    overflow: auto;
  }
  .zoom-cursor {
    cursor: zoom-in;
  }
  .zoomed .img-zoom-wrapper {
    cursor: zoom-out;
  }

  .img-zoom-wrapper {
    background: none;
    border: none;
    padding: 0;
    outline: none;
    margin: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 100%;
    height: 100%;
  }

  .comparison-label {
    position: absolute;
    top: 10px;
    left: 10px;
    background: var(--shadow-overlay);
    color: #fff;
    padding: var(--space-1) var(--space-2-5);
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
    font-weight: bold;
    pointer-events: none;
    z-index: 20;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    border: 1px solid rgba(255, 255, 255, 0.15);
  }
  .comparison-label.active {
    background: var(--accent);
    color: var(--on-accent);
  }

  .selection-badge {
    position: absolute;
    top: 10px;
    right: 10px;
    background: var(--accent);
    color: var(--on-accent);
    width: 24px;
    height: 24px;
    border-radius: var(--radius-full);
    display: flex;
    align-items: center;
    justify-content: center;
    font-weight: bold;
    font-size: var(--text-base);
    z-index: 30;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.5);
    border: 2px solid #fff;
  }
  .target-pill {
    position: absolute;
    top: 42px;
    right: 10px;
    background: rgba(var(--accent-rgb), 0.26);
    color: var(--accent);
    border: 1px solid rgba(var(--accent-rgb), 0.45);
    border-radius: var(--radius-full);
    padding: 4px 10px;
    font-size: var(--text-xs);
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    z-index: 30;
    pointer-events: none;
  }

  .action-flash {
    position: absolute;
    inset: 0;
    background: white;
    opacity: 0;
    pointer-events: none;
    z-index: 100;
    transition: opacity var(--transition-fast);
  }
  .action-flash.visible {
    opacity: 0.15;
  }

  .prefetch {
    position: absolute;
    width: 1px;
    height: 1px;
    visibility: hidden;
    pointer-events: none;
    z-index: var(--z-below);
  }

  .favorite-chip {
    position: absolute;
    top: var(--space-4);
    right: var(--space-4);
    height: 28px;
    padding: 0 var(--space-2-5);
    border-radius: var(--radius-full);
    display: flex;
    align-items: center;
    gap: var(--space-1-5);
    font-size: var(--text-caption);
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    background: var(--overlay-tinted);
    border: 1px solid rgba(255, 255, 255, 0.25);
    color: var(--star);
    z-index: 50;
    pointer-events: none;
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.35);
  }

  .label-badge {
    position: absolute;
    bottom: var(--space-6);
    right: var(--space-6);
    width: 24px;
    height: 24px;
    border-radius: var(--radius-full);
    z-index: 50;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.5);
    border: 2px solid rgba(255, 255, 255, 0.8);
    animation: pop-in 0.3s cubic-bezier(0.34, 1.56, 0.64, 1);
  }

  @keyframes pop-in {
    0% { transform: scale(0.5); opacity: 0; }
    100% { transform: scale(1); opacity: 1; }
  }

  .info-overlay {
    position: absolute;
    bottom: var(--space-5);
    left: var(--space-5);
    background:
      linear-gradient(165deg, rgba(var(--accent-rgb), 0.08), transparent 40%),
      var(--overlay-bg);
    border: 1px solid var(--border-main);
    border-radius: var(--radius-lg);
    padding: var(--space-4) var(--space-5);
    font-size: var(--text-base);
    color: var(--text-main);
    z-index: 60;
    backdrop-filter: var(--blur-sm);
    display: flex;
    flex-direction: column;
    gap: var(--space-1-5);
    min-width: 280px;
    box-shadow: 0 12px 28px var(--shadow-main);
  }

  .comparison-info-left {
    left: var(--space-5);
    right: auto;
  }

  .comparison-info-right {
    left: auto;
    right: var(--space-5);
  }

  .info-actions {
    display: flex;
    justify-content: flex-end;
    margin-bottom: 2px;
  }
  .info-action-btn {
    background: var(--key-bg);
    border: 1px solid var(--border-main);
    color: var(--text-muted);
    padding: var(--space-1) var(--space-2);
    border-radius: var(--radius-md);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .info-action-btn:hover {
    color: var(--text-main);
    border-color: rgba(var(--accent-rgb), 0.45);
  }
  .info-row {
    display: flex;
    gap: var(--space-4);
  }
  .info-row .label {
    color: var(--text-muted);
    min-width: 80px;
  }
  .info-row .val {
    word-break: break-all;
    font-weight: 500;
  }

  .info-row .val.diff,
  .info-row .val .diff {
    color: var(--accent);
    font-weight: 700;
  }

  .divider-small {
    height: 1px;
    background: var(--border-main);
    margin: 4px 0;
    opacity: 0.5;
  }

  .spinner {
    width: 40px;
    height: 40px;
    border: 4px solid var(--border-main);
    border-top-color: var(--accent);
    border-radius: var(--radius-full);
    animation: spin 0.8s linear infinite;
    position: absolute;
    z-index: 10;
  }
</style>
