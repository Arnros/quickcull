<script lang="ts">
  import { onMount } from "svelte";
  import { i18n } from "../i18n.svelte";
  import Kbd from "./Kbd.svelte";

  const STORAGE_KEY = "quickcull_shortcut_onboarding_seen_v1";
  let visible = $state(false);

  function dismiss() {
    visible = false;
    try {
      localStorage.setItem(STORAGE_KEY, "1");
    } catch {
      // ignore storage failures
    }
  }

  onMount(() => {
    try {
      visible = localStorage.getItem(STORAGE_KEY) !== "1";
    } catch {
      visible = true;
    }

    const onKeydown = (e: KeyboardEvent) => {
      if (!visible) return;
      if (e.key === "Escape") {
        e.preventDefault();
        dismiss();
      }
    };
    window.addEventListener("keydown", onKeydown, { capture: true });
    return () => window.removeEventListener("keydown", onKeydown, { capture: true });
  });
</script>

{#if visible}
  <div class="onboarding-overlay" role="dialog" aria-modal="true" aria-label={i18n.t("onboarding_title")}>
    <div class="onboarding-card">
      <h3>{i18n.t("onboarding_title")}</h3>
      <p>{i18n.t("onboarding_desc")}</p>
      <div class="grid">
        <div class="row"><Kbd key="D / →" /><span>{i18n.t("onboarding_nav_next")}</span></div>
        <div class="row"><Kbd key="Q / ←" /><span>{i18n.t("onboarding_nav_prev")}</span></div>
        <div class="row"><Kbd key="S" /><span>{i18n.t("onboarding_star")}</span></div>
        <div class="row"><Kbd key="X" /><span>{i18n.t("onboarding_trash")}</span></div>
        <div class="row"><Kbd key="U" /><span>{i18n.t("onboarding_undo")}</span></div>
        <div class="row"><Kbd key="?" /><span>{i18n.t("onboarding_help")}</span></div>
      </div>
      <div class="actions">
        <button class="btn" onclick={dismiss}>{i18n.t("onboarding_got_it")}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .onboarding-overlay {
    position: absolute;
    inset: 0;
    z-index: var(--z-modal, 1400);
    background: color-mix(in srgb, var(--bg-app) 82%, transparent);
    backdrop-filter: blur(6px);
    display: grid;
    place-items: center;
    padding: var(--space-6);
  }
  .onboarding-card {
    width: min(560px, 92vw);
    background: var(--bg-surface);
    border: 1px solid var(--border-main);
    border-radius: var(--radius-2xl);
    box-shadow: var(--shadow-modal);
    padding: var(--space-6);
    animation: intro 180ms ease-out;
  }
  h3 {
    margin: 0 0 var(--space-2) 0;
    font-family: var(--serif);
    font-size: 26px;
    letter-spacing: 0.01em;
  }
  p {
    margin: 0 0 var(--space-4) 0;
    color: var(--text-muted);
  }
  .grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-2) var(--space-3);
    margin-bottom: var(--space-4);
  }
  .row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-main);
    font-size: var(--text-sm);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
  }
  .btn {
    border: 1px solid var(--border-main);
    background: var(--accent);
    color: var(--on-accent);
    border-radius: var(--radius-md);
    padding: 8px 12px;
    cursor: pointer;
  }
  @keyframes intro {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }
  @media (max-width: 700px) {
    .grid { grid-template-columns: 1fr; }
  }
</style>
