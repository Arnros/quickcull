<script lang="ts">
  let { key, variant = 'default' } = $props<{
    key: string;
    variant?: 'default' | 'accent' | 'outline';
  }>();

  const displayKey = $derived.by(() => {
    const k = key.toLowerCase();
    if (k === ' ') return 'Space';
    if (k === 'arrowleft') return '←';
    if (k === 'arrowright') return '→';
    if (k === 'arrowup') return '↑';
    if (k === 'arrowdown') return '↓';
    if (k === 'escape') return 'Esc';
    if (k === 'tab') return 'Tab';
    if (k === 'backspace') return '⌫';
    if (k === 'enter') return '↵';
    return key.toUpperCase();
  });
</script>

<kbd class="kbd {variant}">
  {displayKey}
</kbd>

<style>
  .kbd {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 1.8em;
    height: 1.8em;
    padding: 0 6px;
    font-family: var(--mono);
    font-size: 0.72em;
    font-weight: 700;
    line-height: 1;
    border-radius: 5px;
    user-select: none;
    transition: all var(--transition-fast) var(--easing);
    letter-spacing: 0.02em;
  }

  .default {
    background: color-mix(in srgb, var(--key-bg) 92%, transparent);
    color: var(--text-main);
    border: 1px solid var(--key-border);
    box-shadow: 0 1px 0 var(--key-border), inset 0 -1px 0 rgba(0, 0, 0, 0.08);
  }

  .accent {
    background: color-mix(in srgb, var(--accent-dim) 82%, transparent);
    color: var(--accent);
    border: 1px solid rgba(var(--accent-rgb), 0.4);
    box-shadow: 0 1px 0 rgba(var(--accent-rgb), 0.32), inset 0 -1px 0 rgba(var(--accent-rgb), 0.18);
  }

  .outline {
    background: transparent;
    color: color-mix(in srgb, var(--accent) 88%, var(--text-main));
    border: 1px solid rgba(var(--accent-rgb), 0.4);
    font-size: 0.65em;
    opacity: 0.9;
  }
</style>
