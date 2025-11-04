<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte';
  import { activityStore, refreshActivity } from '@stores/activity';

  export let open = false;
  export let onClose: () => void = () => {};

  $: state = $activityStore;
  $: loading = state.loading;
  $: error = state.error;
  $: items = state.items ?? [];
  $: unsupported = state.unsupported;

  let panelEl: HTMLElement | null = null;
  let previousFocused: HTMLElement | null = null;
  let keydownHandler: ((event: KeyboardEvent) => void) | null = null;

  onMount(() => {
    if (!state.items.length && !state.loading) {
      refreshActivity();
    }
  });

  $: if (open) {
    handleOpened();
  } else {
    removeKeydown();
  }

  async function handleOpened() {
    previousFocused = (document.activeElement as HTMLElement) ?? null;
    await tick();
    panelEl?.focus({ preventScroll: true });
    if (!keydownHandler) {
      keydownHandler = (event: KeyboardEvent) => {
        if (event.key === 'Escape') {
          event.stopPropagation();
          onClose();
        }
      };
      window.addEventListener('keydown', keydownHandler, { capture: true });
    }
  }

  function handleClose() {
    onClose();
    if (previousFocused) {
      previousFocused.focus({ preventScroll: true });
      previousFocused = null;
    }
    removeKeydown();
  }

  function removeKeydown() {
    if (keydownHandler) {
      window.removeEventListener('keydown', keydownHandler, { capture: true });
      keydownHandler = null;
    }
  }

  onDestroy(() => {
    removeKeydown();
  });

  function badgeClass(status?: string) {
    switch (status) {
      case 'success':
        return 'bg-state-ok/10 text-state-ok';
      case 'warning':
        return 'bg-state-warn/10 text-state-warn';
      case 'error':
        return 'bg-state-critical/10 text-state-critical';
      case 'running':
        return 'bg-accent-subtle text-accent-emphasis';
      default:
        return 'bg-surface-2 text-text-muted';
    }
  }

  function formatDate(ts?: string) {
    if (!ts) return '';
    const date = new Date(ts);
    if (Number.isNaN(date.getTime())) return '';
    return date.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
  }
</script>

{#if open}
  <div class="activity-overlay" role="dialog" aria-modal="true" aria-labelledby="activity-tray-title">
    <div class="activity-overlay__scrim" on:click={handleClose} aria-hidden="true"></div>
    <section class="activity-overlay__panel" role="document" bind:this={panelEl} tabindex="-1">
      <header class="flex items-center justify-between gap-3 border-b border-border-subtle px-5 py-4">
        <div>
          <h2 id="activity-tray-title" class="text-base font-semibold text-text-primary">Activity</h2>
          <p class="text-xs text-text-muted">Recent tasks, updates, and automation.</p>
        </div>
        <button class="h-9 w-9 rounded-full border border-border-subtle text-text-muted hover:bg-surface-2" on:click={handleClose} aria-label="Close activity" type="button">
          ✕
        </button>
      </header>

      <div class="flex-1 overflow-y-auto px-5 py-4 space-y-4">
        {#if loading}
          <div class="space-y-2">
            {#each Array(5) as _}
              <div class="h-4 rounded bg-surface-2 animate-pulse"></div>
            {/each}
          </div>
        {:else if error}
          <div class="space-y-3">
            <p class="text-sm text-state-warn">{error}</p>
            <button class="px-4 py-2 rounded-xl border border-border-subtle text-xs font-semibold" on:click={refreshActivity}>Retry</button>
          </div>
        {:else if unsupported}
          <p class="text-sm text-text-muted">Activity history will appear once orchestration telemetry ships for this build.</p>
        {:else if items.length === 0}
          <p class="text-sm text-text-muted">No recent tasks yet. Deploy or update a service to track progress here.</p>
        {:else}
          <ul class="space-y-3">
            {#each items as item}
              <li class="rounded-xl border border-border-subtle p-3 flex flex-col gap-2">
                <div class="flex items-start justify-between gap-3">
                  <div>
                    <p class="text-sm font-semibold text-text-primary">{item.title}</p>
                    {#if item.detail}
                      <p class="text-xs text-text-muted">{item.detail}</p>
                    {/if}
                  </div>
                  {#if item.status}
                    <span class={`px-2 py-0.5 rounded-full text-[11px] font-semibold ${badgeClass(item.status)}`}>{item.status}</span>
                  {/if}
                </div>
                {#if item.ts}
                  <p class="text-[11px] text-text-muted">{formatDate(item.ts)}</p>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
      </div>
      <footer class="border-t border-border-subtle px-5 py-3 flex items-center justify-between text-xs text-text-muted">
        <span>Items auto-refresh every few minutes.</span>
        <button class="px-3 py-1.5 rounded-lg border border-border-subtle font-semibold" on:click={refreshActivity} type="button">Refresh now</button>
      </footer>
    </section>
  </div>
{/if}

<style>
  .activity-overlay {
    position: fixed;
    inset: 0;
    z-index: 80;
    display: flex;
    align-items: flex-end;
    justify-content: center;
    pointer-events: none;
  }

  .activity-overlay__scrim {
    position: absolute;
    inset: 0;
    background: var(--scrim-soft);
    pointer-events: auto;
  }

  .activity-overlay__panel {
    pointer-events: auto;
    width: min(640px, 100%);
    max-height: 90vh;
    background: var(--surface-1);
    border-radius: 24px 24px 0 0;
    border: 1px solid var(--border-subtle);
    box-shadow: 0 -18px 40px rgba(15, 23, 42, 0.28);
    display: flex;
    flex-direction: column;
    transform: translateY(0);
    animation: tray-up var(--transition-duration) var(--transition-easing);
  }

  @media (min-width: 1024px) {
    .activity-overlay {
      align-items: center;
    }

    .activity-overlay__panel {
      border-radius: 24px;
      max-height: 80vh;
    }
  }

  @keyframes tray-up {
    from {
      transform: translateY(24px);
      opacity: 0;
    }
    to {
      transform: translateY(0);
      opacity: 1;
    }
  }
</style>
