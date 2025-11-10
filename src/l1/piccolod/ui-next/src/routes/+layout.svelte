<script lang="ts">
  import { QueryClientProvider } from '@tanstack/svelte-query';
  import '../app.css';
  import { queryClient } from '$lib/clients/queryClient';
  import favicon from '$lib/assets/favicon.svg';
  import { preferencesStore } from '$lib/stores/preferences';
  import AppShell from '$lib/components/AppShell.svelte';

  const prefs = preferencesStore;
</script>

<svelte:head>
  <link rel="icon" href={favicon} />
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
