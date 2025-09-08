<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '@api/client';
  export let params: { name: string };
  let data: any = null; let error = ''; let loading = true;
  onMount(async () => {
    try { data = await api(`/apps/${params.name}`); }
    catch (e: any) { error = e?.message || 'Failed to load'; }
    finally { loading = false; }
  });
</script>

<h2 class="text-xl font-semibold mb-4">App: {params.name}</h2>
{#if loading}
  <p>Loading…</p>
{:else if error}
  <p class="text-red-600">{error}</p>
{:else}
  <pre class="text-xs overflow-auto">{JSON.stringify(data, null, 2)}</pre>
{/if}

