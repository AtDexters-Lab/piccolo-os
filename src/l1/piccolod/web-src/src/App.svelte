<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte';
  import Router from 'svelte-spa-router';
  import { wrap } from 'svelte-spa-router/wrap';
  import { get } from 'svelte/store';

  import { sessionStore, bootstrapSession } from './stores/session';
  import { deviceStore, setRemoteSummary, markRemoteHydrated, recordRemoteError, markRemoteUnsupported } from './stores/device';
  import { preferencesStore, applyTheme, applyBackground } from './stores/preferences';
  import { refreshActivity } from '@stores/activity';
  import { apiProd, type ErrorResponse } from '@api/client';
  import Toast from '@components/Toast.svelte';
  import RouteLoading from '@components/RouteLoading.svelte';
  import QuickSettingsPanel from '@components/quick-settings/QuickSettingsPanel.svelte';
  import RemoteSetupWizard from '@components/remote/RemoteSetupWizard.svelte';
  import ActivityTray from '@components/activity/ActivityTray.svelte';
  import Home from '@routes/home/index.svelte';
  import AppsInstalled from '@routes/apps/index.svelte';
  import AppDetails from '@routes/apps/[name].svelte';
  import Catalog from '@routes/apps/catalog.svelte';
  import Storage from '@routes/storage/index.svelte';
  import Updates from '@routes/updates/index.svelte';
  import Remote from '@routes/remote/index.svelte';
  import Devices from '@routes/devices/index.svelte';
  import Settings from '@routes/settings/index.svelte';
  import Lock from '@routes/lock/index.svelte';
  import Login from '@routes/login/index.svelte';
  import Setup from '@routes/setup/index.svelte';

  const DEMO = __DEMO__;
  const DEBUG = import.meta.env.VITE_UI_DEBUG === '1';

  type Layout = 'phone' | 'tablet' | 'desktop';
  type NavItem = { id: string; label: string; path: string; badge?: number | null };
  type RemoteStatusPayload = {
    enabled?: boolean;
    state?: string | null;
    hostname?: string | null;
    portal_hostname?: string | null;
    public_url?: string | null;
    warnings?: string[];
  };

  let layout: Layout = 'desktop';
  let currentPath = '/';
  let deviceName = 'Piccolo Home';
  let quickSettingsOpen = false;
  let remoteWizardOpen = false;
  let sessionExpired = false;
  let installBanner = false;
  let remoteHydratePending = false;
  let remoteHydrateTimer: ReturnType<typeof setTimeout> | null = null;
  let systemMediaQuery: MediaQueryList | null = null;
  let systemMediaListener: ((event: MediaQueryListEvent) => void) | null = null;
  let preferencesUnsubscribe: (() => void) | null = null;
  let preferences = { theme: 'system', background: 'aurora' };
  let activityOpen = false;
  let activityOpenListener: (() => void) | null = null;
  let wizardListener: (() => void) | null = null;
  let session = { authenticated: false, volumes_locked: false } as { authenticated: boolean; volumes_locked?: boolean };
  let sessionUnsubscribe: (() => void) | null = null;

  const navItems: NavItem[] = [
    { id: 'home', label: 'Home', path: '/' },
    { id: 'apps', label: 'Apps', path: '/apps' },
    { id: 'devices', label: 'Devices', path: '/devices' },
    { id: 'settings', label: 'Settings', path: '/settings' }
  ];

  const authGuard = async () => {
    if (DEMO) return true;
    const session = get(sessionStore);
    if (session.authenticated) return true;
    try {
      await bootstrapSession();
    } catch {
      /* swallow */
    }
    const refreshed = get(sessionStore);
    if (refreshed.authenticated) return true;
    if (refreshed.volumes_locked) {
      window.location.hash = '/lock';
      return false;
    }

    try {
      const init = await apiProd<{ initialized?: boolean }>('/auth/initialized');
      if (!init?.initialized) {
        window.location.hash = '/setup';
        return false;
      }
    } catch {
      /* ignore */
    }
    window.location.hash = '/login';
    return false;
  };

  const routes: Record<string, any> = {
    '/': wrap({ component: Home, conditions: [authGuard], loadingComponent: RouteLoading }),
    '/apps': wrap({ component: AppsInstalled, conditions: [authGuard], loadingComponent: RouteLoading }),
    '/apps/catalog': wrap({ component: Catalog, conditions: [authGuard], loadingComponent: RouteLoading }),
    '/apps/:name': wrap({ component: AppDetails, conditions: [authGuard], loadingComponent: RouteLoading }),
    '/devices': wrap({ component: Devices, conditions: [authGuard], loadingComponent: RouteLoading }),
    '/storage': wrap({ component: Storage, conditions: [authGuard], loadingComponent: RouteLoading }),
    '/updates': wrap({ component: Updates, conditions: [authGuard], loadingComponent: RouteLoading }),
    '/remote': wrap({ component: Remote, conditions: [authGuard], loadingComponent: RouteLoading }),
    '/settings': wrap({ component: Settings, conditions: [authGuard], loadingComponent: RouteLoading }),
    '/lock': wrap({ component: Lock }),
    '/login': wrap({ component: Login }),
    '/setup': wrap({ component: Setup })
  };

  function ensureHashRouting() {
    if (!window.location.hash && window.location.pathname !== '/') {
      window.location.replace('/#' + window.location.pathname);
    }
  }

  function updateCurrentPath() {
    const raw = window.location.hash || '#/';
    const clean = raw.startsWith('#') ? raw.slice(1) : raw;
    currentPath = clean || '/';
  }

  function computeLayout() {
    if (window.matchMedia('(min-width: 1024px)').matches) {
      layout = 'desktop';
    } else if (window.matchMedia('(min-width: 768px)').matches) {
      layout = 'tablet';
    } else {
      layout = 'phone';
    }
  }

  function navigate(path: string) {
    quickSettingsOpen = false;
    activityOpen = false;
    const target = path.startsWith('#') ? path : `#${path}`;
    if (window.location.hash === target) {
      // re-tap behavior: scroll to top
      const main = document.getElementById('main-content');
      if (main) {
        main.scrollTo({ top: 0, behavior: 'smooth' });
      }
      return;
    }
    window.location.hash = target;
  }

  function isActive(path: string): boolean {
    if (!path.startsWith('/')) path = `/${path}`;
    if (path === '/') return currentPath === '/';
    if (currentPath === path) return true;
    if (path === '/apps') {
      return currentPath.startsWith('/apps');
    }
    return currentPath.startsWith(path);
  }

  let deviceNameHydrated = false;

  async function hydrateDeviceName() {
    try {
      const res = await apiProd<{ device_name?: string }>('/device');
      deviceName = res?.device_name?.trim() || 'Piccolo Home';
      deviceStore.update((prev) => ({
        ...prev,
        name: deviceName
      }));
      deviceNameHydrated = true;
    } catch {
      deviceName = 'Piccolo Home';
      deviceStore.update((prev) => ({
        ...prev,
        name: deviceName
      }));
    }
  }

  function applyRemoteStatus(payload: RemoteStatusPayload | null) {
    const host = payload?.portal_hostname || payload?.hostname || payload?.public_url || null;
    setRemoteSummary({
      enabled: payload?.enabled,
      hostname: host,
      state: payload?.state,
      warnings: payload?.warnings
    });
  }

  async function hydrateRemoteStatus() {
    try {
      const data = await apiProd<RemoteStatusPayload>('/remote/status');
      if (data) {
        applyRemoteStatus(data);
      } else {
        setRemoteSummary(null);
      }
      markRemoteHydrated();
      recordRemoteError(null);
    } catch (err: unknown) {
      const error = err as ErrorResponse | undefined;
      if (error?.code === 404) {
        markRemoteUnsupported();
        recordRemoteError(null);
        return;
      }
      if (error?.code === 401 || error?.code === 403) {
        // wait until authenticated to hydrate
        return;
      }
      recordRemoteError(error?.message || 'Failed to load remote status');
      throw err;
    }
  }

  function attachListeners() {
    window.addEventListener('resize', computeLayout);
    window.addEventListener('hashchange', async () => {
      updateCurrentPath();
      await tick();
      const main = document.getElementById('main-content');
      if (main) {
        main.focus();
      }
    });
    window.addEventListener('piccolo-session-expired', () => {
      sessionExpired = true;
      sessionStore.set({ authenticated: false });
      window.location.hash = '/login';
    });
    window.addEventListener('storage', (event: StorageEvent) => {
      if (event.key === 'piccolo_install_started') {
        installBanner = event.newValue === '1';
      }
    });
    activityOpenListener = () => {
      activityOpen = true;
      refreshActivity();
    };
    window.addEventListener('piccolo-open-activity', activityOpenListener);
    window.addEventListener('keydown', (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      const tag = target?.tagName?.toLowerCase();
      const isTypingContext = target?.isContentEditable || tag === 'input' || tag === 'textarea';
      if (event.key === '/' && !event.metaKey && !event.ctrlKey && !event.altKey && !isTypingContext) {
        event.preventDefault();
        if (layout === 'desktop') {
          const panel = document.getElementById('quick-settings-desktop');
          panel?.focus();
          panel?.scrollTo({ top: 0, behavior: 'smooth' });
        } else {
          quickSettingsOpen = true;
        }
      }
      if (event.key === 'Escape') {
        if (quickSettingsOpen) quickSettingsOpen = false;
        if (activityOpen) activityOpen = false;
      }
    });
  }

  function dismissInstallBanner() {
    try {
      localStorage.removeItem('piccolo_install_started');
    } catch {
      /* ignore */
    }
    installBanner = false;
  }

  async function handleLogout() {
    try {
      await apiProd('/auth/logout', { method: 'POST' });
    } catch {
      /* ignore network errors; session fallback below */
    } finally {
      quickSettingsOpen = false;
      activityOpen = false;
      sessionStore.set({ authenticated: false });
      window.location.hash = '/login';
    }
  }

  function handleQuickSettingsToggle() {
    quickSettingsOpen = !quickSettingsOpen;
  }

  function openRemoteWizard() {
    remoteWizardOpen = true;
    quickSettingsOpen = false;
  }

  function closeRemoteWizard() {
    remoteWizardOpen = false;
  }

  function toggleActivityTray() {
    activityOpen = !activityOpen;
    if (activityOpen) {
      refreshActivity();
    }
  }

  function ensureTheme() {
    const root = document.documentElement;
    if (!root.dataset.theme) {
      root.dataset.theme = 'light';
    }
  }

  function bindSystemThemeListener(themeMode: string) {
    if (typeof window === 'undefined') return;
    if (systemMediaListener && systemMediaQuery) {
      systemMediaQuery.removeEventListener('change', systemMediaListener);
      systemMediaListener = null;
    }
    if (themeMode !== 'system') return;
    systemMediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    systemMediaListener = () => applyTheme('system');
    systemMediaQuery.addEventListener('change', systemMediaListener);
  }

  onMount(async () => {
    ensureHashRouting();
    ensureTheme();

    try {
      installBanner = localStorage.getItem('piccolo_install_started') === '1';
    } catch {
      installBanner = false;
    }

    preferencesUnsubscribe = preferencesStore.subscribe((value) => {
      preferences = value;
      applyTheme(value.theme);
      applyBackground(value.background);
      bindSystemThemeListener(value.theme);
    });
    sessionUnsubscribe = sessionStore.subscribe((value) => {
      session = value ?? { authenticated: false, volumes_locked: false };
    });

    await bootstrapSession();
    session = get(sessionStore) ?? { authenticated: false, volumes_locked: false };
    await Promise.allSettled([hydrateDeviceName(), hydrateRemoteStatus()]);
    const initialSession = get(sessionStore);
    if (!initialSession.authenticated && initialSession.volumes_locked) {
      window.location.hash = '/lock';
    }

    computeLayout();
    updateCurrentPath();
    attachListeners();
    const openWizardListener = () => openRemoteWizard();
    window.addEventListener('remote-wizard-open', openWizardListener);
    activityOpenListener = () => {
      activityOpen = true;
      refreshActivity();
    };
    window.addEventListener('piccolo-open-activity', activityOpenListener);
    wizardListener = openWizardListener;
  });

  $: if (session.authenticated) {
    sessionExpired = false;
  }

  $: if (layout === 'desktop' && quickSettingsOpen) {
    quickSettingsOpen = false;
  }

  $: if (quickSettingsOpen && layout !== 'desktop') {
    tick().then(() => {
      const el = document.querySelector('[data-quick-settings-modal] [data-focus-initial]') as HTMLElement | null;
      el?.focus();
    });
  }

  function scheduleRemoteHydrateRetry() {
    if (remoteHydrateTimer) return;
    remoteHydrateTimer = setTimeout(() => {
      remoteHydrateTimer = null;
      triggerRemoteHydrate();
    }, 5000);
  }

  function triggerRemoteHydrate() {
    if (remoteHydratePending || !session.authenticated || $deviceStore.remoteHydrated) return;
    remoteHydratePending = true;
    hydrateRemoteStatus()
      .catch(() => {
        scheduleRemoteHydrateRetry();
      })
      .finally(() => {
        remoteHydratePending = false;
      });
  }

  $: if (session.authenticated && !$deviceStore.remoteHydrated) {
    triggerRemoteHydrate();
  }

  $: if (session.authenticated && currentPath === '/lock') {
    navigate('/');
  }

  $: if (session.authenticated && !deviceNameHydrated) {
    hydrateDeviceName();
  }

  onDestroy(() => {
    preferencesUnsubscribe?.();
    sessionUnsubscribe?.();
    if (systemMediaListener && systemMediaQuery) {
      systemMediaQuery.removeEventListener('change', systemMediaListener);
    }
    if (remoteHydrateTimer) {
      clearTimeout(remoteHydrateTimer);
      remoteHydrateTimer = null;
    }
    if (activityOpenListener) {
      window.removeEventListener('piccolo-open-activity', activityOpenListener);
    }
    if (wizardListener) {
      window.removeEventListener('remote-wizard-open', wizardListener);
    }
  });
