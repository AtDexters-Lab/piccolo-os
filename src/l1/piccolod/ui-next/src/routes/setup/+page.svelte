<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { onDestroy, onMount } from 'svelte';
  import Stepper from '$lib/components/Stepper.svelte';
  import Button from '$lib/components/ui/Button.svelte';
  import type { StepDefinition } from '$lib/types/wizard';
  import { createSetupController, type SetupState, type SubmissionMode } from '$lib/stores/setupState';

  const baseSteps: StepDefinition[] = [
    { id: 'intro', label: 'Welcome', description: 'Review prerequisites' },
    { id: 'credentials', label: 'Credentials', description: 'Unlock Piccolo' },
    { id: 'done', label: 'Finish', description: 'Ready to sign in' }
  ];

  const controller = createSetupController();
  let setupState: SetupState = { phase: 'loading' };
  let steps: StepDefinition[] = baseSteps;
  let activeStep: 'intro' | 'credentials' | 'done' = 'intro';
  let password = '';
  let confirmPassword = '';
  let recoveryKey = '';
  let localError = '';
  let showCredentials = false;
  let redirectProvided = false;
  let redirectTarget = '/';

  const unsubscribe = controller.subscribe((state) => {
    setupState = state;
    if (state.phase === 'ready') {
      showCredentials = false;
    }
  });

  const unsubscribePage = page.subscribe(($page) => {
    const raw = decodeRedirect($page.url.searchParams.get('redirect'));
    redirectProvided = Boolean(raw);
    redirectTarget = sanitizeRedirect(raw);
  });

  onMount(() => {
    void controller.refresh();
  });

  onDestroy(() => {
    unsubscribe();
    unsubscribePage();
  });

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

  function resolveRedirect(kind: 'ready' | 'finish') {
    if (redirectProvided) return redirectTarget;
    return kind === 'ready' ? '/login' : '/';
  }

  $: mode = (() => {
    if (setupState.phase === 'first-run') return 'first-run';
    if (setupState.phase === 'unlock') return 'unlock';
    if (setupState.phase === 'submitting') return setupState.flow;
    return null;
  })() as SubmissionMode | null;
  $: submittingStep = setupState.phase === 'submitting' ? setupState.step : null;
  $: isSubmitting = Boolean(submittingStep);
  $: credentialsActive = setupState.phase === 'submitting'
    ? true
    : Boolean(mode && showCredentials && setupState.phase !== 'ready');
  $: activeStep = setupState.phase === 'ready' ? 'done' : credentialsActive ? 'credentials' : 'intro';
  $: introCtaLabel = setupState.phase === 'ready' ? (redirectProvided ? 'Continue' : 'Go to sign in') : 'Start setup';
  $: finishCtaLabel = redirectProvided && redirectTarget !== '/' ? 'Continue' : 'Go to dashboard';

  $: steps = baseSteps.map((step) => {
    if (step.id === 'credentials' && setupState.phase === 'error') {
      return { ...step, state: 'error' } as StepDefinition;
    }
    if (step.id === 'done' && setupState.phase === 'ready') {
      return { ...step, state: 'success' } as StepDefinition;
    }
    return step;
  });

  $: canSubmit = (() => {
    if (isSubmitting) return false;
    if (mode === 'first-run') return password.length >= 8 && confirmPassword.length >= 8;
    if (mode === 'unlock') return unlockInputProvided();
    return false;
  })();

  function resetForm() {
    password = '';
    confirmPassword = '';
    recoveryKey = '';
    localError = '';
  }

  function passwordsMatch() {
    if (mode === 'first-run') {
      return password.length >= 8 && password === confirmPassword;
    }
    return true;
  }

  function unlockInputProvided() {
    return Boolean(password || recoveryKey.trim());
  }

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    localError = '';
    if (!mode) return;
    if (mode === 'first-run' && !passwordsMatch()) {
      localError = 'Passwords must match and be at least 8 characters.';
      return;
    }
    if (mode === 'unlock' && !unlockInputProvided()) {
      localError = 'Provide your admin password or recovery key to unlock Piccolo.';
      return;
    }
    const submitMode = mode === 'first-run' ? 'first-run' : 'unlock';
    await controller.submitCredentials({
      password,
      recoveryKey: recoveryKey.trim() || undefined,
      mode: submitMode
    });
  }

  function startFlow() {
    if (setupState.phase === 'ready') {
      goto(resolveRedirect('ready'));
      return;
    }
    if (setupState.phase === 'loading') return;
    showCredentials = true;
    localError = '';
  }

  function finish() {
    goto(resolveRedirect('finish'));
  }

  function retry() {
    resetForm();
    showCredentials = false;
    controller.retry();
  }

  const statusChip = null;
</script>

<svelte:head>
  <title>Piccolo · Initial Setup</title>
</svelte:head>

