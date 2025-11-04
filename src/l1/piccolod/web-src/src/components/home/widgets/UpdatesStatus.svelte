<script lang="ts">
  import { onMount } from 'svelte';
  import { apiProd, type ErrorResponse } from '@api/client';

  type OsUpdate = {
    current_version?: string;
    available_version?: string;
    pending?: boolean;
  };

  type AppUpdates = {
    apps?: { name?: string; update_available?: boolean }[];
  };

  let os: OsUpdate | null = null;
  let appsNeedingUpdate = 0;
  let loading = true;
  let error: string | null = null;

  async function load() {
    loading = true;
    error = null;
    try {
      const [osResp, appResp] = await Promise.all([
        apiProd<OsUpdate>('/updates/os').catch((err: ErrorResponse) => {
          if (err.code === 404) return null;
          throw err;
        }),
        apiProd<AppUpdates>('/updates/apps').catch((err: ErrorResponse) => {
          if (err.code === 404) return null;
          throw err;
        })
      ]);
      os = osResp ?? null;
      appsNeedingUpdate = (appResp?.apps ?? []).filter((app) => !!app.update_available).length;
    } catch (err: any) {
      const e = err as ErrorResponse;
      error = e?.message || 'Unable to load updates.';
      os = null;
      appsNeedingUpdate = 0;
    } finally {
      loading = false;
    }
  }

  function gotoUpdates() {
    window.location.hash = '/updates';
  }

  function statusBadge(): { label: string; badge: string } {
    if (!os) {
      return { label: 'Unknown', badge: 'bg-state-notice/10 text-state-notice' };
    }
    if (os.pending) {
      return { label: 'Reboot to apply', badge: 'bg-state-warn/10 text-state-warn' };
    }
    if (os.available_version && os.available_version !== os.current_version) {
      return { label: 'Update available', badge: 'bg-state-notice/10 text-state-notice' };
    }
    return { label: 'Up to date', badge: 'bg-state-ok/10 text-state-ok' };
  }

  function description(): string {
    if (!os) return 'Check for updates to confirm current version.';
    if (os.pending) return `${os.available_version ?? 'Update'} ready; reboot when convenient.`;
    if (os.available_version && os.available_version !== os.current_version) {
      return `${os.available_version} available (current ${os.current_version ?? 'unknown'}).`;
    }
    if (appsNeedingUpdate > 0) {
      return `${appsNeedingUpdate} app${appsNeedingUpdate > 1 ? 's' : ''} can be updated.`;
    }
    return `Running ${os.current_version ?? 'latest build'}.`;
  }

  onMount(() => {
    load();
  });

  $: badgeInfo = statusBadge();
</script>

<article data-testid="home-widget-updates" class="rounded-2xl border border-border-subtle bg-surface-1 p-5 shadow-sm flex flex-col gap-3 min-h-[180px]">
  <header class="flex items-start justify-between gap-3">
    <div>
      <p class="text-xs uppercase tracking-[0.14em] text-text-muted">Updates</p>
      <h3 class="text-lg font-semibold text-text-primary">OS &amp; apps</h3>
    </div>
    {#if !loading}
      {#if error}
        <span class="px-2 py-1 rounded-full text-xs font-semibold bg-state-warn/10 text-state-warn">Check</span>
      {:else if badgeInfo}
        <span class={`px-2 py-1 rounded-full text-xs font-semibold ${badgeInfo.badge}`}>{badgeInfo.label}</span>
      {/if}
    {/if}
  </header>

  {#if loading}
    <div class="space-y-2">
      <div class="h-3 rounded-full bg-surface-2 animate-pulse"></div>
      <div class="h-3 rounded-full bg-surface-2 animate-pulse w-4/5"></div>
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
      <p class="text-sm text-text-muted">{description()}</p>
      {#if appsNeedingUpdate > 0}
        <p class="text-xs text-text-muted">{appsNeedingUpdate} app{appsNeedingUpdate > 1 ? 's' : ''} awaiting updates.</p>
      {/if}
    </div>
    <button class="inline-flex items-center gap-2 text-xs font-semibold text-accent-emphasis" on:click={gotoUpdates}>
      Open updates
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" class="h-3.5 w-3.5">
        <path d="M7 17 17 7" />
        <path d="M8 7h9v9" />
      </svg>
    </button>
  {/if}
</article>
