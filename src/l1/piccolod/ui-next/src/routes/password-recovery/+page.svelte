<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { onDestroy } from 'svelte';
  import Button from '$lib/components/ui/Button.svelte';
  import { resetPasswordWithRecovery } from '$lib/api/setup';
  import type { ApiError } from '$lib/api/http';

  let recoveryKey = '';
  let newPassword = '';
  let confirmPassword = '';
  let submitting = false;
  let success = false;
  let errorMessage = '';
  let infoMessage = '';
  let redirectTarget = '/login';
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
    if (!value) return '/login';
    const trimmed = value.trim();
    if (!trimmed.startsWith('/')) return '/login';
    if (trimmed.startsWith('//')) return '/login';
    return trimmed || '/login';
  }

  function decodeRedirect(value: string | null): string | null {
    if (!value) return null;
    try {
      return decodeURIComponent(value);
    } catch {
      return value;
    }
  }

  $: passwordValid = newPassword.length >= 8 && newPassword === confirmPassword;
  $: recoveryKeyProvided = recoveryKey.trim().length > 0;
  $: canSubmit = passwordValid && recoveryKeyProvided && !submitting;

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    errorMessage = '';
    infoMessage = '';
    if (!passwordValid) {
      errorMessage = 'Passwords must match and be at least 8 characters.';
      return;
    }
    if (!recoveryKeyProvided) {
      errorMessage = 'Enter the 24-word recovery key.';
      return;
    }
    submitting = true;
    try {
      await resetPasswordWithRecovery({ recoveryKey: recoveryKey.trim(), newPassword });
      success = true;
      infoMessage = 'Password reset successful. Use the new password to unlock or sign in.';
    } catch (error) {
      const apiError = error as ApiError | undefined;
      errorMessage = apiError?.message ?? 'Unable to reset password with the recovery key.';
    } finally {
      submitting = false;
    }
  }

  function handleContinue() {
    goto(redirectTarget || '/login');
  }
</script>

<svelte:head>
  <title>Piccolo · Password Recovery</title>
</svelte:head>

<main class="flex flex-col gap-6">
  <section class="rounded-3xl border border-white/30 bg-white/80 backdrop-blur-xl p-6 elev-3 text-ink space-y-3">
    <div class="flex flex-wrap items-baseline justify-between gap-4">
      <div>
        <p class="meta-label">Recovery</p>
        <h1 class="mt-2 text-2xl font-semibold">Reset admin password</h1>
      </div>
      <span class="text-xs text-muted">Recovery resets mark credentials as stale for audit.</span>
    </div>
    <p class="text-sm text-muted max-w-3xl">
      Paste the 24-word recovery key to set a new admin password. Piccolo unlocks temporarily to rotate the credentials,
      relocks, and flags the password and recovery key as stale until you acknowledge the warning from the dashboard.
    </p>
  </section>

  {#if success}
    <section class="rounded-3xl border border-emerald-200 bg-emerald-50/90 px-6 py-5 text-sm text-emerald-900 elev-2 flex flex-col gap-3">
      <p class="font-semibold text-lg text-emerald-900">Password updated</p>
      <p>Continue to {redirectProvided ? 'your destination' : 'the unlock screen'} and sign in with the new password.</p>
      <div class="flex flex-wrap gap-3">
        <Button variant="primary" on:click={handleContinue}>Continue</Button>
        <Button variant="secondary" href="/unlock">Go to unlock</Button>
      </div>
    </section>
  {:else}
    <form class="rounded-3xl border border-white/30 bg-white/90 backdrop-blur-xl elev-2 p-6 flex flex-col gap-4" on:submit={handleSubmit}>
      {#if errorMessage}
        <p class="text-sm text-red-600">{errorMessage}</p>
      {/if}
      {#if infoMessage}
        <p class="text-sm text-green-700">{infoMessage}</p>
      {/if}
      <div class="flex flex-col gap-2">
        <label class="text-sm font-medium text-ink" for="recovery-key">Recovery key</label>
        <textarea
          id="recovery-key"
          class="min-h-[110px] rounded-2xl border border-slate-200 px-4 py-3 text-base focus:border-accent focus:outline-none"
          bind:value={recoveryKey}
          placeholder="word1 word2 ... word24"
          spellcheck={false}
          disabled={submitting}
        ></textarea>
        <p class="text-xs text-muted">Paste the exact 24 words generated during setup. Keep spaces between words.</p>
      </div>
      <div class="grid gap-4 md:grid-cols-2">
        <div class="flex flex-col gap-2">
          <label class="text-sm font-medium text-ink" for="new-password">New password</label>
          <input
            id="new-password"
            class="w-full rounded-2xl border border-slate-200 px-4 py-3 text-base focus:border-accent focus:outline-none"
            type="password"
            bind:value={newPassword}
            placeholder="New admin password"
            autocomplete="new-password"
            disabled={submitting}
          />
        </div>
        <div class="flex flex-col gap-2">
          <label class="text-sm font-medium text-ink" for="confirm-password">Confirm password</label>
          <input
            id="confirm-password"
            class="w-full rounded-2xl border border-slate-200 px-4 py-3 text-base focus:border-accent focus:outline-none"
            type="password"
            bind:value={confirmPassword}
            placeholder="Repeat password"
            autocomplete="new-password"
            disabled={submitting}
          />
        </div>
      </div>
      <div class="rounded-2xl border border-slate-200 px-4 py-3 text-xs text-muted">
        <p class="font-semibold text-ink">What happens next</p>
        <ul class="mt-1 list-disc list-inside space-y-1">
          <li>Piccolo unlocks temporarily to rotate the credentials, then relocks.</li>
          <li>Password & recovery key are marked as stale until you acknowledge the warning after signing in.</li>
          <li>The operation is rate limited and audited. Keep this browser open until it completes.</li>
        </ul>
      </div>
      <div class="flex flex-wrap gap-3">
        <Button variant="ghost" type="button" href="/unlock">Back to unlock</Button>
        <Button variant="primary" type="submit" disabled={!canSubmit} loading={submitting}>
          {submitting ? 'Resetting…' : 'Reset password'}
        </Button>
      </div>
    </form>
  {/if}
</main>

<style>
  textarea {
    resize: vertical;
  }
</style>
