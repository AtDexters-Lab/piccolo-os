<script lang="ts">
  import { QueryClientProvider } from '@tanstack/svelte-query';
  import '../app.css';
  import { queryClient } from '$lib/clients/queryClient';
  import { preferencesStore } from '$lib/stores/preferences';
  import AppShell from '$lib/components/AppShell.svelte';
  import { onMount } from 'svelte';

const prefs = preferencesStore;

onMount(() => {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return;
  const applyScheme = (matches: boolean) => {
    prefs.update((current) => ({ ...current, theme: matches ? 'dark' : 'light' }));
  };
  const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
  applyScheme(mediaQuery.matches);
  const listener = (event: MediaQueryListEvent) => applyScheme(event.matches);
  if (typeof mediaQuery.addEventListener === 'function') {
    mediaQuery.addEventListener('change', listener);
    return () => mediaQuery.removeEventListener('change', listener);
  } else {
    mediaQuery.addListener(listener);
    return () => mediaQuery.removeListener(listener);
  }
});
</script>

<svelte:head>
  <link rel="icon" href="/piccolo-p.svg" media="(prefers-color-scheme: light)" />
  <link rel="icon" href="/piccolo-p-white.svg" media="(prefers-color-scheme: dark)" />
  <link rel="preconnect" href="https://fonts.googleapis.com" />
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="anonymous" />
  <link
    rel="stylesheet"
    href="https://fonts.googleapis.com/css2?family=Comfortaa:wght@500;600&family=Inter:wght@400;500;600;700&display=swap"
  />
</svelte:head>

<QueryClientProvider client={queryClient}>
  <div class="min-h-screen" data-theme={$prefs.theme}>
    <AppShell>
      <slot />
    </AppShell>
  </div>
</QueryClientProvider>
