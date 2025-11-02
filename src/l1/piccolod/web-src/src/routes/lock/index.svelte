<script lang="ts">
  import { onMount } from 'svelte';
  import { apiProd, type ErrorResponse } from '@api/client';
  import { deviceStore, setRemoteSummary, markRemoteUnsupported } from '@stores/device';
  import { sessionStore } from '@stores/session';

  type RemoteStatus = {
    enabled?: boolean;
    portal_hostname?: string | null;
    hostname?: string | null;
    warnings?: string[];
  };

  let remoteState: 'loading' | 'on' | 'off' | 'warning' = 'loading';
  let remoteHostname: string | null = null;
  let remoteWarnings: string[] = [];
  let remoteError = '';

  $: deviceName = $deviceStore.name || 'Piccolo Home';
  $: locked = !!$sessionStore.volumes_locked;
  $: remoteBadgeClass = remoteState === 'on'
    ? 'bg-state-ok/10 text-state-ok'
    : remoteState === 'warning'
    ? 'bg-state-warn/10 text-state-warn'
    : 'bg-state-notice/10 text-state-notice';
  $: remoteBadgeLabel = remoteState === 'on' ? 'Reachable' : remoteState === 'warning' ? 'Attention' : 'LAN only';

  function goToLogin() {
    window.location.hash = '/login';
  }

  onMount(async () => {
    try {
      const status = await apiProd<RemoteStatus>('/remote/status');
      if (status) {
        remoteHostname = status.portal_hostname || status.hostname || null;
        remoteWarnings = status.warnings ?? [];
        remoteState = status.enabled ? (remoteWarnings.length ? 'warning' : 'on') : 'off';
        const storeState = status.enabled ? (remoteWarnings.length ? 'warning' : 'active') : 'disabled';
        setRemoteSummary({
          enabled: status.enabled,
          hostname: remoteHostname,
          state: storeState,
          warnings: remoteWarnings
        });
      } else {
        remoteState = 'off';
        setRemoteSummary(null);
      }
    } catch (err: any) {
      const error = err as ErrorResponse | undefined;
      remoteState = 'off';
      if (error?.code === 401 || error?.code === 403) {
        remoteError = 'Sign in to view remote status.';
        return;
      }
      if (error?.code === 404) {
        remoteHostname = null;
        remoteWarnings = [];
        remoteState = 'off';
        remoteError = 'Remote access is not available on this device.';
        markRemoteUnsupported();
        return;
      }
      remoteError = error?.message || 'Unable to load remote status.';
      setRemoteSummary(null);
    }
  });
</script>

<section class="flex flex-col items-center justify-center text-center gap-8 py-12 min-h-[65vh]">
  <div class="flex flex-col items-center gap-4 max-w-md">
    <span class="inline-flex items-center justify-center h-16 w-16 rounded-full bg-accent-subtle text-accent-emphasis">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" class="h-9 w-9">
        <rect x="3" y="11" width="18" height="11" rx="2" />
        <path d="M7 11V7a5 5 0 0 1 10 0v4" />
      </svg>
    </span>
    <div class="space-y-1">
      <h1 class="text-2xl font-semibold text-text-primary">{deviceName} is locked</h1>
      <p class="text-sm text-text-muted">
        {#if locked}
          Encrypted volumes stay sealed until you sign in.
        {:else}
          Unlock to access services and storage.
        {/if}
      </p>
    </div>
  </div>

  <div class="w-full max-w-md bg-surface-1 border border-border-subtle rounded-2xl p-6 space-y-4">
    <div class="text-left space-y-3">
      <div class="flex items-center justify-between">
        <p class="text-sm font-semibold text-text-primary">Remote access</p>
        <span class={`text-xs font-semibold px-2 py-0.5 rounded-full ${remoteBadgeClass}`}>
          {remoteBadgeLabel}
        </span>
      </div>
      {#if remoteState === 'on'}
        <p class="text-xs text-text-muted">
          Portal reachable at {remoteHostname}. TPM keeps data sealed until you unlock.
        </p>
      {:else if remoteState === 'warning'}
        <p class="text-xs text-text-muted">Portal reachable with warnings:</p>
        <ul class="text-xs text-state-warn space-y-1">
          {#each remoteWarnings as warn}
            <li>• {warn}</li>
          {/each}
        </ul>
      {:else if remoteError}
        <p class="text-xs text-state-warn">{remoteError}</p>
      {:else}
        <p class="text-xs text-text-muted">Unlock on the LAN at http://piccolo.local to continue.</p>
      {/if}
    </div>

    <div class="text-left border-t border-border-subtle pt-4 space-y-3">
      <p class="text-xs uppercase tracking-[0.12em] text-text-muted">Status hints</p>
      <ul class="text-sm text-text-muted space-y-2">
        <li>• Check Ethernet link and mDNS if the portal is missing.</li>
        <li>• Updates resume after unlock; pending reboots are held.</li>
        <li>• Exported PCV and snapshots remain encrypted and safe.</li>
      </ul>
    </div>

    <div class="flex flex-col gap-3">
      <button class="w-full px-4 py-3 rounded-xl bg-accent text-text-inverse text-sm font-semibold" on:click={goToLogin}>
        Unlock with password
      </button>
      <button class="w-full px-4 py-3 rounded-xl border border-border-subtle text-sm font-semibold text-text-muted" disabled>
        Use recovery key (coming soon)
      </button>
    </div>
  </div>
</section>
