<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { onDestroy } from 'svelte';
  import Button from '$lib/components/ui/Button.svelte';
  import { login } from '$lib/api/setup';
  import { platformController } from '$lib/stores/platform';
  import { primeCsrfToken, resetCsrfToken } from '$lib/api/http';
  import type { ApiError } from '$lib/api/http';

  export let data;

  let password = '';
  let submitting = false;
  let localError = '';
  let redirectTarget = '/';
  let redirectProvided = false;

  const unsubscribe = page.subscribe(($page) => {
    const raw = decodeRedirect($page.url.searchParams.get('redirect'));
    redirectProvided = Boolean(raw);
    redirectTarget = sanitizeRedirect(raw);
  });

  onDestroy(() => {
    unsubscribe();
  });

  function sanitizeRedirect(value: string | null): string {
    if (!value) return '/';
    const trimmed = value.trim();
    if (!trimmed.startsWith('/')) return '/';
    if (trimmed.startsWith('//')) return '/';
    return trimmed || '/';
  }

  function decodeRedirect(value: string | null): string | null {
    if (!value) return null;
    try {
      return decodeURIComponent(value);
    } catch {
      return value;
    }
  }

  async function handleLogin(event: SubmitEvent) {
    event.preventDefault();
    localError = '';
    if (!password) {
      localError = 'Enter the admin password to continue.';
      return;
    }
    submitting = true;
    try {
      await login(password);
      resetCsrfToken();
      await platformController.refreshSession();
      await primeCsrfToken();
      goto(redirectTarget || '/');
    } catch (error) {
      const apiError = error as ApiError | undefined;
      localError = apiError?.message ?? 'Login failed. Check the password and try again.';
    } finally {
      submitting = false;
    }
  }
</script>

<svelte:head>
  <title>Piccolo · Sign in</title>
</svelte:head>

{#if data.redirectTo}
  {#if typeof window !== 'undefined'}
    {goto(data.redirectTo)}
  {/if}
  <p class="text-sm text-muted">You are already signed in. Redirecting…</p>
{:else}
<div class="flex flex-col gap-6">
  <section class="rounded-3xl border border-white/30 bg-white/80 backdrop-blur-xl p-6 elev-3 text-ink space-y-2">
    <p class="meta-label">Sign in</p>
    <h1 class="text-2xl font-semibold">Unlock access</h1>
    <p class="text-sm text-muted max-w-2xl">Use the admin password to access the Piccolo dashboard.</p>
  </section>

  <form class="rounded-3xl border border-white/30 bg-white/90 backdrop-blur-xl elev-2 p-6 flex flex-col gap-4" on:submit={handleLogin}>
    {#if localError}
      <p class="text-sm text-red-600">{localError}</p>
    {/if}
    <div class="flex flex-col gap-2">
      <label class="text-sm font-medium text-ink" for="login-password">Admin password</label>
      <input
        id="login-password"
        class="w-full rounded-2xl border border-slate-200 px-4 py-3 text-base focus:border-accent focus:outline-none"
        type="password"
        bind:value={password}
        placeholder="••••••••"
        disabled={submitting}
      />
      <p class="text-xs text-muted">Forgot it? <a class="font-semibold text-accent underline" href="/password-recovery?redirect=%2Flogin">Reset with your recovery key</a>.</p>
    </div>
    <div class="rounded-2xl border border-slate-200 px-4 py-3 text-xs text-muted">
      <p class="font-semibold text-ink">Need help?</p>
      <p class="mt-1">Use the recovery reset link above if you’ve lost the password.</p>
    </div>
    <div class="flex gap-3 flex-wrap">
      <Button variant="ghost" type="button" href="/">
        Cancel
      </Button>
      <Button variant="primary" type="submit" loading={submitting} disabled={!password}>
        {submitting ? 'Signing in…' : redirectProvided ? 'Continue' : 'Sign in'}
      </Button>
    </div>
  </form>
</div>
{/if}
