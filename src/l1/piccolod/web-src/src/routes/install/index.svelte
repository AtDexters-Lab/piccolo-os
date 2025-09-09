<script lang="ts">
  import { onMount } from 'svelte';
  import { api, demo } from '@api/client';
  import { toast } from '@stores/ui';
  let targets: any = null; let loading = true; let error = '';
  let plan: any = null; let planning = false;
  onMount(async () => {
    try { targets = await api('/install/targets'); }
    catch (e: any) { error = e?.message || 'Failed to load targets'; }
    finally { loading = false; }
  });
  async function simulate(id: string) {
    planning = true;
    try { plan = await api('/install/plan' + (demo ? '' : ''), { method: demo ? 'GET' : 'POST', body: demo ? undefined : JSON.stringify({ id, simulate: true }) }); }
    catch (e: any) { toast(e?.message || 'Simulation failed', 'error'); }
    finally { planning = false; }
  }
</script>

<h2 class="text-xl font-semibold mb-4">Install</h2>
{#if loading}
  <p>Loading targets…</p>
{:else if error}
  <p class="text-red-600">{error}</p>
{:else}
  <div class="bg-white rounded border p-4">
    <h3 class="font-medium mb-2">Targets</h3>
    <ul class="text-sm list-disc ml-5">
      {#each targets.targets ?? [] as t}
        <li>
          <span class="font-mono">{t.id}</span> — {t.model} ({Math.round((t.size_bytes||0)/1e9)} GB)
          <button class="ml-2 px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={() => simulate(t.id)} disabled={planning}>Simulate</button>
        </li>
      {/each}
    </ul>
    {#if plan}
      <h4 class="font-medium mt-3 mb-1">Plan</h4>
      <pre class="text-xs bg-gray-50 border rounded p-2 max-h-64 overflow-auto">{JSON.stringify(plan, null, 2)}</pre>
    {/if}
  </div>
{/if}

