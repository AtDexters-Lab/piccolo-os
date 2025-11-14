<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { onDestroy, onMount } from 'svelte';
  import Stepper from '$lib/components/Stepper.svelte';
  import Button from '$lib/components/ui/Button.svelte';
  import type { StepDefinition } from '$lib/types/wizard';
import { createSetupController, type SetupState, type SubmissionMode } from '$lib/stores/setupState';
import { platformController } from '$lib/stores/platform';
import { fetchRecoveryKeyStatus, generateRecoveryKey, acknowledgeStaleness, type RecoveryKeyStatus } from '$lib/api/setup';
import type { ApiError } from '$lib/api/http';

const baseSteps: StepDefinition[] = [
  { id: 'intro', label: 'Welcome', description: 'Review prerequisites' },
  { id: 'credentials', label: 'Credentials', description: 'Unlock Piccolo' },
  { id: 'recovery', label: 'Recovery key', description: 'Save the 24-word key' },
  { id: 'done', label: 'Finish', description: 'Ready to sign in' }
];

  const controller = createSetupController();
  let setupState: SetupState = { phase: 'loading' };
  let steps: StepDefinition[] = baseSteps;
  let activeStep: 'intro' | 'credentials' | 'recovery' | 'done' = 'intro';
  let password = '';
  let confirmPassword = '';
  let localError = '';
  let showCredentials = false;
  let redirectProvided = false;
  let redirectTarget = '/';
  let passwordRecoveryTarget = '/unlock';
  let recoveryStatus: RecoveryKeyStatus | null = null;
  let recoveryStatusError = '';
  let recoveryStatusLoading = false;
  let generatedRecoveryWords: string[] = [];
  let generatingRecoveryKey = false;
  let recoveryAcknowledged = false;
  let recoveryCopyNotice = '';
  let showRegenerateConfirm = false;
  let continueExistingLoading = false;
  let continueExistingError = '';
  let recoveryStepComplete = false;
  let regenerateError = '';

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
    passwordRecoveryTarget = redirectProvided ? redirectTarget || '/unlock' : '/unlock';
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

  async function loadRecoveryStatus() {
    if (recoveryStatusLoading) return;
    recoveryStatusLoading = true;
    recoveryStatusError = '';
    try {
      recoveryStatus = await fetchRecoveryKeyStatus();
    } catch (error) {
      const apiError = error as ApiError | undefined;
      recoveryStatusError = apiError?.message ?? 'Unable to check recovery key status.';
    } finally {
      recoveryStatusLoading = false;
    }
  }

  async function performRecoveryGeneration(): Promise<boolean> {
    generatingRecoveryKey = true;
    recoveryStatusError = '';
    recoveryCopyNotice = '';
    recoveryAcknowledged = false;
    try {
      generatedRecoveryWords = await generateRecoveryKey();
      recoveryStatus = { present: true, stale: false };
      return true;
    } catch (error) {
      const apiError = error as ApiError | undefined;
      recoveryStatusError = apiError?.message ?? 'Failed to generate the recovery key.';
      return false;
    } finally {
      generatingRecoveryKey = false;
    }
  }

  async function handleGenerateRecoveryKey() {
    await performRecoveryGeneration();
  }

  function retryRecoveryStatusCheck() {
    recoveryStatusError = '';
    recoveryStatus = null;
    void loadRecoveryStatus();
  }

  function openRegenerateModal() {
    regenerateError = '';
    showRegenerateConfirm = true;
  }

  function closeRegenerateModal() {
    showRegenerateConfirm = false;
  }

  async function confirmRegenerate() {
    regenerateError = '';
    const success = await performRecoveryGeneration();
    if (success) {
      showRegenerateConfirm = false;
    } else if (!regenerateError) {
      regenerateError = recoveryStatusError || 'Unable to regenerate the recovery key.';
    }
  }

  async function copyRecoveryKey() {
    if (!generatedRecoveryWords.length) {
      recoveryCopyNotice = 'Generate the recovery key before copying.';
      return;
    }
    const words = generatedRecoveryWords.join(' ');
    if (typeof window !== 'undefined' && window.isSecureContext && navigator?.clipboard?.writeText) {
      try {
        await navigator.clipboard.writeText(words);
        recoveryCopyNotice = 'Copied to clipboard. Keep it safe!';
        return;
      } catch {
        // fall through to legacy path
      }
    }

    try {
      const textarea = document.createElement('textarea');
      textarea.value = words;
      textarea.setAttribute('readonly', 'true');
      textarea.style.position = 'absolute';
      textarea.style.left = '-9999px';
      document.body.appendChild(textarea);
      textarea.select();
      textarea.setSelectionRange(0, textarea.value.length);
      const successful = document.execCommand('copy');
      document.body.removeChild(textarea);
      if (!successful) throw new Error('copy failed');
      recoveryCopyNotice = 'Copied to clipboard. Keep it safe!';
    } catch {
      recoveryCopyNotice = 'Unable to copy automatically. Select the words and copy manually or download the file.';
    }
  }

  function downloadRecoveryKey() {
    if (!generatedRecoveryWords.length) return;
    const blob = new Blob([generatedRecoveryWords.join(' ')], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = 'piccolo-recovery-key.txt';
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    setTimeout(() => URL.revokeObjectURL(url), 0);
  }

  function resolveRedirect(kind: 'ready' | 'finish') {
    if (redirectProvided) return redirectTarget;
    return kind === 'ready' ? '/login' : '/';
  }

  $: passwordRecoveryHref = `/password-recovery?redirect=${encodeURIComponent(passwordRecoveryTarget)}`;

  $: shouldCheckRecoveryStatus = setupState.phase === 'ready' && !recoveryStatus && !recoveryStatusLoading && !recoveryStatusError;
  $: if (shouldCheckRecoveryStatus) {
    void loadRecoveryStatus();
  }

  $: hasRecoveryWords = generatedRecoveryWords.length > 0;
  $: needsServerRecovery = !recoveryStatus || !recoveryStatus.present || Boolean(recoveryStatus.stale);
  $: pendingAcknowledgement = hasRecoveryWords && !recoveryAcknowledged;
    $: focusRecoveryOverride = (() => {
    const search = typeof window !== 'undefined' ? new URLSearchParams(window.location.search) : null;
    return search?.get('focus') === 'recovery';
  })();

$: showRecoveryStep = setupState.phase === 'ready' && (!recoveryStepComplete && (needsServerRecovery || pendingAcknowledgement || focusRecoveryOverride || hasRecoveryWords));

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
  $: activeStep = showRecoveryStep
    ? 'recovery'
    : setupState.phase === 'ready'
      ? 'done'
      : credentialsActive
        ? 'credentials'
        : 'intro';
  $: introCtaLabel = setupState.phase === 'ready' ? (redirectProvided ? 'Continue' : 'Go to sign in') : 'Start setup';
  $: finishCtaLabel = redirectProvided && redirectTarget !== '/' ? 'Continue' : 'Go to dashboard';

  $: steps = baseSteps.map((step) => {
    if (step.id === 'credentials' && setupState.phase === 'error') {
      return { ...step, state: 'error' } as StepDefinition;
    }
    if (step.id === 'recovery') {
      if (recoveryStatusError) {
        return { ...step, state: 'error' } as StepDefinition;
      }
      if (!showRecoveryStep && recoveryStatus?.present) {
        return { ...step, state: 'success' } as StepDefinition;
      }
      return step;
    }
    if (step.id === 'done' && setupState.phase === 'ready' && !showRecoveryStep) {
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
    localError = '';
  }

  function passwordsMatch() {
    if (mode === 'first-run') {
      return password.length >= 8 && password === confirmPassword;
    }
    return true;
  }

  function unlockInputProvided() {
    return Boolean(password);
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
      localError = 'Enter the admin password to unlock Piccolo. Use your recovery key on the reset page if needed.';
      return;
    }
    const submitMode = mode === 'first-run' ? 'first-run' : 'unlock';
    await controller.submitCredentials({
      password,
      mode: submitMode
    });
  }

  function startFlow() {
    if (setupState.phase === 'ready') {
      return;
    }
    if (setupState.phase === 'loading') return;
    showCredentials = true;
    localError = '';
  }

  function finish() {
    goto(resolveRedirect('finish'));
  }

  async function clearFocusQuery() {
    if (typeof window === 'undefined') return;
    const url = new URL(window.location.href);
    if (!url.searchParams.has('focus')) return;
    url.searchParams.delete('focus');
    const search = url.search ? `?${url.searchParams.toString()}` : '';
    await goto(`${url.pathname}${search}${url.hash}`, { replaceState: true, noScroll: true });
  }

  async function handleRecoveryContinue({ useExisting } = { useExisting: false }) {
    if (!useExisting) {
      if (!generatedRecoveryWords.length || !recoveryAcknowledged) {
        recoveryCopyNotice = 'Generate and save the recovery key before continuing.';
        return;
      }
      recoveryCopyNotice = '';
      generatedRecoveryWords = [];
      recoveryAcknowledged = false;
      recoveryStatus = { present: true, stale: false };
      recoveryStepComplete = true;
      await clearFocusQuery();
      return;
    }

    continueExistingLoading = true;
    continueExistingError = '';
    try {
      await acknowledgeStaleness({ recovery: true });
      await platformController.refreshSession();
      recoveryStatus = { present: true, stale: false };
      recoveryStepComplete = true;
      await clearFocusQuery();
    } catch (error) {
      const apiError = error as ApiError | undefined;
      continueExistingError = apiError?.message ?? 'Unable to continue with the existing key.';
      return;
    } finally {
      continueExistingLoading = false;
    }
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
            {#if mode !== 'first-run'}
              <p class="text-xs text-muted">
                Forgot it? <a class="font-semibold text-accent underline" href={passwordRecoveryHref}>Reset with your recovery key</a>
              </p>
            {/if}
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
          {/if}
        </div>
      <div class="rounded-2xl border border-slate-200 px-4 py-3 text-xs text-muted">
        <p class="font-semibold text-ink">Requirements</p>
        <ul class="mt-1 list-disc list-inside space-y-1">
          <li>Minimum 8 characters</li>
          <li>Match confirmation exactly</li>
          <li>If the password is lost, use the recovery page to reset it before unlocking.</li>
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
  {:else if activeStep === 'recovery'}
    <section class="rounded-3xl border border-white/30 bg-white/90 backdrop-blur-xl elev-2 p-6 flex flex-col gap-4 text-ink">
      <p class="meta-label meta-label--lg">Recovery</p>
      <h2 class="text-2xl font-semibold">Save your recovery key</h2>
      <p class="text-sm text-muted">Piccolo only shows this key once. Store it somewhere offline before you continue.</p>
      {#if recoveryStatusLoading}
        <p class="text-sm text-muted">Preparing recovery status…</p>
      {:else}
        {#if recoveryStatusError}
          <div class="flex flex-col gap-2">
            <p class="text-sm text-red-600">{recoveryStatusError}</p>
            <Button variant="secondary" size="compact" type="button" on:click={retryRecoveryStatusCheck}>
              Retry status
            </Button>
          </div>
        {/if}
        {#if generatedRecoveryWords.length}
          <div class="recovery-word-grid">
            {#each generatedRecoveryWords as word, index}
              <div class="recovery-word-chip">
                <span class="recovery-word-index">{index + 1}</span>
                <span>{word}</span>
              </div>
            {/each}
          </div>
          <div class="flex flex-wrap gap-3">
            <Button variant="secondary" type="button" on:click={copyRecoveryKey}>Copy words</Button>
            <Button variant="secondary" type="button" on:click={downloadRecoveryKey}>Download .txt</Button>
          </div>
          {#if recoveryCopyNotice}
            <p class="text-xs text-muted">{recoveryCopyNotice}</p>
          {/if}
          <label class="mt-4 flex items-center gap-2 text-sm text-ink">
            <input type="checkbox" bind:checked={recoveryAcknowledged} class="h-4 w-4" />
            <span>I saved this recovery key in a secure, offline place.</span>
          </label>
          <div class="flex gap-3 flex-wrap mt-4">
            <Button
              variant="primary"
              type="button"
              on:click={() => handleRecoveryContinue()}
              disabled={!generatedRecoveryWords.length || !recoveryAcknowledged || generatingRecoveryKey}
            >
              Continue
            </Button>
          </div>
        {:else}
          {#if recoveryStatus?.present}
            <div class="flex flex-wrap gap-3">
              <Button variant="primary" type="button" on:click={openRegenerateModal}>
                Regenerate key
              </Button>
              {#if recoveryStatus?.stale}
                <Button
                  variant="secondary"
                  type="button"
                  on:click={() => handleRecoveryContinue({ useExisting: true })}
                  loading={continueExistingLoading}
                >
                  Continue with existing key
                </Button>
                {#if continueExistingError}
                  <p class="text-xs text-red-600 w-full">{continueExistingError}</p>
                {/if}
              {/if}
            </div>
          {:else}
            <p class="text-sm text-muted">Generate the 24-word key now. You can’t finish setup until it’s saved.</p>
            <div class="flex gap-3 flex-wrap">
              <Button variant="primary" type="button" on:click={handleGenerateRecoveryKey} loading={generatingRecoveryKey}>
                {generatingRecoveryKey ? 'Generating…' : 'Generate recovery key'}
              </Button>
            </div>
          {/if}
        {/if}
      {/if}
    </section>
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

{#if showRegenerateConfirm}
  <div class="modal-backdrop">
    <div class="modal-card">
      <h3 class="text-lg font-semibold text-ink">Regenerate recovery key?</h3>
      <p class="text-sm text-muted mt-2">This will invalidate the previous key. Save the new 24 words immediately—Piccolo only shows them once.</p>
      {#if regenerateError}
        <p class="text-sm text-red-600 mt-2">{regenerateError}</p>
      {/if}
      <div class="mt-4 flex gap-3 flex-wrap">
        <Button variant="ghost" type="button" on:click={closeRegenerateModal} disabled={generatingRecoveryKey}>
          Cancel
        </Button>
        <Button variant="primary" type="button" on:click={confirmRegenerate} loading={generatingRecoveryKey}>
          Regenerate key
        </Button>
      </div>
    </div>
  </div>
{/if}

<style>
  .recovery-word-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
    gap: 0.75rem;
    margin-top: 1rem;
  }

  .recovery-word-chip {
    border-radius: var(--radius-lg);
    border: 1px solid var(--card-border);
    background: rgba(255, 255, 255, 0.8);
    padding: 0.65rem 0.85rem;
    display: flex;
    gap: 0.35rem;
    font-weight: 600;
  }

  .recovery-word-index {
    color: rgb(var(--sys-ink-muted));
  }

  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(15, 19, 32, 0.55);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 1.5rem;
    z-index: 50;
  }

  .modal-card {
    width: min(480px, 100%);
    border-radius: var(--radius-xl);
    border: 1px solid var(--card-border);
    background: var(--card-gradient);
    padding: 2rem;
    box-shadow: var(--shadow-strong);
  }
</style>
