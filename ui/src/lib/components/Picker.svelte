<script lang="ts">
  import { onMount } from "svelte";
  import { appState } from "../appState.svelte";
  import { viewState } from "../viewState.svelte";
  import { api } from "../api";
  import { i18n } from "../i18n.svelte";
  import Settings from "./Settings.svelte";
  import Help from "./Help.svelte";
  import Logo from "./Logo.svelte";
  import { review } from "../../../wailsjs/go/models";
  import { logger } from "../logger";

  let path = $state("");
  let error = $state("");
  let bookmarks = $state<review.Bookmark[]>([]);
  let recent = $state<string[]>([]);

  onMount(async () => {
    logger.debug('Picker mounted');
    const bRes = await api.getBookmarks();
    bookmarks = bRes.bookmarks;
    const rRes = await api.getRecent();
    recent = rRes.recent;
  });

  async function openFolder(targetPath?: string) {
    const p = targetPath || path;
    if (!p) return;
    error = "";
    logger.info('User attempting to open folder', { path: p });
    try {
      await appState.selectFolder(p);
    } catch (e: any) {
      logger.error('Failed to open folder from Picker', { path: p, error: e.message });
      error = e.message;
    }
  }
</script>

<div class="picker">
  <div class="toolbar">
    <button
      class="icon-btn"
      onclick={() => appState.toggleTheme()}
      title={i18n.t('toggle_theme')}
    >
      {viewState.config?.theme === "light" ? "🌙" : "☀"}
    </button>
    <button
      class="icon-btn"
      onclick={() => (viewState.helpOpen = true)}
      title={i18n.t("help")}
    >
      ?
    </button>
    <button
      class="icon-btn"
      onclick={() => (viewState.settingsOpen = true)}
      title={i18n.t("settings")}
    >
      ⚙
    </button>
  </div>

  <div class="card">
    <div class="header">
      <div class="title-row">
        <div class="logo-wrapper"><Logo size="48px" /></div>
        <h1><span class="accent">quick</span>cull</h1>
      </div>
      <p class="tagline">{i18n.t("tagline")}</p>
    </div>

    {#if recent.length === 0}
      <div class="welcome-box">
        <h2>{i18n.t("welcome_title")}</h2>
        <p>{i18n.t("welcome_desc")}</p>
        <ul class="features">
          <li>✨ {i18n.t("features_speed")}</li>
          <li>🔒 {i18n.t("features_privacy")}</li>
          <li>📸 {i18n.t("features_pro")}</li>
        </ul>
        <p class="hint">{i18n.t("get_started")}</p>
      </div>
    {/if}

    {#if recent.length > 0}
      <div class="section">
        <label for="recent-list">{i18n.t("recent_folders")}</label>
        <div class="list" id="recent-list">
          {#each recent as r}
            <button class="item" onclick={() => openFolder(r)}>
              <span class="icon">📁</span>
              <div class="info">
                <span class="name">{r.split("/").pop() || r}</span>
                <span class="path">{r}</span>
              </div>
            </button>
          {/each}
        </div>
      </div>
    {/if}

    <div class="section">
      <label for="path-input">{i18n.t("open_another")}</label>
      <div class="input-row">
        <input
          id="path-input"
          type="text"
          bind:value={path}
          placeholder={i18n.t('placeholder_path')}
          onkeydown={(e) => e.key === "Enter" && openFolder()}
        />
        <button
          class="secondary"
          onclick={async () => {
            try {
              const res = await api.browseDialog();
              if (res.path) {
                path = res.path;
                await openFolder();
              }
            } catch (e: any) {
              logger.error('Failed to open browse dialog', { error: e.message });
              error = e.message;
            }
          }}
        >
          <span class="icon">📁</span>
          {i18n.t("browse")}
        </button>
        <button class="primary" onclick={() => openFolder()}
          >{i18n.t("folder")}</button
        >
      </div>
      {#if error}<p class="error">{error}</p>{/if}
    </div>

    <div class="footer">
      {i18n.t('picker_tip')} <code>quickcull -p /path/to/photos</code>
    </div>
  </div>

  {#if appState.loading}
    <div class="loading-overlay">
      <div class="loading-content">
        <div class="spinner"></div>
        <p>{i18n.t('scanning')}</p>
      </div>
    </div>
  {/if}

  {#if viewState.settingsOpen}
    <Settings />
  {/if}

  {#if viewState.helpOpen}
    <Help />
  {/if}
</div>

<style>
  .picker {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
    background:
      radial-gradient(circle at 12% 18%, rgba(var(--accent-rgb), 0.1), transparent 36%),
      radial-gradient(circle at 84% 86%, rgba(var(--accent-rgb), 0.08), transparent 36%),
      var(--bg-app);
    padding: 20px;
    position: relative;
  }

  .toolbar {
    position: absolute;
    top: 20px;
    right: 20px;
    display: flex;
    gap: 8px;
  }

  .icon-btn {
    background: var(--bg-surface);
    border: 1px solid var(--border-main);
    color: var(--text-main);
    padding: 8px;
    border-radius: 8px;
    cursor: pointer;
    font-size: 16px;
    width: 40px;
    height: 40px;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .icon-btn:hover {
    border-color: var(--accent);
    background: var(--accent-dim);
  }

  .card {
    background:
      linear-gradient(165deg, rgba(var(--accent-rgb), 0.08), transparent 45%),
      var(--bg-surface);
    border: 1px solid var(--border-main);
    border-radius: var(--radius-2xl);
    padding: 40px;
    width: 560px;
    max-width: calc(100vw - 40px);
    box-shadow: var(--shadow-modal);
  }

  .header {
    margin-bottom: 32px;
    text-align: center;
    display: flex;
    flex-direction: column;
    align-items: center;
  }
  .title-row {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 16px;
    margin-bottom: 8px;
  }
  .logo-wrapper {
    color: var(--accent);
    display: flex;
    align-items: center;
  }
  h1 {
    font-family: var(--serif);
    font-size: 42px;
    margin: 0;
    color: var(--text-main);
  }
  h1 span {
    color: var(--accent);
  }
  .tagline {
    color: var(--text-muted);
    font-size: 13px;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    margin-top: 8px;
  }

  .welcome-box {
    background: var(--bg-app);
    border: 1px dashed var(--border-main);
    border-radius: 12px;
    padding: 24px;
    margin-bottom: 32px;
    text-align: left;
  }
  .welcome-box h2 {
    font-size: 18px;
    margin: 0 0 8px 0;
    color: var(--text-main);
    text-align: left;
  }
  .welcome-box p {
    font-size: 13px;
    color: var(--text-muted);
    margin: 0 0 16px 0;
  }
  .features {
    list-style: none;
    padding: 0;
    margin: 0 0 20px 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .features li {
    font-size: 12px;
    color: var(--text-main);
    font-weight: 500;
  }
  .hint {
    font-weight: 600;
    color: var(--accent) !important;
    margin-bottom: 0 !important;
  }

  .section {
    margin-bottom: 24px;
  }
  label {
    display: block;
    font-size: 11px;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.12em;
    margin-bottom: 12px;
    font-weight: bold;
  }

  .list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .item {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px;
    background: var(--bg-app);
    border: 1px solid var(--border-main);
    border-radius: 8px;
    cursor: pointer;
    text-align: left;
    transition: all 0.15s;
    color: var(--text-main);
  }
  .item:hover {
    border-color: var(--accent);
    background: var(--accent-dim);
  }
  .item .icon {
    font-size: 20px;
    flex-shrink: 0;
  }
  .item .info {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .item .name {
    font-weight: bold;
    color: var(--text-main);
    font-size: 14px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .item .path {
    font-size: 10px;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .input-row {
    display: flex;
    gap: 10px;
    width: 100%;
  }
  input {
    flex: 1;
    min-width: 0;
    padding: 12px;
    border-radius: 8px;
    border: 1px solid var(--border-main);
    background: var(--bg-app);
    color: var(--text-main);
    outline: none;
    font-size: 14px;
  }
  input:focus {
    border-color: var(--accent);
  }

  button.primary {
    background: var(--accent);
    color: #111;
    border: none;
    padding: 12px 24px;
    border-radius: 8px;
    font-weight: bold;
    cursor: pointer;
    white-space: nowrap;
    flex-shrink: 0;
  }
  button.primary:hover {
    opacity: 0.9;
  }

  button.secondary {
    background: var(--bg-surface);
    color: var(--text-main);
    border: 1px solid var(--border-main);
    padding: 12px 16px;
    border-radius: 8px;
    font-weight: 500;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 8px;
    white-space: nowrap;
    flex-shrink: 0;
  }
  button.secondary:hover {
    border-color: var(--accent);
    background: var(--accent-dim);
  }

  .error {
    color: var(--danger);
    font-size: 13px;
    margin-top: 12px;
  }
  .footer {
    margin-top: 32px;
    padding-top: 20px;
    border-top: 1px solid var(--border-main);
    color: var(--text-muted);
    font-size: 12px;
    text-align: center;
  }
  code {
    background: var(--key-bg);
    padding: 2px 4px;
    border-radius: 4px;
    color: var(--accent);
  }

  .loading-overlay {
    position: fixed;
    inset: 0;
    background: var(--overlay-bg);
    display: flex;
    align-items: center;
    justify-content: center;
    backdrop-filter: var(--blur-md);
    z-index: var(--z-modal);
    animation: fade-in 0.2s ease-out;
  }

  .loading-content {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 16px;
  }

  .loading-content .spinner {
    width: 32px;
    height: 32px;
    border: 3px solid var(--accent-dim);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  .loading-content p {
    font-size: 14px;
    color: var(--text-main);
    font-weight: 600;
  }
</style>
