<script lang="ts">
  let { photos = $bindable({} as Record<string, { Label: number }>) }: { photos?: Record<string, { Label: number }> } = $props();
  let badgeLabel = $derived(photos?.['a.jpg']?.Label ?? 0);
</script>

{#if badgeLabel > 0}
  <div data-testid="label-badge">{badgeLabel}</div>
{/if}
