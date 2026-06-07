<script lang="ts">
  import { type Snippet } from 'svelte';

  let {
    isOpen = false,
    onClose,
    children,
    width = '600px',
    padding = 'var(--space-6)',
    ariaLabel = 'modal',
  } = $props<{
    isOpen: boolean;
    onClose: () => void;
    children: Snippet;
    width?: string;
    padding?: string;
    ariaLabel?: string;
  }>();

  let modalEl = $state<HTMLElement | null>(null);

  function handleKeydown(e: KeyboardEvent) {
    if (!isOpen) return;
    
    if (e.key === "Escape") {
      // Small delay to allow nested components to handle Escape first if needed
      setTimeout(() => {
        if (isOpen) onClose();
      }, 0);
      return;
    }

    if (e.key === "Tab" && modalEl) {
      const focusableElements = modalEl.querySelectorAll(
        'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
      );
      const firstElement = focusableElements[0] as HTMLElement;
      const lastElement = focusableElements[focusableElements.length - 1] as HTMLElement;

      if (e.shiftKey) {
        if (document.activeElement === firstElement) {
          lastElement.focus();
          e.preventDefault();
        }
      } else {
        if (document.activeElement === lastElement) {
          firstElement.focus();
          e.preventDefault();
        }
      }
    }
  }

  $effect(() => {
    if (isOpen && modalEl) {
      const firstFocusable = modalEl.querySelector(
        'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
      ) as HTMLElement;
      firstFocusable?.focus();
    }
  });
</script>

<svelte:window onkeydown={handleKeydown} />

{#if isOpen}
  <div
    class="overlay"
    role="presentation"
    onclick={onClose}
  >
    <div
      bind:this={modalEl}
      class="modal"
      role="dialog"
      aria-modal="true"
      aria-label={ariaLabel}
      tabindex="-1"
      style:max-width={width}
      style:padding={padding}
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
    >
      {@render children()}
    </div>
  </div>
{/if}

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background:
      radial-gradient(circle at 12% 10%, rgba(var(--accent-rgb), 0.14), transparent 36%),
      radial-gradient(circle at 88% 88%, rgba(var(--accent-rgb), 0.1), transparent 34%),
      var(--overlay-bg);
    backdrop-filter: var(--blur-md);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: var(--z-modal);
    animation: fade-in var(--transition-base) ease-out;
  }

  .modal {
    background: linear-gradient(165deg, rgba(var(--accent-rgb), 0.07), transparent 42%), var(--bg-surface);
    border: 1px solid var(--border-main);
    border-radius: var(--radius-2xl);
    box-shadow: var(--shadow-modal);
    width: 90%;
    max-height: 85vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    animation: slide-up var(--transition-base) cubic-bezier(0.16, 1, 0.3, 1);
  }

  @keyframes slide-up {
    from {
      opacity: 0;
      transform: translateY(20px) scale(0.98);
    }
    to {
      opacity: 1;
      transform: translateY(0) scale(1);
    }
  }
</style>
