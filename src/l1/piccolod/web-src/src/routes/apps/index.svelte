<script lang="ts">
  import { onMount } from 'svelte';
  import { apiProd } from '@api/client';
  import { toast } from '@stores/ui';

  type AppRecord = {
    name: string;
    image?: string;
    status?: string;
    pinned?: boolean;
    public_port?: number;
    local_url?: string | null;
  };

  type AppsResponse = {
    data?: AppRecord[];
  };

  let apps: AppRecord[] = [];
  let loading = true;
  let error = '';
  let filter = '';
  let showRunningOnly = false;
  let workingApp: { name: string; action: 'start' | 'stop' } | null = null;

  onMount(() => {
    load();
  });

  async function load() {
    loading = true;
    error = '';
    try {
      const res = await apiProd<AppsResponse>('/apps');
      apps = res?.data ?? [];
    } catch (err: any) {
      error = err?.message || 'Failed to load apps';
      apps = [];
    } finally {
      loading = false;
    }
  }

  function normalizedStatus(app: AppRecord): string {
    return (app.status ?? '').toLowerCase();
  }

  function isRunning(app: AppRecord): boolean {
    return normalizedStatus(app) === 'running';
  }

  function isWorking(name: string, action: 'start' | 'stop'): boolean {
    return workingApp?.name === name && workingApp?.action === action;
  }

  async function start(name: string) {
    workingApp = { name, action: 'start' };
    try {
      await apiProd(`/apps/${encodeURIComponent(name)}/start`, { method: 'POST' });
      toast(`Started ${name}`, 'success');
      await load();
    } catch (err: any) {
      toast(err?.message || 'Start failed', 'error');
    } finally {
      workingApp = null;
    }
  }

  async function stop(name: string) {
    workingApp = { name, action: 'stop' };
    try {
      await apiProd(`/apps/${encodeURIComponent(name)}/stop`, { method: 'POST' });
      toast(`Stopped ${name}`, 'success');
      await load();
    } catch (err: any) {
      toast(err?.message || 'Stop failed', 'error');
    } finally {
      workingApp = null;
    }
  }

  function badgeFor(app: AppRecord) {
    const status = normalizedStatus(app);
    if (status === 'running') return { label: 'Running', badge: 'bg-state-ok/10 text-state-ok' };
    if (status === 'stopped') return { label: 'Stopped', badge: 'bg-state-notice/10 text-state-notice' };
    if (status === 'error') return { label: 'Error', badge: 'bg-state-critical/10 text-state-critical' };
    return { label: app.status ?? 'Unknown', badge: 'bg-surface-2 text-text-muted' };
  }

  function matchesFilter(app: AppRecord): boolean {
    if (!filter.trim()) return true;
    const query = filter.trim().toLowerCase();
    return app.name?.toLowerCase().includes(query) || app.image?.toLowerCase().includes(query);
  }

  $: filteredApps = apps.filter((app) => matchesFilter(app) && (!showRunningOnly || isRunning(app)));
  $: totalApps = apps.length;
  $: runningApps = apps.filter(isRunning).length;

  function openCatalog() {
    window.location.hash = '/apps/catalog';
  }

  function openDetails(app: AppRecord) {
    window.location.hash = `/apps/${encodeURIComponent(app.name)}`;
  }

  function openApp(app: AppRecord) {
    if (app.local_url) {
      window.open(app.local_url, '_blank', 'noopener');
    } else {
      openDetails(app);
    }
  }
</script>

