<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { onDestroy, onMount } from 'svelte';
  import Button from '$lib/components/ui/Button.svelte';
  import { createSetupController, type SetupState } from '$lib/stores/setupState';

  const controller = createSetupController();
  let setupState: SetupState = { phase: 'loading' };
  let password = '';
  let localError = '';
  let submitting = false;
  let redirectTarget = '/';
  let passwordRecoveryTarget = '/login';

  function sanitizeRedirect(value: string | null): string {
    if (!value) return '/';
    const trimmed = value.trim();
    if (!trimmed.startsWith('/')) return '/';
    if (trimmed.startsWith('//')) return '/';
    return trimmed;
  }

  function decodeRedirect(value: string | null): string | null {
    if (!value) return null;
    try {
      return decodeURIComponent(value);
    } catch {
      return value;
    }
  }

  const unsubscribe = controller.subscribe((state) => {
    setupState = state;
    if (state.phase === 'ready') {
      goto(redirectTarget);
    }
  });

  const unsubscribePage = page.subscribe(($page) => {
    const decoded = decodeRedirect($page.url.searchParams.get('redirect'));
    redirectTarget = sanitizeRedirect(decoded);
    passwordRecoveryTarget = redirectTarget || '/login';
  });

  onMount(() => {
    void controller.refresh();
  });

  onDestroy(() => {
    unsubscribe();
    unsubscribePage();
  });

  $: passwordRecoveryHref = `/password-recovery?redirect=${encodeURIComponent(passwordRecoveryTarget)}`;

  async function handleUnlock(event: SubmitEvent) {
    event.preventDefault();
    localError = '';
    if (!password) {
      localError = 'Enter your admin password. Use your recovery key on the reset page if needed.';
      return;
    }
    submitting = true;
    await controller.submitCredentials({
      password,
      mode: 'unlock'
    });
    submitting = false;
  }

  function resetForm() {
    password = '';
    localError = '';
  }
</script>

<svelte:head>
  <title>Piccolo · Unlock</title>
</svelte:head>

<div class="flex flex-col gap-6">
  <section class="rounded-3xl border border-white/30 bg-white/80 backdrop-blur-xl p-6 elev-3 text-ink space-y-3">
    <div class="flex flex-wrap items-baseline justify-between gap-3">
      <div>
        <p class="meta-label">Unlock</p>
        <h1 class="mt-2 text-2xl font-semibold">Unlock Piccolo</h1>
      </div>
      <span class="status-chip">Device locked</span>
    </div>
    <p class="text-sm text-muted">
      The device rebooted or locked itself. Enter the admin password to bring services back online, or use the recovery reset link below if the password is lost.
    </p>
  </section>

  {#if setupState.phase === 'error'}
    <div class="rounded-3xl border border-red-200 bg-red-50/90 px-5 py-4 text-sm text-red-900 elev-1 flex flex-col gap-3">
      <p>{setupState.message}</p>
      <div class="flex gap-2">
        <Button variant="secondary" size="compact" on:click={() => controller.retry()}>
          Try again
        </Button>
      </div>
    </div>
  {/if}

  <form
    class="rounded-3xl border border-white/30 bg-white/90 backdrop-blur-xl elev-2 p-6 flex flex-col gap-4"
    on:submit={handleUnlock}
  >
    {#if localError}
      <p class="text-sm text-red-600">{localError}</p>
    {/if}
    <div class="flex flex-col gap-2">
      <label class="text-sm font-medium text-ink" for="unlock-password">Admin password</label>
      <input
        id="unlock-password"
        class="w-full rounded-2xl border border-slate-200 px-4 py-3 text-base focus:border-accent focus:outline-none"
        type="password"
        bind:value={password}
        placeholder="••••••••"
        disabled={submitting}
      />
      <p class="text-xs text-muted">
        Forgot it? <a class="font-semibold text-accent underline" href={passwordRecoveryHref}>Reset with your recovery key</a>
      </p>
    </div>
    <div class="rounded-2xl border border-slate-200 px-4 py-3 text-xs text-muted">
      <p class="font-semibold text-ink">Tip</p>
      <p class="mt-1">Unlock requires the admin password. Recovery reset lives on its own page for auditing.</p>
    </div>
    <div class="flex gap-3 flex-wrap">
      <Button variant="ghost" type="button" on:click={resetForm} disabled={submitting}>
        Reset form
      </Button>
      <Button variant="primary" type="submit" disabled={submitting}>
        {submitting ? 'Unlocking…' : 'Unlock Piccolo'}
      </Button>
    </div>
  </form>
</div>

<style>
  form {
    transition: opacity var(--motion-dur-fast) var(--motion-ease-standard);
  }

  .status-chip {
    border-radius: var(--radius-pill);
    background: rgb(var(--sys-warning) / 0.16);
    color: rgb(var(--sys-on-warning));
    font-size: 0.85rem;
    font-weight: 600;
    padding: 0.35rem 0.85rem;
  }

</style>
