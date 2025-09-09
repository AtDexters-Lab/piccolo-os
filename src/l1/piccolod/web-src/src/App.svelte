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
  import Install from '@routes/install/index.svelte';
  import Backup from '@routes/backup/index.svelte';
  import Events from '@routes/events/index.svelte';
  import Settings from '@routes/settings/index.svelte';
  const routes: Record<string, any> = {

    '/': Dashboard,
    '/apps': Apps,
    '/apps/:name': AppDetails,
    '/storage': Storage,
    '/updates': Updates,
    '/remote': Remote,
    '/install': Install,
    '/backup': Backup,
    '/events': Events,
    '/settings': Settings,
  };
  let menuOpen = false;
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
    <div class="max-w-6xl mx-auto px-4 py-3 flex items-center justify-between relative">
      <h1 class="font-semibold">Piccolo OS</h1>
      <button class="md:hidden px-3 py-2 text-sm border rounded" aria-controls="main-nav" aria-expanded={menuOpen} on:click={() => menuOpen = !menuOpen}>Menu</button>
      <nav
        id="main-nav"
        class="text-sm hidden md:flex md:flex-wrap md:static md:bg-transparent md:border-0 md:p-0"
        class:flex={menuOpen}
        class:fixed={menuOpen}
        class:right-4={menuOpen}
        class:top-12={menuOpen}
        class:z-40={menuOpen}
        class:bg-white={menuOpen}
        class:border={menuOpen}
        class:rounded={menuOpen}
        class:p-3={menuOpen}
        class:flex-col={menuOpen}
      >
        <a href="/#/" class="hover:underline">Dashboard</a>
        <a href="/#/apps" class="hover:underline">Apps</a>
        <a href="/#/storage" class="hover:underline">Storage</a>
        <a href="/#/updates" class="hover:underline">Updates</a>
        <a href="/#/remote" class="hover:underline">Remote</a>
        <a href="/#/install" class="hover:underline">Install</a>
        <a href="/#/backup" class="hover:underline">Backup</a>
        <a href="/#/events" class="hover:underline">Events</a>
        <a href="/#/settings" class="hover:underline">Settings</a>
      </nav>
    </div>
  </header>

  <section class="max-w-6xl mx-auto p-4" id="router-root">
    <Router {routes} />
    <p class="text-sm text-gray-500 mt-6">Session: {JSON.stringify($sessionStore)}</p>
  </section>
  <Toast />
</main>
