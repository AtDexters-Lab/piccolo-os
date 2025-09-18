<script lang="ts">
  import { onMount } from 'svelte';
  import { apiProd } from '@api/client';
  import { buildServiceLink } from '@lib/serviceLinks';
  let data: any = null; let loading = true; let error = '';
  onMount(async () => {
    try { data = await apiProd('/services'); }
    catch (e: any) { error = e?.message || 'Failed to load services'; }
    finally { loading = false; }
  });

  const serviceLink = (service: any): string | null => buildServiceLink(service);
</script>

<div class="p-4 bg-white rounded border">
  <h3 class="font-medium mb-2">Services</h3>
  {#if loading}
    <p class="text-sm text-gray-500">Loading…</p>
  {:else if error}
    <p class="text-sm text-red-600">{error}</p>
  {:else}
    <p class="text-sm text-gray-700 mb-2">{data.services?.length || 0} running</p>
    <ul class="text-sm list-disc ml-5 space-y-1">
      {#each (data.services?.slice(0,5) ?? []) as s}
        <li>
          <span class="font-mono">{s.app}</span>/{s.name}
          {#if serviceLink(s)}
            → <a class="text-blue-600 underline" href={serviceLink(s) || '#'} target="_blank" rel="noopener">{serviceLink(s)}</a>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>
