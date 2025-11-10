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
    class={`ui-btn ui-btn--link ui-btn--${variant} ui-btn--${size} ${stretch ? 'ui-btn--stretch' : ''}`}
    href={href}
    target={target}
    rel={computedRel}
    aria-disabled={disabled}
    on:click={handleAnchorClick}
  >
    <slot />
  </a>
{:else}
  <button
    {type}
    class={`ui-btn ui-btn--${variant} ui-btn--${size} ${stretch ? 'ui-btn--stretch' : ''}`}
    disabled={disabled}
    on:click={handleButtonClick}
  >
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

  .ui-btn:disabled,
  .ui-btn[aria-disabled='true'] {
    opacity: 0.55;
    pointer-events: none;
  }

  .ui-btn--primary {
    color: rgb(var(--sys-on-accent));
    background: linear-gradient(135deg, rgb(var(--sys-accent)), rgb(var(--sys-accent-hero)));
    box-shadow: 0 18px 35px rgb(var(--sys-accent) / 0.3);
  }

  .ui-btn--primary:hover {
    box-shadow: 0 25px 45px rgb(var(--sys-accent) / 0.35);
    transform: translateY(-1px);
  }

  .ui-btn--primary:active {
    box-shadow: 0 10px 25px rgb(var(--sys-accent) / 0.3);
    transform: translateY(1px);
  }

  .ui-btn--secondary {
    color: rgb(var(--sys-ink));
    background: rgb(var(--sys-surface-variant));
    border-color: rgb(var(--sys-ink) / 0.08);
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.45);
  }

  .ui-btn--secondary:hover {
    background: rgb(var(--sys-surface-muted));
  }

  .ui-btn--ghost {
    color: rgb(var(--sys-ink));
    background: transparent;
    border-color: rgb(var(--sys-ink) / 0.18);
  }

  .ui-btn--ghost:hover {
    background: rgb(var(--sys-surface-muted) / 0.5);
  }

  .ui-btn--compact {
    padding: 0.5rem 1.25rem;
    font-size: 0.9rem;
  }

  .ui-btn--link {
    text-decoration: none;
  }
</style>
