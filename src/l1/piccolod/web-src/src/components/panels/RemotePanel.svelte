<script lang="ts">
  import { onMount } from 'svelte';
  import { apiProd } from '@api/client';

  type RemoteState = 'disabled' | 'provisioning' | 'preflight_required' | 'active' | 'warning' | 'error';

  interface RemoteStatusPayload {
    enabled?: boolean;
    state?: RemoteState;
    solver?: string;
    public_url?: string | null;
    hostname?: string | null;
    portal_hostname?: string | null;
    tld?: string | null;
    issuer?: string | null;
    expires_at?: string | null;
    next_renewal?: string | null;
    warnings?: string[];
  }

  let status: RemoteStatusPayload | null = null;
  let loading = true;
  let error = '';

  const stateBadges: Record<RemoteState, string> = {
    disabled: 'bg-slate-100 text-slate-700',
    provisioning: 'bg-blue-100 text-blue-800',
    preflight_required: 'bg-sky-100 text-sky-800',
    active: 'bg-emerald-100 text-emerald-700',
    warning: 'bg-amber-100 text-amber-900',
    error: 'bg-rose-100 text-rose-900'
  };

  function formatDate(ts?: string | null): string {
    if (!ts) return '-';
    const date = new Date(ts);
    if (Number.isNaN(date.getTime())) return '-';
    return date.toLocaleDateString();
  }

  function computeState(payload: RemoteStatusPayload | null): RemoteState {
    if (!payload) return 'disabled';
    if (payload.state) return payload.state;
    if (!payload.enabled) return 'disabled';
    if (payload.warnings && payload.warnings.length) return 'warning';
    return 'active';
  }

  onMount(async () => {
    try {
      status = await apiProd('/remote/status');
    } catch (e: any) {
      error = e?.message || 'Failed to load remote status';
    } finally {
      loading = false;
    }
  });
</script>

<div class="p-4 bg-white rounded border border-slate-200">
  <h3 class="font-medium text-slate-900 mb-2">Remote Access</h3>
  {#if loading}
    <p class="text-sm text-slate-500">Loading…</p>
  {:else if error}
    <p class="text-sm text-rose-600">{error}</p>
  {:else if status}
    <div class="flex items-center justify-between">
      <div>
        <p class="text-sm text-slate-600">{status.portal_hostname || status.hostname || status.public_url || 'No portal host configured'}</p>
        {#if status.tld}
          <p class="text-xs text-slate-500">Domain: {status.tld}</p>
        {/if}
        <p class="text-xs text-slate-500">Cert expires {formatDate(status.expires_at)} | Next renewal {formatDate(status.next_renewal)}</p>
      </div>
      <span class={`px-2 py-0.5 rounded-full text-xs font-semibold ${stateBadges[computeState(status)]}`}>
        {computeState(status).toUpperCase()}
      </span>
    </div>
    {#if status.warnings && status.warnings.length}
      <ul class="mt-2 text-xs text-amber-900 bg-amber-50 border border-amber-200 rounded p-2 space-y-1">
        {#each status.warnings.slice(0, 2) as warning}
          <li>• {warning}</li>
        {/each}
      </ul>
    {/if}
  {:else}
    <p class="text-sm text-slate-500">Remote access is disabled.</p>
  {/if}
</div>
