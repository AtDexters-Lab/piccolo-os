<script lang="ts">
  import { deviceStore } from '@stores/device';
  import { featuresStore, enableFeature, type FeatureRecord } from '@stores/features';
  import { writable } from 'svelte/store';

  const STORAGE_REMOTE = 'piccolo_nudge_remote_hidden';
  const STORAGE_FEATURE = 'piccolo_feature_nudges';

  const remoteDismissed = writable<boolean>(false);
  const featureDismissals = writable<Record<string, boolean>>({});

  function loadDismissed() {
    try {
      const value = localStorage.getItem(STORAGE_REMOTE);
      remoteDismissed.set(value === '1');
    } catch {
      remoteDismissed.set(false);
    }

    try {
      const raw = localStorage.getItem(STORAGE_FEATURE);
      featureDismissals.set(raw ? JSON.parse(raw) : {});
    } catch {
      featureDismissals.set({});
    }
  }

  loadDismissed();

  $: remoteEnabled = $deviceStore.remoteEnabled ?? false;
  $: remoteHydrated = !!$deviceStore.remoteHydrated;
  $: remoteSupported = $deviceStore.remoteSupported ?? true;
  $: remoteHidden = $remoteDismissed;
  $: showRemoteNudge = remoteHydrated && remoteSupported && !remoteEnabled && !remoteHidden;

  $: features = $featuresStore;
  $: dismissals = $featureDismissals;
  $: nextFeatureNudge = features.find((feature) => !feature.enabled && !dismissals[feature.id]);

  function enableRemote() {
    window.location.hash = '/remote';
  }

  function dismissRemote() {
    try {
      localStorage.setItem(STORAGE_REMOTE, '1');
    } catch {
      /* ignore */
    }
    remoteDismissed.set(true);
  }

  function dismissFeature(id: string) {
    featureDismissals.update((prev) => {
      const next = { ...prev, [id]: true };
      try {
        localStorage.setItem(STORAGE_FEATURE, JSON.stringify(next));
      } catch {
        /* ignore */
      }
      return next;
    });
  }

  const SETTINGS_READY_EVENT = 'piccolo-settings-ready';

  function sendSettingsSection(id: string) {
    requestAnimationFrame(() => {
      window.dispatchEvent(new CustomEvent('piccolo-open-settings-section', { detail: id }));
    });
  }

  function exploreFeature(feature: FeatureRecord) {
    dismissFeature(feature.id);
    const currentHash = window.location.hash || '#/';
    const normalized = currentHash.startsWith('#') ? currentHash.slice(1) : currentHash;

    if (normalized === '/settings') {
      sendSettingsSection(feature.id);
      return;
    }

    const handleReady = () => {
      window.removeEventListener(SETTINGS_READY_EVENT, handleReady);
      sendSettingsSection(feature.id);
    };

    window.addEventListener(SETTINGS_READY_EVENT, handleReady, { once: true });
    window.location.hash = '/settings';
  }

  function enableFeatureFlow(feature: FeatureRecord) {
    enableFeature(feature.id);
    dismissFeature(feature.id);
  }
</script>

{#if showRemoteNudge}
  <div class="rounded-2xl border border-border-subtle bg-accent-subtle/60 text-text-primary p-4 flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
    <div class="flex items-start gap-3">
      <span class="inline-flex items-center justify-center h-10 w-10 rounded-full bg-accent text-text-inverse font-semibold">R</span>
      <div>
        <p class="text-sm font-semibold text-text-primary">Remote is off</p>
        <p class="text-xs text-text-muted">Enable remote access to reach Piccolo securely over the internet. TPM devices keep data sealed until you unlock.</p>
      </div>
    </div>
    <div class="flex items-center gap-2">
      <button class="px-4 py-2 rounded-xl bg-accent text-text-inverse text-xs font-semibold" on:click={enableRemote}>Enable remote</button>
      <button class="px-3 py-2 rounded-xl border border-border-subtle text-xs font-semibold text-text-muted" on:click={dismissRemote}>Hide</button>
    </div>
  </div>
{:else if nextFeatureNudge}
  <div class="rounded-2xl border border-border-subtle bg-surface-1/90 text-text-primary p-4 flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
    <div class="flex items-start gap-3">
      <span class="inline-flex items-center justify-center h-10 w-10 rounded-full bg-surface-2 text-text-primary font-semibold">+</span>
      <div>
        <p class="text-sm font-semibold text-text-primary">{nextFeatureNudge.name}</p>
        <p class="text-xs text-text-muted">{nextFeatureNudge.description}</p>
      </div>
    </div>
    <div class="flex items-center gap-2">
      <button class="px-4 py-2 rounded-xl bg-surface-2 text-xs font-semibold text-text-primary" on:click={() => exploreFeature(nextFeatureNudge)}>Learn more</button>
      <button class="px-3 py-2 rounded-xl border border-border-subtle text-xs font-semibold text-text-muted" on:click={() => dismissFeature(nextFeatureNudge.id)}>Hide</button>
      <button class="px-4 py-2 rounded-xl bg-accent text-text-inverse text-xs font-semibold" on:click={() => enableFeatureFlow(nextFeatureNudge)}>Enable</button>
    </div>
  </div>
{/if}
