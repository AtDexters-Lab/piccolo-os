<script lang="ts">
  import { onMount } from 'svelte';
  import { apiProd } from '@api/client';
  import { toast } from '@stores/ui';
  import { sessionStore } from '@stores/session';

  let disks: any = null;
  let loading = true;
  let error = '';

  let recovery: any = null;
  let loadingRecovery = false;
  let working = false;
  let unlockPassword = '';

  $: session = $sessionStore;

  async function loadDisks() {
    loading = true;
    error = '';
    try {
      disks = await apiProd('/storage/disks');
    } catch (e: any) {
      error = e?.message || 'Failed to load disks';
      disks = null;
    } finally {
      loading = false;
    }
  }

  async function loadRecovery() {
    loadingRecovery = true;
    try {
      recovery = await apiProd('/crypto/recovery-key');
    } catch (e: any) {
      recovery = null;
    } finally {
      loadingRecovery = false;
    }
  }

  onMount(() => {
    loadDisks();
    loadRecovery();
  });

  async function unlock() {
    if (!unlockPassword) {
      toast('Enter your admin password to unlock', 'error');
      return;
    }
    working = true;
    try {
      await apiProd('/crypto/unlock', {
        method: 'POST',
        body: JSON.stringify({ password: unlockPassword })
      });
      toast('Volumes unlocked', 'success');
      await loadDisks();
      try {
        const sessionInfo: any = await apiProd('/auth/session');
        sessionStore.set(sessionInfo);
      } catch (err) {
        console.warn('Unable to refresh session', err);
      }
    } catch (e: any) {
      toast(e?.message || 'Unlock failed', 'error');
    } finally {
      working = false;
      unlockPassword = '';
    }
  }

  async function generateRecoveryKey() {
    working = true;
    try {
      const res: any = await apiProd('/crypto/recovery-key/generate', { method: 'POST' });
      recovery = { words: res.words, present: true };
      toast('Recovery key generated', 'success');
    } catch (e: any) {
      toast(e?.message || 'Recovery key generation failed', 'error');
    } finally {
      working = false;
    }
  }

  $: volumes = disks?.disks ?? [];
  $: volumesLocked = !!session?.volumes_locked;
  $: storageStatusTitle = volumesLocked ? 'Volumes locked' : 'Volumes ready';
  $: storageStatusCopy = volumesLocked
    ? 'Unlock encrypted volumes before deploying services.'
    : volumes.length
      ? `${volumes.length} volume${volumes.length > 1 ? 's' : ''} mounted and ready for Piccolo services.`
      : 'Piccolo is unlocked, but no additional disks were detected.';
  $: storageHeroHint = volumesLocked
    ? 'Encryption keeps device data sealed until the admin password is provided.'
    : 'Encrypted app data lives under /var/piccolo/apps/<app>/data.';
</script>

