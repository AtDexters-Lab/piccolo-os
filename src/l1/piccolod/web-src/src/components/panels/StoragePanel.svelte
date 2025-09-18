<script lang="ts">
  import { onMount } from 'svelte';
  import { apiProd } from '@api/client';
  let disks: any = null; let loading = true; let error = '';
  onMount(async () => {
    try {
      disks = await apiProd('/storage/disks');
    } catch (e: any) { error = e?.message || 'Failed to load storage'; }
    finally { loading = false; }
  });
</script>

<div class="p-4 bg-white rounded border">
  <h3 class="font-medium mb-2">Storage</h3>
  {#if loading}
    <p class="text-sm text-gray-500">Loading…</p>
  {:else if error}
    <p class="text-sm text-red-600">{error}</p>
  {:else}
    <p class="text-sm text-gray-700">{disks?.disks?.length || 0} disks detected</p>
  {/if}
</div>
