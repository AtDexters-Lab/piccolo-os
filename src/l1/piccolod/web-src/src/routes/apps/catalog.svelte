<script lang="ts">
  import { onMount } from 'svelte';
  import { api, apiProd, demo } from '@api/client';
  import { toast } from '@stores/ui';
  import { link } from 'svelte-spa-router';
  let catalog: any = null; let loading = true; let error = '';
  let yamlText = 'name: example\nimage: alpine:latest\n';
  onMount(async () => {
    try { catalog = await apiProd('/catalog'); }
    catch (e: any) { error = e?.message || 'Failed to load catalog'; }
    finally { loading = false; }
  });
  function genYaml(app: any): string {
    const name = app.name || 'app';
    const image = app.image || 'alpine:latest';
    return `name: ${name}\nimage: ${image}\n`;
  }
  async function installFromYaml(yaml: string) {
    try {
      if (demo) {
        toast('Installed (demo)', 'success');
        window.location.hash = '/apps';
        return;
      }
      const res: any = await api('/apps', { method: 'POST', headers: { 'Content-Type': 'application/x-yaml' }, body: yaml });
      toast(res?.message || 'Installed', 'success');
      window.location.hash = '/apps';
    } catch (e: any) {
      toast(e?.message || 'Install failed', 'error');
    }
  }
</script>

<h2 class="text-xl font-semibold mb-4">App Catalog</h2>
{#if loading}
  <p>Loading…</p>
{:else if error}
  <p class="text-red-600">{error}</p>
{:else}
  <div class="bg-white rounded border p-4">
    <h3 class="font-medium mb-2">Curated Apps</h3>
    <ul class="text-sm space-y-2">
      {#each catalog.apps ?? [] as app}
        <li class="border rounded p-2">
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0">
              <div class="font-medium">{app.name}</div>
              <div class="text-xs text-gray-600">{app.description}</div>
              <div class="text-xs text-gray-500 font-mono">{app.image}</div>
            </div>
            <div class="shrink-0 space-x-2">
              <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={() => installFromYaml(genYaml(app))}>Install</button>
              <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={() => { yamlText = genYaml(app); }}>View YAML</button>
            </div>
          </div>
        </li>
      {/each}
    </ul>
  </div>
  <div class="mt-4 bg-white rounded border p-4">
    <h3 class="font-medium mb-2">Install from YAML</h3>
    <p class="text-xs text-gray-600 mb-2">Paste an app.yaml and click Install. In demo mode, this shows a success toast and navigates to Apps.</p>
    <textarea class="w-full border rounded p-2 text-xs font-mono min-h-[120px]" bind:value={yamlText}></textarea>
    <div class="mt-2">
      <button class="px-2 py-1 text-xs border rounded hover:bg-gray-50" on:click={() => installFromYaml(yamlText)}>Install</button>
      <a class="ml-2 text-xs text-blue-600 underline" href="/#/apps">Back to Apps</a>
    </div>
  </div>
{/if}
