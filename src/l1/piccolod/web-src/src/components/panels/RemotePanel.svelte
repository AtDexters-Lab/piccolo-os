<script lang="ts">
  import { onMount } from 'svelte';
  import { api, apiProd } from '@api/client';
  let status: any = null; let loading = true; let error = '';
  onMount(async () => {
    try { status = await apiProd('/remote/status'); }
    catch (e: any) { error = e?.message || 'Failed to load remote status'; }
    finally { loading = false; }
  });
</script>

<div class="p-4 bg-white rounded border">
  <h3 class="font-medium mb-2">Remote Access</h3>
  {#if loading}
    <p class="text-sm text-gray-500">Loading…</p>
  {:else if error}
    <p class="text-sm text-red-600">{error}</p>
  {:else}
    {#if status.enabled}
      <p class="text-sm">Enabled: <a class="text-blue-600 underline" href={status.public_url} rel="noopener" target="_blank">{status.public_url}</a></p>
      <p class="text-xs text-gray-600">Cert: {status.issuer}, expires {status.expires_at}</p>
    {:else}
      <p class="text-sm">Disabled</p>
      {#if status.warnings?.length}
        <ul class="text-xs text-yellow-800 bg-yellow-50 border border-yellow-200 rounded p-2 mt-2 list-disc ml-5">
          {#each status.warnings as w}
            <li>{w}</li>
          {/each}
        </ul>
      {/if}
    {/if}
  {/if}
</div>
