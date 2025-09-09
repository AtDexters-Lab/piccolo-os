<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '@api/client';
  let osu: any = null; let apps: any = null; let loading = true; let error = '';
  onMount(async () => {
    try { [osu, apps] = await Promise.all([api('/updates/os'), api('/updates/apps')]); }
    catch (e: any) { error = e?.message || 'Failed to load updates'; }
    finally { loading = false; }
  });
</script>

<div class="p-4 bg-white rounded border">
  <h3 class="font-medium mb-2">Updates</h3>
  {#if loading}
    <p class="text-sm text-gray-500">Loading…</p>
  {:else if error}
    <p class="text-sm text-red-600">{error}</p>
  {:else}
    <p class="text-sm">OS: {osu.current_version} → {osu.available_version} {#if osu.pending}(pending){/if}</p>
    <p class="text-sm">Apps with updates: {apps.apps?.filter((a) => a.update_available).length || 0}</p>
  {/if}
</div>
