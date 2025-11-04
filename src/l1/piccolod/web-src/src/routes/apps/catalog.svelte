<script lang="ts">
  import { onMount } from 'svelte';
  import { apiProd, demo } from '@api/client';
  import { toast } from '@stores/ui';

  type CatalogApp = {
    name: string;
    description?: string;
    image?: string;
    template?: string;
    categories?: string[];
  };

  type CatalogResponse = {
    apps?: CatalogApp[];
  };

  let apps: CatalogApp[] = [];
  let loading = true;
  let error = '';
  let filter = '';
  let yamlText = 'name: wordpress\nimage: docker.io/library/wordpress:6\nlisteners:\n  - name: web\n    guest_port: 80\n    flow: tcp\n    protocol: http\n';
  let installingApp: string | null = null;
  let yamlInstalling = false;

  onMount(async () => {
    try {
      const res = await apiProd<CatalogResponse>('/catalog');
      apps = res?.apps ?? [];
    } catch (err: any) {
      error = err?.message || 'Failed to load catalog';
    } finally {
      loading = false;
    }
  });

  function filteredApps() {
    if (!filter.trim()) return apps;
    const q = filter.trim().toLowerCase();
    return apps.filter((app) => app.name.toLowerCase().includes(q) || app.description?.toLowerCase().includes(q));
  }

  function genYaml(app: CatalogApp): string {
    if (app?.template) return app.template;
    const name = app?.name || 'app';
    const image = app?.image || 'alpine:latest';
    return `name: ${name}\nimage: ${image}\n`; 
  }

  async function installFromYaml(yaml: string, appName?: string) {
    try {
      if (appName) installingApp = appName; else yamlInstalling = true;
      if (demo) {
        toast('Installed (demo)', 'success');
        window.location.hash = '/apps';
        return;
      }
      const res: any = await apiProd('/apps', { method: 'POST', headers: { 'Content-Type': 'application/x-yaml' }, body: yaml });
      toast(res?.message || 'Installed', 'success');
      window.location.hash = '/apps';
    } catch (err: any) {
      toast(err?.message || 'Install failed', 'error');
    } finally {
      installingApp = null;
      yamlInstalling = false;
    }
  }
</script>

<section class="space-y-6">
  <header class="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
    <div>
      <h2 class="text-2xl font-semibold text-text-primary">Catalog</h2>
      <p class="text-sm text-text-muted">Curated services ship ready to deploy with Piccolo defaults.</p>
    </div>
    <div class="flex items-center gap-2 w-full md:max-w-sm">
      <label for="catalog-search" class="sr-only">Search catalog</label>
      <input id="catalog-search" class="flex-1 rounded-xl border border-border-subtle bg-surface-1 px-4 py-2 text-sm" placeholder="Search" bind:value={filter} />
    </div>
  </header>

  {#if loading}
    <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-4">
      {#each Array(6) as _}
        <div class="h-40 rounded-2xl border border-border-subtle bg-surface-1 animate-pulse" aria-hidden="true"></div>
      {/each}
    </div>
  {:else if error}
    <div class="rounded-2xl border border-state-warn/30 bg-state-warn/10 p-5 text-state-warn">{error}</div>
  {:else}
    <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-4">
      {#each filteredApps() as app}
        <article class="rounded-2xl border border-border-subtle bg-surface-1 p-4 shadow-sm flex flex-col gap-3">
          <div>
            <h3 class="text-base font-semibold text-text-primary">{app.name}</h3>
            {#if app.description}
              <p class="text-xs text-text-muted">{app.description}</p>
            {/if}
            {#if app.image}
              <p class="text-[11px] text-text-muted font-mono mt-1">{app.image}</p>
            {/if}
          </div>
          <div class="flex items-center gap-2 text-xs text-text-muted">
            {#if app.categories}
              {#each app.categories.slice(0, 3) as category}
                <span class="px-2 py-0.5 rounded-full bg-surface-2">{category}</span>
              {/each}
            {/if}
          </div>
          <div class="flex items-center gap-2">
            <button class="px-3 py-2 rounded-xl bg-accent text-text-inverse text-xs font-semibold disabled:opacity-60" on:click={() => installFromYaml(genYaml(app), app.name)} disabled={installingApp === app.name || yamlInstalling}>
              {installingApp === app.name ? 'Installing…' : 'Install'}
            </button>
            <button class="px-3 py-2 rounded-xl border border-border-subtle text-xs font-semibold" on:click={() => { yamlText = genYaml(app); }}>
              View YAML
            </button>
          </div>
        </article>
      {/each}
    </div>

    <div class="rounded-2xl border border-border-subtle bg-surface-1 p-5 space-y-3">
      <h3 class="text-base font-semibold text-text-primary">Install from YAML</h3>
      <p class="text-xs text-text-muted">Paste an app manifest to deploy custom services. YAML posts directly to the apps API.</p>
      <textarea class="w-full rounded-xl border border-border-subtle bg-surface-0 p-3 text-xs font-mono min-h-[140px]" bind:value={yamlText}></textarea>
      <div class="flex items-center gap-3">
        <button class="px-4 py-2 rounded-xl bg-accent text-text-inverse text-xs font-semibold disabled:opacity-60" on:click={() => installFromYaml(yamlText)} disabled={yamlInstalling || installingApp !== null}>
          {yamlInstalling ? 'Installing…' : 'Install'}
        </button>
        <button class="text-xs font-semibold text-accent-emphasis" on:click={() => window.location.hash = '/apps'}>Back to apps</button>
      </div>
    </div>
  {/if}
</section>
