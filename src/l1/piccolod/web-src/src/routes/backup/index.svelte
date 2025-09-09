<script lang="ts">
  import { onMount } from 'svelte';
  import { api, demo } from '@api/client';
  import { toast } from '@stores/ui';
  let list: any = null; let loading = true; let error = '';
  onMount(async () => {
    try { list = await api('/backup/list'); }
    catch (e: any) { error = e?.message || 'Failed to load backups'; }
    finally { loading = false; }
  });
  async function exportCfg() {
    try { const r = await api('/backup/export', { method: demo ? 'GET' : 'POST' }); toast(r.message || 'Exported', 'success'); }
    catch (e: any) { toast(e?.message || 'Export failed', 'error'); }
  }
</script>

<h2 class="text-xl font-semibold mb-4">Backup</h2>
{#if loading}
  <p>Loading…</p>
{:else if error}
  <p class="text-red-600">{error}</p>
{:else}
  <div class="bg-white rounded border p-4">
    <div class="flex items-center justify-between mb-2">
      <h3 class="font-medium">Backups</h3>
      <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={exportCfg}>Export Config</button>
    </div>
    <ul class="text-sm list-disc ml-5">
      {#each list.backups ?? [] as b}
        <li>{b.id} — {Math.round((b.size_bytes||0)/1024)} KB at {b.created_at}</li>
      {/each}
    </ul>
  </div>
{/if}

