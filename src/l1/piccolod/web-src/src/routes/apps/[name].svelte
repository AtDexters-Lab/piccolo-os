<script lang="ts">
  import { onMount } from 'svelte';
  import { api, demo } from '@api/client';
  import { toast } from '@stores/ui';
  export let params: { name: string };
  let resp: any = null; let error = ''; let loading = true;
  let logs: any = null; let loadingLogs = false;
  onMount(async () => {
    await load();
    await loadLogs();
  });
  async function load() {
    loading = true; error = '';
    try { resp = await api(`/apps/${params.name}`); }
    catch (e: any) { error = e?.message || 'Failed to load'; }
    finally { loading = false; }
  }
  async function loadLogs() {
    loadingLogs = true; try { logs = await api(`/apps/${params.name}/logs`); } finally { loadingLogs = false; }
  }
  async function doUpdate() {
    try { await api(`/apps/${params.name}/update`, { method: demo ? 'GET' : 'POST' }); toast(`Updated ${params.name}`, 'success'); }
    catch (e: any) { toast(e?.message || 'Update failed', 'error'); }
  }
  async function doRevert() {
    try { await api(`/apps/${params.name}/revert`, { method: demo ? 'GET' : 'POST' }); toast(`Reverted ${params.name}`, 'success'); }
    catch (e: any) { toast(e?.message || 'Revert failed', 'error'); }
  }
  async function doUninstall() {
    if (!confirm(`Uninstall ${params.name}?`)) return;
    try { await api(`/apps/${params.name}`, { method: 'DELETE' }); toast(`Uninstalled ${params.name}`, 'success'); }
    catch (e: any) { toast(e?.message || 'Uninstall failed', 'error'); }
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
      <p class="text-sm">Image: {resp.data.app.image}</p>
      <p class="text-sm">Status: {resp.data.app.status}</p>
      <div class="mt-3 space-x-2">
        <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={doUpdate}>Update</button>
        <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={doRevert}>Revert</button>
        <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={doUninstall}>Uninstall</button>
      </div>
      <h4 class="font-medium mt-4 mb-1">Services</h4>
      <ul class="text-sm list-disc ml-5">
        {#each resp.data.services ?? [] as s}
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
