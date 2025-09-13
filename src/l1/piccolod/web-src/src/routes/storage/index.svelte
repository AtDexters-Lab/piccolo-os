<script lang="ts">
  import { onMount } from 'svelte';
  import { api, demo } from '@api/client';
  import { toast } from '@stores/ui';
  import { sessionStore } from '@stores/session';
  let disks: any = null; let mounts: any = null; let loading = true; let error = '';
  let working = false;
  let recovery: any = null; let loadingRecovery = false;
  // Encrypt-in-place state
  let eipPath = '';
  let eipPlan: any = null;
  let eipWorking = false;
  $: session = $sessionStore;
  async function load() {
    loading = true; error = '';
    try { [disks, mounts] = await Promise.all([api('/storage/disks'), api('/storage/mounts')]); }
    catch (e: any) { error = e?.message || 'Failed to load storage'; }
    finally { loading = false; }
  }
  onMount(load);
  async function loadRecovery() {
    loadingRecovery = true; try { recovery = await api('/crypto/recovery-key'); } finally { loadingRecovery = false; }
  }
  onMount(loadRecovery);
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
  let unlockPassword = '';
  async function unlock() {
    working = true;
    try {
      if (!demo) {
        if (!unlockPassword) { toast('Enter password', 'error'); return; }
        await api('/crypto/unlock', { method: 'POST', body: JSON.stringify({ password: unlockPassword }) });
      } else {
        await api('/storage/unlock', { method: 'GET' });
      }
      toast('Volumes unlocked', 'success');
      // Refresh session and mounts after unlock attempt
      try { const s = await api('/auth/session'); sessionStore.set(s as any); } catch {}
      await load();
    } catch (e: any) {
      toast(e?.message || 'Unlock failed', 'error');
    } finally {
      working = false;
    }
  }
  async function generateRecovery() {
    working = true;
    try {
      const res = await api('/crypto/recovery-key/generate', { method: 'POST' });
      toast('Recovery key generated', 'success');
      recovery = { words: (res as any).words, present: true };
    } catch (e: any) { toast(e?.message || 'Generate failed', 'error'); }
    finally { working = false; }
  }

  async function eipDryRun() {
    eipWorking = true; eipPlan = null;
    try {
      const res = await api('/storage/encrypt-in-place_dry-run', { method: demo ? 'GET' : 'POST', body: demo ? undefined : JSON.stringify({ path: eipPath, dry_run: true }) });
      eipPlan = res;
      toast('Dry run plan generated', 'success');
    } catch (e: any) {
      toast(e?.message || 'Dry run failed', 'error');
    } finally {
      eipWorking = false;
    }
  }
  async function eipConfirm() {
    eipWorking = true;
    try {
      await api('/storage/encrypt-in-place_confirm', { method: demo ? 'GET' : 'POST', body: demo ? undefined : JSON.stringify({ path: eipPath, confirm: true }) });
      toast('Encryption completed', 'success');
      eipPlan = null;
      await load();
    } catch (e: any) {
      toast(e?.message || 'Encryption failed', 'error');
    } finally {
      eipWorking = false;
    }
  }
  async function eipSimulateFail() {
    eipWorking = true;
    try {
      await api('/storage/encrypt-in-place_failed', { method: demo ? 'GET' : 'POST', body: demo ? undefined : JSON.stringify({ path: eipPath }) });
      toast('Encryption failed', 'error');
    } catch (e: any) {
      toast(e?.message || 'Encryption failed', 'error');
    } finally {
      eipWorking = false;
    }
  }
</script>

<h2 class="text-xl font-semibold mb-4">Storage</h2>
<!-- Encryption & Unlock Panel -->
<div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
  <div class="bg-white rounded border p-4">
    <div class="flex items-center justify-between mb-2">
      <h3 class="font-medium">Encryption & Unlock</h3>
      <span class="text-xs px-2 py-0.5 rounded"
        class:bg-yellow-100={session?.volumes_locked}
        class:text-yellow-800={session?.volumes_locked}
        class:bg-green-100={!session?.volumes_locked}
        class:text-green-800={!session?.volumes_locked}
      >{session?.volumes_locked ? 'Locked' : 'Unlocked'}</span>
    </div>
    <p class="text-sm text-gray-700">{session?.volumes_locked ? 'Volumes are locked until you unlock.' : 'Volumes are available.'}</p>
    <div class="mt-2 space-y-2">
      <label class="block text-sm">Password
        <input id="unlock-password" type="password" class="mt-1 w-full border rounded p-1 text-sm" bind:value={unlockPassword} autocomplete="current-password" />
      </label>
      <div class="space-x-2">
        <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={unlock} disabled={working}>Unlock volumes</button>
        {#if demo}
          <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={() => { /* simulate failure */ api('/storage/unlock_failed', { method: 'GET' }).then(()=>toast('Unlock failed','error')).catch(()=>toast('Unlock failed','error')); }} disabled={working}>Simulate unlock failure</button>
        {/if}
      </div>
    </div>
  </div>
  <div class="bg-white rounded border p-4">
    <div class="flex items-center justify-between mb-2">
      <h3 class="font-medium">Recovery Key</h3>
    </div>
    {#if loadingRecovery}
      <p class="text-sm text-gray-500">Loading…</p>
    {:else if recovery}
      {#if recovery.present === false}
        <p class="text-sm text-gray-700">No recovery key set.</p>
        <button class="mt-2 px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={generateRecovery} disabled={working}>Generate Recovery Key</button>
      {:else}
        {#if recovery.words}
          <p class="text-sm text-gray-700 mb-2">Write these 24 words down and store securely:</p>
          <div class="grid grid-cols-2 md:grid-cols-3 gap-1 text-xs">
            {#each recovery.words as w, i}
              <div class="px-2 py-1 bg-gray-50 border rounded"><span class="text-gray-500 mr-1">{i+1}.</span>{w}</div>
            {/each}
          </div>
        {:else}
          <pre class="text-xs bg-gray-50 border rounded p-2 max-h-40 overflow-auto">{JSON.stringify(recovery, null, 2)}</pre>
        {/if}
      {/if}
    {:else}
      <p class="text-sm text-gray-500">No recovery info.</p>
    {/if}
  </div>
</div>

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
          <li class="flex items-center gap-2">
            <span class="font-mono text-xs">{m.path}</span>
            {#if m.label}
              <span class="text-gray-700">— {m.label}</span>
            {:else if m.device}
              <span class="text-gray-500">— {m.device}</span>
            {:else if m.type}
              <span class="text-gray-500">— {m.type}</span>
            {/if}
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
  <div class="mt-4 bg-white rounded border p-4">
    <h3 class="font-medium mb-2">Encrypt Existing Data In Place</h3>
    <label class="block text-sm">Path to encrypt
      <input class="mt-1 w-full border rounded p-1 text-sm" bind:value={eipPath} placeholder="/var/piccolo/storage/app/data" />
    </label>
    <div class="mt-2 space-x-2">
      <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={eipDryRun} disabled={eipWorking}>Dry-run</button>
      <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={eipConfirm} disabled={eipWorking}>Confirm</button>
      {#if demo}
        <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={eipSimulateFail} disabled={eipWorking}>Simulate failure</button>
      {/if}
    </div>
    {#if eipPlan}
      <h4 class="font-medium mt-3 mb-1">Plan</h4>
      <pre class="text-xs bg-gray-50 border rounded p-2 max-h-64 overflow-auto">{JSON.stringify(eipPlan, null, 2)}</pre>
    {/if}
  </div>
{/if}
