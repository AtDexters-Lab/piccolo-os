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
  import Login from '@routes/login/index.svelte';
  import Setup from '@routes/setup/index.svelte';
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
    '/login': Login,
    '/setup': Setup,
  };
  let menuOpen = false;
  function updateNavDisplay() {
    const el = document.getElementById('main-nav') as HTMLElement | null;
    if (!el) return;
    el.classList.remove('hidden');
    const mobile = window.matchMedia('(max-width: 767px)').matches;
    if (mobile) {
      el.style.display = menuOpen ? 'flex' : 'none';
    } else {
      el.style.display = '';
    }
  }
  onMount(async () => {
    await bootstrapSession();
    // Enforce hash-based deep links for v1
    if (!window.location.hash && window.location.pathname !== '/') {
      window.location.replace('/#' + window.location.pathname);
    }
    updateNavDisplay();
    window.addEventListener('resize', updateNavDisplay);
  });
</script>

<main class="min-h-screen bg-gray-50 text-gray-900">
  <header class="border-b bg-white">
    <div class="max-w-6xl mx-auto px-4 py-3 flex items-center justify-between relative">
      <h1 class="font-semibold">
        <a href="/#/" class="inline-flex items-center" aria-label="Piccolo">
          <img src="/branding/piccolo.svg" alt="Piccolo" class="h-6 w-auto" loading="lazy" />
        </a>
      </h1>
      <button class="md:hidden px-3 py-2 text-sm border rounded inline-flex items-center justify-center cursor-pointer min-h-[44px]" aria-controls="main-nav" aria-expanded={menuOpen} aria-label="Toggle menu" on:click={() => { menuOpen = !menuOpen; updateNavDisplay(); }}>Menu</button>
      {#if menuOpen}
        <nav
          id="main-nav"
          class="text-sm flex fixed right-4 top-12 z-40 bg-white border rounded p-3 flex-col gap-2"
        >
          <a href="/#/" class="hover:underline" on:click={() => (menuOpen = false)}>Dashboard</a>
          <a href="/#/apps" class="hover:underline" on:click={() => (menuOpen = false)}>Apps</a>
          <a href="/#/storage" class="hover:underline" on:click={() => (menuOpen = false)}>Storage</a>
          <a href="/#/updates" class="hover:underline" on:click={() => (menuOpen = false)}>Updates</a>
          <a href="/#/remote" class="hover:underline" on:click={() => (menuOpen = false)}>Remote</a>
          <a href="/#/install" class="hover:underline" on:click={() => (menuOpen = false)}>Install</a>
          <a href="/#/backup" class="hover:underline" on:click={() => (menuOpen = false)}>Backup</a>
          <a href="/#/events" class="hover:underline" on:click={() => (menuOpen = false)}>Events</a>
          <a href="/#/settings" class="hover:underline" on:click={() => (menuOpen = false)}>Settings</a>
          {#if !$sessionStore.authenticated}
            <a href="/#/login" class="hover:underline" on:click={() => (menuOpen = false)}>Sign in</a>
          {:else}
            <button class="text-left hover:underline" on:click={() => { fetch((import.meta.env.VITE_API_DEMO==='1'? '/api/v1/demo':'/api/v1') + '/auth/logout').finally(()=>{ sessionStore.set({authenticated:false}); window.location.hash = '/login'; }); menuOpen=false; }}>Logout</button>
          {/if}
        </nav>
      {/if}
      <nav id="main-nav-desktop" class="text-sm hidden md:flex md:flex-wrap md:static md:bg-transparent md:border-0 md:p-0 gap-2 md:gap-4">
        <a href="/#/" class="hover:underline">Dashboard</a>
        <a href="/#/apps" class="hover:underline">Apps</a>
        <a href="/#/storage" class="hover:underline">Storage</a>
        <a href="/#/updates" class="hover:underline">Updates</a>
        <a href="/#/remote" class="hover:underline">Remote</a>
        <a href="/#/install" class="hover:underline">Install</a>
        <a href="/#/backup" class="hover:underline">Backup</a>
        <a href="/#/events" class="hover:underline">Events</a>
        <a href="/#/settings" class="hover:underline">Settings</a>
        {#if !$sessionStore.authenticated}
          <a href="/#/login" class="hover:underline">Sign in</a>
        {:else}
          <button class="hover:underline" on:click={() => { fetch((import.meta.env.VITE_API_DEMO==='1'? '/api/v1/demo':'/api/v1') + '/auth/logout').finally(()=>{ sessionStore.set({authenticated:false}); window.location.hash = '/login'; }); }}>Logout</button>
        {/if}
      </nav>
    </div>
  </header>

  <section class="max-w-6xl mx-auto p-4" id="router-root">
    <Router {routes} useHash={true} />
    <p class="text-sm text-gray-500 mt-6">Session: {JSON.stringify($sessionStore)}</p>
  </section>
  <Toast />
</main>
