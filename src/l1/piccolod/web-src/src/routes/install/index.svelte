<script lang="ts">
  import { onMount } from 'svelte';
  import { api, demo } from '@api/client';
  import { toast } from '@stores/ui';
  let targets: any = null; let loading = true; let error = '';
  let plan: any = null; let planning = false;
  let selectedId: string = '';
  let confirmText: string = '';
  let working = false;
  onMount(async () => {
    try { targets = await api('/install/targets'); }
    catch (e: any) { error = e?.message || 'Failed to load targets'; }
    finally { loading = false; }
  });
  async function simulate(id: string) {
    planning = true;
    try {
      plan = await api('/install/plan' + (demo ? '' : ''), { method: demo ? 'GET' : 'POST', body: demo ? undefined : JSON.stringify({ id, simulate: true }) });
      selectedId = id; confirmText = '';
    }
    catch (e: any) { toast(e?.message || 'Simulation failed', 'error'); }
    finally { planning = false; }
  }
  async function fetchLatest(simFail = false) {
    working = true;
    try {
      const path = simFail ? '/install/fetch-latest/verify_failed' : '/install/fetch-latest';
      const res: any = await api(path, { method: demo ? 'GET' : 'POST' });
      if (res?.verified) {
        toast(`Fetched ${res.version} (verified)`, 'success');
      } else {
        toast('Fetched latest', 'success');
      }
    } catch (e: any) {
      toast(e?.message || 'Fetch latest failed', 'error');
    } finally { working = false; }
  }
  async function runInstall() {
    if (!selectedId || confirmText !== selectedId) return;
    working = true;
    try {
      const res: any = await api('/install/run', { method: demo ? 'GET' : 'POST', body: demo ? undefined : JSON.stringify({ id: selectedId }) });
      toast(res?.message || 'Installation started', 'success');
      try { localStorage.setItem('piccolo_install_started', '1'); } catch {}
      try { window.dispatchEvent(new Event('piccolo-install-started')); } catch {}
    } catch (e: any) {
      toast(e?.message || 'Install failed', 'error');
    } finally { working = false; }
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
      <div class="mt-3">
        <label class="block text-sm">Type the target id to confirm install
          <input class="mt-1 w-full border rounded p-1 text-sm font-mono" bind:value={confirmText} placeholder={selectedId || '/dev/disk/by-id/...'} />
        </label>
        <button class="mt-2 px-2 py-1 text-xs border rounded hover:bg-gray-50 disabled:opacity-50" on:click={runInstall} disabled={working || !selectedId || confirmText !== selectedId}>Run install</button>
      </div>
    {/if}
  </div>
  <div class="mt-4 bg-white rounded border p-4">
    <h3 class="font-medium mb-2">Fetch Latest OS Image</h3>
    <div class="space-x-2">
      <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={() => fetchLatest(false)} disabled={working}>Fetch latest</button>
      {#if demo}
        <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={() => fetchLatest(true)} disabled={working}>Simulate verify failed</button>
      {/if}
    </div>
  </div>
{/if}
