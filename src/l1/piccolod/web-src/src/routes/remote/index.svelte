<script lang="ts">
  import { onMount } from 'svelte';
  import { apiProd } from '@api/client';
  import { toast } from '@stores/ui';
  let status: any = null; let loading = true; let error = '';
  let form = { endpoint: '', device_key: '', hostname: '' };
  let working = false;
  async function load() {
    loading = true; error = '';
    try { status = await apiProd('/remote/status'); }
    catch (e: any) { error = e?.message || 'Failed to load status'; }
    finally { loading = false; }
  }
  onMount(load);
  async function configure() {
    working = true; error = '';
    try {
      const payload = {
        endpoint: form.endpoint,
        device_id: form.device_key,
        device_secret: '',
        hostname: form.hostname,
      };
      await apiProd('/remote/configure', { method: 'POST', body: JSON.stringify(payload) });
      toast('Remote configured', 'success');
      await load();
    } catch (e: any) { toast(e?.message || 'Configure failed', 'error'); }
    finally { working = false; }
  }
  async function disable() {
    working = true;
    try {
      await apiProd('/remote/disable', { method: 'POST' });
      toast('Remote disabled', 'success');
      await load();
    }
    catch (e: any) { toast(e?.message || 'Disable failed', 'error'); }
    finally { working = false; }
  }

  async function rotate() {
    working = true;
    try {
      await apiProd('/remote/rotate', { method: 'POST' });
      toast('Credentials rotated', 'success');
      await load();
    } catch (e: any) {
      toast(e?.message || 'Rotate failed', 'error');
    } finally {
      working = false;
    }
  }
</script>

<h2 class="text-xl font-semibold mb-4">Remote</h2>
{#if loading}
  <p>Loading…</p>
{:else if error}
  <p class="text-red-600">{error}</p>
{:else}
  <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
    <div class="bg-white rounded border p-4">
      <h3 class="font-medium mb-2">Status</h3>
      {#if status.enabled}
        <p class="text-sm">Enabled: <a class="text-blue-600 underline" href={status.public_url} target="_blank" rel="noopener">{status.public_url}</a></p>
        <p class="text-xs text-gray-600">Issuer: {status.issuer} — Expires: {status.expires_at}</p>
        <div class="mt-2 space-x-2">
          <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={disable} disabled={working}>Disable</button>
          <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={rotate} disabled={working}>Rotate credentials</button>
        </div>
      {:else}
        <p class="text-sm">Disabled</p>
      {/if}
      {#if status.warnings?.length}
        <ul class="text-xs text-yellow-800 bg-yellow-50 border border-yellow-200 rounded p-2 mt-2 list-disc ml-5">
          {#each status.warnings as w}
            <li>{w}</li>
          {/each}
        </ul>
      {/if}
    </div>
    <div class="bg-white rounded border p-4">
      <h3 class="font-medium mb-2">Configure</h3>
      <div class="space-y-2">
        <label class="block text-sm">Endpoint <input class="mt-0.5 w-full border rounded p-1 text-sm" bind:value={form.endpoint} placeholder="https://nexus.example.com" /></label>
        <label class="block text-sm">Device Key <input class="mt-0.5 w-full border rounded p-1 text-sm" bind:value={form.device_key} placeholder="xxxx-xxxx" /></label>
        <label class="block text-sm">Hostname <input class="mt-0.5 w-full border rounded p-1 text-sm" bind:value={form.hostname} placeholder="mybox.example.com" /></label>
      </div>
      <div class="mt-2 space-x-2">
        <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={configure} disabled={working}>Enable Remote</button>
      </div>
    </div>
  </div>
{/if}
