<script lang="ts">
  import { createEventDispatcher } from 'svelte';

  type ButtonVariant = 'primary' | 'secondary' | 'ghost';
  type ButtonSize = 'default' | 'compact';

  export let variant: ButtonVariant = 'primary';
  export let size: ButtonSize = 'default';
  export let type: 'button' | 'submit' | 'reset' = 'button';
  export let href: string | undefined = undefined;
  export let disabled = false;
  export let stretch = false;
  export let target: string | undefined = undefined;
  export let rel: string | undefined = undefined;
  export let loading = false;

  $: computedRel = rel ?? (target === '_blank' ? 'noopener noreferrer' : undefined);
  const dispatch = createEventDispatcher<{ click: MouseEvent }>();

  function emit(event: MouseEvent) {
    dispatch('click', event);
  }

  function handleAnchorClick(event: MouseEvent) {
    if (disabled) {
      event.preventDefault();
      event.stopPropagation();
      return;
    }
    emit(event);
  }

  function handleButtonClick(event: MouseEvent) {
    if (disabled) {
      event.preventDefault();
      return;
    }
    emit(event);
  }
</script>

{#if href}
  <a
    class={`ui-btn ui-btn--link ui-btn--${variant} ui-btn--${size} ${stretch ? 'ui-btn--stretch' : ''} ${loading ? 'ui-btn--loading' : ''}`}
    href={href}
    target={target}
    rel={computedRel}
    aria-disabled={disabled || loading}
    on:click={handleAnchorClick}
  >
    {#if loading}
      <span class="ui-btn__spinner" aria-hidden="true"></span>
    {/if}
    <slot />
  </a>
{:else}
  <button
    {type}
    class={`ui-btn ui-btn--${variant} ui-btn--${size} ${stretch ? 'ui-btn--stretch' : ''} ${loading ? 'ui-btn--loading' : ''}`}
    disabled={disabled || loading}
    on:click={handleButtonClick}
  >
    {#if loading}
      <span class="ui-btn__spinner" aria-hidden="true"></span>
    {/if}
    <slot />
  </button>
{/if}

<style>
  .ui-btn {
    font-family: var(--font-ui);
    font-size: 1rem;
    font-weight: 600;
    border-radius: var(--radius-pill);
    padding: 0.9rem 1.8rem;
    transition: box-shadow var(--motion-dur-fast) var(--motion-ease-emphasized),
      transform var(--motion-dur-fast) var(--motion-ease-standard),
      background var(--motion-dur-fast) var(--motion-ease-standard);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.35rem;
    border: 1px solid transparent;
    text-decoration: none;
  }

  .ui-btn--stretch {
    width: 100%;
  }

  .ui-btn__spinner {
    width: 1rem;
    height: 1rem;
    border-radius: 999px;
    border: 2px solid rgba(255, 255, 255, 0.6);
    border-top-color: rgba(255, 255, 255, 0.1);
    animation: ui-btn-spin 600ms linear infinite;
    margin-right: 0.5rem;
  }

  .ui-btn--secondary .ui-btn__spinner,
  .ui-btn--ghost .ui-btn__spinner {
    border-color: rgba(20, 24, 33, 0.45);
    border-top-color: transparent;
  }

  .ui-btn--loading {
    pointer-events: none;
  }

  @keyframes ui-btn-spin {
    from {
      transform: rotate(0deg);
    }
    to {
      transform: rotate(360deg);
    }
  }

  .ui-btn:disabled,
  .ui-btn[aria-disabled='true'] {
    opacity: 0.55;
    pointer-events: none;
  }

  .ui-btn--primary {
    color: rgb(var(--sys-on-accent));
    background: var(--btn-primary-bg);
    box-shadow: var(--btn-primary-shadow);
  }

  .ui-btn--primary:active {
    background: var(--btn-primary-bg-pressed);
    box-shadow: var(--btn-primary-shadow-pressed);
    transform: translateY(1px);
  }

  .ui-btn--secondary {
    color: var(--btn-secondary-fg);
    background: var(--btn-secondary-bg);
    border-color: var(--btn-secondary-outline);
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.45);
  }

  .ui-btn--ghost {
    color: rgb(var(--sys-ink));
    background: transparent;
    border-color: var(--btn-secondary-outline);
  }

  .ui-btn--compact {
    padding: 0.5rem 1.25rem;
    font-size: 0.9rem;
  }

  .ui-btn--link {
    text-decoration: none;
  }

  @media (hover: hover) and (pointer: fine) {
    .ui-btn--primary:hover {
      background: var(--btn-primary-bg-hover);
      box-shadow: var(--btn-primary-shadow-hover);
      transform: translateY(-1px);
    }

    .ui-btn--secondary:hover {
      background: var(--btn-secondary-hover-bg);
    }

    .ui-btn--ghost:hover {
      background: var(--btn-ghost-hover-bg);
    }
  }
</style>
