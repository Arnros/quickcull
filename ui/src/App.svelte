<script lang="ts">
  import { onMount } from 'svelte';
  import { appState } from './lib/appState.svelte';
  import { viewState } from './lib/viewState.svelte';
  import { shortcutService } from './lib/shortcutService.svelte';
  import { toastService } from './lib/toast.svelte';
  import { EventsOn } from '../wailsjs/runtime/runtime';
  import { EVENTS } from './lib/constants';
  import Picker from './lib/components/Picker.svelte';
  import Review from './lib/components/Review.svelte';

  onMount(() => {
    appState.init();

    const handleKeydown = (e: KeyboardEvent) => {
      shortcutService.handleKeydown(e);
    };
    window.addEventListener('keydown', handleKeydown);

    const saveOnUnload = () => {
      if (viewState.current === 'review') {
        appState.persistPositionNow();
      }
    };
    window.addEventListener('beforeunload', saveOnUnload);
    return () => {
      window.removeEventListener('keydown', handleKeydown);
      window.removeEventListener('beforeunload', saveOnUnload);
    };
  });
</script>

<main class={viewState.config?.theme === 'light' ? 'theme-light' : ''}>
  {#if viewState.current === 'picker'}
    <Picker />
  {:else}
    <Review />
  {/if}

  {#if toastService.toast}
    <div class="toast-container">
      <div class="toast {toastService.toast.type}" role="alert">
        {toastService.toast.msg}
      </div>
    </div>
  {/if}
</main>

<style>
  main {
    width: 100%;
    height: 100%;
    overflow: hidden;
    background: var(--bg-app);
    color: var(--text-main);
  }

  .toast-container {
    position: fixed;
    bottom: 90px;
    left: 50%;
    transform: translateX(-50%);
    z-index: var(--z-toast);
    pointer-events: none;
  }
  .toast {
    padding: 14px 28px;
    background:
      linear-gradient(165deg, rgba(var(--accent-rgb), 0.09), transparent 42%),
      var(--bg-surface);
    border: 1px solid var(--border-main);
    border-radius: var(--radius-xl);
    color: var(--text-main);
    font-size: 15px;
    font-weight: 600;
    box-shadow: 0 15px 45px var(--shadow-main);
    border-left: 6px solid var(--accent);
    animation: toast-in 0.25s cubic-bezier(0.175, 0.885, 0.32, 1.275);
  }
  .toast.star { border-left-color: var(--star); }
  .toast.danger { border-left-color: var(--danger); }
  .toast.success { border-left-color: var(--success); }

  @keyframes toast-in {
    from { opacity: 0; transform: translateY(30px); }
    to { opacity: 1; transform: translateY(0); }
  }
</style>
