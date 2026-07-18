<script lang="ts">
  import { appState } from '../appState.svelte';
  import { filterState } from '../filterState.svelte';
  import { viewState } from '../viewState.svelte';
  import { api } from '../api';
  import { i18n } from '../i18n.svelte';
  import { onMount } from 'svelte';
  import { trashService } from '../trashService.svelte';
  import { toastService } from '../toast.svelte';
  import Icon from './Icon.svelte';
  import { shortcutService } from '../shortcutService.svelte';
  import { watchService } from '../watchService.svelte';
  import { logger } from '../logger';
  import Modal from './Modal.svelte';
  import { localizeBackendError } from '../backendError';

  let sysInfo = $state<{ exiftool: boolean; os: string; arch: string; capabilities?: { rawPreview: boolean; rawMetadata: boolean; heicDecode: boolean; exifWrite: boolean } } | null>(null);
  let recordingAction = $state<string | null>(null);
  let activeTab = $state<'general' | 'shortcuts' | 'maintenance'>('general');
  let exifPathFeedback = $state<{ type: 'ok' | 'error'; text: string } | null>(null);
  let applyingExifPath = $state(false);

  // Virtual List state for Trash
  let trashViewport = $state<HTMLDivElement | null>(null);
  let trashScrollTop = $state(0);
  let trashViewportHeight = $state(0);
  const TRASH_ITEM_HEIGHT = 44;

  let visibleTrashItems = $derived.by(() => {
    if (!trashService.items.length || trashViewportHeight === 0) return [];
    const start = Math.max(0, Math.floor(trashScrollTop / TRASH_ITEM_HEIGHT) - 5);
    const end = Math.min(trashService.items.length, Math.ceil((trashScrollTop + trashViewportHeight) / TRASH_ITEM_HEIGHT) + 5);
    
    return trashService.items.slice(start, end).map((item, i) => ({
      path: item,
      y: (start + i) * TRASH_ITEM_HEIGHT
    }));
  });

  async function refreshSysInfo() {
    sysInfo = await api.sysCheck();
    appState.runtimeCapabilities = sysInfo?.capabilities || null;
  }

  onMount(async () => {
    await refreshSysInfo();
  });

  async function save() {
    if (!viewState.config) return;
    viewState.config.exiftoolPath = (viewState.config.exiftoolPath || '').trim();

    try {
      await api.updateConfig(viewState.config);
    } catch (e: any) {
      logger.error('Failed to save settings config', { error: e?.message || String(e) });
      return;
    }

    viewState.settingsOpen = false;

    try {
      await refreshSysInfo();
    } catch (e: any) {
      logger.warn('Failed to refresh runtime capabilities after saving settings', {
        error: e?.message || String(e),
      });
    }

    if (filterState.filterMode === 'duplicates') {
      filterState.duplicateGroups = []; // Reset cache
      try {
        await appState.updateFilteredIndices();
      } catch (e: any) {
        logger.warn('Failed to refresh duplicate indices after saving settings', {
          error: e?.message || String(e),
        });
      }
    }
  }

  async function applyExiftoolPath() {
    if (!viewState.config || applyingExifPath) return;
    applyingExifPath = true;
    exifPathFeedback = null;
    viewState.config.exiftoolPath = (viewState.config.exiftoolPath || '').trim();

    try {
      await api.updateConfig(viewState.config);
      await refreshSysInfo();
      if (sysInfo?.exiftool) {
        exifPathFeedback = { type: 'ok', text: i18n.t('exiftool_path_applied') };
      } else {
        exifPathFeedback = { type: 'error', text: i18n.t('exiftool_not_found') };
      }
    } catch (e: any) {
      logger.warn('Failed to apply exiftool path in settings', { error: e?.message || String(e) });
      const message = typeof e?.message === 'string' ? e.message : String(e);
      exifPathFeedback = { type: 'error', text: localizeBackendError(message) };
    } finally {
      applyingExifPath = false;
    }
  }

  function handleTrashScroll(e: UIEvent) {
    trashScrollTop = (e.target as HTMLDivElement).scrollTop;
  }

  function startRecording(id: string) {
    recordingAction = id;
    shortcutService.isRecording = true;
    // Force focus away from button to ensure window catches the event
    if (document.activeElement instanceof HTMLElement) {
      document.activeElement.blur();
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (recordingAction) {
      e.preventDefault();
      e.stopPropagation();
      
      const key = e.key.toLowerCase();
      logger.debug('Shortcut recording key pressed', { action: recordingAction, key });
      
      if (key === 'escape') {
        recordingAction = null;
        shortcutService.isRecording = false;
        return;
      }

      if (viewState.config) {
        const newShortcuts = { ...(viewState.config.shortcuts || {}) };
        newShortcuts[recordingAction] = key;
        viewState.config.shortcuts = newShortcuts;
      }
      
      recordingAction = null;
      shortcutService.isRecording = false;
      return;
    }

    if (e.key === 'Escape') {
      if (trashService.pickerOpen) {
        trashService.pickerOpen = false;
      } else if (viewState.settingsOpen) {
        viewState.settingsOpen = false;
      }
    }
  }

  function resetShortcuts() {
    if (viewState.config) {
      viewState.config.shortcuts = {};
    }
  }

  function resetConfig() {
    if (!confirm(i18n.t('confirm_reset_to_defaults'))) return;
    if (viewState.config) {
      viewState.config.theme = 'dark';
      viewState.config.duplicateThreshold = 90;
      viewState.config.autoRefresh = false;
      viewState.config.autoRefreshSeconds = 5;
      viewState.config.shortcuts = {};
      // We don't reset window size as it's environment-dependent
    }
  }

  // Use standard event listener with capture for robust shortcut recording
  $effect(() => {
    window.addEventListener('keydown', handleKeydown, { capture: true });
    return () => {
      window.removeEventListener('keydown', handleKeydown, { capture: true });
    };
  });

  $effect(() => {
    if (trashService.pickerOpen && trashViewport) {
      trashViewportHeight = trashViewport.clientHeight;
    }
  });
</script>

<Modal
  isOpen={viewState.settingsOpen}
  onClose={() => viewState.settingsOpen = false}
  width="min(980px, 94vw)"
  padding="0"
  ariaLabel="settings-title"
>
  <div class="settings-shell">
    <header class="settings-masthead">
      <div class="masthead-copy">
        <p class="kicker">{i18n.t('settings')}</p>
        <h2 id="settings-title">{i18n.t('settings')}</h2>
        <p class="masthead-note">{i18n.t('settings_general')} · {i18n.t('settings_analysis')} · {i18n.t('settings_workflow')}</p>
      </div>
      <div class="masthead-meta">
        {#if appState.stats?.version}
          <span class="version-badge">v{appState.stats.version}</span>
        {/if}
        {#if sysInfo}
          <div class="sys-badge" class:ok={sysInfo.exiftool} title={i18n.t('exiftool_status')}>
            {sysInfo.os} · {sysInfo.arch}
          </div>
        {/if}
      </div>
    </header>

    <div class="settings-layout">
      <nav class="settings-nav" aria-label={i18n.t('settings')}>
        <button class="tab-btn" class:active={activeTab === 'general'} onclick={() => activeTab = 'general'}>
          <span>{i18n.t('settings_general')}</span>
        </button>
        <button class="tab-btn" class:active={activeTab === 'shortcuts'} onclick={() => activeTab = 'shortcuts'}>
          <span>{i18n.t('settings_shortcuts')}</span>
        </button>
        <button class="tab-btn" class:active={activeTab === 'maintenance'} onclick={() => activeTab = 'maintenance'}>
          <span>{i18n.t('settings_maintenance')}</span>
        </button>
      </nav>

      <div class="tab-content">
        {#if viewState.config}
          {#if activeTab === 'general'}
            <section class="settings-card">
              <h3>{i18n.t('settings_general')}</h3>
              <div class="field-grid two-col">
                <div class="field">
                  <label for="lang-select">{i18n.t('language')}</label>
                  <select id="lang-select" value={i18n.lang} onchange={(e) => i18n.setLang(e.currentTarget.value as any)}>
                    <option value="en">{i18n.t('language') === 'Language' ? 'English' : 'Anglais'}</option>
                    <option value="fr">{i18n.t('language') === 'Language' ? 'French' : 'Français'}</option>
                  </select>
                </div>
                <div class="field">
                  <label for="theme-select">{i18n.t('theme')}</label>
                  <select id="theme-select" bind:value={viewState.config.theme}>
                    <option value="dark">{i18n.t('theme_dark')}</option>
                    <option value="light">{i18n.t('theme_light')}</option>
                  </select>
                </div>
              </div>
            </section>

            <section class="settings-card">
              <h3>{i18n.t('settings_analysis')}</h3>
              <div class="field">
                <label for="exif-path">{i18n.t('exiftool_path')}</label>
                <div class="input-with-btn">
                  {#if viewState.config}
                    <input
                      id="exif-path"
                      type="text"
                      bind:value={viewState.config.exiftoolPath}
                      placeholder={i18n.t('placeholder_exiftool')}
                      oninput={() => (exifPathFeedback = null)}
                    />
                    <button class="mini-btn" onclick={applyExiftoolPath} disabled={applyingExifPath}>
                      {i18n.t('apply')}
                    </button>
                    <button class="mini-btn icon-btn" onclick={async () => {
                      const res = await api.exiftoolDialog();
                      if (res?.path && viewState.config) {
                        viewState.config.exiftoolPath = res.path;
                        await applyExiftoolPath();
                      }
                    }}>...</button>
                  {/if}
                </div>
                {#if exifPathFeedback}
                  <p class={exifPathFeedback.type === 'ok' ? 'ok-text' : 'error-text'}>{exifPathFeedback.text}</p>
                {/if}
                {#if sysInfo && !sysInfo.exiftool}
                  <p class="error-text">{i18n.t('exiftool_not_found')}</p>
                {/if}
                {#if sysInfo?.capabilities}
                  <div class="capability-list">
                    <p class="help-text">{i18n.t('cap_raw_preview')}: {sysInfo.capabilities.rawPreview ? i18n.t('cap_status_ok') : i18n.t('cap_status_ko')}</p>
                    <p class="help-text">{i18n.t('cap_raw_metadata')}: {sysInfo.capabilities.rawMetadata ? i18n.t('cap_status_ok') : i18n.t('cap_status_ko')}</p>
                    <p class="help-text">{i18n.t('cap_heic_decode')}: {sysInfo.capabilities.heicDecode ? i18n.t('cap_status_ok') : i18n.t('cap_status_ko')}</p>
                    <p class="help-text">{i18n.t('cap_exif_write')}: {sysInfo.capabilities.exifWrite ? i18n.t('cap_status_ok') : i18n.t('cap_status_ko')}</p>
                  </div>
                {/if}
              </div>
              <div class="field">
                <label for="dup-threshold">{i18n.t('similarity_threshold')}: {viewState.config.duplicateThreshold}%</label>
                <input id="dup-threshold" type="range" min="50" max="100" step="1" bind:value={viewState.config.duplicateThreshold} />
              </div>
            </section>

            <section class="settings-card">
              <h3>{i18n.t('settings_workflow')}</h3>
              <div class="switch-row">
                <label class="switch">
                  <input id="auto-advance" type="checkbox" checked={appState.autoAdvance} onchange={() => appState.toggleAutoAdvance()} />
                  <span class="slider"></span>
                </label>
                <div class="switch-copy">
                  <label for="auto-advance">{i18n.t('auto_advance_label')}</label>
                  <p class="help-text">{i18n.t('auto_advance_help')}</p>
                </div>
              </div>
              <div class="switch-row">
                <label class="switch">
                  <input id="auto-refresh" type="checkbox" bind:checked={viewState.config.autoRefresh} />
                  <span class="slider"></span>
                </label>
                <label for="auto-refresh">{i18n.t('auto_refresh')}</label>
              </div>
              <div class="switch-row">
                <label class="switch">
                  <input id="debug-toggle" type="checkbox" bind:checked={viewState.config.debug} />
                  <span class="slider"></span>
                </label>
                <label for="debug-toggle">{i18n.t('enable_debug')}</label>
              </div>
              <div class="field" class:disabled={!viewState.config.autoRefresh}>
                <label for="auto-refresh-interval">{i18n.t('auto_refresh_interval')}: {viewState.config.autoRefreshSeconds}{i18n.t('unit_seconds')}</label>
                <input
                  id="auto-refresh-interval"
                  type="range"
                  min="1"
                  max="60"
                  step="1"
                  bind:value={viewState.config.autoRefreshSeconds}
                  disabled={!viewState.config.autoRefresh}
                />
              </div>
            </section>
          {/if}

          {#if activeTab === 'shortcuts'}
            <section class="settings-card">
              <div class="section-header-row">
                <h3>{i18n.t('settings_shortcuts')}</h3>
                <button class="mini-btn" onclick={resetShortcuts}>{i18n.t('shortcut_reset_all')}</button>
              </div>
              <div class="shortcuts-grid">
                {#each shortcutService.getDefinitions() as def}
                  {#if !def.id.startsWith('ACTION_LABEL_')}
                    <div class="shortcut-item">
                      <span class="shortcut-label">{i18n.t(def.descriptionKey as any)}</span>
                      <button
                        class="shortcut-key-btn"
                        class:recording={recordingAction === def.id}
                        onclick={() => startRecording(def.id)}
                      >
                        {#if recordingAction === def.id}
                          {i18n.t('shortcut_press_key')}
                        {:else}
                          {(shortcutService.activeMappings[def.id] || []).join(' / ').toUpperCase()}
                        {/if}
                      </button>
                    </div>
                  {/if}
                {/each}
              </div>
            </section>
          {/if}

          {#if activeTab === 'maintenance'}
            <section class="settings-card">
              <h3>{i18n.t('settings_maintenance')}</h3>
              <div class="maintenance-grid">
                <button class="btn secondary-action" disabled={trashService.loading} onclick={() => trashService.list().then(() => trashService.pickerOpen = true)}>
                  <Icon name="refresh" size={14} />
                  {i18n.t('restore_all_from_trash')}
                </button>
                <button class="btn secondary-action" onclick={async () => {
                  try {
                    await api.openConfigFolder();
                  } catch (e: any) {
                    logger.error('Failed to open config folder', { error: e.message });
                    toastService.error(i18n.t('unsupported_platform'));
                  }
                }}>
                  <Icon name="folder" size={14} />
                  {i18n.t('open_config_folder')}
                </button>
                <button class="btn secondary-action" onclick={async () => {
                  try {
                    await api.exportLogs();
                  } catch (e) {
                    logger.error('Export logs failed', { error: e });
                  }
                }}>
                  <Icon name="info" size={14} />
                  {i18n.t('export_logs')}
                </button>
              </div>
            </section>

            <section class="settings-card danger-zone-box">
              <h3>{i18n.t('danger_zone')}</h3>
              <div class="danger-grid">
                <button class="btn danger" onclick={() => { if(confirm(i18n.t('confirm_clear_stars'))) { appState.resetStars(); viewState.settingsOpen = false; } }}>
                  {i18n.t('clear_all_stars')}
                </button>
                <button class="btn danger" onclick={() => { if(confirm(i18n.t('confirm_clear_labels'))) { appState.resetLabels(); viewState.settingsOpen = false; } }}>
                  {i18n.t('clear_all_labels')}
                </button>
                <button class="btn danger" onclick={() => { if(confirm(i18n.t('confirm_reset_app_cache'))) { appState.resetAppCache(); } }}>
                  {i18n.t('reset_app_cache')}
                </button>
                <button class="btn danger" onclick={resetConfig}>
                  {i18n.t('reset_to_defaults')}
                </button>
              </div>
            </section>

            <section class="settings-card perf-debug-box">
              <h3>{i18n.t('settings_perf_debug')}</h3>
              <div class="perf-grid">
                <div class="perf-item"><span>{i18n.t('perf_bg_workers')}</span><strong>{(appState.stats as any).ioWorkers || 0}</strong></div>
                <div class="perf-item"><span>{i18n.t('perf_bg_hash_deferred')}</span><strong>{(appState.stats as any).hashDeferred ? 'ON' : 'OFF'}</strong></div>
                <div class="perf-item"><span>{i18n.t('perf_auto_refresh_backoff')}</span><strong>×{watchService.debugBackoffFactor()}</strong></div>
                <div class="perf-item"><span>{i18n.t('perf_auto_refresh_next_tick')}</span><strong>{watchService.debugCurrentIntervalSeconds()}{i18n.t('unit_seconds')}</strong></div>
                <div class="perf-item"><span>{i18n.t('perf_cache_gc_meta')}</span><strong>{(appState.stats as any).cacheMetaGc || 0}</strong></div>
                <div class="perf-item"><span>{i18n.t('perf_cache_gc_hash')}</span><strong>{(appState.stats as any).cacheHashGc || 0}</strong></div>
                <div class="perf-item"><span>{i18n.t('perf_cache_gc_files')}</span><strong>{(appState.stats as any).cacheDerivedGc || 0}</strong></div>
                <div class="perf-item"><span>{i18n.t('perf_analysis_events')}</span><strong>{appState.perf.progressEvents}</strong></div>
                <div class="perf-item"><span>{i18n.t('perf_state_events')}</span><strong>{appState.perf.stateEvents}</strong></div>
                <div class="perf-item"><span>{i18n.t('perf_analysis_flushes')}</span><strong>{appState.perf.analysisFlushes}</strong></div>
                <div class="perf-item"><span>{i18n.t('perf_scheduler_mode')}</span><strong>{appState.perf.schedulerMode}</strong></div>
                <div class="perf-item"><span>{i18n.t('perf_nav_promotions')}</span><strong>{appState.perf.navPromotionTotal}</strong></div>
                <div class="perf-item"><span>{i18n.t('perf_view_ready_last')}</span><strong>{appState.perf.viewReadyLatencyMs}{i18n.t('unit_milliseconds')}</strong></div>
                <div class="perf-item"><span>{i18n.t('perf_view_ready_p50')}</span><strong>{appState.perf.viewReadyP50Ms}{i18n.t('unit_milliseconds')}</strong></div>
                <div class="perf-item"><span>{i18n.t('perf_active_mode_time')}</span><strong>{appState.perf.activeModeMs}{i18n.t('unit_milliseconds')}</strong></div>
                <div class="perf-item"><span>{i18n.t('perf_idle_mode_time')}</span><strong>{appState.perf.idleModeMs}{i18n.t('unit_milliseconds')}</strong></div>
              </div>
            </section>
          {/if}
        {/if}
      </div>
    </div>

    <div class="settings-footer">
      <button class="btn" onclick={() => viewState.settingsOpen = false}>{i18n.t('cancel')}</button>
      <button class="btn primary" onclick={save}>{i18n.t('save')}</button>
    </div>
  </div>
</Modal>

<Modal
  isOpen={trashService.pickerOpen}
  onClose={() => trashService.pickerOpen = false}
  width="min(800px, 92vw)"
  padding="0"
  ariaLabel="trash-picker-title"
>
  <div class="trash-modal-root" style="height: 80vh; display: flex; flex-direction: column; padding: 0;">
    <div style="padding: var(--space-6) var(--space-8); border-bottom: 1px solid var(--border-main); display: flex; justify-content: space-between; align-items: center; width: 100%; box-sizing: border-box;">
      <div class="header-info">
        <h2 id="trash-picker-title" style="margin:0;">{i18n.t('restore_from_trash_select')}</h2>
        <div class="trash-count">{trashService.selectedItems.length} / {trashService.items.length} {i18n.t('selected')}</div>
      </div>
      <div class="trash-picker-toolbar">
        <button class="mini-btn" onclick={() => trashService.selectAll()}>{i18n.t('select_all')}</button>
        <button class="mini-btn" onclick={() => trashService.clearSelection()}>{i18n.t('clear_selection')}</button>
      </div>
    </div>
    
    <div class="trash-modal-body trash-body" style="flex: 1; width: 100%;">
      {#if trashService.items.length === 0}
        <div class="trash-empty">{i18n.t('trash_empty')}</div>
      {:else}
        <div 
          bind:this={trashViewport}
          class="trash-viewport" 
          onscroll={handleTrashScroll}
        >
          <div class="trash-content" style="height: {trashService.items.length * TRASH_ITEM_HEIGHT}px">
            {#each visibleTrashItems as item (item.path)}
              <button
                class="trash-list-item"
                class:selected={trashService.selectedItems.includes(item.path)}
                style="transform: translateY({item.y}px); height: {TRASH_ITEM_HEIGHT}px;"
                onclick={() => trashService.toggleItem(item.path)}
              >
                <div class="checkbox">
                  {#if trashService.selectedItems.includes(item.path)}
                    <Icon name="star" size={14} />
                  {/if}
                </div>
                <span class="path-text">{item.path}</span>
              </button>
            {/each}
          </div>
        </div>
      {/if}
    </div>

    <div class="trash-modal-footer" style="width: 100%; box-sizing: border-box;">
      <button class="btn" onclick={() => (trashService.pickerOpen = false)}>{i18n.t('cancel')}</button>
      <button
        class="btn primary"
        disabled={trashService.selectedItems.length === 0 || trashService.restoring}
        onclick={() => trashService.restore()}
      >
        {trashService.restoring ? i18n.t('loading') : i18n.t('restore_selected')}
      </button>
    </div>
  </div>
</Modal>

<style>
  .settings-shell {
    display: flex;
    flex-direction: column;
    min-height: min(82vh, 760px);
    max-height: 82vh;
    background:
      radial-gradient(circle at 95% 0%, rgba(var(--accent-rgb), 0.15) 0%, transparent 36%),
      radial-gradient(circle at 0% 100%, rgba(var(--accent-rgb), 0.08) 0%, transparent 42%),
      var(--bg-surface);
  }
  .settings-masthead {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: var(--space-5);
    padding: var(--space-6) var(--space-8) var(--space-5);
    border-bottom: 1px solid var(--border-main);
    background: linear-gradient(180deg, rgba(var(--accent-rgb), 0.08) 0%, transparent 100%);
  }
  .masthead-copy {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .kicker {
    margin: 0;
    font-family: "Avenir Next", "Segoe UI", sans-serif;
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.2em;
    color: var(--accent);
    font-weight: 700;
  }
  h2 {
    margin: 0;
    font-family: "Iowan Old Style", "Palatino Linotype", serif;
    font-size: 28px;
    letter-spacing: 0.01em;
    color: var(--text-main);
    line-height: 1.05;
  }
  .masthead-note {
    margin: 0;
    color: var(--text-muted);
    font-size: var(--text-caption);
    letter-spacing: 0.05em;
    text-transform: uppercase;
  }
  .masthead-meta {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }
  .version-badge {
    font-size: var(--text-xs);
    font-family: var(--mono);
    color: var(--text-muted);
    border: 1px solid var(--border-main);
    padding: 5px var(--space-2);
    border-radius: var(--radius-full);
    background: var(--bg-inset);
  }
  .sys-badge {
    font-family: var(--mono);
    font-size: var(--text-xs);
    padding: 5px var(--space-2);
    border-radius: var(--radius-full);
    background: var(--danger-dim);
    color: var(--danger);
    border: 1px solid rgba(239, 68, 68, 0.32);
  }
  .sys-badge.ok {
    background: rgba(16, 185, 129, 0.12);
    color: var(--success);
    border-color: rgba(16, 185, 129, 0.35);
  }

  .settings-layout {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: 220px minmax(0, 1fr);
    overflow: hidden;
  }
  .settings-nav {
    padding: var(--space-5);
    border-right: 1px solid var(--border-subtle);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    background: linear-gradient(180deg, rgba(var(--accent-rgb), 0.07), transparent 30%);
  }
  .tab-btn {
    padding: var(--space-3) var(--space-3);
    border-radius: var(--radius-lg);
    border: 1px solid transparent;
    color: var(--text-muted);
    font-size: var(--text-caption);
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    text-align: left;
    transition: transform var(--transition-base), border-color var(--transition-base), color var(--transition-base), background var(--transition-base);
  }
  .tab-btn:hover {
    border-color: var(--border-main);
    color: var(--text-main);
    background: var(--bg-inset);
    transform: translateX(2px);
  }
  .tab-btn.active {
    border-color: rgba(var(--accent-rgb), 0.6);
    color: var(--text-main);
    background: linear-gradient(90deg, rgba(var(--accent-rgb), 0.22), rgba(var(--accent-rgb), 0.06));
    box-shadow: inset 2px 0 0 var(--accent);
  }

  .tab-content {
    min-height: 0;
    overflow-y: auto;
    padding: var(--space-5);
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }
  .settings-card {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-5);
    border-radius: var(--radius-xl);
    border: 1px solid var(--border-main);
    background:
      linear-gradient(165deg, rgba(255, 255, 255, 0.02) 0%, transparent 38%),
      var(--bg-inset);
    box-shadow: 0 10px 24px rgba(0, 0, 0, 0.14);
    animation: card-reveal 220ms ease-out both;
  }
  h3 {
    margin: 0;
    font-family: "Avenir Next", "Segoe UI", sans-serif;
    font-size: var(--text-sm);
    text-transform: uppercase;
    letter-spacing: 0.16em;
    color: var(--accent);
  }
  .section-header-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--space-3);
  }
  .field-grid.two-col {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-3);
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: var(--space-1-5);
  }
  .field.disabled {
    opacity: 0.55;
    pointer-events: none;
  }
  label {
    font-size: var(--text-caption);
    color: var(--text-muted);
    font-weight: 600;
    letter-spacing: 0.02em;
  }
  input[type="text"], select {
    background: var(--bg-app);
    border: 1px solid var(--border-main);
    color: var(--text-main);
    padding: 11px 12px;
    border-radius: var(--radius-lg);
    outline: none;
    font-size: var(--text-base);
    transition: border-color var(--transition-base), box-shadow var(--transition-base);
  }
  input[type="text"]:focus, select:focus {
    border-color: rgba(var(--accent-rgb), 0.8);
    box-shadow: 0 0 0 3px rgba(var(--accent-rgb), 0.2);
  }
  .input-with-btn {
    display: flex;
    gap: var(--space-2);
  }
  .input-with-btn input {
    flex: 1;
  }

  .switch-row {
    display: grid;
    grid-template-columns: auto 1fr;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) 0;
  }
  .switch-copy {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .help-text {
    color: var(--text-muted);
    font-size: var(--text-sm);
    line-height: 1.45;
    margin: 0;
  }
  .capability-list {
    margin-top: 4px;
    display: grid;
    gap: 3px;
  }
  .error-text, .ok-text {
    margin: 2px 0 0;
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .error-text {
    color: var(--danger);
  }
  .ok-text {
    color: var(--success);
  }

  .shortcuts-grid {
    display: grid;
    gap: var(--space-2);
  }
  .shortcut-item {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: var(--space-3);
    align-items: center;
    padding: var(--space-2-5) var(--space-3);
    border-radius: var(--radius-lg);
    border: 1px solid var(--border-main);
    background: var(--bg-app);
  }
  .shortcut-label {
    font-size: var(--text-sm);
    color: var(--text-main);
    font-weight: 600;
  }
  .shortcut-key-btn {
    background: var(--key-bg);
    border: 1px solid var(--border-main);
    color: var(--accent);
    padding: var(--space-1) var(--space-3);
    border-radius: var(--radius-md);
    font-size: var(--text-xs);
    font-family: var(--mono);
    min-width: 90px;
    text-align: center;
  }
  .shortcut-key-btn:hover {
    border-color: rgba(var(--accent-rgb), 0.7);
  }
  .shortcut-key-btn.recording {
    background: var(--accent);
    color: var(--color-slate-950);
    border-color: var(--accent);
    animation: pulse 1.2s infinite;
  }

  .maintenance-grid, .danger-grid, .perf-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-3);
  }
  .danger-zone-box {
    border-color: rgba(239, 68, 68, 0.36);
    background: linear-gradient(180deg, rgba(239, 68, 68, 0.12), rgba(239, 68, 68, 0.03));
  }
  .danger-zone-box h3 {
    color: var(--danger);
  }
  .perf-debug-box {
    background: linear-gradient(180deg, rgba(var(--accent-rgb), 0.08), var(--bg-inset));
  }
  .perf-item {
    display: flex;
    justify-content: space-between;
    gap: var(--space-3);
    background: var(--bg-app);
    border: 1px solid var(--border-main);
    border-radius: var(--radius-lg);
    padding: var(--space-2) var(--space-2-5);
    font-size: var(--text-caption);
  }
  .perf-item span {
    color: var(--text-muted);
  }
  .perf-item strong {
    color: var(--text-main);
    font-family: var(--mono);
  }

  .mini-btn {
    padding: 7px var(--space-3);
    border-radius: var(--radius-md);
    border: 1px solid var(--border-main);
    background: var(--bg-app);
    color: var(--text-main);
    font-size: var(--text-caption);
    font-weight: 700;
  }
  .mini-btn:hover {
    border-color: rgba(var(--accent-rgb), 0.7);
    background: var(--key-bg);
  }
  .icon-btn {
    min-width: 36px;
  }
  .btn {
    padding: var(--space-2-5) var(--space-5);
    border-radius: var(--radius-lg);
    border: 1px solid var(--border-main);
    background: var(--bg-app);
    color: var(--text-main);
    font-size: var(--text-base);
    font-weight: 700;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
  }
  .btn:hover:not(:disabled) {
    background: var(--key-bg);
    border-color: var(--text-muted);
  }
  .btn.primary {
    background: var(--accent);
    border-color: var(--accent);
    color: var(--color-slate-950);
  }
  .btn.primary:hover:not(:disabled) {
    filter: brightness(1.03);
  }
  .btn.secondary-action {
    border-color: rgba(var(--accent-rgb), 0.55);
    color: var(--accent);
    background: rgba(var(--accent-rgb), 0.06);
  }
  .btn.secondary-action:hover:not(:disabled) {
    background: rgba(var(--accent-rgb), 0.14);
  }
  .btn.danger {
    color: var(--danger);
    border-color: rgba(239, 68, 68, 0.36);
    background: rgba(239, 68, 68, 0.08);
  }
  .btn.danger:hover:not(:disabled) {
    background: rgba(239, 68, 68, 0.14);
  }
  .btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .settings-footer {
    padding: var(--space-4) var(--space-8);
    border-top: 1px solid var(--border-main);
    display: flex;
    justify-content: flex-end;
    gap: var(--space-3);
    background: linear-gradient(180deg, transparent 0%, rgba(0, 0, 0, 0.12) 100%);
  }

  input[type="range"] {
    width: 100%;
    accent-color: var(--accent);
    cursor: pointer;
  }
  .switch {
    position: relative;
    display: inline-block;
    width: 36px;
    height: 21px;
  }
  .switch input {
    opacity: 0;
    width: 0;
    height: 0;
  }
  .slider {
    position: absolute;
    inset: 0;
    border-radius: 20px;
    background-color: var(--bg-app);
    border: 1px solid var(--border-main);
    transition: all var(--transition-base);
  }
  .slider:before {
    position: absolute;
    content: "";
    height: 14px;
    width: 14px;
    left: 2px;
    top: 2px;
    border-radius: 50%;
    background-color: var(--text-muted);
    transition: all var(--transition-base);
  }
  input:checked + .slider {
    background-color: rgba(var(--accent-rgb), 0.25);
    border-color: var(--accent);
  }
  input:checked + .slider:before {
    transform: translateX(14px);
    background-color: var(--accent);
  }

  .trash-body {
    padding: 0;
    display: block;
    overflow: hidden;
  }
  .trash-modal-footer {
    padding: var(--space-5) var(--space-8);
    border-top: 1px solid var(--border-main);
    display: flex;
    justify-content: flex-end;
    gap: var(--space-3);
    background: rgba(0, 0, 0, 0.1);
  }
  .trash-viewport {
    height: 100%;
    overflow-y: auto;
    position: relative;
  }
  .trash-content {
    position: relative;
    width: 100%;
  }
  .trash-list-item {
    position: absolute;
    left: 0;
    right: 0;
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: 0 var(--space-8);
    background: transparent;
    border: none;
    border-bottom: 1px solid var(--border-main);
    color: var(--text-main);
    text-align: left;
    width: 100%;
  }
  .trash-list-item:hover {
    background: var(--bg-inset);
  }
  .trash-list-item.selected {
    background: var(--accent-dim);
  }
  .checkbox {
    width: 20px;
    height: 20px;
    border: 2px solid var(--border-main);
    border-radius: var(--radius-sm);
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--accent);
  }
  .selected .checkbox {
    border-color: var(--accent);
    background: var(--accent);
    color: var(--color-slate-950);
  }
  .path-text {
    font-size: var(--text-base);
    font-family: var(--mono);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    flex: 1;
  }
  .trash-empty {
    padding: 60px;
    text-align: center;
    color: var(--text-muted);
    font-size: var(--text-md);
  }
  .trash-count {
    font-size: var(--text-caption);
    color: var(--text-muted);
  }
  .header-info {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }
  .trash-picker-toolbar {
    display: flex;
    gap: var(--space-2);
  }

  @keyframes pulse {
    0% { box-shadow: 0 0 0 0 rgba(var(--accent-rgb), 0.4); }
    70% { box-shadow: 0 0 0 10px rgba(var(--accent-rgb), 0); }
    100% { box-shadow: 0 0 0 0 rgba(var(--accent-rgb), 0); }
  }
  @keyframes card-reveal {
    from { opacity: 0; transform: translateY(10px); }
    to { opacity: 1; transform: translateY(0); }
  }

  @media (max-width: 900px) {
    .settings-layout {
      grid-template-columns: 1fr;
    }
    .settings-nav {
      border-right: 0;
      border-bottom: 1px solid var(--border-subtle);
      padding: var(--space-3) var(--space-5);
      flex-direction: row;
      overflow-x: auto;
    }
    .tab-btn {
      white-space: nowrap;
    }
  }
  @media (max-width: 720px) {
    .settings-shell {
      max-height: 90vh;
    }
    .settings-masthead {
      padding: var(--space-5);
      flex-direction: column;
      gap: var(--space-3);
    }
    h2 {
      font-size: 24px;
    }
    .tab-content {
      padding: var(--space-4);
    }
    .settings-card {
      padding: var(--space-4);
    }
    .field-grid.two-col,
    .maintenance-grid,
    .danger-grid,
    .perf-grid {
      grid-template-columns: 1fr;
    }
    .settings-footer {
      padding: var(--space-4);
      justify-content: stretch;
    }
    .settings-footer .btn {
      flex: 1;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .settings-card,
    .shortcut-key-btn.recording {
      animation: none;
    }
    .tab-btn,
    .btn,
    .mini-btn {
      transition: none;
    }
  }
</style>
