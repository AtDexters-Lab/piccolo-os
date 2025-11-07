<script lang="ts">
  import { onMount } from 'svelte';
  import SystemStatus from '@components/home/widgets/SystemStatus.svelte';
  import NetworkStatus from '@components/home/widgets/NetworkStatus.svelte';
  import StorageStatus from '@components/home/widgets/StorageStatus.svelte';
  import UpdatesStatus from '@components/home/widgets/UpdatesStatus.svelte';
  import PinnedDock from '@components/home/PinnedDock.svelte';
  import ActivityFeed from '@components/home/ActivityFeed.svelte';
  import Nudges from '@components/home/Nudges.svelte';
  import { apiProd, type ErrorResponse } from '@api/client';
  import { deviceStore, type DeviceSummary } from '@stores/device';

  type ChipTone = 'ok' | 'warn' | 'critical' | 'info' | 'idle' | 'pending' | 'disabled';
  type ChipState = { label: string; tone: ChipTone; detail?: string };

  const CHIP_CLASSES: Record<ChipTone, string> = {
    ok: 'chip--ok',
    warn: 'chip--warn',
    critical: 'chip--critical',
    info: 'chip--info',
    idle: 'chip--idle',
    pending: 'chip--pending',
    disabled: 'chip--disabled'
  };

  let storageChip: ChipState = { label: 'Checking…', tone: 'pending' };
  let updatesChip: ChipState = { label: 'Checking…', tone: 'pending' };
  let appsChip: ChipState = { label: 'Loading…', tone: 'pending' };

  const goTo = (hash: string) => {
    window.location.hash = hash.startsWith('#') ? hash : `#${hash}`;
  };
  const openRemoteWizard = () => window.dispatchEvent(new CustomEvent('remote-wizard-open'));
  const handleRemoteCta = () => {
    if (!remoteSummary.remoteSupported) return;
    openRemoteWizard();
  };

  function chipClass({ tone }: ChipState): string {
    return CHIP_CLASSES[tone] ?? CHIP_CLASSES.idle;
  }

  function computeRemoteChip(summary: DeviceSummary): ChipState {
    if (!summary.remoteSupported) {
      return { label: 'Unavailable', tone: 'disabled', detail: 'Remote access not available on this build.' };
    }
    if (summary.remoteError) {
      return { label: 'Error', tone: 'critical', detail: summary.remoteError };
    }
    if (summary.remoteWarnings?.length) {
      return { label: 'Attention', tone: 'warn', detail: summary.remoteWarnings.join(' • ') };
    }
    if (summary.remoteEnabled) {
      const host = summary.remoteHostname ? `Reachable at ${summary.remoteHostname}` : 'Remote tunnel is active.';
      return { label: 'Ready', tone: 'ok', detail: host };
    }
    return { label: 'Off', tone: 'idle', detail: 'Remote portal is currently disabled.' };
  }

  async function loadStorageChip() {
    try {
      const res = await apiProd<{ disks?: { status?: string; size_bytes?: number; used_bytes?: number }[] }>('/storage/disks');
      const disks = res?.disks ?? [];
      if (!disks.length) {
        storageChip = { label: 'Add storage', tone: 'warn', detail: 'No disks detected.' };
        return;
      }
      const unhealthy = disks.filter((disk) => {
        const status = (disk.status ?? '').toLowerCase();
        return status && status !== 'healthy' && status !== 'ok';
      }).length;
      const total = disks.reduce((acc, disk) => acc + (disk.size_bytes ?? 0), 0);
      const used = disks.reduce((acc, disk) => acc + (disk.used_bytes ?? 0), 0);
      const percent = total > 0 ? Math.round((used / total) * 100) : 0;
      const detail = `Using ${percent}% of capacity`;
      storageChip = unhealthy
        ? { label: 'Attention', tone: 'warn', detail: `${unhealthy} disk${unhealthy > 1 ? 's' : ''} need attention.` }
        : { label: 'Healthy', tone: 'ok', detail };
    } catch (err: any) {
      const error = err as ErrorResponse;
      storageChip = { label: 'Unavailable', tone: 'critical', detail: error?.message || 'Unable to read storage status.' };
    }
  }

  async function loadUpdatesChip() {
    try {
      const res = await apiProd<{ current_version?: string; available_version?: string; pending?: boolean }>('/updates/os').catch((err: ErrorResponse) => {
        if (err?.code === 404) return null;
        throw err;
      });
      if (!res) {
        updatesChip = { label: 'Unknown', tone: 'info', detail: 'Updates service not reporting yet.' };
        return;
      }
      const { pending, available_version, current_version } = res;
      if (pending) {
        updatesChip = { label: 'Action needed', tone: 'warn', detail: 'Reboot to finish applying the update.' };
        return;
      }
      if (available_version && available_version !== current_version) {
        updatesChip = { label: 'Update available', tone: 'warn', detail: `Ready to install ${available_version}.` };
        return;
      }
      updatesChip = { label: 'Up to date', tone: 'ok', detail: `Running ${current_version ?? 'latest build'}.` };
    } catch (err: any) {
      const error = err as ErrorResponse;
      updatesChip = { label: 'Error', tone: 'critical', detail: error?.message || 'Failed to check updates.' };
    }
  }

  async function loadAppsChip() {
    try {
      const res = await apiProd<{ data?: { status?: string }[] }>('/apps');
      const apps = res?.data ?? [];
      if (!apps.length) {
        appsChip = { label: 'Explore apps', tone: 'info', detail: 'Install a curated service to get started.' };
        return;
      }
      const running = apps.filter((app) => (app.status ?? '').toLowerCase() === 'running').length;
      const errored = apps.filter((app) => (app.status ?? '').toLowerCase() === 'error').length;
      if (errored) {
        appsChip = { label: 'Attention', tone: 'warn', detail: `${errored} app${errored > 1 ? 's' : ''} require fixes.` };
        return;
      }
      appsChip = running
        ? { label: `${running} running`, tone: 'ok', detail: `${running} of ${apps.length} apps online.` }
        : { label: 'Stopped', tone: 'idle', detail: 'Start a service from the catalog.' };
    } catch (err: any) {
      const error = err as ErrorResponse;
      appsChip = { label: 'Unavailable', tone: 'critical', detail: error?.message || 'Failed to load apps.' };
    }
  }

  async function refreshSystemBar() {
    await Promise.allSettled([loadStorageChip(), loadUpdatesChip(), loadAppsChip()]);
  }

  onMount(() => {
    refreshSystemBar();
  });

  $: remoteSummary = $deviceStore;
  $: remoteChip = computeRemoteChip(remoteSummary);
  $: remoteChipLabel = remoteChip.label;
  $: remoteCtaDisabled = !remoteSummary.remoteSupported;
  $: remoteCtaLabel = remoteCtaDisabled
    ? 'Remote unavailable'
    : remoteSummary.remoteEnabled
      ? 'Review remote access'
      : 'Enable remote access';
