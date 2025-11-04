<script lang="ts">
  import { onMount } from 'svelte';
  import { apiProd, type ErrorResponse } from '@api/client';

  type AppRecord = {
    name: string;
    status?: string;
    pinned?: boolean;
    icon?: string | null;
    local_url?: string | null;
  };

  type AppsResponse = {
    data?: AppRecord[];
  };

  const MAX_DOCK = 6;

  let loading = true;
  let error: string | null = null;
  let apps: AppRecord[] = [];

  $: dockApps = selectDockApps(apps);

  function selectDockApps(list: AppRecord[]): AppRecord[] {
    if (!list?.length) return [];
    const pinned = list.filter((app) => app.pinned);
    if (pinned.length >= MAX_DOCK) return pinned.slice(0, MAX_DOCK);
    const remainder = list.filter((app) => !app.pinned);
    return [...pinned, ...remainder].slice(0, MAX_DOCK);
  }

  async function load() {
    loading = true;
    error = null;
    try {
      const res = await apiProd<AppsResponse>('/apps');
      apps = res?.data ?? [];
    } catch (err: any) {
      const e = err as ErrorResponse;
      error = e?.message || 'Unable to load apps.';
      apps = [];
    } finally {
      loading = false;
    }
  }

  function openApp(app: AppRecord) {
    if (app.local_url) {
      window.open(app.local_url, '_blank', 'noopener');
    } else {
      window.location.hash = `/apps/${encodeURIComponent(app.name)}`;
    }
  }

  function editDock() {
    window.location.hash = '/apps';
  }

  function appInitial(app: AppRecord) {
    if (app.icon) return null;
    return app.name?.slice(0, 1)?.toUpperCase() ?? '?';
  }

  onMount(() => {
    load();
  });
</script>

<section class="rounded-2xl border border-border-subtle bg-surface-1 p-5 shadow-sm">
  <header class="flex items-center justify-between gap-3 mb-4">
    <div>
      <p class="text-xs uppercase tracking-[0.14em] text-text-muted">Dock</p>
      <h3 class="text-lg font-semibold text-text-primary">Pinned apps</h3>
    </div>
    <button class="inline-flex items-center gap-2 text-xs font-semibold text-accent-emphasis" on:click={editDock}>
      Edit dock
    </button>
  </header>

  {#if loading}
    <div class="grid grid-cols-3 sm:grid-cols-6 gap-3">
      {#each Array(MAX_DOCK) as _, index}
        <div class="h-20 rounded-xl bg-surface-2 animate-pulse" aria-hidden="true"></div>
      {/each}
    </div>
  {:else if error}
    <div class="space-y-3">
      <p class="text-sm text-state-warn">{error}</p>
      <button class="inline-flex items-center gap-2 text-xs font-semibold text-accent-emphasis" on:click={load}>
        Retry
      </button>
    </div>
  {:else if dockApps.length}
    <div class="grid grid-cols-3 sm:grid-cols-6 gap-3">
      {#each dockApps as app}
        <button class="group flex flex-col items-center justify-center gap-2 rounded-xl border border-border-subtle bg-surface-1 hover:border-accent transition" on:click={() => openApp(app)}>
          {#if app.icon}
            <img src={app.icon} alt={`${app.name} icon`} class="h-12 w-12" loading="lazy" />
          {:else}
            <span class="h-12 w-12 rounded-2xl bg-accent-subtle text-accent-emphasis text-lg font-semibold flex items-center justify-center">
              {appInitial(app)}
            </span>
          {/if}
          <span class="text-xs font-medium text-text-primary truncate max-w-[72px]">
            {app.name}
          </span>
        </button>
      {/each}
    </div>
  {:else}
    <div class="space-y-3 text-sm text-text-muted">
      <p>No pinned apps yet. Pin up to six essentials for instant access.</p>
      <button class="inline-flex items-center gap-2 text-xs font-semibold text-accent-emphasis" on:click={() => window.location.hash = '/apps/catalog'}>
        Browse catalog
      </button>
    </div>
  {/if}
</section>
