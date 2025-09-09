<script lang="ts">
  import { onMount } from 'svelte';
  import { api, demo } from '@api/client';
  import { toast } from '@stores/ui';
  let disks: any = null; let mounts: any = null; let loading = true; let error = '';
  let working = false;
  async function load() {
    loading = true; error = '';
    try { [disks, mounts] = await Promise.all([api('/storage/disks'), api('/storage/mounts')]); }
    catch (e: any) { error = e?.message || 'Failed to load storage'; }
    finally { loading = false; }
  }
  onMount(load);
  async function initDisk(id: string) {
    if (!confirm(`Initialize disk ${id}? This may erase data.`)) return;
    working = true;
    try {
      await api(`/storage/disks/${encodeURIComponent(id)}/init`, { method: demo ? 'GET' : 'POST' });
      toast(`Initialized ${id}`, 'success');
      await load();
    } catch (e: any) { toast(e?.message || 'Initialize failed', 'error'); }
    finally { working = false; }
  }
  async function useDisk(id: string) {
    working = true;
    try {
      await api(`/storage/disks/${encodeURIComponent(id)}/use`, { method: demo ? 'GET' : 'POST' });
      toast(`Using ${id}`, 'success');
      await load();
    } catch (e: any) { toast(e?.message || 'Use as-is failed', 'error'); }
    finally { working = false; }
  }
  async function setDefaultRoot(path: string) {
    working = true;
    try {
      await api('/storage/default-root', { method: demo ? 'GET' : 'POST', body: demo ? undefined : JSON.stringify({ path }) });
      toast('Default data root updated', 'success');
      await load();
    } catch (e: any) { toast(e?.message || 'Update default root failed', 'error'); }
    finally { working = false; }
  }
</script>

<h2 class="text-xl font-semibold mb-4">Storage</h2>
{#if loading}
  <p>Loading…</p>
{:else if error}
  <p class="text-red-600">{error}</p>
{:else}
  <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
    <div class="bg-white rounded border p-4">
      <div class="flex items-center justify-between mb-2">
        <h3 class="font-medium">Disks</h3>
        <span class="text-xs text-gray-500">{disks.disks?.length || 0} detected</span>
      </div>
      <ul class="text-sm space-y-2">
        {#each disks.disks ?? [] as d}
          <li class="border rounded p-2">
            <div class="flex items-start justify-between gap-2">
              <div class="min-w-0">
                <div class="font-mono text-xs">{d.id}</div>
                <div class="text-xs text-gray-600">{d.model} — {Math.round((d.size_bytes||0)/1e9)} GB</div>
              </div>
              <div class="shrink-0 space-x-2">
                <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={() => useDisk(d.id)} disabled={working}>Use as-is</button>
                <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={() => initDisk(d.id)} disabled={working}>Initialize</button>
              </div>
            </div>
          </li>
        {/each}
      </ul>
    </div>
    <div class="bg-white rounded border p-4">
      <div class="flex items-center justify-between mb-2">
        <h3 class="font-medium">Mounts</h3>
        <span class="text-xs text-gray-500">{mounts.mounts?.length || 0} active</span>
      </div>
      <ul class="text-sm list-disc ml-5">
        {#each mounts.mounts ?? [] as m}
          <li>
            <span class="font-mono">{m.path}</span> — {m.label}
            {#if m.default}
              <span class="ml-2 text-xs px-2 py-0.5 rounded bg-blue-100 text-blue-800">default</span>
            {:else}
              <button class="ml-2 px-2 py-0.5 text-xs border rounded hover:bg-gray-50" on:click={() => setDefaultRoot(m.path)} disabled={working}>Set default</button>
            {/if}
          </li>
        {/each}
      </ul>
    </div>
  </div>
{/if}

