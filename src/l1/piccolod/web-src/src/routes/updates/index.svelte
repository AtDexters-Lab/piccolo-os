<script lang="ts">
  import { onMount } from 'svelte';
  import { api, demo, type ErrorResponse } from '@api/client';
  import { toast } from '@stores/ui';
  let os: any = null; let appUpdates: any = null; let loading = true; let error = '';
  let working = false;
  async function load() {
    loading = true; error = '';
    try {
      const [osResp, appsResp] = await Promise.all([
        api('/updates/os').catch((err: ErrorResponse) => {
          if (err?.code === 404) return null;
          throw err;
        }),
        api('/updates/apps').catch((err: ErrorResponse) => {
          if (err?.code === 404) return null;
          throw err;
        })
      ]);
      os = osResp;
      appUpdates = appsResp;
    }
    catch (e: any) { error = e?.message || 'We could not fetch updates right now. Try again in a minute.'; }
    finally { loading = false; }
  }
  onMount(load);
  async function applyOS() {
    working = true; try { await api('/updates/os/apply', { method: demo ? 'GET' : 'POST' }); toast('OS update applied', 'success'); await load(); } catch (e: any) { toast(e?.message || 'Apply failed', 'error'); } finally { working = false; }
  }
  async function rollbackOS() {
    working = true; try { await api('/updates/os/rollback', { method: demo ? 'GET' : 'POST' }); toast('OS rolled back', 'success'); await load(); } catch (e: any) { toast(e?.message || 'Rollback failed', 'error'); } finally { working = false; }
  }
  async function updateApp(name: string) {
    working = true; try { await api(`/apps/${name}/update`, { method: demo ? 'GET' : 'POST' }); toast(`Updated ${name}`, 'success'); await load(); } catch (e: any) { toast(e?.message || 'Update failed', 'error'); } finally { working = false; }
  }
  async function revertApp(name: string) {
    working = true; try { await api(`/apps/${name}/revert`, { method: demo ? 'GET' : 'POST' }); toast(`Reverted ${name}`, 'success'); await load(); } catch (e: any) { toast(e?.message || 'Revert failed', 'error'); } finally { working = false; }
  }

  $: hasAvailableOsUpdate = !!(os && os.available_version && os.available_version !== os.current_version);
  $: canRollback = !!(os && os.pending);
  $: appList = Array.isArray(appUpdates?.apps) ? appUpdates.apps : [];

  type StatusSummary = { headline: string; detail: string; badge: string | null };

  const sanitizeVersion = (value?: string | null) => {
    if (!value) return 'current build';
    return value.replace(/\\s+build.*/i, '').trim();
  };

  $: osSummary = (() => {
    if (!os) {
      return {
        headline: 'Up to date',
        detail: 'Piccolo will download updates automatically when they are available.',
        badge: null
      } satisfies StatusSummary;
    }
    const current = sanitizeVersion(os.current_version);
    if (os.pending) {
      const target = sanitizeVersion(os.available_version) || current;
      return {
        headline: 'Update ready to finish',
        detail: `Reboot to complete installing version ${target}.`,
        badge: 'Action required'
      } satisfies StatusSummary;
    }
    if (hasAvailableOsUpdate) {
      const target = sanitizeVersion(os.available_version);
      return {
        headline: 'Update available',
        detail: `Version ${target} is ready to install. Currently running ${current}.`,
        badge: 'Update available'
      } satisfies StatusSummary;
    }
    return {
      headline: 'Up to date',
      detail: `Running version ${current}. Piccolo will alert you when a new release is ready.`,
      badge: null
    } satisfies StatusSummary;
  })();
</script>

<div class="updates-header">
  <h1>Updates</h1>
  <p>Keep Piccolo current with the latest OS and app releases.</p>
</div>

