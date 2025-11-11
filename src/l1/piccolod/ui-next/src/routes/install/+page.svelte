<script lang="ts">
  import { createMutation, createQuery } from '@tanstack/svelte-query';
  import Stepper from '$lib/components/Stepper.svelte';
  import type { StepDefinition } from '$lib/types/wizard';
  import type { PageData } from './$types';
  import type { InstallPlan, InstallTarget, FetchLatestImage } from '$lib/api/install';
  import { fetchInstallTargets, fetchLatestImage, requestInstallPlan, runInstall } from '$lib/api/install';
  import type { ApiError } from '$lib/api/http';
  import DiskCard from '$lib/components/install/DiskCard.svelte';
  import PlanSummary from '$lib/components/install/PlanSummary.svelte';
  import ProgressPanel from '$lib/components/install/ProgressPanel.svelte';
  import Button from '$lib/components/ui/Button.svelte';

  export let data: PageData;

  type Step = 'intro' | 'disk' | 'plan' | 'install' | 'finish';

const baseSteps: StepDefinition[] = [
  { id: 'intro', label: 'Prepare', description: 'Review safety checks' },
  { id: 'disk', label: 'Disk', description: 'Choose destination' },
  { id: 'plan', label: 'Plan', description: 'Simulate changes' },
  { id: 'install', label: 'Install', description: 'Write & verify' },
  { id: 'finish', label: 'Finish', description: 'Reboot & continue' }
];

  const targetsQuery = createQuery<InstallTarget[]>(() => ({
    queryKey: ['install-targets'],
    queryFn: fetchInstallTargets,
    initialData: data.targets,
    staleTime: 30_000
  }));

  let activeStep: Step = 'intro';
  let selectedTargetId = data.targets[0]?.id ?? '';
  let confirmTargetInput = '';
  let planConfirmInput = '';
  let installPlan: InstallPlan | null = null;
  let latestInfo: FetchLatestImage | null = null;
  let requestLatest = false;
let infoMessage = '';
let errorMessage = '';
let progressNotes: string[] = [];
let installComplete = false;
let planHasError = false;
let installHasError = false;
let steps: StepDefinition[] = baseSteps;

$: steps = (() => {
  const started = activeStep !== 'intro';
  return baseSteps.map((step) => {
    if (step.id === 'plan') {
      if (planHasError) {
        return { ...step, state: 'error' } as StepDefinition;
      }
      return step;
    }
    if (step.id === 'install') {
      if (installHasError) {
        return { ...step, state: 'error' } as StepDefinition;
      }
      if (installComplete) {
        return { ...step, state: 'success' } as StepDefinition;
      }
      return step;
    }
    if (step.id === 'finish' && installComplete && activeStep === 'finish') {
      return { ...step, state: 'success' } as StepDefinition;
    }
    return step;
  });
})();

  let targets: InstallTarget[] = data.targets;
  let selectedTarget: InstallTarget | null = targets[0] ?? null;
  let loadingTargets = false;
  let canContinueToPlan = false;
  let canRunInstall = false;
  $: targets = targetsQuery.data ?? data.targets ?? [];
  $: loadingTargets = targetsQuery.isLoading || targetsQuery.isFetching;
  $: selectedTarget = targets.find((target) => target.id === selectedTargetId) ?? null;
  $: canContinueToPlan = Boolean(selectedTarget) && confirmTargetInput.trim() === selectedTarget?.id;
  $: canRunInstall = Boolean(installPlan) && planConfirmInput.trim() === installPlan?.target;

  const planMutation = createMutation(() => ({
    mutationFn: ({ targetId, fetchLatest }: { targetId: string; fetchLatest?: boolean }) =>
      requestInstallPlan({ targetId, fetchLatest }),
    onSuccess: (plan) => {
      installPlan = plan;
      planConfirmInput = '';
      infoMessage = 'Install plan ready. Review and confirm.';
      errorMessage = '';
      activeStep = 'plan';
      planHasError = false;
    },
    onError: (err: ApiError) => {
      errorMessage = err.message ?? 'Failed to generate install plan';
      planHasError = true;
    }
  }));

  const latestMutation = createMutation(() => ({
    mutationFn: fetchLatestImage,
    onSuccess: (info) => {
      latestInfo = info;
      infoMessage = info.version ? `Latest image ${info.version} verified` : 'Latest signed image verified';
      errorMessage = '';
    },
    onError: (err: ApiError) => {
      errorMessage = err.message ?? 'Unable to fetch latest image';
    }
  }));

  const installMutation = createMutation(() => ({
    mutationFn: ({ targetId, fetchLatest, acknowledgement }: { targetId: string; fetchLatest?: boolean; acknowledgement: string }) =>
      runInstall({ targetId, fetchLatest, acknowledgeId: acknowledgement }),
    onSuccess: () => {
      progressNotes = [...progressNotes, `Install request accepted at ${new Date().toLocaleTimeString()}`];
      infoMessage = 'Install request accepted. The device will reboot when finished.';
      errorMessage = '';
      installComplete = true;
      installHasError = false;
      activeStep = 'install';
    },
    onError: (err: ApiError) => {
      errorMessage = err.message ?? 'Install failed. Please review logs.';
      installHasError = true;
      activeStep = 'plan';
    }
  }));

