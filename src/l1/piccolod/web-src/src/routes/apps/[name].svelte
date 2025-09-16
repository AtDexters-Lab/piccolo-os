<script lang="ts">
  import { onMount } from 'svelte';
  import { api, apiProd, demo } from '@api/client';
  import { toast } from '@stores/ui';
  export let params: { name: string };
  let resp: any = null; let error = ''; let loading = true;
  let logs: any = null; let loadingLogs = false;
  let showUninstall = false; let purgeData = false;
  let working = false;
  onMount(async () => {
    await load();
    await loadLogs();
  });
  async function load() {
    loading = true; error = '';
    try { resp = await apiProd(`/apps/${params.name}`); }
    catch (e: any) { error = e?.message || 'Failed to load'; }
    finally { loading = false; }
  }
  async function loadLogs() {
    loadingLogs = true; try { logs = await apiProd(`/apps/${params.name}/logs`); } finally { loadingLogs = false; }
  }
  async function doUpdate() {
    try { await apiProd(`/apps/${params.name}/update`, { method: 'POST' }); toast(`Updated ${params.name}`, 'success'); }
    catch (e: any) { toast(e?.message || 'Update failed', 'error'); }
  }
  async function doRevert() {
    try { await apiProd(`/apps/${params.name}/revert`, { method: 'POST' }); toast(`Reverted ${params.name}`, 'success'); }
    catch (e: any) { toast(e?.message || 'Revert failed', 'error'); }
  }
  async function doUninstall(purge = false) {
    try {
      const q = purge ? '?purge=true' : '';
      await apiProd(`/apps/${params.name}${q}`, { method: 'DELETE' });
      toast(`Uninstalled ${params.name}`, 'success');
      showUninstall = false; purgeData = false;
    } catch (e: any) { toast(e?.message || 'Uninstall failed', 'error'); }
  }

  async function backupApp() {
    working = true;
    try {
      const r: any = await api(`/backup/app/${params.name}`, { method: demo ? 'GET' : 'POST' });
      toast(r?.message || `Backup created for ${params.name}`, 'success');
    } catch (e: any) {
      toast(e?.message || 'Backup failed', 'error');
    } finally { working = false; }
  }
  async function restoreApp() {
    if (!confirm(`Restore app '${params.name}' from latest backup?`)) return;
    working = true;
    try {
      const r: any = await api(`/restore/app/${params.name}`, { method: demo ? 'GET' : 'POST' });
      toast(r?.message || `Restore started for ${params.name}`, 'success');
    } catch (e: any) {
      toast(e?.message || 'Restore failed', 'error');
    } finally { working = false; }
  }
</script>

<h2 class="text-xl font-semibold mb-4">App: {params.name}</h2>
{#if loading}
  <p>Loading…</p>
{:else if error}
  <p class="text-red-600">{error}</p>
{:else}
  <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
    <div class="bg-white rounded border p-4 md:col-span-2">
      <h3 class="font-medium mb-2">Overview</h3>
      <p class="text-sm">Image: {resp?.data?.app?.image || 'n/a'}</p>
      <p class="text-sm">Status: {resp?.data?.app?.status || 'unknown'}</p>
      <div class="mt-3 space-x-2">
        <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={doUpdate}>Update</button>
        <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={doRevert}>Revert</button>
        <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={() => showUninstall = !showUninstall}>Uninstall</button>
        <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={backupApp} disabled={working}>Backup app</button>
        <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={restoreApp} disabled={working}>Restore app</button>
      </div>
      {#if showUninstall}
        <div class="mt-3 border rounded p-3 bg-red-50 border-red-200">
          <p class="text-sm text-red-800">Uninstall will remove the service. Optionally delete its data as well.</p>
          <label class="text-sm mt-2 inline-flex items-center gap-2"><input type="checkbox" bind:checked={purgeData}> Delete data too</label>
          <div class="mt-2 space-x-2">
            <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={() => doUninstall(purgeData)}>Confirm uninstall</button>
            <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={() => { showUninstall = false; purgeData = false; }}>Cancel</button>
          </div>
        </div>
      {/if}
      <h4 class="font-medium mt-4 mb-1">Services</h4>
      <ul class="text-sm list-disc ml-5">
        {#each (resp?.data?.services ?? []) as s}
          <li>
            {s.name}: {s.protocol} {#if s.local_url}
              — <a class="text-blue-600 underline" href={s.local_url} target="_blank" rel="noopener">Open locally</a>
            {:else if s.host_port}
              — <a class="text-blue-600 underline" href={`http://127.0.0.1:${s.host_port}/`} target="_blank" rel="noopener">Open locally</a>
            {/if}
          </li>
        {/each}
      </ul>
    </div>
    <div class="bg-white rounded border p-4">
      <h3 class="font-medium mb-2">Logs (recent)</h3>
      <a class="text-xs text-blue-600 underline" href="/api/v1/logs/bundle" target="_blank" rel="noopener">Download logs bundle</a>
      {#if loadingLogs}
        <p class="text-sm text-gray-500">Loading…</p>
      {:else if logs}
        <pre class="text-xs max-h-64 overflow-auto">{JSON.stringify(logs.entries ?? logs, null, 2)}</pre>
      {:else}
        <p class="text-sm text-gray-500">No logs.</p>
      {/if}
    </div>
  </div>
{/if}
