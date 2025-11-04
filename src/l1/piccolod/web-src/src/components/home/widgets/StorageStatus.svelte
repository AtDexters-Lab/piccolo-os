<script lang="ts">
  import { onMount } from 'svelte';
  import { apiProd, type ErrorResponse } from '@api/client';

  type Disk = {
    name?: string;
    size_bytes?: number;
    used_bytes?: number;
    status?: string;
    parity?: string | null;
  };

  type StorageResponse = {
    disks?: Disk[];
  };

  let loading = true;
  let error: string | null = null;
  let disks: Disk[] = [];

  const statusMap: Record<string, { label: string; badge: string }> = {
    healthy: { label: 'OK', badge: 'bg-state-ok/10 text-state-ok' },
    warning: { label: 'Attention', badge: 'bg-state-warn/10 text-state-warn' },
    degraded: { label: 'Degraded', badge: 'bg-state-degraded/10 text-state-degraded' }
  };

  $: summary = computeSummary(disks);
  $: badge = summary.badge;

  function computeSummary(items: Disk[]) {
    if (!items.length) {
      return {
        label: 'No disks detected',
        detail: 'Add storage to start deploying services.',
        badge: statusMap.warning
      };
    }
    const total = items.reduce((acc, disk) => acc + (disk.size_bytes ?? 0), 0);
    const used = items.reduce((acc, disk) => acc + (disk.used_bytes ?? 0), 0);
    const unhealthy = items.filter((disk) => (disk.status ?? '').toLowerCase() !== '' && (disk.status ?? '').toLowerCase() !== 'healthy');
    const percent = total > 0 ? Math.round((used / total) * 100) : 0;
    return {
      label: `${items.length} disk${items.length > 1 ? 's' : ''} • ${formatBytes(used)} used of ${formatBytes(total)}`,
      detail: unhealthy.length ? `${unhealthy.length} needs attention` : 'All volumes online',
      badge: unhealthy.length ? statusMap.degraded : statusMap.healthy,
      percent
    };
  }

  async function load() {
    loading = true;
    error = null;
    try {
      const res = await apiProd<StorageResponse>('/storage/disks');
      disks = res?.disks ?? [];
    } catch (err: any) {
      const e = err as ErrorResponse;
      error = e?.message || 'Unable to load storage.';
      disks = [];
    } finally {
      loading = false;
    }
  }

  function formatBytes(value: number) {
    if (!value) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    let idx = 0;
    let result = value;
    while (result >= 1024 && idx < units.length - 1) {
      result /= 1024;
      idx += 1;
    }
    return `${result.toFixed(result >= 10 ? 0 : 1)} ${units[idx]}`;
  }

  function gotoStorage() {
    window.location.hash = '/storage';
  }

  onMount(() => {
    load();
  });
</script>

<article data-testid="home-widget-storage" class="rounded-2xl border border-border-subtle bg-surface-1 p-5 shadow-sm flex flex-col gap-3 min-h-[180px]">
  <header class="flex items-start justify-between gap-3">
    <div>
      <p class="text-xs uppercase tracking-[0.14em] text-text-muted">Storage</p>
      <h3 class="text-lg font-semibold text-text-primary">Capacity</h3>
    </div>
    {#if badge}
      <span class={`px-2 py-1 rounded-full text-xs font-semibold ${badge.badge}`}>
        {badge.label}
      </span>
    {/if}
  </header>

  {#if loading}
    <div class="space-y-2">
      <div class="h-3 rounded-full bg-surface-2 animate-pulse"></div>
      <div class="h-3 rounded-full bg-surface-2 animate-pulse w-3/4"></div>
    </div>
  {:else if error}
    <div class="space-y-3">
      <p class="text-sm text-state-warn">{error}</p>
      <button class="inline-flex items-center gap-2 text-xs font-semibold text-accent-emphasis" on:click={load}>
        Retry
      </button>
    </div>
  {:else}
    <div class="space-y-3">
      <p class="text-sm text-text-primary font-semibold">{summary.label}</p>
      <p class="text-xs text-text-muted">{summary.detail}</p>
      {#if summary.percent !== undefined}
        <div class="h-2 rounded-full bg-surface-2 overflow-hidden">
          <div class="h-full bg-accent" style={`width: ${summary.percent}%`}></div>
        </div>
      {/if}
    </div>
    <button class="inline-flex items-center gap-2 text-xs font-semibold text-accent-emphasis" on:click={gotoStorage}>
      Manage storage
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" class="h-3.5 w-3.5">
        <path d="M7 17 17 7" />
        <path d="M8 7h9v9" />
      </svg>
    </button>
  {/if}
</article>
