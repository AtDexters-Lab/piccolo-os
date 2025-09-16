<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { sessionStore, bootstrapSession } from './stores/session';
  import { apiProd } from '@api/client';
  import Router, { link } from 'svelte-spa-router';
  import { wrap } from 'svelte-spa-router/wrap';
  import { get } from 'svelte/store';
  import Dashboard from '@routes/index.svelte';
  import AppDetails from '@routes/apps/[name].svelte';
  import Toast from '@components/Toast.svelte';
  import RouteLoading from '@components/RouteLoading.svelte';
  import Apps from '@routes/apps/index.svelte';
  import Catalog from '@routes/apps/catalog.svelte';
  import Storage from '@routes/storage/index.svelte';
  import Updates from '@routes/updates/index.svelte';
  import Remote from '@routes/remote/index.svelte';
  import Install from '@routes/install/index.svelte';
  import Backup from '@routes/backup/index.svelte';
  import Events from '@routes/events/index.svelte';
  import Settings from '@routes/settings/index.svelte';
  import Login from '@routes/login/index.svelte';
  import Setup from '@routes/setup/index.svelte';
  const DEMO = __DEMO__;
  const DEBUG = (import.meta.env.VITE_UI_DEBUG === '1');
  const authGuard = async () => {
    if (DEMO) return true;
    if (get(sessionStore).authenticated) return true;
    try { await bootstrapSession(); } catch {}
    const ok = !!get(sessionStore).authenticated;
    if (!ok) {
      // Check if admin is initialized; if not, redirect to setup
      try {
        const init: any = await apiProd('/auth/initialized');
        if (!init?.initialized) {
          window.location.hash = '/setup';
          return false;
        }
      } catch {}
      window.location.hash = '/login';
      return false;
    }
    return ok;
  };
  const routes: Record<string, any> = {
    '/': wrap({ component: Dashboard, conditions: [authGuard] }),
    '/apps': wrap({ component: Apps, conditions: [authGuard] }),
    '/apps/catalog': wrap({ component: Catalog, conditions: [authGuard] }),
    '/apps/:name': wrap({ component: AppDetails, conditions: [authGuard] }),
    '/storage': wrap({ component: Storage, conditions: [authGuard] }),
    '/updates': wrap({ component: Updates, conditions: [authGuard] }),
    '/remote': wrap({ component: Remote, conditions: [authGuard] }),
    '/install': wrap({ component: Install, conditions: [authGuard] }),
    '/backup': wrap({ component: Backup, conditions: [authGuard] }),
    '/events': wrap({ component: Events, conditions: [authGuard] }),
    '/settings': wrap({ component: Settings, conditions: [authGuard] }),
    '/login': wrap({ component: Login }),
    '/setup': wrap({ component: Setup }),
  };
  let menuOpen = false;
  // Global post-install banner state
  let showInstallBanner = false;
  let sessionExpired = false;
  let moreOpen = false;
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
  async function focusMain() {
    await tick();
    const main = document.getElementById('router-root') as HTMLElement | null;
    main?.focus();
  }
  async function toggleMenu() {
    menuOpen = !menuOpen;
    updateNavDisplay();
    if (menuOpen) {
      await tick();
      const first = document.querySelector('#main-nav a, #main-nav button') as HTMLElement | null;
      first?.focus();
    }
  }
  onMount(async () => {
    await bootstrapSession();
    // Enforce hash-based deep links for v1
    if (!window.location.hash && window.location.pathname !== '/') {
      window.location.replace('/#' + window.location.pathname);
    }
    try { showInstallBanner = localStorage.getItem('piccolo_install_started') === '1'; } catch {}
    window.addEventListener('storage', (e) => {
      if (e.key === 'piccolo_install_started') {
        showInstallBanner = e.newValue === '1';
      }
    });
    window.addEventListener('piccolo-install-started', () => { showInstallBanner = true; });
    window.addEventListener('piccolo-session-expired', () => {
      sessionExpired = true;
      sessionStore.set({ authenticated: false });
      window.location.hash = '/login';
    });
    updateNavDisplay();
    window.addEventListener('resize', updateNavDisplay);
    window.addEventListener('keydown', (e: KeyboardEvent) => {
      if (e.key === 'Escape' && menuOpen) {
        menuOpen = false;
        updateNavDisplay();
      }
    });
    window.addEventListener('hashchange', () => {
      focusMain();
    });
  });
  $: if ($sessionStore.authenticated) {
    sessionExpired = false;
  }
  function goToLogin() {
    window.location.hash = '/login';
  }
  function dismissInstallBanner() {
    try { localStorage.removeItem('piccolo_install_started'); } catch {}
    showInstallBanner = false;
  }
  function isActive(path: string) {
    const h = window.location.hash || '#/';
    if (!path.startsWith('/')) path = '/' + path;
    return h.startsWith('#' + path);
  }
</script>

