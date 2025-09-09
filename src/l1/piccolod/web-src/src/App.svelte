<script lang="ts">
  import { onMount } from 'svelte';
  import { sessionStore, bootstrapSession } from './stores/session';
  import Router, { link } from 'svelte-spa-router';
  import Dashboard from '@routes/index.svelte';
  import Apps from '@routes/apps/index.svelte';
  import AppDetails from '@routes/apps/[name].svelte';
  import Toast from '@components/Toast.svelte';
  import Storage from '@routes/storage/index.svelte';
  import Updates from '@routes/updates/index.svelte';
  import Remote from '@routes/remote/index.svelte';
  const routes: Record<string, any> = {
    '/': Dashboard,
    '/apps': Apps,
    '/apps/:name': AppDetails,
    '/storage': Storage,
    '/updates': Updates,
    '/remote': Remote,
  };
  onMount(async () => {
    await bootstrapSession();
    // Enforce hash-based deep links for v1
    if (!window.location.hash && window.location.pathname !== '/') {
      window.location.replace('/#' + window.location.pathname);
    }
  });
</script>

<main class="min-h-screen bg-gray-50 text-gray-900">
  <header class="border-b bg-white">
    <div class="max-w-6xl mx-auto px-4 py-3 flex items-center justify-between">
      <h1 class="font-semibold">Piccolo OS</h1>
      <nav class="text-sm space-x-4">
        <a href="/#/" class="hover:underline">Dashboard</a>
        <a href="/#/apps" class="hover:underline">Apps</a>
        <a href="/#/storage" class="hover:underline">Storage</a>
        <a href="/#/updates" class="hover:underline">Updates</a>
        <a href="/#/remote" class="hover:underline">Remote</a>
      </nav>
    </div>
  </header>

  <section class="max-w-6xl mx-auto p-4" id="router-root">
    <Router {routes} />
    <p class="text-sm text-gray-500 mt-6">Session: {JSON.stringify($sessionStore)}</p>
  </section>
  <Toast />
</main>
