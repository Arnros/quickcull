<script lang="ts">
  import Icon from './Icon.svelte';
  import { type Snippet } from 'svelte';

  let {
    type = 'button',
    variant = 'secondary', // 'primary', 'secondary', 'danger', 'ghost', 'mini'
    icon,
    onclick,
    disabled = false,
    title,
    children,
    class: className = '',
    ariaLabel,
    active = false,
  } = $props<{
    type?: 'button' | 'submit';
    variant?: 'primary' | 'secondary' | 'danger' | 'ghost' | 'mini' | 'action';
    icon?: string;
    onclick?: (e: MouseEvent) => void;
    disabled?: boolean;
    title?: string;
    children?: Snippet;
    class?: string;
    ariaLabel?: string;
    active?: boolean;
  }>();
</script>

<button
  {type}
  class="btn {variant} {className}"
  class:active
  {disabled}
  {title}
  aria-label={ariaLabel || title}
  {onclick}
>
  {#if icon}
    <Icon name={icon} size={variant === 'mini' ? 14 : 18} />
  {/if}
  {#if children}
    <span>{@render children()}</span>
  {/if}
</button>

<style>
  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-4);
    border-radius: var(--radius-lg);
    font-size: var(--text-base);
    font-weight: 700;
    letter-spacing: 0.02em;
    cursor: pointer;
    transition: all var(--transition-base) var(--easing);
    border: 1px solid var(--border-main);
    white-space: nowrap;
    outline: none;
    box-shadow: 0 1px 0 rgba(255, 255, 255, 0.04) inset;
  }

  .btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .btn:focus-visible {
    outline: none;
    box-shadow: 0 0 0 3px rgba(var(--accent-rgb), 0.25);
    border-color: rgba(var(--accent-rgb), 0.8);
  }

  /* Variants */
  .primary {
    background: var(--accent);
    color: var(--on-accent);
    border-color: rgba(var(--accent-rgb), 0.7);
  }
  .primary:hover:not(:disabled) {
    background: var(--accent-hover);
    box-shadow: 0 8px 22px rgba(var(--accent-rgb), 0.35);
    transform: translateY(-1px);
  }

  .secondary {
    background: color-mix(in srgb, var(--bg-surface) 78%, transparent);
    border-color: var(--border-main);
    color: var(--text-main);
  }
  .secondary:hover:not(:disabled) {
    background: var(--bg-inset);
    border-color: rgba(var(--accent-rgb), 0.45);
  }

  .action {
    background: var(--bg-app);
    border-color: var(--border-main);
    color: var(--text-main);
  }
  .action:hover:not(:disabled) {
    background: var(--bg-inset);
    border-color: rgba(var(--accent-rgb), 0.4);
    transform: translateY(-1px);
  }

  .danger {
    background: rgba(239, 68, 68, 0.08);
    border-color: rgba(239, 68, 68, 0.35);
    color: var(--danger);
  }
  .danger:hover:not(:disabled) {
    background: rgba(239, 68, 68, 0.14);
    border-color: rgba(239, 68, 68, 0.55);
  }

  .ghost {
    background: transparent;
    color: var(--text-muted);
    border-color: transparent;
  }
  .ghost:hover:not(:disabled) {
    color: var(--text-main);
    background: var(--bg-inset);
    border-color: var(--border-main);
  }

  .mini {
    padding: var(--space-1) var(--space-2);
    font-size: var(--text-caption);
    border-radius: var(--radius-md);
    background: color-mix(in srgb, var(--bg-app) 88%, transparent);
    border-color: var(--border-main);
    color: var(--text-muted);
  }
  .mini:hover:not(:disabled) {
    border-color: rgba(var(--accent-rgb), 0.6);
    color: var(--text-main);
  }
  .mini.active {
    background: var(--accent);
    color: var(--on-accent);
    border-color: rgba(var(--accent-rgb), 0.8);
  }

  .active {
    border-color: rgba(var(--accent-rgb), 0.7);
    background: rgba(var(--accent-rgb), 0.12);
  }
</style>