function resetNotices() {
  infoMessage = '';
  errorMessage = '';
  planHasError = false;
  installHasError = false;
  installComplete = false;
}

  function startWizard() {
    resetNotices();
    progressNotes = [];
    activeStep = 'disk';
  }

  function handleTargetSelect(id: string) {
    selectedTargetId = id;
    confirmTargetInput = '';
    installPlan = null;
    planConfirmInput = '';
    requestLatest = false;
    latestInfo = null;
    resetNotices();
  }

  function goBackToIntro() {
    resetNotices();
    activeStep = 'intro';
  }

  function goBackToDisks() {
    resetNotices();
    installPlan = null;
    planConfirmInput = '';
    activeStep = 'disk';
  }

  async function generatePlan(fetchLatest: boolean) {
    if (!selectedTarget) return;
    resetNotices();
    installPlan = null;
    planConfirmInput = '';
    requestLatest = fetchLatest;
    try {
      await planMutation.mutateAsync({ targetId: selectedTarget.id, fetchLatest });
    } catch {
      // handled in mutation
    }
  }

  async function handleRunInstall(event: SubmitEvent) {
    event.preventDefault();
    if (!installPlan || !canRunInstall) {
      errorMessage = 'Type the disk identifier exactly to proceed.';
      return;
    }
    resetNotices();
    progressNotes = ['Submitting install job…'];
    installComplete = false;
    activeStep = 'install';
    try {
      await installMutation.mutateAsync({
        targetId: installPlan.target,
        fetchLatest: requestLatest,
        acknowledgement: planConfirmInput.trim()
      });
    } catch {
      // handled in mutation
    }
  }

  function refreshTargets() {
    targetsQuery.refetch?.();
  }

  function markFinished() {
    activeStep = 'finish';
  }
</script>

<svelte:head>
  <title>Piccolo · Install to Disk</title>
</svelte:head>

