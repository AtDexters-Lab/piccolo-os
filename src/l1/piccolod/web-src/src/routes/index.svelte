<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '@api/client';
  let health: any = null;
  let services: any = null;
  let loading = true; let error = '';
  onMount(async () => {
    try {
      [health, services] = await Promise.all([
        api('/health'),
        api('/services'),
      ]);
    } catch (e: any) {
      error = e?.message || 'Failed to load';
    } finally {
      loading = false;
    }
  });
</script>

<h2 class="text-xl font-semibold mb-4">Dashboard</h2>
{#if loading}
  <p>Loading…</p>
{:else if error}
  <p class="text-red-600">{error}</p>
{:else}
  <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
    <div class="p-4 bg-white rounded border">
      <h3 class="font-medium mb-2">Health</h3>
      <pre class="text-xs overflow-auto">{JSON.stringify(health, null, 2)}</pre>
    </div>
    <div class="p-4 bg-white rounded border">
      <h3 class="font-medium mb-2">Services</h3>
      <pre class="text-xs overflow-auto">{JSON.stringify(services, null, 2)}</pre>
    </div>
  </div>
{/if}

