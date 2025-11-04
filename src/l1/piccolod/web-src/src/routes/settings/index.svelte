<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { preferencesStore, backgroundOptions, setTheme, setBackground, type ThemeMode } from '@stores/preferences';
  import { featuresStore, enableFeature, disableFeature, type FeatureRecord } from '@stores/features';

  const themes: { id: ThemeMode; label: string; description: string }[] = [
    { id: 'system', label: 'Follow system', description: 'Match your operating system preference.' },
    { id: 'light', label: 'Light', description: 'Bright surfaces and subtle shadows.' },
    { id: 'dark', label: 'Dark', description: 'Dimmed surfaces and neon highlights.' },
    { id: 'high-contrast', label: 'High contrast', description: 'Solid backgrounds and strong outlines.' }
  ];

  $: preferences = $preferencesStore;
  $: features = $featuresStore;

  let featuresSection: HTMLElement | null = null;

  function scrollToFeatureSection(id?: FeatureRecord['id']) {
    if (!featuresSection) return;
    featuresSection.scrollIntoView({ behavior: 'smooth', block: 'start' });
    if (id) {
      const card = featuresSection.querySelector<HTMLElement>(`[data-feature-id="${id}"]`);
      card?.focus({ preventScroll: true });
    }
  }

  const sectionListener = (event: Event) => {
    const custom = event as CustomEvent<string>;
    scrollToFeatureSection(custom.detail);
  };

  onMount(() => {
    window.addEventListener('piccolo-open-settings-section', sectionListener);
    window.dispatchEvent(new CustomEvent('piccolo-settings-ready'));
  });

  onDestroy(() => {
    window.removeEventListener('piccolo-open-settings-section', sectionListener);
  });

  function selectTheme(theme: ThemeMode) {
    setTheme(theme);
  }

  function selectBackground(backgroundId: string) {
    setBackground(backgroundId);
  }

  function toggleFeature(feature: FeatureRecord) {
    if (feature.enabled) {
      disableFeature(feature.id);
    } else {
      enableFeature(feature.id);
    }
  }
</script>

<section class="space-y-10">
  <header>
    <h2 class="text-2xl font-semibold text-text-primary">Settings</h2>
    <p class="text-sm text-text-muted">Personalize Piccolo and enable optional capabilities.</p>
  </header>

  <section aria-labelledby="appearance-title" class="space-y-6">
    <div>
      <h3 id="appearance-title" class="text-lg font-semibold text-text-primary">Appearance</h3>
      <p class="text-sm text-text-muted">Choose a theme and background that fits your workspace.</p>
    </div>

    <div class="grid gap-4 lg:grid-cols-2">
      <article class="rounded-2xl border border-border-subtle bg-surface-1 p-5 space-y-4">
        <header class="space-y-1">
          <h4 class="text-sm font-semibold text-text-primary">Theme</h4>
          <p class="text-xs text-text-muted">Switch between light, dark, or high-contrast modes. System follows your OS preference.</p>
        </header>
        <div class="grid gap-2">
          {#each themes as theme}
            <button
              type="button"
              class={`flex items-start justify-between gap-4 rounded-xl border border-border-subtle px-4 py-3 text-left transition hover:border-accent focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus ${preferences.theme === theme.id ? 'bg-accent-subtle/40 border-accent/60' : ''}`}
              on:click={() => selectTheme(theme.id)}
            >
              <span>
                <span class="block text-sm font-semibold text-text-primary">{theme.label}</span>
                <span class="block text-xs text-text-muted">{theme.description}</span>
              </span>
              <span class="text-xs font-semibold text-text-muted" aria-hidden="true">{preferences.theme === theme.id ? 'Selected' : 'Select'}</span>
            </button>
          {/each}
        </div>
      </article>

      <article class="rounded-2xl border border-border-subtle bg-surface-1 p-5 space-y-4">
        <header class="space-y-1">
          <h4 class="text-sm font-semibold text-text-primary">Background</h4>
          <p class="text-xs text-text-muted">Piccolo ships with six built-in backgrounds. High-contrast mode enforces a solid surface.</p>
        </header>
        <div class="grid gap-3 sm:grid-cols-2">
          {#each backgroundOptions as option}
            <button
              type="button"
              class={`flex flex-col gap-2 rounded-2xl border border-border-subtle bg-surface-1 p-3 text-left transition hover:border-accent focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus ${preferences.background === option.id ? 'bg-accent-subtle/40 border-accent/60' : ''}`}
              on:click={() => selectBackground(option.id)}
            >
              <span
                class="h-20 w-full rounded-xl border border-border-subtle"
                style={`background:${option.preview}`}
              />
              <div>
                <span class="block text-sm font-semibold text-text-primary">{option.name}</span>
                <span class="block text-[11px] text-text-muted">{option.description}</span>
              </div>
            </button>
          {/each}
        </div>
      </article>
    </div>
  </section>

  <section id="features" aria-labelledby="features-title" class="space-y-6" bind:this={featuresSection}>
    <div>
      <h3 id="features-title" class="text-lg font-semibold text-text-primary">Optional capabilities</h3>
      <p class="text-sm text-text-muted">Enable shared infrastructure when you are ready. Piccolo hides advanced flows until you switch them on.</p>
    </div>

    <div class="grid gap-4">
      {#each features as feature}
        <article class="rounded-2xl border border-border-subtle bg-surface-1 p-5 flex flex-col gap-3" tabindex="-1" data-feature-id={feature.id}>
          <header class="flex items-start justify-between gap-3">
            <div>
              <h4 class="text-sm font-semibold text-text-primary">{feature.name}</h4>
              <p class="text-xs text-text-muted">{feature.description}</p>
            </div>
            <button
              type="button"
              class={`px-4 py-2 rounded-xl text-xs font-semibold border border-border-subtle transition ${feature.enabled ? 'bg-accent text-text-inverse border-transparent' : ''}`}
              on:click={() => toggleFeature(feature)}
            >
              {feature.enabled ? 'Disable' : 'Enable'}
            </button>
          </header>
          {#if feature.enabled}
            <p class="text-xs text-state-notice bg-state-notice/10 border border-state-notice/20 rounded-xl px-3 py-2">Enabled — related options now appear in Home and Quick settings.</p>
          {/if}
        </article>
      {/each}
    </div>
  </section>
</section>
