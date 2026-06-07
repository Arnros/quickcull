<script lang="ts">
  import { appState } from "../appState.svelte";
  import { i18n } from "../i18n.svelte";
  import { shortcutService } from "../shortcutService.svelte";
  import { resolveContextShortcutHints } from "../shortcutContext";
  import Modal from "./Modal.svelte";
  import Kbd from "./Kbd.svelte";

  const categories = $derived.by(() => {
    const defs = shortcutService.getDefinitions();
    const mappings = shortcutService.activeMappings;
    
    // Group by category
    const cats: Record<string, any[]> = {
      navigation: [],
      view: [],
      action: [],
      system: []
    };

    for (const def of defs) {
      // Skip label shortcuts in general help to keep it clean, or group them
      if (def.id.startsWith('ACTION_LABEL_')) continue;
      
      const keys = mappings[def.id] || [];
      if (keys.length === 0) continue;

      cats[def.category].push({
        keys,
        action: i18n.t(def.descriptionKey as any)
      });
    }

    // Add labels as a single entry
    cats.action.push({
      keys: ['0', '5'],
      isRange: true,
      action: i18n.t('help_action_label')
    });

    return [
      { title: i18n.t("folder"), items: cats.navigation },
      { title: i18n.t("grid"), items: cats.view },
      { title: i18n.t("kb_shortcuts"), items: cats.action },
      { title: i18n.t("settings"), items: cats.system }
    ];
  });

  const contextHints = $derived(resolveContextShortcutHints({
    view: appState.view,
    filterBarOpen: appState.filterBarOpen,
  }));
</script>

<Modal
  isOpen={appState.helpOpen}
  onClose={() => (appState.helpOpen = false)}
  width="min(900px, 95vw)"
  padding="40px"
  ariaLabel="help-title"
>
  <div class="help-grid">
    {#each categories as cat}
      {#if cat.items.length > 0}
        <div class="category">
          <h3>{cat.title}</h3>
          <div class="list">
            {#each cat.items as item}
              <div class="row">
                <span class="key-col">
                  {#if item.isRange}
                    <Kbd key={item.keys[0]} /> - <Kbd key={item.keys[1]} />
                  {:else}
                    {#each item.keys as k, i}
                      {#if i > 0}<span class="sep">/</span>{/if}
                      <Kbd key={k} />
                    {/each}
                  {/if}
                </span>
                <span class="action">{item.action}</span>
              </div>
            {/each}
          </div>
        </div>
      {/if}
    {/each}
  </div>
  <div class="context-conflicts">
    <h3>{i18n.t('help_conflicts_title')}</h3>
    <div class="conflict-row">
      <span class="key-col"><Kbd key="S" /></span>
      <span class="action">{i18n.t('help_conflict_s_review')}</span>
      <span class="sep">/</span>
      <span class="action">{i18n.t('help_conflict_s_filters')}</span>
    </div>
    <div class="conflict-row">
      <span class="key-col"><Kbd key={`0-${appState.maxLabel}`} /></span>
      <span class="action">{i18n.t('help_conflict_label_review')}</span>
      <span class="sep">/</span>
      <span class="action">{i18n.t('help_conflict_label_filters')}</span>
    </div>
    <p class="active-context">{i18n.t('shortcut_hint_title')}: {i18n.t(contextHints.starHintKey as any)} · {i18n.t(contextHints.labelHintKey as any)}</p>
  </div>
  <div class="help-footer-row">
    <p class="footer">{i18n.t("help_footer")}</p>
    {#if appState.stats?.version}
      <p class="version-tag">{i18n.t('version')} {appState.stats.version}</p>
    {/if}
  </div>
</Modal>

<style>
  .help-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: var(--space-8);
  }
  .category {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }
  h3 {
    font-size: var(--text-sm);
    color: var(--accent);
    text-transform: uppercase;
    letter-spacing: 0.14em;
    font-weight: 800;
    margin: 0;
  }
  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2-5);
  }
  .row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }
  .key-col {
    min-width: 85px;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 4px;
  }
  .sep {
    font-size: 10px;
    color: var(--text-muted);
    opacity: 0.5;
  }
  .action {
    font-size: var(--text-caption);
    color: var(--text-main);
    opacity: 0.82;
    font-weight: 600;
  }
  .footer {
    text-align: center;
    font-size: var(--text-sm);
    color: var(--text-muted);
    opacity: 0.7;
  }
  .help-footer-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-top: var(--space-4);
    padding-top: var(--space-4);
    border-top: 1px solid var(--border-main);
  }
  .version-tag {
    font-size: var(--text-xs);
    color: var(--text-muted);
    font-family: var(--mono);
    opacity: 0.72;
  }
  .context-conflicts {
    margin-top: var(--space-5);
    border-top: 1px solid var(--border-main);
    padding-top: var(--space-4);
  }
  .conflict-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-bottom: var(--space-2);
  }
  .active-context {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--text-muted);
    opacity: 0.8;
  }
</style>
