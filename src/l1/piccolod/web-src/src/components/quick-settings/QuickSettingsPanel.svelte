<script lang="ts">
  import { createEventDispatcher, onDestroy, onMount } from 'svelte';
  import { apiProd, type ErrorResponse } from '@api/client';
  import { toast } from '@stores/ui';
  import { setRemoteSummary, resetRemoteSummary, markRemoteUnsupported, deviceStore } from '@stores/device';
  import { sessionStore } from '@stores/session';
  import { ICONS } from '@lib/iconPaths';
  import { get } from 'svelte/store';

  export let layout: 'desktop' | 'mobile' = 'desktop';

  type RemoteState = 'disabled' | 'provisioning' | 'preflight_required' | 'active' | 'warning' | 'error';

  type RemoteStatus = {
    enabled?: boolean;
    hostname?: string | null;
    portal_hostname?: string | null;
    public_url?: string | null;
    tld?: string | null;
    state?: RemoteState;
    warnings?: string[];
  };

  type OsUpdate = {
    current_version?: string;
    available_version?: string;
    pending?: boolean;
  };

  const dispatch = createEventDispatcher<{ close: void; logout: void }>();

  let remoteStatus: RemoteStatus | null = null;
  let osUpdate: OsUpdate | null = null;
  let loading = true;
  let error: string | null = null;
  let exportBusy = false;
  let updateBusy = false;
  let lockBusy = false;
  let unsubscribeSession: (() => void) | null = null;
  let unsubscribeDevice: (() => void) | null = null;
  let remoteSupported = true;

  $: remoteEnabled = remoteStatus?.enabled ?? false;
  $: remoteWarnings = remoteStatus?.warnings ?? [];
  $: portalHost = remoteStatus?.portal_hostname || remoteStatus?.hostname || remoteStatus?.public_url || null;
  $: remoteBadgeClass = (() => {
    if (!remoteSupported) {
      return 'bg-surface-2 text-text-muted';
    }
    if (remoteWarnings.length) {
      return 'bg-state-warn/10 text-state-warn';
    }
    return remoteEnabled ? 'bg-state-ok/10 text-state-ok' : 'bg-state-notice/10 text-state-notice';
  })();

  $: updateLabel = (() => {
    if (!osUpdate) return 'Check updates';
    if (osUpdate.pending) return 'Reboot to apply';
    if (osUpdate.available_version && osUpdate.available_version !== osUpdate.current_version) {
      return 'Apply update';
    }
    return 'Up to date';
  })();

  $: updateSubtext = (() => {
    if (!osUpdate) return 'Fetch current system status.';
    if (osUpdate.pending) return `Ready to finalize ${osUpdate.available_version ?? 'update'}.`;
    if (osUpdate.available_version && osUpdate.available_version !== osUpdate.current_version) {
      return `${osUpdate.available_version} available (current ${osUpdate.current_version ?? 'unknown'}).`;
    }
    return `Current version ${osUpdate.current_version ?? 'unknown'}.`;
  })();

  async function load() {
    loading = true;
    error = null;
    try {
      const [remote, updates] = await Promise.all([
        apiProd<RemoteStatus>('/remote/status').catch((err: ErrorResponse) => {
          if (err.code === 404) return null;
          throw err;
        }),
        apiProd<OsUpdate>('/updates/os').catch((err: ErrorResponse) => {
          if (err.code === 404) return null;
          throw err;
        })
      ]);
      remoteStatus = remote;
      if (remote === null) {
        markRemoteUnsupported();
      } else if (remote) {
        setRemoteSummary({
          enabled: remote.enabled,
          hostname: remote.portal_hostname || remote.hostname || remote.public_url || null,
          state: remote.state,
          warnings: remote.warnings
        });
      } else {
        setRemoteSummary(null);
      }
      osUpdate = updates;
    } catch (err: any) {
      error = err?.message || 'Failed to load quick settings.';
    } finally {
      loading = false;
    }
  }

  function close() {
    dispatch('close');
  }

  function ensureActionAvailable(action: string) {
    toast(`${action} is coming soon`, 'info');
  }

  async function handleLock() {
    if (lockBusy) return;
    lockBusy = true;
    ensureActionAvailable('Device locking');
    lockBusy = false;
  }

  function handleRemoteManage() {
    close();
    window.dispatchEvent(new CustomEvent('remote-wizard-open'));
  }

  async function handleUpdate() {
    if (updateBusy) return;
    updateBusy = true;
    ensureActionAvailable('OS update');
    updateBusy = false;
  }

  function handleNetworkDetails() {
    close();
    window.location.hash = '/remote';
  }

  async function handleExportLogs() {
    if (exportBusy) return;
    exportBusy = true;
    ensureActionAvailable('Log export');
    exportBusy = false;
  }

  function handleLogoutClick() {
    close();
    dispatch('logout');
  }

  onMount(() => {
    load();
    unsubscribeSession = sessionStore.subscribe((session) => {
      if (session.authenticated) {
        load();
      } else {
        remoteStatus = null;
        osUpdate = null;
        error = null;
        resetRemoteSummary();
      }
    });
    unsubscribeDevice = deviceStore.subscribe((device) => {
      remoteSupported = device.remoteSupported ?? true;
    });
  });

  onDestroy(() => {
    unsubscribeSession?.();
    unsubscribeDevice?.();
  });
