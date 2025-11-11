<script lang="ts">
  import { goto } from '$app/navigation';
  import { createMutation } from '@tanstack/svelte-query';
  import type { ApiError } from '$lib/api/http';
  import { createAdmin, initCrypto } from '$lib/api/setup';
  import Stepper from '$lib/components/Stepper.svelte';
  import Button from '$lib/components/ui/Button.svelte';
  import type { StepDefinition } from '$lib/types/wizard';
  import type { PageData } from './$types';

  export let data: PageData;

  type Step = 'intro' | 'credentials' | 'done';

const baseSteps: StepDefinition[] = [
  { id: 'intro', label: 'Welcome', description: 'Review prerequisites' },
  { id: 'credentials', label: 'Credentials', description: 'Create admin password' },
  { id: 'done', label: 'Finish', description: 'Ready to sign in' }
];

let steps: StepDefinition[] = baseSteps;

$: steps = baseSteps.map((step) => {
  if (step.id === 'credentials' && error) {
    return { ...step, state: 'error' } as StepDefinition;
  }
  if (step.id === 'done' && activeStep === 'done') {
    return { ...step, state: 'success' } as StepDefinition;
  }
  return step;
});

  let activeStep: Step = data.initialized ? 'done' : 'intro';
  let password = '';
  let confirmPassword = '';
  let error = '';
  let infoMessage = data.initialized ? 'Admin account already exists. Sign in instead.' : '';

  const mutation = createMutation(() => ({
    mutationFn: async ({ password }: { password: string }) => {
      await createAdmin(password);
      try {
        await initCrypto(password);
      } catch (err) {
        const message = (err as ApiError | undefined)?.message?.toLowerCase() ?? '';
        if (!message.includes('already')) throw err;
      }
    },
    onSuccess: () => {
      infoMessage = 'Admin created successfully.';
      activeStep = 'done';
    },
    onError: (err: ApiError) => {
      error = err.message || 'Setup failed';
      if (/locked/i.test(error)) {
        goto('/lock');
      }
      if (/already/.test(error)) {
        goto('/login');
      }
    }
  }));

  function passwordsMatch() {
    return password.length >= 8 && password === confirmPassword;
  }

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    error = '';
    if (!passwordsMatch()) {
      error = 'Passwords must match and be at least 8 characters.';
      return;
    }
    await mutation.mutateAsync({ password });
  }

  function goToCredentials() {
    if (data.initialized) {
      goto('/login');
      return;
    }
    activeStep = 'credentials';
  }

  function finish() {
    goto('/');
  }
</script>

<svelte:head>
  <title>Piccolo · Initial Setup</title>
</svelte:head>

<div class="flex flex-col gap-6">
  <section class="rounded-3xl border border-white/30 bg-white/80 backdrop-blur-xl p-6 elev-3 text-ink">
    <p class="meta-label">First run</p>
    <h1 class="mt-2 text-2xl font-semibold">Create admin credentials</h1>
    <p class="mt-2 text-sm text-muted max-w-2xl">
      Choose a strong password for the Piccolo administrator. This account unlocks encrypted volumes and enables remote access.
    </p>
  </section>

  <Stepper steps={steps} activeId={activeStep} />

  {#if activeStep === 'intro'}
    <section class="rounded-3xl border border-white/30 bg-white/85 backdrop-blur-xl elev-2 p-6 flex flex-col gap-4">
      <p class="text-sm text-muted max-w-2xl">
        Before we create the admin account, make sure you have physical access to the device and
        have unlocked encrypted volumes if prompted. The admin password controls remote access and recovery actions.
      </p>
      <ul class="rounded-2xl border border-slate-200 px-4 py-4 text-sm text-ink space-y-2">
        <li>• Minimum 8 characters, avoid obvious strings.</li>
        <li>• This password unlocks volumes and remote access.</li>
        <li>• Keep it confidential—there is only one admin per device.</li>
      </ul>
      <div class="flex gap-3">
        <Button variant="primary" on:click={goToCredentials}>
          {data.initialized ? 'Go to sign in' : 'Start setup'}
        </Button>
      </div>
    </section>
  {:else if activeStep === 'credentials'}
    <form class="rounded-3xl border border-white/30 bg-white/90 backdrop-blur-xl elev-2 p-6 flex flex-col gap-4" on:submit={handleSubmit}>
      {#if infoMessage}
        <div class="rounded-2xl border border-blue-200 bg-blue-50 p-4 text-sm text-blue-900">
          {infoMessage}
        </div>
      {/if}
      {#if error}
        <p class="text-sm text-red-600">{error}</p>
      {/if}
      <div>
        <label class="text-sm font-medium text-ink" for="admin-password">Password</label>
        <input
          id="admin-password"
          class="mt-2 w-full rounded-2xl border border-slate-200 px-4 py-3 text-base focus:border-accent focus:outline-none"
          type="password"
          bind:value={password}
          placeholder="New admin password"
          autocomplete="new-password"
        />
      </div>
      <div>
        <label class="text-sm font-medium text-ink" for="confirm-password">Confirm password</label>
        <input
          id="confirm-password"
          class="mt-2 w-full rounded-2xl border border-slate-200 px-4 py-3 text-base focus:border-accent focus:outline-none"
          type="password"
          bind:value={confirmPassword}
          placeholder="Repeat password"
          autocomplete="new-password"
        />
      </div>
      <div class="rounded-2xl border border-slate-200 px-4 py-3 text-xs text-muted">
        <p class="font-semibold text-ink">Requirements</p>
        <ul class="mt-1 list-disc list-inside space-y-1">
          <li>Minimum 8 characters</li>
          <li>Match confirmation exactly</li>
          <li>Avoid device serials or obvious strings</li>
        </ul>
      </div>
      <div class="flex gap-3">
        <Button variant="ghost" type="button" on:click={() => (activeStep = 'intro')}>
          Back
        </Button>
        <Button variant="primary" disabled={!passwordsMatch() || mutation.isPending} type="submit">
          {mutation.isPending ? 'Creating…' : 'Create admin'}
        </Button>
      </div>
    </form>
  {:else if activeStep === 'done'}
    <section class="rounded-3xl border border-white/30 bg-white/90 backdrop-blur-xl elev-2 p-6 flex flex-col gap-4 text-ink">
      <p class="meta-label meta-label--lg">Complete</p>
      <h2 class="text-2xl font-semibold">Admin ready</h2>
      <p class="text-sm text-muted">Use the password you just created to sign in and finish device setup.</p>
      <div class="flex gap-3">
        <Button variant="primary" on:click={finish}>
          Go to dashboard
        </Button>
        <Button variant="secondary" on:click={() => goto('/login')}>
          Sign in now
        </Button>
      </div>
    </section>
  {/if}
</div>