</script>

<div class="space-y-6">
  <section class="system-bar" role="status" aria-live="polite">
    <div class="system-bar__primary">
      <p class="system-bar__eyebrow">System control</p>
      <h2>Keep Piccolo on track</h2>
      <p>Remote access, storage, updates, and apps roll up here so you can act fast.</p>
    </div>
    <button class="system-bar__cta" on:click={handleRemoteCta} disabled={remoteCtaDisabled}>
      {remoteCtaLabel}
    </button>
    <div class="system-bar__chips">
      <button
        class={`chip ${chipClass(remoteChip)}`}
        on:click={openRemoteWizard}
        title={remoteChip.detail ?? remoteChip.label}
        aria-label={`Remote status: ${remoteChip.label}`}
        disabled={!remoteSummary.remoteSupported}
      >
        Remote · {remoteChipLabel}
      </button>
      <button
        class={`chip ${chipClass(storageChip)}`}
        on:click={() => goTo('/storage')}
        title={storageChip.detail ?? storageChip.label}
        aria-label={`Storage status: ${storageChip.label}`}
      >
        Storage · {storageChip.label}
      </button>
      <button
        class={`chip ${chipClass(updatesChip)}`}
        on:click={() => goTo('/updates')}
        title={updatesChip.detail ?? updatesChip.label}
        aria-label={`Updates status: ${updatesChip.label}`}
      >
        Updates · {updatesChip.label}
      </button>
      <button
        class={`chip ${chipClass(appsChip)}`}
        on:click={() => goTo('/apps')}
        title={appsChip.detail ?? appsChip.label}
        aria-label={`Apps status: ${appsChip.label}`}
      >
        Apps · {appsChip.label}
      </button>
    </div>
  </section>

  <Nudges />

  <section class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
    <SystemStatus />
    <NetworkStatus />
    <StorageStatus />
    <UpdatesStatus />
  </section>

  <PinnedDock />

  <ActivityFeed />
