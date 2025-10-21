<script lang="ts">
  import { onMount } from 'svelte';
  import { apiProd } from '@api/client';
  import { toast } from '@stores/ui';
  import { buildServiceLink, buildRemoteServiceLink } from '@lib/serviceLinks';

  export let params: { name: string };

  let data: any = null;
  let loading = true;
  let error = '';
  let working = false;
  let workingAction: 'start' | 'stop' | 'uninstall' | null = null;
  let showUninstall = false;
  let purgeData = false;
  let status = 'unknown';
  let isRunning = false;

  async function load() {
    loading = true;
    error = '';
    try {
      data = await apiProd(`/apps/${params.name}`);
      status = (data?.data?.app?.status || 'unknown').toLowerCase();
      isRunning = status === 'running';
    } catch (e: any) {
      error = e?.message || 'Failed to load app details';
    } finally {
      loading = false;
    }
  }

  onMount(load);

  async function startApp() {
    working = true;
    workingAction = 'start';
    try {
      await apiProd(`/apps/${params.name}/start`, { method: 'POST' });
      toast(`Started ${params.name}`, 'success');
      await load();
    } catch (e: any) {
      toast(e?.message || 'Failed to start app', 'error');
    } finally {
      working = false;
      workingAction = null;
    }
  }

  async function stopApp() {
    working = true;
    workingAction = 'stop';
    try {
      await apiProd(`/apps/${params.name}/stop`, { method: 'POST' });
      toast(`Stopped ${params.name}`, 'success');
      await load();
    } catch (e: any) {
      toast(e?.message || 'Failed to stop app', 'error');
    } finally {
      working = false;
      workingAction = null;
    }
  }

  async function uninstallApp(purge = false) {
    working = true;
    workingAction = 'uninstall';
    try {
      const suffix = purge ? '?purge=true' : '';
      await apiProd(`/apps/${params.name}${suffix}`, { method: 'DELETE' });
      toast(`Uninstalled ${params.name}`, 'success');
      window.location.hash = '/apps';
    } catch (e: any) {
      toast(e?.message || 'Failed to uninstall app', 'error');
    } finally {
      working = false;
      workingAction = null;
      showUninstall = false;
      purgeData = false;
    }
  }

  const serviceLink = (service: any): string | null => buildServiceLink(service);
  const remoteServiceLink = (service: any): string | null => buildRemoteServiceLink(service);
</script>

<h2 class="text-xl font-semibold mb-4">App: {params.name}</h2>
{#if loading}
  <p>Loading…</p>
{:else if error}
  <p class="text-red-600">{error}</p>
{:else}
  <div class="bg-white rounded border p-4">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h3 class="font-medium">Overview</h3>
        <p class="text-sm text-gray-700">Image: <span class="font-mono">{data?.data?.app?.image || 'unknown'}</span></p>
        <p class="text-sm text-gray-700">Status: {data?.data?.app?.status || 'unknown'}</p>
      </div>
      <div class="space-x-2">
        <button
          class="px-2 py-1 text-xs border rounded hover:bg-gray-50 disabled:opacity-50"
          on:click={startApp}
          disabled={working || isRunning}
        >
          {workingAction === 'start' ? 'Starting…' : 'Start'}
        </button>
        <button
          class="px-2 py-1 text-xs border rounded hover:bg-gray-50 disabled:opacity-50"
          on:click={stopApp}
          disabled={working || !isRunning}
        >
          {workingAction === 'stop' ? 'Stopping…' : 'Stop'}
        </button>
        <button
          class="px-2 py-1 text-xs border rounded hover:bg-gray-50 disabled:opacity-50"
          on:click={() => { showUninstall = true; }}
          disabled={working}
        >
          {workingAction === 'uninstall' ? 'Uninstalling…' : 'Uninstall'}
        </button>
      </div>
    </div>

    {#if showUninstall}
      <div class="mt-4 bg-red-50 border border-red-200 rounded p-3">
        <p class="text-sm text-red-800">Uninstall will remove the app. Optionally purge its data.</p>
        <label class="mt-2 inline-flex items-center gap-2 text-sm text-red-900">
          <input type="checkbox" bind:checked={purgeData}> Delete stored data
        </label>
        <div class="mt-2 space-x-2">
          <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50 disabled:opacity-50" on:click={() => uninstallApp(purgeData)} disabled={working}>Confirm uninstall</button>
          <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={() => { showUninstall = false; purgeData = false; }}>Cancel</button>
        </div>
      </div>
    {/if}

    <h3 class="font-medium mt-6 mb-2">Service Endpoints</h3>
    {#if (data?.data?.services ?? []).length === 0}
      <p class="text-sm text-gray-600">No running services.</p>
    {:else}
      <div class="grid gap-4 md:grid-cols-2">
        {#each data.data.services as service}
          <div class="rounded-lg border border-slate-200 bg-slate-50 p-4 shadow-sm">
            <div class="flex items-start justify-between gap-3">
              <div>
                <p class="text-base font-semibold text-slate-900 flex items-center gap-2">
                  {service.name}
                  <span class="text-xs font-medium uppercase tracking-wide text-slate-500">
                    {service.protocol?.toUpperCase?.() || service.protocol}
                  </span>
                </p>
                <p class="text-xs text-slate-500 mt-1">
                  Guest {service.guest_port ?? '—'} · Host {service.host_port ?? '—'} · Public {service.public_port ?? '—'}
                </p>
              </div>
              <span class={`px-2 py-0.5 text-xs font-semibold rounded-full ${isRunning ? 'bg-emerald-100 text-emerald-700' : 'bg-slate-200 text-slate-600'}`}>
                {isRunning ? 'Running' : 'Stopped'}
              </span>
            </div>
            <div class="mt-4 space-y-3">
              <div class="flex flex-wrap items-center gap-2">
                <span class="text-xs font-semibold uppercase tracking-wide text-slate-500">Local</span>
                {#if serviceLink(service)}
                  {#if isRunning}
                    <a
                      class="inline-flex items-center gap-1 rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm transition hover:bg-blue-700 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500"
                      href={serviceLink(service) || '#'}
                      target="_blank"
                      rel="noopener"
                    >
                      Open
                    </a>
                  {:else}
                    <span class="text-xs text-slate-400">Start the app to open</span>
                  {/if}
                {:else}
                  <span class="text-xs text-slate-400">No local endpoint published</span>
                {/if}
              </div>
              {#if service?.remote_host}
                <div class="flex flex-wrap items-center gap-2">
                  <span class="text-xs font-semibold uppercase tracking-wide text-slate-500">Remote</span>
                  {#if remoteServiceLink(service) && isRunning}
                    <a
                      class="inline-flex items-center gap-1 rounded-md border border-blue-600 px-3 py-1.5 text-sm font-medium text-blue-600 transition hover:bg-blue-50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500"
                      href={remoteServiceLink(service) || '#'}
                      target="_blank"
                      rel="noopener"
                    >
                      {service.remote_host}
                    </a>
                  {:else}
                    <span class="text-xs font-mono text-slate-500">{service.remote_host}</span>
                  {/if}
                </div>
              {/if}
            </div>
          </div>
        {/each}
      </div>
      <p class="text-xs text-gray-500 mt-3">Remote access publishes listener hosts as <code>listener.user-domain</code>; ports follow the <code>remote_ports</code> setting or default to 80/443.</p>
    {/if}
  </div>
{/if}
