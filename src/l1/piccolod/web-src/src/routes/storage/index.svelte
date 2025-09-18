<script lang="ts">
  import { onMount } from 'svelte';
  import { apiProd } from '@api/client';
  import { toast } from '@stores/ui';
  import { sessionStore } from '@stores/session';

  let disks: any = null;
  let loading = true;
  let error = '';

  let recovery: any = null;
  let loadingRecovery = false;
  let working = false;
  let unlockPassword = '';

  $: session = $sessionStore;

  async function loadDisks() {
    loading = true;
    error = '';
    try {
      disks = await apiProd('/storage/disks');
    } catch (e: any) {
      error = e?.message || 'Failed to load disks';
    } finally {
      loading = false;
    }
  }

  async function loadRecovery() {
    loadingRecovery = true;
    try {
      recovery = await apiProd('/crypto/recovery-key');
    } catch (e: any) {
      recovery = null;
    } finally {
      loadingRecovery = false;
    }
  }

  onMount(() => {
    loadDisks();
    loadRecovery();
  });

  async function unlock() {
    if (!unlockPassword) {
      toast('Enter your admin password to unlock', 'error');
      return;
    }
    working = true;
    try {
      await apiProd('/crypto/unlock', {
        method: 'POST',
        body: JSON.stringify({ password: unlockPassword })
      });
      toast('Volumes unlocked', 'success');
      await loadDisks();
      try {
        const sessionInfo: any = await apiProd('/auth/session');
        sessionStore.set(sessionInfo);
      } catch {}
    } catch (e: any) {
      toast(e?.message || 'Unlock failed', 'error');
    } finally {
      working = false;
      unlockPassword = '';
    }
  }

  async function generateRecoveryKey() {
    working = true;
    try {
      const res: any = await apiProd('/crypto/recovery-key/generate', { method: 'POST' });
      recovery = { words: res.words, present: true };
      toast('Recovery key generated', 'success');
    } catch (e: any) {
      toast(e?.message || 'Recovery key generation failed', 'error');
    } finally {
      working = false;
    }
  }
</script>

<h2 class="text-xl font-semibold mb-4">Storage</h2>
<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
  <div class="bg-white rounded border p-4">
    <div class="flex items-center justify-between mb-2">
      <h3 class="font-medium">Volumes</h3>
      <span class="text-xs px-2 py-0.5 rounded"
        class:bg-yellow-100={session?.volumes_locked}
        class:text-yellow-800={session?.volumes_locked}
        class:bg-green-100={!session?.volumes_locked}
        class:text-green-800={!session?.volumes_locked}
      >{session?.volumes_locked ? 'Locked' : 'Unlocked'}</span>
    </div>
    {#if loading}
      <p class="text-sm text-gray-500">Loading disks…</p>
    {:else if error}
      <p class="text-sm text-red-600">{error}</p>
    {:else}
      <ul class="text-sm space-y-2">
        {#each disks?.disks ?? [] as disk}
          <li class="border rounded p-2">
            <div class="font-mono text-xs">{disk.id || disk.path}</div>
            <div class="text-xs text-gray-600">{disk.model || 'Unknown model'} — {disk.size_bytes ? Math.round(disk.size_bytes / 1e9) : '?'} GB</div>
          </li>
        {/each}
        {#if (disks?.disks ?? []).length === 0}
          <li class="text-xs text-gray-500">No additional disks detected.</li>
        {/if}
      </ul>
    {/if}
    <div class="mt-3">
      <label class="text-sm block">Unlock with password
        <input type="password" class="mt-1 w-full border rounded p-2 text-sm" bind:value={unlockPassword} autocomplete="current-password" />
      </label>
      <button class="mt-2 px-2 py-1 text-xs border rounded hover:bg-gray-50 disabled:opacity-50" on:click={unlock} disabled={working}>Unlock volumes</button>
    </div>
  </div>

  <div class="bg-white rounded border p-4">
    <h3 class="font-medium mb-2">Recovery Key</h3>
    {#if loadingRecovery}
      <p class="text-sm text-gray-500">Checking recovery key…</p>
    {:else if recovery?.words}
      <p class="text-sm text-gray-700 mb-2">Write these words down and store them safely.</p>
      <div class="grid grid-cols-2 md:grid-cols-3 gap-1 text-xs">
        {#each recovery.words as word, i}
          <span class="border rounded px-2 py-1 bg-gray-50">{i + 1}. {word}</span>
        {/each}
      </div>
    {:else}
      <p class="text-sm text-gray-700">No recovery key has been generated yet.</p>
      <button class="mt-2 px-2 py-1 text-xs border rounded hover:bg-gray-50 disabled:opacity-50" on:click={generateRecoveryKey} disabled={working}>Generate Recovery Key</button>
    {/if}
  </div>
</div>

<p class="mt-6 text-xs text-gray-500">Encrypted application data lives under <code class="font-mono">/var/piccolo/apps/&lt;app&gt;/data</code>. Unlock to mount volumes before starting services.</p>
