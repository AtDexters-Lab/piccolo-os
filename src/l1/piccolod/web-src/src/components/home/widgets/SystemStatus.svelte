<script lang="ts">
  import { onMount } from 'svelte';
  import { apiProd, type ErrorResponse } from '@api/client';

  type HealthComponent = {
    name?: string;
    level?: string;
    message?: string;
  };

  type HealthDetail = {
    overall?: string;
    components?: HealthComponent[];
  };

  const styles: Record<string, { label: string; badge: string }> = {
    healthy: { label: 'OK', badge: 'bg-state-ok/10 text-state-ok' },
    degraded: { label: 'Degraded', badge: 'bg-state-degraded/10 text-state-degraded' },
    unhealthy: { label: 'Critical', badge: 'bg-state-critical/10 text-state-critical' }
  };

  let status: keyof typeof styles | null = null;
  let summary = '';
  let loading = true;
  let error: string | null = null;

  function normalizeLevel(level?: string | null): keyof typeof styles {
    switch ((level ?? '').toLowerCase()) {
      case 'warn':
      case 'warning':
      case 'degraded':
        return 'degraded';
      case 'error':
      case 'critical':
      case 'fatal':
        return 'unhealthy';
      default:
        return 'healthy';
    }
  }

  function buildSummary(overall: string, components: HealthComponent[] = []): string {
    const normalized = overall.toLowerCase();
    if (normalized === 'ok' || normalized === 'healthy' || !components.length) {
      return 'All core services are responding.';
    }
    const match = components.find((component) => component.level?.toLowerCase() === normalized && component.message);
    if (match?.message) return match.message;
    const firstMessage = components.find((component) => !!component.message)?.message;
    if (firstMessage) return firstMessage;
    return `System reported ${normalized} status.`;
  }

  async function load() {
    loading = true;
    error = null;
    try {
      const res = await apiProd<HealthDetail>('/health/detail');
      const overall = res?.overall ?? 'ok';
      status = normalizeLevel(overall);
      summary = buildSummary(overall, res?.components);
    } catch (err: any) {
      const e = err as ErrorResponse;
      error = e?.message || 'Unable to load system status.';
      status = null;
    } finally {
      loading = false;
    }
  }

  function viewDetails() {
    window.location.hash = '/updates';
  }

  onMount(() => {
    load();
  });
</script>

<article data-testid="home-widget-system" class="rounded-2xl border border-border-subtle bg-surface-1 p-5 shadow-sm flex flex-col gap-3 min-h-[180px]">
  <header class="flex items-start justify-between gap-3">
    <div>
      <p class="text-xs uppercase tracking-[0.14em] text-text-muted">System</p>
      <h3 class="text-lg font-semibold text-text-primary">Health</h3>
    </div>
    {#if status}
      <span class={`px-2 py-1 rounded-full text-xs font-semibold ${styles[status].badge}`}>
        {styles[status].label}
      </span>
    {/if}
  </header>

  {#if loading}
    <div class="space-y-2">
      <div class="h-3 rounded-full bg-surface-2 animate-pulse"></div>
      <div class="h-3 rounded-full bg-surface-2 animate-pulse w-2/3"></div>
    </div>
  {:else if error}
    <div class="space-y-3">
      <p class="text-sm text-state-warn">{error}</p>
      <button class="inline-flex items-center gap-2 text-xs font-semibold text-accent-emphasis" on:click={load}>
        Retry
      </button>
    </div>
  {:else}
    <p class="text-sm text-text-muted leading-relaxed">{summary}</p>
    <button class="inline-flex items-center gap-2 text-xs font-semibold text-accent-emphasis" on:click={viewDetails}>
      View details
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" class="h-3.5 w-3.5">
        <path d="M7 17 17 7" />
        <path d="M8 7h9v9" />
      </svg>
    </button>
  {/if}
</article>
