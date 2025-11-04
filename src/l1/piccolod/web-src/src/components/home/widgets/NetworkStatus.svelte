<script lang="ts">
  import { onMount } from 'svelte';
  import { apiProd, type ErrorResponse } from '@api/client';

  type NetworkSummary = {
    primary_ip?: string | null;
    mdns?: string | null;
    dns_status?: string | null;
    remote_reachable?: boolean;
    last_checked?: string | null;
  };

  let data: NetworkSummary | null = null;
  let loading = true;
  let error: string | null = null;
  let unsupported = false;

  async function load() {
    loading = true;
    error = null;
    unsupported = false;
    try {
      const res = await apiProd<NetworkSummary>('/network/status');
      data = res ?? null;
    } catch (err: any) {
      const e = err as ErrorResponse;
      if (e?.code === 404) {
        unsupported = true;
        data = null;
      } else {
        error = e?.message || 'Unable to load network status.';
      }
    } finally {
      loading = false;
    }
  }

  function gotoDetails() {
    window.location.hash = '/remote';
  }

  onMount(() => {
    load();
  });

  function formatDate(ts?: string | null): string {
    if (!ts) return '';
    const d = new Date(ts);
    if (Number.isNaN(d.getTime())) return '';
    return d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
  }
</script>

<article data-testid="home-widget-network" class="rounded-2xl border border-border-subtle bg-surface-1 p-5 shadow-sm flex flex-col gap-3 min-h-[180px]">
  <header class="flex items-start justify-between gap-3">
    <div>
      <p class="text-xs uppercase tracking-[0.14em] text-text-muted">Network</p>
      <h3 class="text-lg font-semibold text-text-primary">Reachability</h3>
    </div>
  </header>

  {#if loading}
    <div class="space-y-2">
      <div class="h-3 rounded-full bg-surface-2 animate-pulse"></div>
      <div class="h-3 rounded-full bg-surface-2 animate-pulse w-1/2"></div>
    </div>
  {:else if error}
    <div class="space-y-3">
      <p class="text-sm text-state-warn">{error}</p>
      <button class="inline-flex items-center gap-2 text-xs font-semibold text-accent-emphasis" on:click={load}>
        Retry
      </button>
    </div>
  {:else if unsupported}
    <div class="space-y-3">
      <p class="text-sm text-text-muted">Network diagnostics will appear here once the backend exposes the summary endpoint.</p>
      <button class="inline-flex items-center gap-2 text-xs font-semibold text-accent-emphasis" on:click={gotoDetails}>
        Troubleshooting guide
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" class="h-3.5 w-3.5">
          <path d="M7 17 17 7" />
          <path d="M8 7h9v9" />
        </svg>
      </button>
    </div>
  {:else if data}
    <div class="space-y-3">
      <div class="flex items-center justify-between text-sm text-text-primary">
        <span class="text-xs uppercase tracking-[0.14em] text-text-muted">Primary IP</span>
        <span class="font-mono text-sm">{data.primary_ip ?? '—'}</span>
      </div>
      <div class="flex items-center justify-between text-sm text-text-primary">
        <span class="text-xs uppercase tracking-[0.14em] text-text-muted">mDNS</span>
        <span class="font-mono text-sm">{data.mdns ?? 'piccolo.local'}</span>
      </div>
      <div class="flex items-center justify-between text-sm text-text-primary">
        <span class="text-xs uppercase tracking-[0.14em] text-text-muted">Remote</span>
        <span class={`inline-flex items-center gap-2 text-xs font-semibold px-2 py-0.5 rounded-full ${data.remote_reachable ? 'bg-state-ok/10 text-state-ok' : 'bg-state-notice/10 text-state-notice'}`}>
          {data.remote_reachable ? 'Reachable' : 'LAN only'}
        </span>
      </div>
      {#if data.last_checked}
        <p class="text-[11px] text-text-muted">Last checked {formatDate(data.last_checked)}</p>
      {/if}
    </div>
    <button class="inline-flex items-center gap-2 text-xs font-semibold text-accent-emphasis" on:click={gotoDetails}>
      Troubleshoot
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" class="h-3.5 w-3.5">
        <path d="M7 17 17 7" />
        <path d="M8 7h9v9" />
      </svg>
    </button>
  {:else}
    <p class="text-sm text-text-muted">No network data available yet.</p>
  {/if}
</article>
