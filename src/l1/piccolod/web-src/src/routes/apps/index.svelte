<script lang="ts">
  import { onMount } from 'svelte';
  import { api, demo } from '@api/client';
  import { toast } from '@stores/ui';
  let resp: any = null; let error = ''; let loading = true;
  onMount(async () => {
    await load();
  });
  async function load() {
    loading = true; error = '';
    try { resp = await api('/apps'); }
    catch (e: any) { error = e?.message || 'Failed to load apps'; }
    finally { loading = false; }
  }
  async function start(name: string) {
    try { await api(`/apps/${name}/start`, { method: demo ? 'GET' : 'POST' }); toast(`Started ${name}`, 'success'); await load(); }
    catch (e: any) { toast(e?.message || 'Start failed', 'error'); }
  }
  async function stop(name: string) {
    try { await api(`/apps/${name}/stop`, { method: demo ? 'GET' : 'POST' }); toast(`Stopped ${name}`, 'success'); await load(); }
    catch (e: any) { toast(e?.message || 'Stop failed', 'error'); }
  }
</script>

<h2 class="text-xl font-semibold mb-4">Apps</h2>
{#if loading}
  <p>Loading…</p>
{:else if error}
  <p class="text-red-600">{error}</p>
{:else}
  <table class="w-full text-sm bg-white rounded border">
    <thead class="bg-gray-50 text-gray-700">
      <tr>
        <th class="text-left p-2 border-b">Name</th>
        <th class="text-left p-2 border-b">Image</th>
        <th class="text-left p-2 border-b">Status</th>
        <th class="text-left p-2 border-b">Actions</th>
      </tr>
    </thead>
    <tbody>
      {#each (resp.data ?? []) as app}
        <tr class="border-b last:border-b-0">
          <td class="p-2 align-top"><a class="text-blue-600 underline" href={`/apps/${app.name}`}>{app.name}</a></td>
          <td class="p-2 align-top">{app.image}</td>
          <td class="p-2 align-top">
            <span class="px-2 py-0.5 rounded text-xs"
              class:bg-green-100={app.status === 'running'} class:text-green-800={app.status === 'running'}
              class:bg-yellow-100={app.status === 'stopped'} class:text-yellow-800={app.status === 'stopped'}
              class:bg-red-100={app.status === 'error'} class:text-red-800={app.status === 'error'}>{app.status}</span>
          </td>
          <td class="p-2 align-top space-x-2">
            <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={() => start(app.name)}>Start</button>
            <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={() => stop(app.name)}>Stop</button>
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
{/if}
