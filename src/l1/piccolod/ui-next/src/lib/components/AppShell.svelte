<script lang="ts">
  import StalenessBanner from '$lib/components/StalenessBanner.svelte';
  import Button from '$lib/components/ui/Button.svelte';
  import { logout } from '$lib/api/setup';
  import { resetCsrfToken, primeCsrfToken } from '$lib/api/http';
  import { platformController, platformState } from '$lib/stores/platform';
  import { goto } from '$app/navigation';

  export let title = 'Piccolo';

  let loggingOut = false;
  let logoutError = '';

  async function handleLogout() {
    loggingOut = true;
    logoutError = '';
    try {
      await primeCsrfToken();
      await logout();
      resetCsrfToken();
      await platformController.refreshSession();
      goto('/login');
    } catch (error) {
      logoutError = (error as Error)?.message ?? 'Unable to log out.';
    } finally {
      loggingOut = false;
    }
  }

  $: session = $platformState.session;
</script>

<div class="min-h-screen text-ink" style="background: var(--hero-gradient);">
  <header class="border-b border-white/20 bg-white/40 backdrop-blur-xl text-ink">
    <div class="mx-auto flex max-w-5xl items-center justify-between px-6 py-4">
      <div
        class="flex items-center gap-3 cursor-pointer"
        aria-label={title}
        role="button"
        tabindex="0"
        on:click={() => goto('/') }
        on:keydown={(event) => {
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault();
            goto('/');
          }
        }}
      >
        <div class="logo-full hidden xl:block">
          <img src="/piccolo.svg" alt="Piccolo" class="logo-full__img logo-full__img--light" />
          <img src="/piccolo-white.svg" alt="Piccolo" class="logo-full__img logo-full__img--dark" />
        </div>
        <img src="/piccolo-p.svg" alt="Piccolo mark" class="logo-mark h-10 w-10 xl:hidden" />
      </div>
      <div class="flex items-center gap-3">
        <slot name="header-actions"></slot>
        {#if session?.authenticated}
          <div class="flex flex-col items-end gap-1">
            {#if logoutError}
              <p class="text-xs text-red-600">{logoutError}</p>
            {/if}
            <Button variant="ghost" size="compact" on:click={handleLogout} loading={loggingOut}>
              {loggingOut ? 'Signing out…' : 'Sign out'}
            </Button>
          </div>
        {/if}
      </div>
    </div>
  </header>
  <main class="mx-auto max-w-5xl px-6 py-10">
    <StalenessBanner />
    <slot />
  </main>
</div>

<style>
  .logo-full__img {
    display: none;
    height: 32px;
    width: auto;
  }

  .logo-full__img--light {
    display: block;
  }

  :global([data-theme='dark']) .logo-full__img--light {
    display: none;
  }

  :global([data-theme='dark']) .logo-full__img--dark {
    display: block;
  }

  .logo-mark {
    transition: filter 150ms ease;
  }

  :global([data-theme='dark']) .logo-mark {
    filter: brightness(0) invert(1);
  }
</style>