</script>

<div class="app-shell" data-layout={layout}>
  <a href="#main-content" class="skip-link">Skip to content</a>
  <div class="app-shell__body" data-layout={layout}>
    <aside class="app-shell__nav-desktop hidden lg:block" aria-label="Primary">
      <div class="flex items-center gap-3 mb-6">
        <img src="/branding/piccolo.svg" alt="Piccolo logo" class="h-6 w-auto" loading="lazy" />
        <div>
          <p class="text-sm text-text-muted uppercase tracking-[0.08em]">Device</p>
          <p class="text-lg font-semibold text-text-primary">{deviceName}</p>
        </div>
      </div>
      <nav class="space-y-2">
        {#each navItems as item}
          <button
            class="w-full text-left px-3 py-2 rounded-lg transition-colors duration-[var(--transition-duration)]"
            class:bg-surface-2={isActive(item.path)}
            class:text-accent-emphasis={isActive(item.path)}
            class:text-text-muted={!isActive(item.path)}
            aria-current={isActive(item.path) ? 'page' : undefined}
            on:click={() => navigate(item.path)}
          >
            <span class="text-sm font-medium flex items-center justify-between">
              <span>{item.label}</span>
              {#if item.badge}
                <span class="ml-2 inline-flex items-center justify-center text-xs font-semibold px-2 py-0.5 rounded-full bg-accent-subtle text-accent-emphasis">
                  {item.badge}
                </span>
              {/if}
            </span>
          </button>
        {/each}
      </nav>
      <div class="mt-8 border-t border-border-subtle pt-4">
        {#if !session.authenticated}
          <button class="w-full px-3 py-2 rounded-lg border border-border-subtle text-sm font-semibold" on:click={() => navigate('/login')}>
            Sign in
          </button>
        {:else}
          <button class="w-full px-3 py-2 rounded-lg border border-border-subtle text-sm font-semibold" on:click={handleLogout}>
            Sign out
          </button>
        {/if}
      </div>
    </aside>

    <main class="app-shell__main">
      <header class="flex items-center justify-between px-5 h-[var(--header-height)] border-b border-border-subtle bg-surface-1/70 backdrop-blur-sm sticky top-0 z-30">
        <div class="flex items-center gap-3">
          <button class="lg:hidden inline-flex items-center justify-center h-10 w-10 rounded-full border border-border-subtle" on:click={() => navigate('/settings')} aria-label="Open settings">
            <span class="sr-only">Settings</span>
            <svg class="h-5 w-5 text-text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="3.5" />
              <path d="M19.4 12a7.38 7.38 0 0 0-.08-.99l2.11-1.65-2-3.46-2.49 1a7.42 7.42 0 0 0-1.71-.99L14.5 2h-5l-.73 2.91a7.35 7.35 0 0 0-1.71.99l-2.49-1-2 3.46 2.11 1.65a7.35 7.35 0 0 0 0 1.98L1.57 14.6l2 3.46 2.49-1a7.35 7.35 0 0 0 1.71.99L9.5 22h5l.73-2.91a7.42 7.42 0 0 0 1.71-.99l2.49 1 2-3.46-2.11-1.65c.05-.33.08-.66.08-.99z" />
            </svg>
          </button>
          <div>
            <p class="text-xs uppercase tracking-[0.18em] text-text-muted">Piccolo Home</p>
            <h1 class="text-lg font-semibold text-text-primary leading-tight">{deviceName}</h1>
          </div>
        </div>
        <div class="flex items-center gap-3">
          <button
            class="inline-flex items-center justify-center gap-2 px-4 py-2 rounded-full bg-surface-2 text-sm font-semibold text-text-primary border border-border-subtle hover:bg-surface-1 transition"
            aria-pressed={activityOpen}
            on:click={toggleActivityTray}
          >
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M3 5h18" />
              <path d="M7 5v14" />
              <rect x="7" y="9" width="13" height="10" rx="2" />
            </svg>
            Activity
          </button>
          <button
            class="inline-flex items-center justify-center gap-2 px-4 py-2 rounded-full bg-surface-2 text-sm font-semibold text-text-primary border border-border-subtle hover:bg-surface-1 transition"
            aria-pressed={layout !== 'desktop' ? String(quickSettingsOpen) : undefined}
            aria-controls="quick-settings-drawer"
            on:click={handleQuickSettingsToggle}
          >
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M12 6v12" />
              <path d="M18 12H6" />
            </svg>
            Quick settings
          </button>
          {#if !session.authenticated}
            <a class="inline-flex lg:hidden items-center justify-center gap-2 px-4 py-2 rounded-full bg-surface-2 text-sm font-semibold text-text-primary border border-border-subtle hover:bg-surface-1 transition" href="/#/login">
              Sign in
            </a>
          {:else}
            <button
              class="inline-flex lg:hidden items-center justify-center gap-2 px-4 py-2 rounded-full border border-border-subtle text-sm font-semibold text-text-muted"
              on:click={handleLogout}
            >
              Logout
            </button>
          {/if}
        </div>
      </header>

      <section class="app-shell__page" id="main-content" tabindex="-1">
        {#if sessionExpired}
          <div class="mb-4 p-4 border border-state-critical/20 rounded-xl bg-state-critical/10 text-state-critical" role="alert">
            <p class="text-sm font-semibold">Session expired</p>
            <p class="text-xs mt-1">Please sign in again to continue.</p>
            <button class="mt-3 inline-flex items-center gap-2 px-3 py-1.5 rounded-lg border border-border-subtle text-xs font-semibold" on:click={() => navigate('/login')}>
              Sign in
            </button>
          </div>
        {/if}

        {#if installBanner}
          <div class="mb-4 p-4 rounded-xl border border-state-notice/30 bg-state-notice/10 text-text-primary flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
            <div>
              <p class="text-sm font-semibold text-text-primary">Installation in progress</p>
              <p class="text-xs text-text-muted">The device will reboot when complete. You can safely leave this page.</p>
            </div>
            <button class="self-start md:self-center px-3 py-1.5 rounded-lg border border-border-subtle text-xs font-semibold" on:click={dismissInstallBanner}>
              Dismiss
            </button>
          </div>
        {/if}

        <Router {routes} useHash={true} />
        {#if DEBUG}
          <pre class="mt-8 text-xs bg-surface-2 border border-border-subtle rounded-lg p-3 text-text-muted">Session: {JSON.stringify(session, null, 2)}</pre>
        {/if}
      </section>
    </main>

    {#if quickSettingsOpen && layout === 'desktop'}
      <div class="quick-settings-overlay quick-settings-overlay--desktop" role="dialog" aria-modal="true" aria-labelledby="quick-settings-desktop-title" data-quick-settings-modal>
        <div class="quick-settings-overlay__scrim" aria-hidden="true" on:click={() => quickSettingsOpen = false}></div>
        <div class="quick-settings-overlay__panel quick-settings-overlay__panel--desktop" role="document">
          <div class="quick-settings-overlay__header">
            <div>
              <p class="text-xs uppercase tracking-[0.12em] text-text-muted">Quick settings</p>
              <h2 id="quick-settings-desktop-title" class="text-lg font-semibold text-text-primary">Quick settings</h2>
            </div>
            <button class="quick-settings-overlay__close" on:click={() => quickSettingsOpen = false}>
              <span class="sr-only">Close quick settings</span>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" class="h-5 w-5">
                <path d="M18 6 6 18" />
                <path d="m6 6 12 12" />
              </svg>
            </button>
          </div>
          <QuickSettingsPanel layout="desktop" on:close={() => (quickSettingsOpen = false)} on:logout={handleLogout} />
        </div>
      </div>
    {/if}
  </div>

  {#if quickSettingsOpen && layout !== 'desktop'}
    <div class="quick-settings-overlay" role="dialog" aria-modal="true" aria-labelledby="quick-settings-mobile-title" data-quick-settings-modal>
      <div class="quick-settings-overlay__scrim" aria-hidden="true" on:click={() => quickSettingsOpen = false}></div>
      <div class="quick-settings-overlay__panel" role="document">
        <div class="quick-settings-overlay__header">
          <div>
            <p class="text-xs uppercase tracking-[0.12em] text-text-muted">Quick settings</p>
            <h2 id="quick-settings-mobile-title" class="text-lg font-semibold text-text-primary">Quick settings</h2>
          </div>
          <button class="quick-settings-overlay__close" on:click={() => quickSettingsOpen = false}>
            <span class="sr-only">Close quick settings</span>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" class="h-5 w-5">
              <path d="M18 6 6 18" />
              <path d="m6 6 12 12" />
            </svg>
          </button>
        </div>
        <QuickSettingsPanel layout="mobile" on:close={() => quickSettingsOpen = false} on:logout={handleLogout} />
      </div>
    </div>
  {/if}

  {#if remoteWizardOpen}
    <RemoteSetupWizard on:close={closeRemoteWizard} on:updated={() => remoteWizardOpen = false} />
  {/if}

  <nav class="app-shell__bottom-nav lg:hidden" aria-label="Primary" class:hidden={remoteWizardOpen}>
    {#each navItems as item}
      <button
        class:!text-accent-emphasis={isActive(item.path)}
        class:!bg-accent-subtle={isActive(item.path)}
        aria-current={isActive(item.path) ? 'page' : undefined}
        on:click={() => navigate(item.path)}
      >
        <span aria-hidden="true" class="text-lg">
          {#if item.id === 'home'}🏠{:else if item.id === 'apps'}📦{:else if item.id === 'devices'}🖥️{:else if item.id === 'settings'}⚙️{:else}⬜{/if}
        </span>
        <span>{item.label}</span>
      </button>
    {/each}
  </nav>
  <ActivityTray open={activityOpen} onClose={() => (activityOpen = false)} />
  <Toast />
</div>
