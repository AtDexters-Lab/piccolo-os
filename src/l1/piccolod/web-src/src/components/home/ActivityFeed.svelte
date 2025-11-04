<script lang="ts">
  import { onMount } from 'svelte';
  import { activityStore, refreshActivity } from '@stores/activity';

  let loading = true;
  let error: string | null = null;
  let items = [];
  let unsupported = false;

  $: activities = $activityStore;
  $: loading = activities.loading;
  $: error = activities.error;
  $: items = activities.items ?? [];
  $: unsupported = activities.unsupported;

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

  function formatTime(ts?: string) {
    if (!ts) return '';
    const d = new Date(ts);
    if (Number.isNaN(d.getTime())) return '';
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }

  onMount(() => {
    if (!activities.items.length && !activities.loading) {
      refreshActivity();
    }
  });

  function openTray() {
    window.dispatchEvent(new CustomEvent('piccolo-open-activity'));
  }
</script>

<section class="rounded-2xl border border-border-subtle bg-surface-1 p-5 shadow-sm flex flex-col gap-3">
  <header class="flex items-center justify-between">
    <div>
      <p class="text-xs uppercase tracking-[0.14em] text-text-muted">Activity</p>
      <h3 class="text-lg font-semibold text-text-primary">Latest tasks</h3>
    </div>
    <button class="text-xs font-semibold text-accent-emphasis" on:click={openTray}>
      Open tray
    </button>
  </header>

  {#if loading}
    <div class="space-y-2">
      {#each Array(3) as _}
        <div class="h-4 rounded bg-surface-2 animate-pulse"></div>
      {/each}
    </div>
  {:else if error}
    <div class="space-y-3">
      <p class="text-sm text-state-warn">{error}</p>
      <button class="inline-flex items-center gap-2 text-xs font-semibold text-accent-emphasis" on:click={refreshActivity}>
        Retry
      </button>
    </div>
  {:else if unsupported}
    <p class="text-sm text-text-muted">Activity feed arrives with the orchestration telemetry update. Until then, recent tasks appear in logs.</p>
  {:else if items.length}
    <ul class="space-y-3">
      {#each items.slice(0, 3) as item}
        <li class="flex items-start justify-between gap-3">
          <div>
            <p class="text-sm text-text-primary font-medium">{item.title}</p>
            {#if item.detail}
              <p class="text-xs text-text-muted">{item.detail}</p>
            {/if}
          </div>
          <div class="flex flex-col items-end gap-1">
            {#if item.status}
              <span class={`px-2 py-0.5 rounded-full text-[11px] font-semibold ${badgeClass(item.status)}`}>{item.status}</span>
            {/if}
            {#if item.ts}
              <span class="text-[11px] text-text-muted">{formatTime(item.ts)}</span>
            {/if}
          </div>
        </li>
      {/each}
    </ul>
  {:else}
    <p class="text-sm text-text-muted">No recent tasks. Deploy or update a service to see activity here.</p>
  {/if}
</section>
