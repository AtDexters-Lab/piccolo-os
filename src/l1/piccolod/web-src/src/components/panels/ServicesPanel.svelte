<script lang="ts">
  import { onMount } from 'svelte';
  import { apiProd } from '@api/client';
  let data: any = null; let loading = true; let error = '';
  onMount(async () => {
    try { data = await apiProd('/services'); }
    catch (e: any) { error = e?.message || 'Failed to load services'; }
    finally { loading = false; }
  });
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
        <li class="flex flex-col">
          <div class="flex items-center justify-between gap-2">
            <a class="font-semibold text-blue-600 hover:underline" href={`#/apps/${encodeURIComponent(s.app)}`}>
              {s.app}
            </a>
            <span class="text-xs text-slate-500 uppercase tracking-wide">{s.protocol?.toUpperCase?.() || s.protocol}</span>
          </div>
          <div class="text-xs text-slate-500">
            Listener: <span class="font-mono text-slate-700">{s.name}</span>
          </div>
          {#if s?.remote_host}
            <span class="text-xs text-slate-500">Remote: <span class="font-mono text-slate-700">{s.remote_host}</span></span>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>