{#if loading}
  <div class="remote-shell remote-shell--loading">
    <p class="remote-shell__headline">Checking for updates…</p>
  </div>
{:else if error}
  <div class="remote-shell remote-shell--error">
    <h2 class="remote-shell__headline">Updates are temporarily unavailable</h2>
    <p class="remote-shell__copy">{error}</p>
    <button class="remote-shell__action" on:click={load} disabled={loading}>Retry</button>
  </div>
{:else}
  <div class="updates-grid">
    <section class="updates-card">
      <div class="updates-card__header">
        <div>
          <h2>{osSummary.headline}</h2>
          <p>{osSummary.detail}</p>
        </div>
        {#if osSummary.badge}
          <span class="updates-card__badge">{osSummary.badge}</span>
        {/if}
      </div>
      <div class="updates-card__actions">
        {#if hasAvailableOsUpdate}
          <button class="updates-button updates-button--primary" on:click={applyOS} disabled={working}>
            {working ? 'Applying…' : 'Apply update'}
          </button>
        {/if}
        {#if canRollback}
          <button class="updates-button updates-button--secondary" on:click={rollbackOS} disabled={working}>
            {working ? 'Working…' : 'Reboot to finish'}
          </button>
        {/if}
        {#if !hasAvailableOsUpdate && !canRollback}
          <button class="updates-button updates-button--secondary" on:click={load} disabled={working}>
            Check again
          </button>
        {/if}
      </div>
    </section>

    <section class="updates-card">
      <div class="updates-card__header">
        <div>
          <h2>App updates</h2>
          <p>
            {#if appList.length === 0}
              All installed apps are current.
            {:else}
              Updates ready for {appList.length} {appList.length === 1 ? 'app' : 'apps'}.
            {/if}
          </p>
        </div>
      </div>
      {#if appList.length > 0}
        <ul class="updates-app-list">
          {#each appList as a}
            <li>
              <div class="updates-app-list__info">
                <p class="updates-app-list__name">{a.name}</p>
                <p class="updates-app-list__meta">
                  {#if a.update_available}
                    {sanitizeVersion(a.available)} available (current {sanitizeVersion(a.current)})
                  {:else}
                    Running {sanitizeVersion(a.current)}
                  {/if}
                </p>
              </div>
              <div class="updates-app-list__actions">
                {#if a.update_available}
                  <button class="updates-button updates-button--tertiary" on:click={() => updateApp(a.name)} disabled={working}>
                    {working ? 'Updating…' : 'Update'}
                  </button>
                {/if}
                {#if a.previous}
                  <button class="updates-button updates-button--secondary" on:click={() => revertApp(a.name)} disabled={working}>
                    Revert
                  </button>
                {/if}
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </section>
  </div>
{/if}

<style>
  .updates-header {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin-bottom: 24px;
  }
  .updates-header h1 {
    font-size: 1.75rem;
    font-weight: 600;
    color: var(--text-primary);
  }
  .updates-header p {
    font-size: 0.95rem;
    color: var(--text-muted);
  }
  .remote-shell {
    border: 1px solid rgba(var(--border-rgb) / 0.16);
    border-radius: 24px;
    background: var(--surface-1);
    padding: 32px;
    text-align: center;
    box-shadow: 0 18px 40px rgba(15, 23, 42, 0.08);
    display: flex;
    flex-direction: column;
    gap: 12px;
    align-items: center;
  }
  .remote-shell__headline {
    font-size: 1.2rem;
    font-weight: 600;
    color: var(--text-primary);
  }
  .remote-shell__copy {
    font-size: 0.9rem;
    color: var(--text-muted);
    max-width: 48ch;
  }
  .remote-shell__action {
    border-radius: 999px;
    border: 1px solid var(--border);
    padding: 10px 20px;
    font-size: 0.9rem;
    font-weight: 600;
    background: var(--surface-0);
    color: var(--text-primary);
    cursor: pointer;
  }
  .remote-shell--error {
    border-color: rgba(var(--state-critical-rgb) / 0.36);
    background: rgba(var(--state-critical-rgb) / 0.12);
  }
  .updates-grid {
    display: grid;
    gap: 20px;
  }
  @media (min-width: 900px) {
    .updates-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
  .updates-card {
    border: 1px solid rgba(var(--border-rgb) / 0.16);
    border-radius: 24px;
    background: var(--surface-1);
    padding: 24px 28px;
    box-shadow: 0 12px 32px rgba(15, 23, 42, 0.08);
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .updates-card__header {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .updates-card__header h2 {
    font-size: 1.1rem;
    font-weight: 600;
    color: var(--text-primary);
  }
  .updates-card__header p {
    font-size: 0.9rem;
    color: var(--text-muted);
  }
  .updates-card__badge {
    align-self: flex-start;
    margin-top: 4px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border-radius: 999px;
    padding: 4px 12px;
    font-size: 0.75rem;
    font-weight: 600;
    letter-spacing: 0.12em;
    background: rgba(var(--accent-rgb) / 0.12);
    color: var(--accent-emphasis);
  }
  .updates-card__actions {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
  }
  .updates-button {
    border-radius: 999px;
    padding: 10px 20px;
    font-size: 0.85rem;
    font-weight: 600;
    cursor: pointer;
    transition: transform var(--transition-duration) var(--transition-easing);
  }
  .updates-button--primary {
    background: var(--accent);
    color: var(--text-inverse);
    border: 1px solid var(--accent-emphasis);
  }
  .updates-button--secondary {
    background: var(--surface-1);
    color: var(--text-primary);
    border: 1px solid rgba(var(--border-rgb) / 0.2);
  }
  .updates-button--tertiary {
    background: rgba(var(--accent-rgb) / 0.12);
    color: var(--accent-emphasis);
    border: 1px solid rgba(var(--accent-rgb) / 0.2);
  }
  .updates-button:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  .updates-app-list {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .updates-app-list li {
    border: 1px solid rgba(var(--border-rgb) / 0.12);
    border-radius: 16px;
    padding: 14px 16px;
    display: flex;
    justify-content: space-between;
    gap: 16px;
    background: var(--surface-0);
  }
  .updates-app-list__info {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .updates-app-list__name {
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--text-primary);
  }
  .updates-app-list__meta {
    font-size: 0.8rem;
    color: var(--text-muted);
  }
  .updates-app-list__actions {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  @media (max-width: 640px) {
    .updates-card {
      padding: 20px;
    }
    .updates-card__actions,
    .updates-app-list__actions {
      flex-direction: column;
      align-items: stretch;
    }
    .updates-button {
      width: 100%;
    }
  }
</style>