<div class="flex flex-col gap-6">
  <section class="rounded-3xl border border-white/30 bg-white/80 backdrop-blur-xl p-6 elev-3 text-ink">
    <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
      <div class="space-y-3 max-w-2xl">
        <p class="meta-label">First run</p>
        <h1 class="text-2xl font-semibold">Create admin credentials</h1>
        <p class="text-sm text-muted">
          Piccolo uses one password to initialize and unlock encrypted volumes. Keep it secret—there’s only one admin per device.
        </p>
        <div class="flex flex-wrap items-center gap-3">
          <Button variant="ghost" size="compact" on:click={() => controller.refresh()}>
            Refresh status
          </Button>
        </div>
      </div>
      <div class="rounded-2xl border border-white/40 bg-white/70 px-5 py-4 text-xs text-muted lg:max-w-sm">
        <p class="font-semibold text-ink text-sm">Safety checklist</p>
        <ul class="mt-2 space-y-1">
          <li>• Keep the device on wired power + network.</li>
          <li>• This flow erases any temporary unlock state.</li>
          <li>• Remote portal stays locked until setup finishes.</li>
        </ul>
      </div>
    </div>
  </section>

  <Stepper steps={steps} activeId={activeStep} />

  {#if setupState.phase === 'error'}
    <div class="rounded-3xl border border-red-200 bg-red-50/90 px-5 py-4 text-sm text-red-900 elev-1 flex flex-col gap-3">
      <div class="flex items-center justify-between gap-3">
        <p>{setupState.message}</p>
        <Button variant="secondary" size="compact" on:click={retry}>
          Try again
        </Button>
      </div>
      {#if setupState.retryAfter}
        <p class="text-xs text-red-800/80">Please wait {setupState.retryAfter} seconds before retrying.</p>
      {/if}
    </div>
  {/if}

  {#if activeStep === 'intro'}
    <section class="rounded-3xl border border-white/30 bg-white/85 backdrop-blur-xl elev-2 p-6 flex flex-col gap-4">
      <p class="text-sm text-muted max-w-2xl">
        Before continuing, confirm you have physical access to the device. This password unlocks remote publish, encrypted volumes, and recovery actions.
      </p>
      <ul class="rounded-2xl border border-slate-200 px-4 py-4 text-sm text-ink space-y-2">
        <li>• Minimum 8 characters (longer is better).</li>
        <li>• Avoid device serials or reused passwords.</li>
        <li>• Store it securely—there’s no second admin.</li>
      </ul>
      <div class="flex gap-3">
        <Button variant="primary" on:click={startFlow}>
          {introCtaLabel}
        </Button>
      </div>
    </section>
  {:else if activeStep === 'credentials' && mode}
    <form class="rounded-3xl border border-white/30 bg-white/90 backdrop-blur-xl elev-2 p-6 flex flex-col gap-4" on:submit={handleSubmit}>
      {#if localError}
        <p class="text-sm text-red-600">{localError}</p>
      {/if}
      <div class="grid gap-4 md:grid-cols-2">
        <div class="flex flex-col gap-2">
          <label class="text-sm font-medium text-ink" for="admin-password">
            {mode === 'first-run' ? 'Create password' : 'Admin password'}
          </label>
          <input
            id="admin-password"
            class="w-full rounded-2xl border border-slate-200 px-4 py-3 text-base focus:border-accent focus:outline-none"
            type="password"
            bind:value={password}
            placeholder={mode === 'first-run' ? 'New admin password' : 'Enter password'}
            autocomplete="new-password"
            disabled={isSubmitting}
          />
        </div>
        {#if mode === 'first-run'}
          <div class="flex flex-col gap-2">
            <label class="text-sm font-medium text-ink" for="confirm-password">Confirm password</label>
            <input
              id="confirm-password"
              class="w-full rounded-2xl border border-slate-200 px-4 py-3 text-base focus:border-accent focus:outline-none"
              type="password"
              bind:value={confirmPassword}
              placeholder="Repeat password"
              autocomplete="new-password"
              disabled={isSubmitting}
            />
          </div>
        {:else}
          <div class="flex flex-col gap-2">
            <label class="text-sm font-medium text-ink" for="recovery-key">Recovery key (optional)</label>
            <textarea
              id="recovery-key"
              class="min-h-[102px] rounded-2xl border border-slate-200 px-4 py-3 text-base focus:border-accent focus:outline-none"
              bind:value={recoveryKey}
              placeholder="24-word key"
              disabled={isSubmitting}
            ></textarea>
            <p class="text-xs text-muted">Leave blank if you prefer to unlock with the admin password.</p>
          </div>
        {/if}
      </div>
      <div class="rounded-2xl border border-slate-200 px-4 py-3 text-xs text-muted">
        <p class="font-semibold text-ink">Requirements</p>
        <ul class="mt-1 list-disc list-inside space-y-1">
          <li>Minimum 8 characters</li>
          <li>Match confirmation exactly</li>
          <li>Recovery key can unlock later from the Unlock page</li>
        </ul>
      </div>
      <div class="flex gap-3 flex-wrap">
        <Button variant="ghost" type="button" on:click={() => (showCredentials = false)} disabled={isSubmitting}>
          Back
        </Button>
        <Button variant="primary" disabled={!canSubmit} loading={isSubmitting} type="submit">
          {isSubmitting
            ? submittingStep === 'crypto-setup'
              ? 'Initializing…'
              : submittingStep === 'crypto-unlock'
                ? 'Unlocking…'
                : 'Finalizing…'
            : mode === 'first-run'
              ? 'Create admin'
              : 'Unlock Piccolo'}
        </Button>
      </div>
    </form>
  {:else if activeStep === 'done' && setupState.phase === 'ready'}
    <section class="rounded-3xl border border-white/30 bg-white/90 backdrop-blur-xl elev-2 p-6 flex flex-col gap-4 text-ink">
      <p class="meta-label meta-label--lg">Complete</p>
      <h2 class="text-2xl font-semibold">Admin ready</h2>
      <p class="text-sm text-muted">Setup finished. Continue to the dashboard or sign in again later.</p>
      <div class="flex gap-3 flex-wrap">
        <Button variant="primary" on:click={finish}>
          {finishCtaLabel}
        </Button>
        {#if !(setupState.session?.authenticated)}
          <Button variant="secondary" on:click={() => goto('/login')}>
            Sign in now
          </Button>
        {/if}
      </div>
    </section>
  {/if}
</div>

<style>
</style>
