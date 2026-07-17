<script lang="ts">
  import { untrack } from 'svelte';
  import { syncService } from '../syncService.svelte';

  let { initialPhotos = {} as Record<string, { Label: number; ID?: string; IsStarred?: boolean; Rotation?: number; IsTrashed?: boolean }> } = $props();
  const startingPhotos = untrack(() => initialPhotos);

  let appState = $state({
    v2: {
      Root: '/media',
      CacheDir: '',
      VisibleOrder: Object.keys(startingPhotos),
      Photos: { ...startingPhotos },
      TrashedCount: 0,
      StarredCount: 0,
      LabeledCount: 0,
      RotatedCount: 0,
      UndoLen: 0,
      IsPartial: false,
    },
    stats: { total: 0, trashedCount: 0, starredCount: 0, labeledCount: 0, undoLen: 0 },
    currentFile: null,
    currentIndex: 0,
    selectionPivot: 0,
    lastNonUndoableAction: '',
    selectedIndices: [],
    sessionVersion: 1,
    updateStarredIndices: () => {},
    validateSelection: () => {},
    starredIndices: [],
  });

  syncService.init(appState as any);

  let badgeLabel = $derived((appState as any).v2.Photos['a.jpg']?.Label ?? 0);
</script>

{#if badgeLabel > 0}
  <div data-testid="label-badge">{badgeLabel}</div>
{/if}
