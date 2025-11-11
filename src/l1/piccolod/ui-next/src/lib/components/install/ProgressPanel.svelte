<script lang="ts">
  export let notes: string[] = [];
  export let pending = false;
  export let complete = false;
</script>

<section class="progress-panel elev-2">
  <header class="flex items-center justify-between">
    <div>
      <p class="meta-label">Progress</p>
      <h3 class="text-lg font-semibold text-ink">{complete ? 'Install queued' : pending ? 'Writing image…' : 'Awaiting confirmation'}</h3>
    </div>
    <div class={`progress-panel__badge ${complete ? 'progress-panel__badge--success' : pending ? 'progress-panel__badge--pending' : ''}`} aria-hidden="true">
      {#if complete}
        ✓
      {:else if pending}
        <span class="animate-spin">⏳</span>
      {:else}
        ·
      {/if}
    </div>
  </header>
  <ul class="mt-4 space-y-2 text-sm text-ink">
    {#each notes as note}
      <li class="progress-panel__note">{note}</li>
    {/each}
    {#if pending}
      <li class="progress-panel__note">Device is writing the signed image…</li>
    {/if}
  </ul>
  {#if complete}
    <p class="mt-4 text-sm text-muted">Stay nearby—the device will reboot automatically when finished.</p>
  {/if}
</section>

<style>
  .progress-panel {
    border-radius: var(--radius-xl);
    border: 1px solid var(--card-border);
    background: var(--card-gradient);
    padding: 1.5rem;
    box-shadow: var(--shadow-strong);
  }

  .progress-panel__badge {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 2.5rem;
    width: 2.5rem;
    border-radius: var(--radius-lg);
    background: rgba(255, 255, 255, 0.85);
    color: rgb(var(--sys-ink-muted));
    font-weight: 600;
  }

  .progress-panel__badge--pending {
    background: var(--btn-secondary-hover-bg);
    color: var(--sys-link);
  }

  .progress-panel__badge--success {
    background: var(--chip-info-bg);
    color: rgb(var(--sys-success));
  }

  .progress-panel__note {
    border-radius: var(--radius-lg);
    border: 1px solid var(--card-border);
    background: rgba(255, 255, 255, 0.9);
    padding: 0.9rem 1rem;
  }

  :global([data-theme='dark']) .progress-panel__note {
    background: rgba(5, 7, 16, 0.6);
  }
</style>