<div class="space-y-8" data-testid="install-wizard">
  <section class="rounded-[40px] border border-white/30 bg-gradient-to-br from-white/85 via-white/70 to-slate-50 p-8 elev-3 text-ink">
    <div class="flex flex-col gap-6 lg:flex-row lg:items-center lg:justify-between">
      <div class="max-w-2xl space-y-3">
        <p class="meta-label">Install</p>
        <h1 class="text-3xl font-semibold">New disk install wizard</h1>
        <p class="text-base text-muted">
          Write the signed Piccolo OS image from this live session onto an internal disk. The flow guides you through
          selecting the correct device, simulating the changes, and monitoring the install until reboot.
        </p>
        <div class="flex flex-wrap gap-3">
          <Button variant="primary" on:click={startWizard}>
            Begin install
          </Button>
          <Button variant="secondary" on:click={refreshTargets}>
            Refresh disks
          </Button>
        </div>
      </div>
      <div class="rounded-3xl border border-white/40 bg-white/70 px-6 py-5 text-sm text-muted">
        <p class="font-semibold text-ink">Safety first</p>
        <ul class="mt-2 list-disc list-inside space-y-1">
          <li>Device must stay on power & wired network.</li>
          <li>Install erases the entire target disk.</li>
          <li>Portal reconnects in ≤60 seconds after reboot.</li>
        </ul>
      </div>
    </div>
  </section>

  <div class="flex flex-col gap-6">
    <Stepper steps={steps} activeId={activeStep} />

    {#if infoMessage}
      <div class="rounded-3xl border border-blue-200 bg-blue-50/80 px-5 py-4 text-sm text-blue-900 elev-1">
        {infoMessage}
      </div>
    {/if}
    {#if errorMessage}
      <div class="rounded-3xl border border-red-200 bg-red-50/90 px-5 py-4 text-sm text-red-800 elev-1">
        {errorMessage}
      </div>
    {/if}

    <div class="flex flex-col gap-6 xl:flex-row xl:items-start xl:gap-8">
      <div class="flex-1 flex flex-col gap-6 min-w-0 xl:pr-4">

      {#if activeStep === 'intro'}
        <section class="rounded-3xl border border-white/30 bg-white/90 p-6 elev-2">
          <h2 class="text-xl font-semibold text-ink">Before you begin</h2>
          <p class="mt-2 text-sm text-muted">Stay near the device during install and confirm you have backed up any data on the target disk.</p>
          <div class="mt-4 flex flex-wrap gap-3">
            <Button variant="primary" on:click={startWizard}>
              Continue
            </Button>
            <Button variant="ghost" href="/">
              Cancel
            </Button>
          </div>
        </section>
      {:else if activeStep === 'disk'}
        <section class="rounded-3xl border border-white/30 bg-white/90 p-6 elev-2">
          <div class="flex flex-col gap-2">
            <h2 class="text-xl font-semibold text-ink">Choose the installation target</h2>
            <p class="text-sm text-muted">Disks pulled from `/dev/disk/by-id` with model, capacity, and detected partitions.</p>
          </div>
          {#if loadingTargets}
            <p class="mt-6 text-sm text-muted">Scanning for disks…</p>
          {:else if targets.length === 0}
            <p class="mt-6 text-sm text-muted">No disks detected. Connect a disk or refresh to try again.</p>
          {:else}
            <div class="mt-6 grid gap-4 lg:grid-cols-2">
              {#each targets as target}
                <DiskCard target={target} selected={target.id === selectedTargetId} on:click={() => handleTargetSelect(target.id)} />
              {/each}
            </div>
            <div class="mt-6 rounded-3xl border border-slate-200/70 bg-white/70 px-5 py-5">
              <label class="text-sm font-semibold text-ink" for="confirm-disk">Type disk id to confirm</label>
              <input
                id="confirm-disk"
                class="mt-3 w-full rounded-2xl border border-slate-200 px-4 py-3 text-base focus:border-accent focus:outline-none"
                type="text"
                bind:value={confirmTargetInput}
                placeholder={selectedTarget ? selectedTarget.id : '/dev/disk/by-id/...'}
                autocomplete="off"
              />
              <p class="mt-2 text-xs text-muted">Exact match required to unlock the next step.</p>
            </div>
            <div class="mt-5 flex flex-wrap gap-3">
              <Button variant="ghost" on:click={goBackToIntro}>
                Back
              </Button>
              <Button variant="primary" disabled={!canContinueToPlan || planMutation.isPending} on:click={() => generatePlan(false)}>
                {planMutation.isPending ? 'Preparing plan…' : 'Simulate install'}
              </Button>
              <Button variant="secondary" disabled={!canContinueToPlan || planMutation.isPending} on:click={() => generatePlan(true)}>
                {planMutation.isPending ? 'Checking latest…' : 'Simulate with latest image'}
              </Button>
            </div>
          {/if}
        </section>
      {:else if activeStep === 'plan'}
        <section class="space-y-5">
          <PlanSummary plan={installPlan} target={selectedTarget} latest={requestLatest ? latestInfo : null} />

          {#if installPlan}
            <div class="rounded-3xl border border-white/30 bg-white/95 p-6 elev-2">
              <div class="flex flex-col gap-2">
                <label class="text-sm font-semibold text-ink" for="plan-confirm">Confirm disk id</label>
                <input
                  id="plan-confirm"
                  class="w-full rounded-2xl border border-slate-200 px-4 py-3 text-base focus:border-accent focus:outline-none"
                  type="text"
                  bind:value={planConfirmInput}
                  placeholder={installPlan.target}
                />
                <p class="text-xs text-muted">Re-type {installPlan.target} to acknowledge the irreversible write.</p>
              </div>
              <div class="mt-4 flex flex-wrap items-center gap-3 text-sm">
                <label class="flex items-center gap-2 text-ink">
                  <input type="checkbox" bind:checked={requestLatest} class="h-4 w-4 rounded border-slate-300 text-accent focus:ring-accent" />
                  Fetch latest signed image before installing
                </label>
                {#if requestLatest}
                  <Button
                    variant="secondary"
                    size="compact"
                    disabled={latestMutation.isPending}
                    on:click={() => {
                      void latestMutation.mutateAsync().catch(() => {});
                    }}
                  >
                    {latestMutation.isPending ? 'Checking…' : 'Verify latest image'}
                  </Button>
                {/if}
              </div>
              <form class="mt-6 flex flex-wrap gap-3" on:submit={handleRunInstall}>
                <Button variant="ghost" type="button" on:click={goBackToDisks}>
                  Back
                </Button>
                <Button variant="primary" type="submit" disabled={!canRunInstall || installMutation.isPending}>
                  {installMutation.isPending ? 'Starting…' : 'Run install'}
                </Button>
              </form>
            </div>
          {:else}
            <div class="rounded-3xl border border-dashed border-slate-200 px-5 py-5 text-sm text-muted">
              Generate a plan to continue.
            </div>
          {/if}
        </section>
      {:else if activeStep === 'install'}
        <section class="rounded-3xl border border-white/30 bg-white/95 p-6 elev-2">
          <h2 class="text-xl font-semibold text-ink">Install in progress</h2>
          <p class="mt-2 text-sm text-muted">Keep the browser open—Piccolo will reboot automatically when the image finishes writing.</p>
          <Button variant="ghost" size="compact" on:click={markFinished}>
            Show post-install steps
          </Button>
        </section>
      {:else if activeStep === 'finish'}
        <section class="rounded-3xl border border-white/30 bg-white/95 p-6 elev-2 text-ink">
          <h2 class="text-2xl font-semibold">Install underway</h2>
          <p class="mt-2 text-sm text-muted">
            The device is applying the image and will reboot. Once it is back online, visit <code>http://piccolo.local</code> to finish the setup wizard.
          </p>
          <div class="mt-4 flex flex-wrap gap-3">
            <Button variant="primary" href="/setup">
              Continue setup
            </Button>
            <Button variant="secondary" href="/">
              Return home
            </Button>
          </div>
        </section>
      {/if}
      </div>

      <div class="space-y-4 xl:w-[320px] xl:flex-shrink-0 xl:sticky xl:top-6">
        <ProgressPanel notes={progressNotes} pending={installMutation.isPending} complete={installComplete} />
        <div class="rounded-3xl border border-white/30 bg-white/85 p-5 text-sm text-muted">
          <p class="font-semibold text-ink">Need to capture logs?</p>
          <p class="mt-1">Use the portal logs bundle tool if the install reports errors before retrying.</p>
        </div>
      </div>
    </div>
  </div>
</div>