</div>

<style>
  .system-bar {
    position: sticky;
    top: calc(var(--header-height) + 12px);
    z-index: 12;
    display: grid;
    grid-template-columns: minmax(0, 2fr) auto;
    gap: 20px;
    padding: 24px 28px;
    border-radius: 24px;
    border: 1px solid rgba(var(--border-rgb) / 0.12);
    background: var(--surface-1);
    box-shadow: 0 22px 48px rgba(15, 23, 42, 0.08);
  }
  .system-bar__primary {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .system-bar__eyebrow {
    text-transform: uppercase;
    letter-spacing: 0.16em;
    font-size: 0.65rem;
    font-weight: 600;
    color: var(--text-muted);
  }
  .system-bar__primary h2 {
    font-size: 1.38rem;
    font-weight: 600;
    color: var(--text-strong);
  }
  .system-bar__primary p {
    font-size: 0.92rem;
    color: var(--text-muted);
    max-width: 36ch;
  }
  .system-bar__cta {
    align-self: start;
    padding: 12px 22px;
    border-radius: 999px;
    border: 1px solid var(--accent-emphasis);
    background: var(--accent);
    color: var(--text-inverse);
    font-size: 0.92rem;
    font-weight: 600;
    cursor: pointer;
    min-height: 44px;
    transition: transform var(--transition-duration) var(--transition-easing);
  }
  .system-bar__cta:hover {
    transform: translateY(-1px);
  }
  .system-bar__cta:disabled {
    cursor: not-allowed;
    opacity: 0.6;
    transform: none;
  }
  .system-bar__chips {
    grid-column: 1 / -1;
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
    gap: 10px;
  }
  .chip {
    padding: 10px 16px;
    border-radius: 16px;
    font-size: 0.78rem;
    font-weight: 600;
    letter-spacing: 0.02em;
    border: 1px solid rgba(var(--border-rgb) / 0.16);
    background: var(--surface-0);
    color: var(--text-primary);
    cursor: pointer;
    min-height: 40px;
    text-align: left;
    transition: border-color var(--transition-duration) var(--transition-easing), background var(--transition-duration) var(--transition-easing), color var(--transition-duration) var(--transition-easing);
  }
  .chip:disabled {
    cursor: not-allowed;
    opacity: 1;
  }
  .chip:focus-visible {
    outline: 2px solid var(--focus-ring);
    outline-offset: 2px;
  }
  .chip--ok {
    border-color: rgba(var(--state-ok-rgb) / 0.5);
    background: rgba(var(--state-ok-rgb) / 0.18);
    color: rgb(var(--state-ok-rgb));
  }
  .chip--warn {
    border-color: rgba(var(--state-warn-rgb) / 0.5);
    background: rgba(var(--state-warn-rgb) / 0.18);
    color: rgb(var(--state-warn-rgb));
  }
  .chip--critical {
    border-color: rgba(var(--state-critical-rgb) / 0.5);
    background: rgba(var(--state-critical-rgb) / 0.18);
    color: rgb(var(--state-critical-rgb));
  }
  .chip--info {
    border-color: rgba(var(--state-notice-rgb) / 0.45);
    background: rgba(var(--state-notice-rgb) / 0.18);
    color: rgb(var(--state-notice-rgb));
  }
  .chip--pending {
    border-color: rgba(var(--border-rgb) / 0.2);
    background: rgba(var(--border-rgb) / 0.06);
    color: var(--text-muted);
  }
  .chip--idle {
    border-color: rgba(var(--border-rgb) / 0.16);
    background: var(--surface-0);
    color: var(--text-muted);
  }
  .chip--disabled {
    border-style: dashed;
    border-color: rgba(var(--border-rgb) / 0.24);
    background: rgba(var(--border-rgb) / 0.04);
    color: rgba(var(--text-muted-rgb) / 0.7);
  }
  @media (max-width: 900px) {
    .system-bar {
      grid-template-columns: minmax(0, 1fr);
      gap: 16px;
      padding: 20px;
      border-radius: 20px;
      top: calc(var(--header-height) + 8px);
    }
    .system-bar__cta {
      width: 100%;
      justify-content: center;
    }
    .system-bar__chips {
      grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
    }
  }
</style>
