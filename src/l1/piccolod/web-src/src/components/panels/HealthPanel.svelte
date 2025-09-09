<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '@api/client';
  let data: any = null; let loading = true; let error = '';
  onMount(async () => {
    try { data = await api('/health'); }
    catch (e: any) { error = e?.message || 'Failed to load health'; }
    finally { loading = false; }
  });
</script>

<div class="p-4 bg-white rounded border">
  <div class="flex items-center justify-between mb-2">
    <h3 class="font-medium">Health</h3>
    {#if data}
      <span class="text-xs px-2 py-0.5 rounded"
        class:bg-green-100={data.overall === 'healthy'}
        class:text-green-800={data.overall === 'healthy'}
        class:bg-yellow-100={data.overall === 'degraded'}
        class:text-yellow-800={data.overall === 'degraded'}
        class:bg-red-100={data.overall === 'unhealthy'}
        class:text-red-800={data.overall === 'unhealthy'}>{data.overall}</span>
    {/if}
  </div>
  {#if loading}
    <p class="text-sm text-gray-500">Loading…</p>
  {:else if error}
    <p class="text-sm text-red-600">{error}</p>
  {:else}
    <p class="text-sm text-gray-700">{data.summary}</p>
  {/if}
</div>

