<script lang="ts">
  import { appState } from '../appState.svelte';

  let currentPath = $derived(appState.currentFile?.filename ?? '');
  let gridLabel = $derived(currentPath ? (appState.v2?.Photos[currentPath]?.Label ?? 0) : 0);
</script>

{#if gridLabel > 0}
  <span data-testid="grid-label">{gridLabel}</span>
  <span data-testid="filmstrip-label">{gridLabel}</span>
{/if}
