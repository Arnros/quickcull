<script lang="ts">
  import Icon from './Icon.svelte';
  import { i18n } from '../i18n.svelte';

  let {
    isStarred = false,
    isSelected = false,
    label = 0,
    burstIndex = 0,
    burstCount = 0,
    role = 'none' // 'none', 'reference', 'active'
  } = $props<{
    isStarred?: boolean;
    isSelected?: boolean;
    label?: number;
    burstIndex?: number;
    burstCount?: number;
    role?: 'none' | 'reference' | 'active';
  }>();

</script>

{#if role === 'reference'}
  <div class="role-badge reference" title={i18n.t('reference')}>R</div>
{/if}
{#if role === 'active'}
  <div class="role-badge active" title={i18n.t('active')}>A</div>
{/if}

{#if label > 0}
  <div class="label-badge-indicator" style="background-color: var(--label-{label})"></div>
{/if}

{#if isStarred}
  <div class="star-badge">
    <Icon name="star" size={12} class="star-badge-icon" />
  </div>
{/if}

{#if isSelected}
  <div class="selection-badge small">✓</div>
{/if}

{#if burstCount > 0}
  <div class="burst-badge">
    {burstIndex}/{burstCount}
  </div>
{/if}

<style>
  .role-badge {
    position: absolute;
    top: 2px;
    left: 2px;
    width: 14px;
    height: 14px;
    border-radius: 2px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-weight: 800;
    font-size: 9px;
    z-index: var(--z-badge);
    box-shadow: 0 2px 4px rgba(0, 0, 0, 0.3);
    color: #fff;
  }
  .role-badge.reference {
    background: var(--text-muted);
  }
  .role-badge.active {
    background: var(--accent);
    color: var(--on-accent);
  }

  .label-badge-indicator {
    position: absolute;
    bottom: 2px;
    left: 2px;
    width: 10px;
    height: 10px;
    border-radius: var(--radius-full);
    z-index: var(--z-badge);
    box-shadow: 0 2px 4px rgba(0, 0, 0, 0.4);
    border: 1px solid var(--dot-border);
  }

  .star-badge {
    position: absolute;
    top: 8px;
    left: 8px;
    background: var(--star);
    color: var(--on-star);
    width: 22px;
    height: 22px;
    border-radius: var(--radius-full);
    display: flex;
    align-items: center;
    justify-content: center;
    font-weight: 700;
    z-index: var(--z-badge);
    border: 1px solid rgba(0, 0, 0, 0.2);
    box-shadow: var(--shadow-badge);
  }
  /* Smaller variant for filmstrip context via CSS cascade or props if needed, but let's just make it standard or inherit */
  :global(.filmstrip) .star-badge {
    top: 2px;
    left: 2px;
    width: 14px;
    height: 14px;
  }
  :global(.filmstrip) .star-badge :global(.star-badge-icon) {
    width: 8px !important;
    height: 8px !important;
  }

  .selection-badge {
    position: absolute;
    top: 8px;
    right: 8px;
    background: var(--accent);
    color: var(--on-accent);
    width: 22px;
    height: 22px;
    border-radius: var(--radius-full);
    display: flex;
    align-items: center;
    justify-content: center;
    font-weight: 800;
    font-size: var(--text-base);
    z-index: var(--z-badge);
    box-shadow: var(--shadow-badge);
    border: 2px solid #fff;
  }
  .selection-badge.small {
    top: 2px;
    right: 2px;
    width: 14px;
    height: 14px;
    font-size: 9px;
    border-width: 1px;
    box-shadow: 0 2px 4px rgba(0, 0, 0, 0.5);
  }

  .burst-badge {
    position: absolute;
    bottom: 2px;
    right: 2px;
    background: var(--success);
    color: white;
    font-size: 8px;
    padding: 1px 3px;
    border-radius: 3px;
    font-weight: bold;
    pointer-events: none;
    z-index: var(--z-badge);
  }
</style>
