<script lang="ts">
  import { onMount } from 'svelte';
  import { api, demo } from '@api/client';
  import { toast } from '@stores/ui';
  let os: any = null; let appUpdates: any = null; let loading = true; let error = '';
  let working = false;
  async function load() {
    loading = true; error = '';
    try { [os, appUpdates] = await Promise.all([api('/updates/os'), api('/updates/apps')]); }
    catch (e: any) { error = e?.message || 'Failed to load updates'; }
    finally { loading = false; }
  }
  onMount(load);
  async function applyOS() {
    working = true; try { await api('/updates/os/apply', { method: demo ? 'GET' : 'POST' }); toast('OS update applied', 'success'); await load(); } catch (e: any) { toast(e?.message || 'Apply failed', 'error'); } finally { working = false; }
  }
  async function rollbackOS() {
    working = true; try { await api('/updates/os/rollback', { method: demo ? 'GET' : 'POST' }); toast('OS rolled back', 'success'); await load(); } catch (e: any) { toast(e?.message || 'Rollback failed', 'error'); } finally { working = false; }
  }
  async function updateApp(name: string) {
    working = true; try { await api(`/apps/${name}/update`, { method: demo ? 'GET' : 'POST' }); toast(`Updated ${name}`, 'success'); await load(); } catch (e: any) { toast(e?.message || 'Update failed', 'error'); } finally { working = false; }
  }
  async function revertApp(name: string) {
    working = true; try { await api(`/apps/${name}/revert`, { method: demo ? 'GET' : 'POST' }); toast(`Reverted ${name}`, 'success'); await load(); } catch (e: any) { toast(e?.message || 'Revert failed', 'error'); } finally { working = false; }
  }
</script>

<h2 class="text-xl font-semibold mb-4">Updates</h2>
{#if loading}
  <p>Loading…</p>
{:else if error}
  <p class="text-red-600">{error}</p>
{:else}
  <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
    <div class="bg-white rounded border p-4">
      <h3 class="font-medium mb-2">OS</h3>
      <p class="text-sm">Current: {os.current_version} — Available: {os.available_version}</p>
      {#if os.pending}
        <p class="text-xs text-gray-600">Pending reboot or apply</p>
      {/if}
      <div class="mt-2 space-x-2">
        <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={applyOS} disabled={working}>Apply</button>
        <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={rollbackOS} disabled={working}>Rollback</button>
      </div>
    </div>
    <div class="bg-white rounded border p-4">
      <h3 class="font-medium mb-2">Apps</h3>
      <ul class="text-sm list-disc ml-5">
        {#each appUpdates.apps ?? [] as a}
          <li>
            {a.name}: {a.current} {#if a.update_available}→ {a.available}{/if}
            {#if a.update_available}
              <button class="ml-2 px-2 py-0.5 text-xs border rounded hover:bg-gray-50" on:click={() => updateApp(a.name)} disabled={working}>Update</button>
            {/if}
            {#if a.previous}
              <button class="ml-2 px-2 py-0.5 text-xs border rounded hover:bg-gray-50" on:click={() => revertApp(a.name)} disabled={working}>Revert</button>
            {/if}
          </li>
        {/each}
      </ul>
    </div>
  </div>
{/if}

