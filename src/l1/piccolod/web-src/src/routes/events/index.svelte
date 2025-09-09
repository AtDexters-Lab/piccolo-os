<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '@api/client';
  let events: any = null; let loading = true; let error = '';
  onMount(async () => {
    try { events = await api('/events'); }
    catch (e: any) { error = e?.message || 'Failed to load events'; }
    finally { loading = false; }
  });
</script>

<h2 class="text-xl font-semibold mb-4">Events</h2>
{#if loading}
  <p>Loading…</p>
{:else if error}
  <p class="text-red-600">{error}</p>
{:else}
  <div class="bg-white rounded border p-4">
    <ul class="text-sm list-disc ml-5">
      {#each events.events ?? [] as e}
        <li><span class="font-mono">{e.ts}</span> [{e.level}] {e.source}: {e.message}</li>
      {/each}
    </ul>
  </div>
{/if}