</script>

<div class="flex flex-col h-full" data-layout={layout}>
  <div class="flex-1 overflow-y-auto">
    {#if loading}
      <div class="px-6 py-5 text-sm text-text-muted">Loading quick settings…</div>
    {:else if error}
      <div class="px-6 py-5 text-sm text-state-critical">{error}</div>
    {:else}
      <ul class="divide-y divide-border-subtle">
        <li class="px-6 py-5 flex flex-col gap-3">
          <div class="flex items-start gap-3">
            <span class="rounded-full bg-accent-subtle text-accent-emphasis p-2 h-10 w-10 flex items-center justify-center">
              <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" class="h-5 w-5">
                <path d={ICONS.lock} />
              </svg>
            </span>
            <div class="flex-1">
              <div class="flex items-center justify-between">
                <p class="text-sm font-semibold text-text-primary">Lock device</p>
              </div>
              <p class="text-xs text-text-muted mt-1">Secure Piccolo immediately; services pause until you unlock.</p>
            </div>
          </div>
          <button class="self-start inline-flex items-center gap-2 px-3 py-1.5 rounded-lg border border-border-subtle text-xs font-semibold" on:click={handleLock} disabled={lockBusy} data-focus-initial>
            {lockBusy ? 'Working…' : 'Lock now'}
          </button>
        </li>

        <li class="px-6 py-5 flex flex-col gap-3">
          <div class="flex items-start gap-3">
            <span class="rounded-full bg-accent-subtle text-accent-emphasis p-2 h-10 w-10 flex items-center justify-center">
              <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" class="h-5 w-5">
                <path d={ICONS.remote} />
              </svg>
            </span>
            <div class="flex-1">
              {#if remoteSupported}
                <div class="flex items-center justify-between">
                  <p class="text-sm font-semibold text-text-primary">Remote access</p>
                  <span class={`text-xs font-semibold px-2 py-0.5 rounded-full ${remoteBadgeClass}`}>
                    {remoteWarnings.length ? 'ATTN' : remoteEnabled ? 'ON' : 'OFF'}
                  </span>
                </div>
                <p class="text-xs text-text-muted mt-1">
                  {#if portalHost}
                    Portal reachable at {portalHost}.
                  {:else}
                    Run the setup wizard to enable remote access.
                  {/if}
                </p>
                {#if remoteWarnings.length}
                  <ul class="mt-2 space-y-1">
                    {#each remoteWarnings as warn}
                      <li class="text-xs text-state-warn">• {warn}</li>
                    {/each}
                  </ul>
                {/if}
              {:else}
                <p class="text-sm font-semibold text-text-primary">Remote access</p>
                <p class="text-xs text-text-muted mt-1">Remote access isn’t available on this device build.</p>
              {/if}
            </div>
          </div>
        {#if remoteSupported}
          <button class="self-start inline-flex items-center gap-2 px-3 py-1.5 rounded-lg border border-border-subtle text-xs font-semibold" on:click={handleRemoteManage}>
            {remoteEnabled ? 'Review remote' : 'Enable remote'}
          </button>
        {/if}
        </li>

        <li class="px-6 py-5 flex flex-col gap-3">
          <div class="flex items-start gap-3">
            <span class="rounded-full bg-accent-subtle text-accent-emphasis p-2 h-10 w-10 flex items-center justify-center">
              <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" class="h-5 w-5">
                <path d={ICONS.update} />
              </svg>
            </span>
            <div class="flex-1">
              <div class="flex items-center justify-between">
                <p class="text-sm font-semibold text-text-primary">Update &amp; reboot</p>
              </div>
              <p class="text-xs text-text-muted mt-1">{updateSubtext}</p>
            </div>
          </div>
          <button class="self-start inline-flex items-center gap-2 px-3 py-1.5 rounded-lg border border-border-subtle text-xs font-semibold" on:click={handleUpdate} disabled={updateBusy}>
            {updateBusy ? 'Working…' : updateLabel}
          </button>
        </li>

        <li class="px-6 py-5 flex flex-col gap-3">
          <div class="flex items-start gap-3">
            <span class="rounded-full bg-accent-subtle text-accent-emphasis p-2 h-10 w-10 flex items-center justify-center">
              <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" class="h-5 w-5">
                <path d={ICONS.network} />
              </svg>
            </span>
            <div class="flex-1">
              <div class="flex items-center justify-between">
                <p class="text-sm font-semibold text-text-primary">Network details</p>
              </div>
              <p class="text-xs text-text-muted mt-1">View IP, mDNS, and troubleshooting guidance.</p>
            </div>
          </div>
          <button class="self-start inline-flex items-center gap-2 px-3 py-1.5 rounded-lg border border-border-subtle text-xs font-semibold" on:click={handleNetworkDetails}>
            Open details
          </button>
        </li>

        <li class="px-6 py-5 flex flex-col gap-3">
          <div class="flex items-start gap-3">
            <span class="rounded-full bg-accent-subtle text-accent-emphasis p-2 h-10 w-10 flex items-center justify-center">
              <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" class="h-5 w-5">
                <path d={ICONS.logs} />
              </svg>
            </span>
            <div class="flex-1">
              <div class="flex items-center justify-between">
                <p class="text-sm font-semibold text-text-primary">Export logs</p>
              </div>
              <p class="text-xs text-text-muted mt-1">Generate a diagnostics bundle for support or troubleshooting.</p>
            </div>
          </div>
          <button class="self-start inline-flex items-center gap-2 px-3 py-1.5 rounded-lg border border-border-subtle text-xs font-semibold" on:click={handleExportLogs} disabled={exportBusy}>
            {exportBusy ? 'Working…' : 'Export logs'}
          </button>
        </li>

        {#if layout === 'desktop'}
          <li class="px-6 py-5 flex flex-col gap-3 border-t border-border-subtle/60">
            <div class="flex items-start gap-3">
              <span class="rounded-full bg-accent-subtle text-accent-emphasis p-2 h-10 w-10 flex items-center justify-center">
                <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" class="h-5 w-5">
                  <path d={ICONS.logout ?? ''} />
                </svg>
              </span>
              <div class="flex-1">
                <div class="flex items-center justify-between">
                  <p class="text-sm font-semibold text-text-primary">Sign out</p>
                </div>
                <p class="text-xs text-text-muted mt-1">End your session on this device.</p>
              </div>
            </div>
            <button data-testid="quick-settings-signout" class="self-start inline-flex items-center gap-2 px-3 py-1.5 rounded-lg border border-border-subtle text-xs font-semibold" on:click={handleLogoutClick}>
              Sign out
            </button>
          </li>
        {/if}
      </ul>
    {/if}
  </div>

  {#if layout === 'mobile'}
    <div class="border-t border-border-subtle px-6 py-4 flex flex-col gap-2">
      <button data-testid="quick-settings-signout" class="w-full px-4 py-3 rounded-xl bg-accent text-text-inverse text-sm font-semibold" on:click={handleLogoutClick}>
        Sign out
      </button>
      <button class="w-full px-4 py-3 rounded-xl bg-surface-2 text-sm font-semibold" on:click={close}>
        Close
      </button>
    </div>
  {/if}
</div>