<section class="space-y-6">
  <header class="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
    <div>
      <h2 class="text-2xl font-semibold text-text-primary">Apps</h2>
      <p class="text-sm text-text-muted">{runningApps} running • {totalApps} installed</p>
    </div>
    <div class="flex items-center gap-2">
      <button class="px-4 py-2 rounded-xl border border-border-subtle text-xs font-semibold" on:click={load} disabled={loading}>
        {loading ? 'Refreshing…' : 'Refresh'}
      </button>
      <button class="px-4 py-2 rounded-xl bg-accent text-text-inverse text-xs font-semibold" on:click={openCatalog}>
        Browse catalog
      </button>
    </div>
  </header>

  <div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
    <div class="flex items-center gap-2 w-full md:max-w-sm">
      <label class="sr-only" for="apps-search">Search apps</label>
      <input id="apps-search" class="flex-1 rounded-xl border border-border-subtle bg-surface-1 px-4 py-2 text-sm" placeholder="Search apps" bind:value={filter} />
    </div>
    <label class="flex items-center gap-2 text-xs font-semibold text-text-muted">
      <input type="checkbox" bind:checked={showRunningOnly} />
      Show running only
    </label>
  </div>

  {#if loading}
    <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-4">
      {#each Array(6) as _, index}
        <div class="h-40 rounded-2xl border border-border-subtle bg-surface-1 animate-pulse" aria-hidden="true"></div>
      {/each}
    </div>
  {:else if error}
    <div class="rounded-2xl border border-state-warn/30 bg-state-warn/10 p-5 text-state-warn space-y-3">
      <p class="text-sm font-semibold">{error}</p>
      <button class="inline-flex items-center gap-2 text-xs font-semibold text-accent-emphasis" on:click={load}>Retry</button>
    </div>
  {:else if filteredApps.length === 0}
    {#if totalApps === 0}
      <div class="rounded-2xl border border-border-subtle bg-surface-1 p-6 text-sm text-text-muted space-y-3">
        <p class="text-text-primary font-medium">Install your first app</p>
        <p>Kickstart your Piccolo with a curated service. You can add or remove apps any time.</p>
        <div class="flex flex-wrap gap-2">
          <button class="px-3 py-2 rounded-xl border border-border-subtle text-xs font-semibold" on:click={openCatalog}>
            Browse catalog
          </button>
        </div>
      </div>
    {:else}
      <div class="rounded-2xl border border-border-subtle bg-surface-1 p-6 text-sm text-text-muted">
        <p>No apps match your filters. Install from the catalog or clear filters to see everything.</p>
      </div>
    {/if}
  {:else}
    <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-4">
      {#each filteredApps as app}
        {@const badge = badgeFor(app)}
        <article class="rounded-2xl border border-border-subtle bg-surface-1 p-4 shadow-sm flex flex-col gap-4">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <h3 class="text-base font-semibold text-text-primary truncate">{app.name}</h3>
              {#if app.image}
                <p class="text-xs text-text-muted truncate font-mono">{app.image}</p>
              {/if}
            </div>
            <span class={`px-2 py-0.5 rounded-full text-xs font-semibold ${badge.badge}`}>{badge.label}</span>
          </div>
          <div class="flex flex-wrap items-center gap-2 text-xs text-text-muted">
            {#if app.public_port}
              <span class="px-2 py-1 rounded-full bg-surface-2 font-mono">Port {app.public_port}</span>
            {/if}
            {#if app.pinned}
              <span class="px-2 py-1 rounded-full bg-accent-subtle text-accent-emphasis">Pinned</span>
            {/if}
          </div>
          <div class="flex items-center gap-2">
            <button class="px-3 py-2 rounded-xl border border-border-subtle text-xs font-semibold flex-1 disabled:opacity-60" on:click={() => isRunning(app) ? stop(app.name) : start(app.name)} disabled={isWorking(app.name, isRunning(app) ? 'stop' : 'start')}>
              {isRunning(app) ? (isWorking(app.name, 'stop') ? 'Stopping…' : 'Stop') : (isWorking(app.name, 'start') ? 'Starting…' : 'Start')}
            </button>
            <button class="px-3 py-2 rounded-xl bg-surface-2 text-xs font-semibold" on:click={() => openApp(app)}>
              Open
            </button>
            <button class="px-3 py-2 rounded-xl border border-border-subtle text-xs font-semibold" on:click={() => openDetails(app)}>
              Details
            </button>
          </div>
        </article>
      {/each}
    </div>
  {/if}
</section>
