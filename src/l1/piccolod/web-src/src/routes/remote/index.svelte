<script lang="ts">
  import { onMount } from 'svelte';
  import { api, demo } from '@api/client';
  import { toast } from '@stores/ui';
  let status: any = null; let loading = true; let error = '';
  let form = { endpoint: '', device_key: '', hostname: '' };
  let working = false;
  async function load() {
    loading = true; error = '';
    try { status = await api('/remote/status'); }
    catch (e: any) { error = e?.message || 'Failed to load status'; }
    finally { loading = false; }
  }
  onMount(load);
  async function configure(simulate?: 'dns'|'port80'|'caa') {
    working = true; error = '';
    try {
      const path = simulate === 'dns' ? '/remote/configure/dns_error' : simulate === 'port80' ? '/remote/configure/port80_blocked' : simulate === 'caa' ? '/remote/configure/caa_error' : '/remote/configure';
      await api(path, { method: demo ? 'GET' : 'POST', body: demo ? undefined : JSON.stringify(form) });
      toast('Remote configured', 'success');
      if (demo) {
        status = { ...(status || {}), enabled: true, public_url: status?.public_url || 'https://demo.piccolo.example', issuer: status?.issuer || 'Let\'s Encrypt', expires_at: status?.expires_at || new Date(Date.now() + 86_400_000).toISOString() };
      } else {
        await load();
      }
    } catch (e: any) { toast(e?.message || 'Configure failed', 'error'); }
    finally { working = false; }
  }
  async function disable() {
    working = true;
    try { await api('/remote/disable', { method: demo ? 'GET' : 'POST' }); toast('Remote disabled', 'success'); if (demo) { status = { ...(status || {}), enabled: false }; } else { await load(); } }
    catch (e: any) { toast(e?.message || 'Disable failed', 'error'); }
    finally { working = false; }
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
        <button class="mt-2 px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={disable} disabled={working}>Disable</button>
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
        <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={() => configure()} disabled={working}>Enable Remote</button>
        <!-- Demo error simulations -->
        {#if demo}
          <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={() => configure('dns')} disabled={working}>Simulate DNS error</button>
          <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={() => configure('port80')} disabled={working}>Simulate port 80 blocked</button>
          <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={() => configure('caa')} disabled={working}>Simulate CAA error</button>
        {/if}
      </div>
    </div>
  </div>
{/if}