<main class="min-h-screen bg-gray-50 text-gray-900">
  <header class="border-b bg-white">
    <div class="max-w-6xl mx-auto px-4 py-3 flex items-center justify-between relative">
      <h1 class="font-semibold">
        <a href="/#/" class="inline-flex items-center" aria-label="Piccolo">
          <img src="/branding/piccolo.svg" alt="Piccolo" class="h-6 w-auto" loading="lazy" />
        </a>
      </h1>
      <button class="md:hidden px-3 py-2 text-sm border rounded inline-flex items-center justify-center cursor-pointer min-h-[44px]" aria-controls="main-nav" aria-expanded={menuOpen} aria-label="Toggle menu" on:click={toggleMenu}>Menu</button>
      {#if menuOpen}
        <nav
          aria-label="Primary"
          id="main-nav"
          class="text-sm flex fixed right-4 top-12 z-40 bg-white border rounded p-3 flex-col gap-2"
        >
          <a href="/#/" class="hover:underline" on:click={() => (menuOpen = false)}>Dashboard</a>
          <a href="/#/apps" class="hover:underline" on:click={() => (menuOpen = false)}>Apps</a>
          <a href="/#/apps/catalog" class="hover:underline" on:click={() => (menuOpen = false)}>Catalog</a>
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
          <button class="text-left hover:underline" on:click={() => { fetch('/api/v1/auth/logout').finally(()=>{ sessionStore.set({authenticated:false}); window.location.hash = '/login'; }); menuOpen=false; }}>Logout</button>
          {/if}
        </nav>
      {/if}
      <nav id="main-nav-desktop" aria-label="Header" class="text-sm hidden md:flex md:flex-wrap md:static md:bg-transparent md:border-0 md:p-0 gap-2 md:gap-4 relative">
        {#if !$sessionStore.authenticated}
          <a href="/#/login" class="hover:underline">Sign in</a>
        {:else}
          <button class="hover:underline" on:click={() => { fetch('/api/v1/auth/logout').finally(()=>{ sessionStore.set({authenticated:false}); window.location.hash = '/login'; }); }}>Logout</button>
        {/if}
      </nav>
    </div>
  </header>

  <a href="#router-root" class="skip-link">Skip to content</a>
  <div class="max-w-6xl mx-auto px-4 md:flex md:gap-6">
    <aside class="hidden md:block md:w-56 py-4">
      <nav aria-label="Sidebar" class="text-sm space-y-4">
        <div>
          <ul class="space-y-1">
            <li><a href="/#/" class="block px-2 py-1 rounded hover:bg-gray-50" aria-current={isActive('/') ? 'page' : undefined}>Overview</a></li>
          </ul>
        </div>
        <div>
          <div class="text-xs uppercase tracking-tight text-gray-500 mb-2">Apps</div>
          <ul class="space-y-1">
            <li><a href="/#/apps" class="block px-2 py-1 rounded hover:bg-gray-50" aria-current={isActive('/apps') ? 'page' : undefined}>Installed</a></li>
            <li><a href="/#/apps/catalog" class="block px-2 py-1 rounded hover:bg-gray-50" aria-current={isActive('/apps/catalog') ? 'page' : undefined}>Catalog</a></li>
          </ul>
        </div>
        <div>
          <div class="text-xs uppercase tracking-tight text-gray-500 mb-2">System</div>
          <ul class="space-y-1">
            <li><a href="/#/storage" class="block px-2 py-1 rounded hover:bg-gray-50" aria-current={isActive('/storage') ? 'page' : undefined}>Storage</a></li>
            <li><a href="/#/updates" class="block px-2 py-1 rounded hover:bg-gray-50" aria-current={isActive('/updates') ? 'page' : undefined}>Updates</a></li>
            <li><a href="/#/remote" class="block px-2 py-1 rounded hover:bg-gray-50" aria-current={isActive('/remote') ? 'page' : undefined}>Remote</a></li>
          </ul>
        </div>
        <div>
          <div class="text-xs uppercase tracking-tight text-gray-500 mb-2">Admin</div>
          <ul class="space-y-1">
            <li><a href="/#/install" class="block px-2 py-1 rounded hover:bg-gray-50" aria-current={isActive('/install') ? 'page' : undefined}>Install</a></li>
            <li><a href="/#/backup" class="block px-2 py-1 rounded hover:bg-gray-50" aria-current={isActive('/backup') ? 'page' : undefined}>Backup</a></li>
            <li><a href="/#/events" class="block px-2 py-1 rounded hover:bg-gray-50" aria-current={isActive('/events') ? 'page' : undefined}>Events</a></li>
            <li><a href="/#/settings" class="block px-2 py-1 rounded hover:bg-gray-50" aria-current={isActive('/settings') ? 'page' : undefined}>Settings</a></li>
          </ul>
        </div>
      </nav>
    </aside>
    <section class="flex-1 py-4" id="router-root" tabindex="-1">
    {#if sessionExpired}
      <div class="mb-4 p-3 border rounded bg-red-50 text-red-900 flex items-start justify-between gap-3" role="alert">
        <div>
          <p class="text-sm font-medium">Session expired</p>
          <p class="text-xs">Please sign in again to continue.</p>
        </div>
        <button class="px-2 py-1 text-xs border rounded hover:bg-red-100" on:click={goToLogin}>Sign in</button>
      </div>
    {/if}
    {#if showInstallBanner}
      <div class="mb-4 p-3 border rounded bg-blue-50 text-blue-900 flex items-start justify-between gap-3">
        <div>
          <p class="text-sm font-medium">Installation in progress</p>
          <p class="text-xs">The device will reboot on completion. You can safely leave this page.</p>
        </div>
        <button class="px-2 py-1 text-xs border rounded hover:bg-blue-100" on:click={dismissInstallBanner}>Dismiss</button>
      </div>
    {/if}
    <Router {routes} useHash={true} />
    {#if DEBUG}
      <p class="text-sm text-gray-500 mt-6">Session: {JSON.stringify($sessionStore)}</p>
    {/if}
    </section>
  </div>
  <Toast />
</main>