<div class="storage-page space-y-6">
  <section class={`storage-hero ${volumesLocked ? 'storage-hero--locked' : 'storage-hero--ready'}`} aria-live="polite">
    <div>
      <p class="storage-hero__eyebrow">Storage status</p>
      <h1 class="storage-hero__title">{storageStatusTitle}</h1>
      <p class="storage-hero__copy">{storageStatusCopy}</p>
    </div>
    <div class="storage-hero__actions">
      {#if volumesLocked}
        <label class="storage-hero__field">
          <span>Admin password</span>
          <input type="password" autocomplete="current-password" bind:value={unlockPassword} placeholder="••••••••" />
        </label>
        <button class="storage-hero__cta" on:click={unlock} disabled={!unlockPassword || working}>
          {working ? 'Unlocking…' : 'Unlock volumes'}
        </button>
      {:else}
        <div class="storage-hero__ready">
          <p class="storage-hero__badge">{volumes.length} mounted</p>
          <button class="storage-hero__ghost" on:click={loadDisks} disabled={loading}>
            {loading ? 'Refreshing…' : 'Refresh disks'}
          </button>
        </div>
      {/if}
      <p class="storage-hero__hint">{storageHeroHint}</p>
    </div>
  </section>

  <div class="storage-grid">
    <section class="storage-card">
      <div class="storage-card__header">
        <div>
          <h2>Volumes</h2>
          <p>Attached disks Piccolo can provision.</p>
        </div>
        <button class="storage-link" on:click={loadDisks} disabled={loading}>
          {loading ? 'Refreshing…' : 'Refresh'}
        </button>
      </div>
      {#if loading}
        <p class="storage-card__body">Loading disks…</p>
      {:else if error}
        <p class="storage-card__error">{error}</p>
      {:else if volumes.length === 0}
        <div class="storage-card__empty">
          <p>No additional disks detected.</p>
          <p>Add a drive or attach a volume, then refresh.</p>
        </div>
      {:else}
        <ul class="storage-volume-list">
          {#each volumes as disk}
            <li>
              <p class="storage-volume-list__name">{disk.id || disk.path}</p>
              <p class="storage-volume-list__meta">{disk.model || 'Unknown model'} · {disk.size_bytes ? Math.round(disk.size_bytes / 1e9) : '?'} GB</p>
            </li>
          {/each}
        </ul>
      {/if}
    </section>

    <section class="storage-card">
      <div class="storage-card__header">
        <div>
          <h2>Recovery key</h2>
          <p>Used once to rekey devices or recover access.</p>
        </div>
      </div>
      {#if loadingRecovery}
        <p class="storage-card__body">Checking recovery key…</p>
      {:else if recovery?.words}
        <div class="storage-recovery">
          <p class="storage-card__body">Write these words down and store them offline. Generating again invalidates the previous key.</p>
          <div class="storage-recovery__grid">
            {#each recovery.words as word, i}
              <span>{i + 1}. {word}</span>
            {/each}
          </div>
        </div>
      {:else}
        <div class="storage-card__empty">
          <p>No recovery key has been generated yet.</p>
          <button class="storage-hero__cta" on:click={generateRecoveryKey} disabled={working}>
            {working ? 'Generating…' : 'Generate recovery key'}
          </button>
        </div>
      {/if}
    </section>
  </div>

  <p class="storage-footer">Need more capacity? Attach a new disk, then unlock and refresh to make it available for services.</p>
</div>

<style>
  .storage-page {
    padding-bottom: 40px;
  }
  .storage-hero {
    border: 1px solid rgba(var(--border-rgb) / 0.16);
    border-radius: 32px;
    padding: 32px;
    display: flex;
    flex-direction: column;
    gap: 20px;
    box-shadow: 0 18px 48px rgba(15, 23, 42, 0.08);
  }
  .storage-hero--locked {
    background: rgba(var(--state-warn-rgb) / 0.08);
  }
  .storage-hero--ready {
    background: rgba(var(--state-ok-rgb) / 0.08);
  }
  .storage-hero__eyebrow {
    text-transform: uppercase;
    letter-spacing: 0.18em;
    font-size: 0.7rem;
    font-weight: 600;
    color: var(--text-muted);
  }
  .storage-hero__title {
    font-size: 1.75rem;
    font-weight: 600;
    color: var(--text-primary);
  }
  .storage-hero__copy {
    font-size: 1rem;
    color: var(--text-muted);
    max-width: 48ch;
  }
  .storage-hero__actions {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .storage-hero__field {
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-size: 0.9rem;
    color: var(--text-primary);
  }
  .storage-hero__field input {
    border: 1px solid rgba(var(--border-rgb) / 0.2);
    border-radius: 16px;
    padding: 12px 14px;
    font-size: 1rem;
    background: var(--surface-1);
  }
  .storage-hero__cta {
    border-radius: 999px;
    border: 1px solid var(--accent-emphasis);
    padding: 12px 20px;
    font-size: 0.95rem;
    font-weight: 600;
    background: var(--accent);
    color: var(--text-inverse);
    cursor: pointer;
  }
  .storage-hero__cta:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  .storage-hero__ready {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .storage-hero__badge {
    display: inline-flex;
    align-items: center;
    padding: 6px 14px;
    border-radius: 999px;
    font-size: 0.85rem;
    font-weight: 600;
    background: rgba(var(--state-ok-rgb) / 0.16);
    color: rgb(var(--state-ok-rgb));
  }
  .storage-hero__ghost {
    border-radius: 999px;
    border: 1px solid rgba(var(--border-rgb) / 0.2);
    padding: 10px 18px;
    font-size: 0.9rem;
    background: var(--surface-1);
    cursor: pointer;
  }
  .storage-hero__ghost:disabled {
    opacity: 0.6;
  }
  .storage-hero__hint {
    font-size: 0.85rem;
    color: var(--text-muted);
  }
  .storage-grid {
    display: grid;
    gap: 20px;
  }
  @media (min-width: 900px) {
    .storage-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
  .storage-card {
    border: 1px solid rgba(var(--border-rgb) / 0.16);
    border-radius: 24px;
    background: var(--surface-1);
    padding: 24px 28px;
    box-shadow: 0 12px 32px rgba(15, 23, 42, 0.08);
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .storage-card__header {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  @media (min-width: 768px) {
    .storage-card__header {
      flex-direction: row;
      align-items: center;
      justify-content: space-between;
    }
  }
  .storage-card__header h2 {
    font-size: 1.1rem;
    font-weight: 600;
    color: var(--text-primary);
  }
  .storage-card__header p {
    font-size: 0.9rem;
    color: var(--text-muted);
  }
  .storage-card__body {
    font-size: 0.9rem;
    color: var(--text-muted);
  }
  .storage-card__error {
    font-size: 0.9rem;
    color: rgb(var(--state-critical-rgb));
  }
  .storage-card__empty {
    border: 1px dashed rgba(var(--border-rgb) / 0.3);
    border-radius: 18px;
    padding: 16px;
    font-size: 0.9rem;
    color: var(--text-muted);
    background: rgba(var(--border-rgb) / 0.04);
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .storage-volume-list {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .storage-volume-list li {
    padding: 12px 14px;
    border: 1px solid rgba(var(--border-rgb) / 0.12);
    border-radius: 16px;
    background: var(--surface-0);
  }
  .storage-volume-list__name {
    font-weight: 600;
    color: var(--text-primary);
  }
  .storage-volume-list__meta {
    font-size: 0.8rem;
    color: var(--text-muted);
  }
  .storage-recovery__grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
    gap: 8px;
    font-size: 0.85rem;
    color: var(--text-primary);
  }
  .storage-recovery__grid span {
    border: 1px solid rgba(var(--border-rgb) / 0.12);
    border-radius: 12px;
    padding: 8px 10px;
    background: var(--surface-0);
  }
  .storage-link {
    border: none;
    background: none;
    color: var(--accent-emphasis);
    font-size: 0.85rem;
    font-weight: 600;
    text-decoration: underline;
    cursor: pointer;
  }
  .storage-footer {
    font-size: 0.85rem;
    color: var(--text-muted);
  }
  @media (max-width: 640px) {
    .storage-hero,
    .storage-card {
      padding: 20px;
    }
  }
</style>
